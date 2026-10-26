// Package agent implements the model-facing runner and the per-run loop.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
)

// Event is one observable step emitted during a turn.
type Event struct {
	Kind    string `json:"kind"`
	Tool    string `json:"tool,omitempty"`
	Summary string `json:"summary,omitempty"`
	Detail  string `json:"detail,omitempty"`
	TaskID  string `json:"taskId,omitempty"`
}

// Sink receives runner events tagged with their run.
type Sink interface {
	Emit(runID string, ev Event)
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(runID string, ev Event)

// Emit calls the function.
func (f SinkFunc) Emit(runID string, ev Event) { f(runID, ev) }

// Decision is the policy verdict for one tool call.
type Decision struct {
	Allowed bool
	Reason  string
}

// Gate authorizes tool calls. Implementations may block waiting for a human.
type Gate interface {
	Authorize(ctx context.Context, run store.Run, taskID string, call llm.ToolCall) (Decision, error)
}

// AllowAll permits every tool call.
type AllowAll struct{}

// Authorize always allows.
func (AllowAll) Authorize(context.Context, store.Run, string, llm.ToolCall) (Decision, error) {
	return Decision{Allowed: true}, nil
}

// Executor exposes the tools available to one agent.
type Executor interface {
	Specs() []llm.ToolSpec
	Execute(ctx context.Context, call llm.ToolCall) (string, error)
}

// Runner runs the provider/tool conversation loop.
type Runner struct {
	Provider      llm.Provider
	Model         string
	Executor      Executor
	Gate          Gate
	Store         *store.Store
	SessionID     string
	RunID         string
	TaskID        string
	MaxIterations int
	MaxToolChars  int
	Sink          Sink
}

// TurnRequest is one user-visible turn.
type TurnRequest struct {
	System      string
	Messages    []llm.Message
	MaxTokens   int
	Temperature float64
	Inject      func() []llm.Message
}

// TurnResult summarizes a turn.
type TurnResult struct {
	Content    string
	Iterations int
	ToolCalls  int
	StopReason string // final|max_iterations|interrupted|error
}

// Run executes the model/tool loop until a final answer, the iteration limit, or
// context cancellation.
func (r *Runner) Run(ctx context.Context, req TurnRequest) (TurnResult, error) {
	if r.Provider == nil {
		return TurnResult{}, fmt.Errorf("no LLM provider configured")
	}
	maxIter := r.MaxIterations
	if maxIter < 0 {
		maxIter = 0
	}
	maxChars := r.MaxToolChars
	if maxChars <= 0 {
		maxChars = 24000
	}
	msgs := append([]llm.Message(nil), req.Messages...)
	var specs []llm.ToolSpec
	if r.Executor != nil {
		specs = r.Executor.Specs()
	}
	result := TurnResult{}

	for i := 0; maxIter == 0 || i < maxIter; i++ {
		if err := ctx.Err(); err != nil {
			result.StopReason = "interrupted"
			return result, err
		}
		if req.Inject != nil {
			if extra := req.Inject(); len(extra) > 0 {
				msgs = append(msgs, extra...)
			}
		}
		maxTokens := req.MaxTokens
		if capper, ok := r.Provider.(interface{ OutputCap() int }); ok {
			if cap := capper.OutputCap(); cap > 0 {
				maxTokens = cap
			}
		}
		llmReq := llm.Request{
			Model:       r.Model,
			System:      req.System,
			Messages:    msgs,
			Tools:       specs,
			MaxTokens:   maxTokens,
			Temperature: req.Temperature,
		}
		var resp llm.Response
		var err error
		if streamer, ok := r.Provider.(llm.StreamProvider); ok {
			resp, err = streamer.ChatStream(ctx, llmReq, func(delta string) {
				r.emit(Event{Kind: "assistant_delta", Detail: delta})
			})
		} else {
			resp, err = r.Provider.Chat(ctx, llmReq)
		}
		if err != nil {
			r.emit(Event{Kind: "error", Summary: "模型调用失败", Detail: err.Error()})
			result.StopReason = "error"
			return result, err
		}
		result.Iterations = i + 1
		assistant := llm.Message{Role: "assistant", Content: resp.Content, Reasoning: resp.Reasoning, ToolCalls: resp.ToolCalls}
		msgs = append(msgs, assistant)
		r.persist(assistant)
		if resp.Content != "" {
			r.emit(Event{Kind: "assistant", Summary: Short(resp.Content, 240), Detail: resp.Content})
		}
		if len(resp.ToolCalls) == 0 {
			result.Content = resp.Content
			result.StopReason = "final"
			return result, nil
		}
		for idx, call := range resp.ToolCalls {
			result.ToolCalls++
			if ctx.Err() != nil {
				// Every tool_call in the assistant message needs a matching tool
				// result, otherwise the next provider call gets an invalid history.
				for _, remaining := range resp.ToolCalls[idx:] {
					msg := llm.Message{
						Role: "tool", ToolCallID: remaining.ID, Name: remaining.Name,
						Content: "cancelled: user interrupted",
					}
					msgs = append(msgs, msg)
					r.persist(msg)
				}
				result.StopReason = "interrupted"
				return result, ctx.Err()
			}
			out := r.execute(ctx, call)
			out = Truncate(out, maxChars)
			toolMsg := llm.Message{Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: out}
			msgs = append(msgs, toolMsg)
			r.persist(toolMsg)
		}
	}
	result.StopReason = "max_iterations"
	r.emit(Event{Kind: "notice", Summary: "达到单轮工具调用上限，先收束再继续"})
	return result, nil
}

func (r *Runner) execute(ctx context.Context, call llm.ToolCall) string {
	args := string(call.Arguments)
	if strings.TrimSpace(args) == "" {
		args = "{}"
	}
	r.emit(Event{Kind: "tool_call", Tool: call.Name, Summary: Short(args, 240), Detail: args})
	if r.Gate != nil {
		if r.Store == nil {
			return "tool error: policy gate requires a store"
		}
		run, err := r.Store.GetRun(r.RunID)
		if err != nil {
			return "tool call denied: cannot resolve run"
		}
		decision, err := r.Gate.Authorize(ctx, run, r.TaskID, call)
		if err != nil {
			return "tool error: " + err.Error()
		}
		if !decision.Allowed {
			reason := decision.Reason
			if reason == "" {
				reason = "denied by policy"
			}
			r.emit(Event{Kind: "tool_blocked", Tool: call.Name, Summary: reason})
			return "tool call denied: " + reason
		}
	}
	if r.Executor == nil {
		return "tool error: no executor"
	}
	out, err := r.Executor.Execute(ctx, call)
	if err != nil {
		out = "tool error: " + err.Error()
	}
	r.emit(Event{Kind: "tool_result", Tool: call.Name, Summary: Short(out, 240), Detail: out})
	return out
}

func (r *Runner) persist(m llm.Message) {
	if r.Store == nil || r.SessionID == "" {
		return
	}
	var calls json.RawMessage
	if len(m.ToolCalls) > 0 {
		calls, _ = json.Marshal(m.ToolCalls)
	}
	_, _ = r.Store.AddMessage(store.Message{
		SessionID:  r.SessionID,
		RunID:      r.RunID,
		Role:       m.Role,
		Content:    m.Content,
		Reasoning:  m.Reasoning,
		ToolCalls:  calls,
		ToolCallID: m.ToolCallID,
		Name:       m.Name,
		Visible:    true,
	})
}

func (r *Runner) emit(ev Event) {
	ev.TaskID = r.TaskID
	if r.Store != nil && r.RunID != "" && ev.Kind != "assistant_delta" {
		_, _ = r.Store.AddEvent(store.Event{
			RunID: r.RunID, TaskID: r.TaskID, Kind: ev.Kind, Tool: ev.Tool,
			Summary: ev.Summary, Detail: Short(ev.Detail, 4000),
		})
	}
	if r.Sink != nil {
		r.Sink.Emit(r.RunID, ev)
	}
}

// Short trims a string to n runes for display.
func Short(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// Truncate limits a tool result while preserving head and tail.
func Truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	head := n * 2 / 3
	tail := n - head - 40
	if tail < 0 {
		tail = 0
	}
	if head >= len(s) {
		return s[:n]
	}
	out := s[:head] + "\n…(tool output truncated)…\n"
	if tail > 0 && len(s)-tail > head {
		out += s[len(s)-tail:]
	}
	return out
}
