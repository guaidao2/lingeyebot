// Package policy implements the tool-call gate: allow, deny, or ask a human.
package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guaidao2/lingeyebot/internal/agent"
	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
)

// Engine enforces constraints, approval rules and the audit log.
type Engine struct {
	Store   *store.Store
	Config  config.Config
	Timeout time.Duration
	// OnAsk presents a pending approval and returns the verdict. When nil the
	// engine polls the database (the server fills the browser queue via OnPending).
	OnAsk func(ctx context.Context, a store.Approval) (bool, error)
	// OnPending is called after a pending approval is stored, so the UI can show it.
	OnPending func(a store.Approval)

	mu sync.RWMutex
}

// New builds an engine.
func New(st *store.Store, cfg config.Config) *Engine {
	return &Engine{Store: st, Config: cfg, Timeout: 15 * time.Minute}
}

// SetConfig updates the runtime policy for future calls.
func (e *Engine) SetConfig(cfg config.Config) {
	e.mu.Lock()
	e.Config = cfg
	e.mu.Unlock()
}

func (e *Engine) config() config.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Config
}

var destructive = []string{
	"rm -rf /", "rm -rf /*", "mkfs", "dd if=", "of=/dev/", "shutdown", "reboot", "poweroff",
	":(){", "chmod -r 777 /", "chmod 777 /", "format c:", "del /f /s /q c:\\",
	"remove-item -recurse -force c:\\", "> /dev/sda", "mimikatz", "secretsdump", "lsass",
	"/etc/shadow", "| sh", "| bash", "powershell -enc", "base64 -d | sh",
}

// Authorize decides one tool call.
func (e *Engine) Authorize(ctx context.Context, run store.Run, taskID string, call llm.ToolCall) (agent.Decision, error) {
	if run.ID == "" {
		return agent.Decision{Allowed: false, Reason: "无法确认所属任务"}, nil
	}
	args := string(call.Arguments)
	command := extractCommand(call.Name, call.Arguments)

	// Explicit deny constraints win over everything.
	if reason := e.denyConstraint(run.ID, command+" "+args); reason != "" {
		e.audit(run.ID, taskID, call.Name, "deny", reason, command)
		return agent.Decision{Allowed: false, Reason: reason}, nil
	}

	cfg := e.config()
	mode := strings.ToLower(cfg.Approval.Mode)
	need := needAsk(cfg, call.Name, command)
	if mode == "auto" {
		if isDestructive(call.Name, command) {
			e.audit(run.ID, taskID, call.Name, "deny", "auto mode blocks destructive command", command)
			return agent.Decision{Allowed: false, Reason: "破坏性命令在自动模式下被拒绝；如需执行请开启人机协同（ask）"}, nil
		}
		need = false
	}
	if mode == "deny" && need {
		e.audit(run.ID, taskID, call.Name, "deny", "approval mode=deny", command)
		return agent.Decision{Allowed: false, Reason: "审批模式为 deny，且该动作需要确认"}, nil
	}
	if need {
		approval, err := e.Store.CreateApproval(store.Approval{
			RunID: run.ID, TaskID: taskID, Tool: call.Name, Input: args,
			Reason: askReason(call.Name, command),
		})
		if err != nil {
			return agent.Decision{}, err
		}
		e.audit(run.ID, taskID, call.Name, "ask", approval.Reason, command)
		if e.OnPending != nil {
			e.OnPending(approval)
		}
		ok, err := e.ask(ctx, approval)
		if err != nil {
			return agent.Decision{}, err
		}
		if ok {
			e.audit(run.ID, taskID, call.Name, "approved", "user approved", command)
			return agent.Decision{Allowed: true}, nil
		}
		e.audit(run.ID, taskID, call.Name, "denied", "user denied or timed out", command)
		return agent.Decision{Allowed: false, Reason: "用户拒绝了该操作"}, nil
	}
	e.audit(run.ID, taskID, call.Name, "allow", "", command)
	return agent.Decision{Allowed: true}, nil
}

func (e *Engine) ask(ctx context.Context, a store.Approval) (bool, error) {
	if e.OnAsk != nil {
		return e.OnAsk(ctx, a)
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			_, _ = e.Store.DecideApproval(a.ID, false)
			return false, nil
		case <-ticker.C:
			current, err := e.Store.GetApproval(a.ID)
			if err != nil {
				continue
			}
			switch current.Status {
			case "approved":
				return true, nil
			case "denied":
				return false, nil
			}
		}
	}
}

func (e *Engine) denyConstraint(runID, haystack string) string {
	constraints, err := e.Store.ListConstraints(runID)
	if err != nil {
		return ""
	}
	lower := strings.ToLower(haystack)
	for _, c := range constraints {
		if c.Kind != "deny" || !c.Active {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(c.Text))
		if text == "" {
			continue
		}
		if strings.Contains(lower, text) {
			return "命中禁止约束：" + c.Text
		}
	}
	return ""
}

func needAsk(cfg config.Config, toolName, command string) bool {
	for _, name := range cfg.Approval.AskTools {
		if name == toolName {
			return true
		}
	}
	return isDestructive(toolName, command)
}

// isDestructive reports commands that must never run silently, even in auto mode.
func isDestructive(toolName, command string) bool {
	if toolName != "shell" {
		return false
	}
	lower := strings.ToLower(command)
	for _, pattern := range destructive {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func askReason(toolName, command string) string {
	if toolName == "shell" {
		return "高风险命令需要确认：" + command
	}
	return "该工具需要人工确认"
}

func extractCommand(toolName string, raw json.RawMessage) string {
	if toolName != "shell" {
		return ""
	}
	var args struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(raw, &args)
	return args.Command
}

func (e *Engine) audit(runID, taskID, tool, action, reason, command string) {
	_ = e.Store.AddAudit(store.AuditEntry{
		RunID: runID, TaskID: taskID, Tool: tool, Action: action, Reason: reason, Command: command,
	})
}

var _ agent.Gate = (*Engine)(nil)

// String renders a short description for diagnostics.
func (e *Engine) String() string {
	return fmt.Sprintf("policy(mode=%s, timeout=%s)", e.Config.Approval.Mode, e.Timeout)
}
