package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
)

type scriptedProvider struct {
	mu        sync.Mutex
	responses []llm.Response
	calls     int
}

func (p *scriptedProvider) ID() string { return "fake" }

func (p *scriptedProvider) Chat(context.Context, llm.Request) (llm.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.calls >= len(p.responses) {
		return llm.Response{Content: "done"}, nil
	}
	r := p.responses[p.calls]
	p.calls++
	return r, nil
}

func (p *scriptedProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type fakeExecutor struct {
	mu    sync.Mutex
	calls []string
	out   string
}

func (e *fakeExecutor) Specs() []llm.ToolSpec {
	return []llm.ToolSpec{{Name: "echo", Description: "echo", Parameters: json.RawMessage(`{"type":"object"}`)}}
}

func (e *fakeExecutor) Execute(_ context.Context, call llm.ToolCall) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, call.Name)
	if e.out != "" {
		return e.out, nil
	}
	return "ok:" + string(call.Arguments), nil
}

func newTestStore(t *testing.T) (*store.Store, store.Run, store.Goal) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	run, goal, err := st.CreateRun(store.Run{Mode: "ctf", Target: "chal", Objective: "拿到 flag", Scope: "chal"})
	if err != nil {
		t.Fatal(err)
	}
	return st, run, goal
}

func TestRunnerExecutesToolsAndPersists(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, err := st.EnsureSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	prov := &scriptedProvider{responses: []llm.Response{{
		Content: "先执行",
		ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "echo", Arguments: json.RawMessage(`{"text":"hi"}`),
		}},
		StopReason: "tool_calls",
	}, {
		Content: "完成", StopReason: "stop",
	}}}
	exec := &fakeExecutor{}
	r := &Runner{Provider: prov, Model: "m", Executor: exec, Store: st, SessionID: sess.ID, RunID: run.ID, MaxIterations: 5}

	res, err := r.Run(context.Background(), TurnRequest{System: "sys", Messages: []llm.Message{{Role: "user", Content: "go"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "完成" || res.ToolCalls != 1 || res.Iterations != 2 {
		t.Fatalf("result = %+v", res)
	}
	if len(exec.calls) != 1 || exec.calls[0] != "echo" {
		t.Fatalf("executor calls = %v", exec.calls)
	}
	msgs, err := st.ListMessages(sess.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.Role)
	}
	joined := strings.Join(roles, ",")
	for _, want := range []string{"assistant", "tool"} {
		if !strings.Contains(joined, want) {
			t.Errorf("persisted roles missing %s: %v", want, roles)
		}
	}
}

func TestRunnerGateDeniesTool(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, _ := st.EnsureSession(run.ID)
	prov := &scriptedProvider{responses: []llm.Response{{
		ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{}`)}},
	}, {Content: "ok"}}}
	exec := &fakeExecutor{}
	r := &Runner{Provider: prov, Executor: exec, Store: st, SessionID: sess.ID, RunID: run.ID, Gate: denyGate{}}
	if _, err := r.Run(context.Background(), TurnRequest{Messages: []llm.Message{{Role: "user", Content: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("denied tool must not execute, got %v", exec.calls)
	}
}

type denyGate struct{}

func (denyGate) Authorize(context.Context, store.Run, string, llm.ToolCall) (Decision, error) {
	return Decision{Allowed: false, Reason: "test deny"}, nil
}

func TestRunnerStopsAtIterationLimit(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, _ := st.EnsureSession(run.ID)
	prov := &scriptedProvider{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "echo", Arguments: json.RawMessage(`{}`)}}},
	}}
	r := &Runner{Provider: prov, Executor: &fakeExecutor{}, Store: st, SessionID: sess.ID, RunID: run.ID, MaxIterations: 2}
	res, err := r.Run(context.Background(), TurnRequest{Messages: []llm.Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != "max_iterations" {
		t.Fatalf("stop reason = %s", res.StopReason)
	}
}

func TestRunnerInterruptPersistsCancelledToolResult(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, _ := st.EnsureSession(run.ID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &Runner{Provider: &scriptedProvider{}, Store: st, SessionID: sess.ID, RunID: run.ID}
	if _, err := r.Run(ctx, TurnRequest{Messages: []llm.Message{{Role: "user", Content: "x"}}}); err == nil {
		t.Fatal("expected cancellation error")
	}
}

type cancellingExecutor struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	calls  int
}

func (e *cancellingExecutor) Specs() []llm.ToolSpec {
	return []llm.ToolSpec{{Name: "echo", Description: "echo", Parameters: json.RawMessage(`{"type":"object"}`)}}
}

func (e *cancellingExecutor) Execute(context.Context, llm.ToolCall) (string, error) {
	e.mu.Lock()
	e.calls++
	first := e.calls == 1
	e.mu.Unlock()
	if first {
		e.cancel()
	}
	return "ran", nil
}

func TestRunnerInterruptPersistsEveryToolResult(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, _ := st.EnsureSession(run.ID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec := &cancellingExecutor{cancel: cancel}
	prov := &scriptedProvider{responses: []llm.Response{{
		ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{}`)},
			{ID: "c2", Name: "echo", Arguments: json.RawMessage(`{}`)},
			{ID: "c3", Name: "echo", Arguments: json.RawMessage(`{}`)},
		},
	}}}
	r := &Runner{Provider: prov, Executor: exec, Store: st, SessionID: sess.ID, RunID: run.ID}
	if _, err := r.Run(ctx, TurnRequest{Messages: []llm.Message{{Role: "user", Content: "go"}}}); err == nil {
		t.Fatal("expected cancellation")
	}
	msgs, err := st.ListMessages(sess.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	results := map[string]bool{}
	for _, m := range msgs {
		if m.Role == "tool" {
			results[m.ToolCallID] = true
		}
	}
	if !results["c1"] || !results["c2"] || !results["c3"] {
		t.Fatalf("every tool_call needs a result, got %+v", results)
	}
}

func TestMessagesFromStoreRepairsOrphans(t *testing.T) {
	msgs := []store.Message{
		{Role: "tool", ToolCallID: "orphan", Content: "stale"},
		{Role: "assistant", ToolCalls: json.RawMessage(`[
			{"id":"c1","name":"a","arguments":{}},
			{"id":"c2","name":"b","arguments":{}}
		]`)},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "user", Content: "next"},
	}
	out := MessagesFromStore(msgs)
	if len(out) != 4 {
		t.Fatalf("expected assistant + 2 tool results + user (orphan dropped), got %d: %+v", len(out), out)
	}
	if out[0].Role != "assistant" {
		t.Fatalf("first role = %s", out[0].Role)
	}
	if out[1].Role != "tool" || out[1].ToolCallID != "c1" {
		t.Fatalf("second = %+v", out[1])
	}
	if out[2].Role != "tool" || out[2].ToolCallID != "c2" || !strings.Contains(out[2].Content, "missing") {
		t.Fatalf("dangling call was not repaired: %+v", out[2])
	}
	for _, m := range out {
		if m.ToolCallID == "orphan" {
			t.Fatalf("orphan tool result leaked: %+v", out)
		}
	}
}

func TestRunnerEventCarriesTaskID(t *testing.T) {
	var got Event
	r := &Runner{TaskID: "task-1", Sink: SinkFunc(func(_ string, ev Event) { got = ev })}
	r.emit(Event{Kind: "assistant_delta", Detail: "chunk"})
	if got.TaskID != "task-1" || got.Kind != "assistant_delta" || got.Detail != "chunk" {
		t.Fatalf("event = %+v", got)
	}
}

type reasoningStreamProvider struct{}

func (p *reasoningStreamProvider) ID() string { return "reasoning-stream" }
func (p *reasoningStreamProvider) Chat(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{Content: "answer", Reasoning: "think"}, nil
}
func (p *reasoningStreamProvider) ChatStreamWithReasoning(_ context.Context, _ llm.Request, onContent, onReasoning func(string)) (llm.Response, error) {
	onReasoning("think")
	onContent("answer")
	return llm.Response{Content: "answer", Reasoning: "think"}, nil
}

func TestRunnerStreamsReasoningWithoutPersistingDeltas(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, err := st.EnsureSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	r := &Runner{
		Provider: &reasoningStreamProvider{}, Model: "m", Store: st, SessionID: sess.ID, RunID: run.ID,
		Sink: SinkFunc(func(_ string, ev Event) { kinds = append(kinds, ev.Kind) }),
	}
	res, err := r.Run(context.Background(), TurnRequest{Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != "final" || res.Content != "answer" {
		t.Fatalf("result = %+v", res)
	}
	if !containsString(kinds, "assistant_reasoning_delta") || !containsString(kinds, "assistant_delta") {
		t.Fatalf("kinds = %v", kinds)
	}
	events, err := st.ListEvents(run.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind == "assistant_reasoning_delta" || event.Kind == "assistant_delta" {
			t.Fatalf("stream delta persisted: %+v", event)
		}
	}
}

type contentStreamProvider struct{}

func (p *contentStreamProvider) ID() string { return "content-stream" }
func (p *contentStreamProvider) Chat(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{Content: "hello"}, nil
}
func (p *contentStreamProvider) ChatStream(_ context.Context, _ llm.Request, onDelta func(string)) (llm.Response, error) {
	onDelta("hello")
	return llm.Response{Content: "hello"}, nil
}

func TestRunnerFallsBackToContentStream(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, err := st.EnsureSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	r := &Runner{
		Provider: &contentStreamProvider{}, Model: "m", Store: st, SessionID: sess.ID, RunID: run.ID,
		Sink: SinkFunc(func(_ string, ev Event) { kinds = append(kinds, ev.Kind) }),
	}
	if _, err := r.Run(context.Background(), TurnRequest{Messages: []llm.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if !containsString(kinds, "assistant_delta") {
		t.Fatalf("content stream fallback not used: %v", kinds)
	}
}

type reasoningOnlyProvider struct{}

func (p *reasoningOnlyProvider) ID() string { return "reasoning-only" }
func (p *reasoningOnlyProvider) Chat(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{Reasoning: "think"}, nil
}
func (p *reasoningOnlyProvider) ChatStreamWithReasoning(_ context.Context, _ llm.Request, _, onReasoning func(string)) (llm.Response, error) {
	onReasoning("think")
	return llm.Response{Reasoning: "think"}, nil
}

func TestRunnerEmitsAssistantEventForReasoningOnly(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, err := st.EnsureSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var assistant Event
	var seen bool
	r := &Runner{
		Provider: &reasoningOnlyProvider{}, Model: "m", Store: st, SessionID: sess.ID, RunID: run.ID,
		Sink: SinkFunc(func(_ string, ev Event) {
			if ev.Kind == "assistant" && !seen {
				seen = true
				assistant = ev
			}
		}),
	}
	res, err := r.Run(context.Background(), TurnRequest{Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !seen || res.StopReason != "final" || res.Content != "" {
		t.Fatalf("seen=%v result=%+v", seen, res)
	}
	if assistant.Summary != "思考完成" || assistant.Detail != "" {
		t.Fatalf("assistant event = %+v", assistant)
	}
	msgs, err := st.ListMessages(sess.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, msg := range msgs {
		if msg.Role == "assistant" && msg.Reasoning == "think" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasoning-only message not persisted: %+v", msgs)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRunnerEmitsAssistantBeforePersisting(t *testing.T) {
	st, run, _ := newTestStore(t)
	sess, err := st.EnsureSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	persistedAtEvent := -1
	checked := false
	r := &Runner{
		Provider: &contentStreamProvider{}, Model: "m", Store: st, SessionID: sess.ID, RunID: run.ID,
		Sink: SinkFunc(func(_ string, ev Event) {
			if ev.Kind != "assistant" || checked {
				return
			}
			checked = true
			msgs, err := st.ListMessages(sess.ID, 100)
			if err != nil {
				t.Fatal(err)
			}
			persistedAtEvent = len(msgs)
		}),
	}
	if _, err := r.Run(context.Background(), TurnRequest{Messages: []llm.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("no assistant event observed")
	}
	if persistedAtEvent != 0 {
		t.Fatalf("assistant was persisted before emit: %d messages", persistedAtEvent)
	}
}
