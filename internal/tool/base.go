// Package tool implements the tools available to agents.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/knowledge"
	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
)

// Func executes one tool call and returns text for the model.
type Func func(ctx context.Context, args []byte) (string, error)

// Spec describes one tool.
type Spec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	ReadOnly    bool
	Dangerous   bool
	Fn          Func
}

// Registry is an ordered tool set.
type Registry struct {
	specs  []Spec
	byName map[string]Spec
}

// NewRegistry builds a registry.
func NewRegistry(specs []Spec) *Registry {
	r := &Registry{byName: map[string]Spec{}}
	for _, s := range specs {
		if s.Name == "" || s.Fn == nil {
			continue
		}
		if _, exists := r.byName[s.Name]; exists {
			continue
		}
		r.specs = append(r.specs, s)
		r.byName[s.Name] = s
	}
	return r
}

// Specs returns provider-facing tool specs.
func (r *Registry) Specs() []llm.ToolSpec {
	out := make([]llm.ToolSpec, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, llm.ToolSpec{Name: s.Name, Description: s.Description, Parameters: s.Parameters})
	}
	return out
}

// Names lists registered tool names.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return out
}

// Execute runs one tool call.
func (r *Registry) Execute(ctx context.Context, call llm.ToolCall) (string, error) {
	s, ok := r.byName[call.Name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
	args := call.Arguments
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	return s.Fn(ctx, args)
}

// Filter returns a registry restricted to the allowed names. An empty list
// returns the registry unchanged; "mcp__*" allows every MCP tool.
func (r *Registry) Filter(allowed []string) *Registry {
	if len(allowed) == 0 {
		return r
	}
	want := map[string]bool{}
	allMCP := false
	for _, n := range allowed {
		if n == "mcp__*" {
			allMCP = true
			continue
		}
		want[n] = true
	}
	var specs []Spec
	for _, s := range r.specs {
		if want[s.Name] || (allMCP && strings.HasPrefix(s.Name, "mcp__")) || strings.HasPrefix(s.Name, "knowledge_") {
			specs = append(specs, s)
		}
	}
	return NewRegistry(specs)
}

// Env is the execution context handed to tools.
type Env struct {
	Store     *store.Store
	Config    config.Config
	Run       store.Run
	TaskID    string
	SessionID string
	Agent     string
	Workspace string
	Skills    SkillsProvider
	MCP       MCPProvider
	Subagents SubagentProvider
	Knowledge *knowledge.Store
	Role      string
	// ValidateGoal refuses a premature goal completion (quantified acceptance).
	ValidateGoal func(goal store.Goal) error
}

// SkillsProvider exposes discovered skills.
type SkillsProvider interface {
	List() []SkillInfo
	Get(name string) (SkillInfo, bool)
}

// SkillInfo is one discovered skill.
type SkillInfo struct {
	Name        string
	Description string
	Path        string
	Body        string
}

// MCPProvider exposes configured MCP servers.
type MCPProvider interface {
	Tools() []MCPTool
	Call(ctx context.Context, server, tool string, args json.RawMessage) (string, error)
}

// MCPTool is one remote MCP tool.
type MCPTool struct {
	Server      string
	Name        string
	Description string
	Schema      json.RawMessage
}

// SubagentProvider manages delegated tasks.
type SubagentProvider interface {
	Spawn(ctx context.Context, runID string, in SpawnInput) (store.Task, error)
	Send(ctx context.Context, runID, taskID, message string) error
	Cancel(ctx context.Context, runID, taskID string) error
	Check(ctx context.Context, runID, taskID string) (store.Task, error)
	List(ctx context.Context, runID string) ([]store.Task, error)
}

// SpawnInput describes one delegated task.
type SpawnInput struct {
	Role           string `json:"role"`
	Task           string `json:"task"`
	Scope          string `json:"scope,omitempty"`
	Context        string `json:"context,omitempty"`
	ExpectedOutput string `json:"expectedOutput,omitempty"`
	Label          string `json:"label,omitempty"`
	NodeID         string `json:"nodeId,omitempty"`
	Budget         int    `json:"budget,omitempty"`
}

// Build assembles the full tool registry for an environment.
func Build(env Env) *Registry {
	specs := []Spec{}
	specs = append(specs, runTools(env)...)
	specs = append(specs, graphTools(env)...)
	specs = append(specs, boardTools(env)...)
	specs = append(specs, execTools(env)...)
	specs = append(specs, fileTools(env)...)
	specs = append(specs, httpTools(env)...)
	specs = append(specs, reportTools(env)...)
	if env.Knowledge != nil {
		specs = append(specs, knowledgeTools(env)...)
	}
	if env.Skills != nil {
		specs = append(specs, skillTools(env)...)
	}
	if env.Subagents != nil {
		specs = append(specs, subagentTools(env)...)
	}
	if env.MCP != nil {
		specs = append(specs, mcpTools(env)...)
	}
	return NewRegistry(specs)
}

func decodeArgs[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	err := json.Unmarshal(raw, &v)
	return v, err
}

func obj(props map[string]any, required ...string) json.RawMessage {
	m := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		m["required"] = required
	}
	b, _ := json.Marshal(m)
	return b
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func strEnum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

func num(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }

func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func arr(desc string, items any) map[string]any {
	return map[string]any{"type": "array", "description": desc, "items": items}
}

func (e Env) resolve(path string) string {
	base := strings.TrimSpace(e.Workspace)
	if base == "" {
		base = "."
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return base
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return base
	}
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(base, full)
	}
	full = filepath.Clean(full)
	rel, err := filepath.Rel(base, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join(base, ".__blocked_path__")
	}
	return full
}

func (e Env) emit(kind, tool, summary, detail string) {
	if e.Store == nil {
		return
	}
	_, _ = e.Store.AddEvent(store.Event{RunID: e.Run.ID, TaskID: e.TaskID, Kind: kind, Tool: tool, Summary: summary, Detail: detail})
}
