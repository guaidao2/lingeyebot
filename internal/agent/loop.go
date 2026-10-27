package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
)

// Loop owns per-run turn orchestration: persistence, interruption, goal
// evaluation and autonomous continuation.
type Loop struct {
	Store       *store.Store
	Config      config.Config
	ProviderFor func(run store.Run) (llm.Provider, string, bool)
	ExecutorFor func(run store.Run, taskID string) (Executor, error)
	GateFor     func(run store.Run, taskID string) Gate
	PromptFor   func(run store.Run, snap store.Snapshot, role string) string
	Sink        Sink
	// OnStop is called when a run is stopped, before tasks are marked cancelled,
	// so subagents can cancel their in-flight contexts.
	OnStop func(runID string)

	mu     sync.Mutex
	states map[string]*runState

	cfgMu sync.RWMutex
}

type runState struct {
	mu      sync.Mutex
	running bool
	pending bool
	cancel  context.CancelFunc
	lastSig string
	idle    int
}

func (l *Loop) state(runID string) *runState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.states == nil {
		l.states = map[string]*runState{}
	}
	st := l.states[runID]
	if st == nil {
		st = &runState{}
		l.states[runID] = st
	}
	return st
}

// SetConfig updates runtime limits for future turns.
func (l *Loop) SetConfig(cfg config.Config) {
	l.cfgMu.Lock()
	l.Config = cfg
	l.cfgMu.Unlock()
}

func (l *Loop) config() config.Config {
	l.cfgMu.RLock()
	defer l.cfgMu.RUnlock()
	return l.Config
}

// Running reports whether a turn is currently executing.
func (l *Loop) Running(runID string) bool {
	st := l.state(runID)
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.running
}

// Submit records a user message and starts (or interrupts into) a new turn.
func (l *Loop) Submit(runID, text string) error {
	run, err := l.Store.GetRun(runID)
	if err != nil {
		return err
	}
	sess, err := l.Store.EnsureSession(runID)
	if err != nil {
		return err
	}
	if _, err := l.Store.AddMessage(store.Message{
		SessionID: sess.ID, RunID: runID, Role: "user", Content: text, Visible: true,
	}); err != nil {
		return err
	}
	_ = l.Store.TouchRun(runID)
	_ = l.Store.ResetAutoTurns(runID)
	if run.Status == "paused" || run.Status == "stalled" || run.Status == "blocked" || run.Status == "achieved" {
		_ = l.Store.UpdateRunStatus(runID, "active")
	}
	st := l.state(runID)
	st.mu.Lock()
	st.idle = 0
	st.lastSig = ""
	st.mu.Unlock()
	l.event(runID, "", "user", "", Short(text, 200), text)
	l.wake(runID, true)
	return nil
}

// Wake asks the loop to run a turn without a new user message. A running turn
// keeps running; the wake is consumed after it ends.
func (l *Loop) Wake(runID string) {
	l.wake(runID, false)
}

func (l *Loop) wake(runID string, interrupt bool) {
	st := l.state(runID)
	st.mu.Lock()
	if st.running {
		st.pending = true
		if interrupt && st.cancel != nil {
			st.cancel()
		}
		st.mu.Unlock()
		return
	}
	st.running = true
	st.pending = false
	st.mu.Unlock()
	if l.Store != nil {
		// Only a freshly started turn may revive an achieved run. If a turn is
		// already running, it will see pending and reactivate itself.
		_, _ = l.Store.ActivateRunIfAchieved(runID)
	}
	go l.turnLoop(runID)
}

// Stop pauses a run and cancels its in-flight turn and subagents.
func (l *Loop) Stop(runID string) error {
	st := l.state(runID)
	st.mu.Lock()
	if st.cancel != nil {
		st.cancel()
	}
	st.pending = false
	st.mu.Unlock()
	_ = l.Store.UpdateRunStatus(runID, "paused")
	if l.OnStop != nil {
		l.OnStop(runID)
	}
	_ = l.Store.CancelActiveTasks(runID)
	l.event(runID, "", "control", "", "用户停止任务", "")
	return nil
}

// Resume reactivates a paused run and wakes the loop.
func (l *Loop) Resume(runID string) error {
	if _, err := l.Store.GetRun(runID); err != nil {
		return err
	}
	if err := l.Store.UpdateRunStatus(runID, "active"); err != nil {
		return err
	}
	_ = l.Store.ResetAutoTurns(runID)
	l.event(runID, "", "control", "", "恢复任务", "")
	l.wake(runID, false)
	return nil
}

// Shutdown cancels every in-flight turn.
func (l *Loop) Shutdown() {
	l.mu.Lock()
	states := make([]*runState, 0, len(l.states))
	for _, st := range l.states {
		states = append(states, st)
	}
	l.mu.Unlock()
	for _, st := range states {
		st.mu.Lock()
		if st.cancel != nil {
			st.cancel()
		}
		st.running = false
		st.pending = false
		st.mu.Unlock()
	}
}

func (l *Loop) finish(runID string) (again bool) {
	st := l.state(runID)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.pending {
		st.pending = false
		return true
	}
	st.running = false
	st.cancel = nil
	return false
}

func (l *Loop) turnLoop(runID string) {
	for {
		run, err := l.Store.GetRun(runID)
		if err != nil || run.Status != "active" {
			if l.finish(runID) {
				continue
			}
			return
		}
		st := l.state(runID)
		st.mu.Lock()
		st.pending = false
		st.mu.Unlock()
		cfg := l.config()

		snap, err := l.Store.Snapshot(runID)
		if err != nil {
			l.fail(runID, "读取任务状态失败", err)
			if l.finish(runID) {
				continue
			}
			return
		}
		cov, _ := l.Store.Coverage(runID)
		sess, err := l.Store.EnsureSession(runID)
		if err != nil {
			l.fail(runID, "创建会话失败", err)
			if l.finish(runID) {
				continue
			}
			return
		}
		raw, err := l.Store.ListMessages(sess.ID, cfg.Runtime.ContextMessages)
		if err != nil {
			l.fail(runID, "读取会话失败", err)
			if l.finish(runID) {
				continue
			}
			return
		}
		provider, model, ok := l.ProviderFor(run)
		if !ok {
			l.event(runID, "", "error", "", "尚未配置 LLM 端点", "请在设置里填写 base URL、API Key 与模型名")
			_ = l.Store.UpdateRunStatus(runID, "blocked")
			if l.finish(runID) {
				continue
			}
			return
		}
		executor, err := l.ExecutorFor(run, "")
		if err != nil {
			l.fail(runID, "初始化工具失败", err)
			if l.finish(runID) {
				continue
			}
			return
		}
		msgs := MessagesFromStore(raw)
		if len(msgs) > 0 {
			contextWindow := cfg.Runtime.ContextWindow
			if cw, ok := provider.(interface{ ContextWindow() int }); ok {
				if value := cw.ContextWindow(); value > 0 {
					contextWindow = value
				}
			}
			compactCtx, cancelCompact := context.WithTimeout(context.Background(), 60*time.Second)
			if compacted, changed := compactMessages(compactCtx, provider, model, msgs, contextWindow, 16); changed {
				msgs = compacted
				l.event(runID, "", "compact", "", "上下文已压缩", "较早消息已生成摘要")
			}
			cancelCompact()
		}
		sys := l.PromptFor(run, snap, "main") + "\n\n" + Overview(run, snap, cov)
		runner := &Runner{
			Provider: provider, Model: model, Executor: executor, Gate: l.GateFor(run, ""),
			Store: l.Store, SessionID: sess.ID, RunID: runID,
			MaxIterations: cfg.Runtime.MaxIterations, MaxToolChars: cfg.Runtime.ToolResultChars,
			Sink: l.Sink,
		}
		ctx, cancel := context.WithCancel(context.Background())
		st.mu.Lock()
		st.cancel = cancel
		st.mu.Unlock()
		res, runErr := runner.Run(ctx, TurnRequest{
			System:    sys,
			Messages:  msgs,
			MaxTokens: cfg.Runtime.MaxOutputTokens,
		})
		cancel()
		st.mu.Lock()
		st.cancel = nil
		st.mu.Unlock()

		if runErr != nil {
			if errors.Is(runErr, context.Canceled) || res.StopReason == "interrupted" {
				st.mu.Lock()
				pending := st.pending
				st.mu.Unlock()
				if pending {
					continue
				}
				if l.finish(runID) {
					continue
				}
				return
			}
			l.fail(runID, "本轮执行失败", runErr)
			if l.finish(runID) {
				continue
			}
			return
		}

		st.mu.Lock()
		pending := st.pending
		st.mu.Unlock()
		if pending {
			continue
		}
		active, err := l.Store.ActiveTaskCount(runID)
		if err != nil {
			l.fail(runID, "检查子代理状态失败", err)
			if l.finish(runID) {
				continue
			}
			return
		}
		if active > 0 {
			if l.finish(runID) {
				continue
			}
			return
		}
		if !run.GoalsEnabled() {
			if l.finish(runID) {
				continue
			}
			return
		}

		settled, err := l.Store.GoalsSettled(runID)
		if err == nil && settled {
			st.mu.Lock()
			if st.pending {
				st.pending = false
				st.mu.Unlock()
				continue
			}
			achieved, err := l.Store.MarkRunAchievedIfActive(runID)
			if err != nil {
				st.running = false
				st.cancel = nil
				st.mu.Unlock()
				l.fail(runID, "写入目标完成状态失败", err)
				return
			}
			if achieved {
				l.event(runID, "", "goal", "", "全部目标已达成，任务结束", "")
			}
			st.running = false
			st.cancel = nil
			st.mu.Unlock()
			return
		}

		if !cfg.Runtime.AutoContinue {
			if l.finish(runID) {
				continue
			}
			return
		}
		turns, _ := l.Store.IncrementAutoTurns(runID)
		if cfg.Runtime.MaxAutoTurns > 0 && turns > cfg.Runtime.MaxAutoTurns {
			l.stall(runID, "自动继续达到上限，等待用户决定")
			if l.finish(runID) {
				continue
			}
			return
		}
		sig := progressSignature(snap)
		st.mu.Lock()
		if sig == st.lastSig {
			st.idle++
		} else {
			st.idle = 0
			st.lastSig = sig
		}
		idle := st.idle
		st.mu.Unlock()
		if cfg.Runtime.MaxAutoTurns > 0 && idle >= 3 {
			l.stall(runID, "连续 3 轮没有产生新事实或证据，等待用户决定")
			if l.finish(runID) {
				continue
			}
			return
		}

		wait := time.Duration(cfg.Runtime.AutoContinueWait) * time.Second
		select {
		case <-time.After(wait):
		}
		if runAfter, err := l.Store.GetRun(runID); err != nil || runAfter.Status != "active" {
			if l.finish(runID) {
				continue
			}
			return
		}
		st.mu.Lock()
		pending = st.pending
		st.mu.Unlock()
		if pending {
			continue
		}
		auto := "[自动继续] 目标尚未完成。基于当前态势选择最能改变判断的下一步；受阻就换路线并说明理由；不要重复已做过的动作。"
		if _, err := l.Store.AddMessage(store.Message{
			SessionID: sess.ID, RunID: runID, Role: "user", Content: auto, Visible: false,
		}); err != nil {
			l.fail(runID, "写入继续指令失败", err)
			if l.finish(runID) {
				continue
			}
			return
		}
		l.event(runID, "", "auto", "", "目标未达成，自动继续", auto)
	}
}

func (l *Loop) fail(runID, summary string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	l.event(runID, "", "error", "", summary, detail)
	_ = l.Store.UpdateRunStatus(runID, "paused")
}

func (l *Loop) stall(runID, reason string) {
	_ = l.Store.UpdateRunStatus(runID, "stalled")
	l.event(runID, "", "control", "", reason, "任务已暂停，等待用户确认下一步")
}

func (l *Loop) event(runID, taskID, kind, tool, summary, detail string) {
	_, _ = l.Store.AddEvent(store.Event{
		RunID: runID, TaskID: taskID, Kind: kind, Tool: tool, Summary: summary, Detail: detail,
	})
	if l.Sink != nil {
		l.Sink.Emit(runID, Event{Kind: kind, Tool: tool, Summary: summary, Detail: detail, TaskID: taskID})
	}
}

// Emit lets other components (subagents) write into the same event stream.
func (l *Loop) Emit(runID, taskID, kind, tool, summary, detail string) {
	l.event(runID, taskID, kind, tool, summary, detail)
}

func progressSignature(snap store.Snapshot) string {
	return fmt.Sprintf("n%d:e%d:f%d:a%d", len(snap.Nodes), len(snap.Evidence), countKind(snap, "finding"), len(snap.Assets))
}

func countKind(snap store.Snapshot, kind string) int {
	n := 0
	for _, node := range snap.Nodes {
		if node.Kind == kind {
			n++
		}
	}
	return n
}
