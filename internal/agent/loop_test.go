package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
)

func newTestLoop(st *store.Store, prov llm.Provider, cfg config.Config) *Loop {
	return &Loop{
		Store:  st,
		Config: cfg,
		ProviderFor: func(store.Run) (llm.Provider, string, bool) {
			return prov, "test-model", true
		},
		ExecutorFor: func(store.Run, string) (Executor, error) { return &fakeExecutor{}, nil },
		GateFor:     func(store.Run, string) Gate { return AllowAll{} },
		PromptFor:   func(store.Run, store.Snapshot, string) string { return "system" },
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestLoopStopsWhenGoalsAreMet(t *testing.T) {
	st, run, goal := newTestStore(t)
	if _, err := st.ProveGoal(goal.ID, nil, "met"); err != nil {
		t.Fatal(err)
	}
	prov := &scriptedProvider{responses: []llm.Response{{Content: "完成"}}}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		r, _ := st.GetRun(run.ID)
		return r.Status == "achieved"
	})
}

func TestLoopRunsToolsThenAwaitsUserWhenAutoContinueOff(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &scriptedProvider{responses: []llm.Response{{Content: "先说结论"}}}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return !loop.Running(run.ID) })
	sess, _ := st.EnsureSession(run.ID)
	msgs, _ := st.ListMessages(sess.ID, 0)
	found := false
	for _, m := range msgs {
		if m.Role == "assistant" && m.Content == "先说结论" {
			found = true
		}
	}
	if !found {
		t.Fatalf("assistant message not persisted: %+v", msgs)
	}
}

func TestLoopAutoContinuesThenStallsWithoutProgress(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &scriptedProvider{responses: []llm.Response{{Content: "没有新发现"}}}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = true
	cfg.Runtime.AutoContinueWait = 1 // int seconds; normalized value is seconds
	cfg.Runtime.MaxAutoTurns = 2
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool {
		r, _ := st.GetRun(run.ID)
		return r.Status == "stalled"
	})
}

func TestLoopStopCancelsTurn(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &blockingProvider{}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return loop.Running(run.ID) })
	if err := loop.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return !loop.Running(run.ID) })
	r, _ := st.GetRun(run.ID)
	if r.Status != "paused" {
		t.Fatalf("status = %s", r.Status)
	}
}

type blockingProvider struct{}

func (blockingProvider) ID() string { return "blocking" }

func (blockingProvider) Chat(ctx context.Context, _ llm.Request) (llm.Response, error) {
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

// switchableProvider blocks while block is set, then answers. It lets a test
// stop an in-flight turn and later resume the same run.
type switchableProvider struct {
	mu    sync.Mutex
	block bool
	calls int
}

func (p *switchableProvider) ID() string { return "switchable" }

func (p *switchableProvider) Chat(ctx context.Context, _ llm.Request) (llm.Response, error) {
	p.mu.Lock()
	p.calls++
	block := p.block
	p.mu.Unlock()
	if block {
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}
	return llm.Response{Content: "再次运行完成"}, nil
}

func (p *switchableProvider) unblock() {
	p.mu.Lock()
	p.block = false
	p.mu.Unlock()
}

func (p *switchableProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// gateProvider blocks each call until release is closed, so a test can pause a
// run while its turn is still in flight.
type gateProvider struct {
	mu      sync.Mutex
	calls   int
	release chan struct{}
}

func (p *gateProvider) ID() string { return "gate" }

func (p *gateProvider) Chat(ctx context.Context, _ llm.Request) (llm.Response, error) {
	p.mu.Lock()
	p.calls++
	release := p.release
	p.mu.Unlock()
	select {
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	case <-release:
		return llm.Response{Content: "gate 完成"}, nil
	}
}

func (p *gateProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// failProvider holds the first call until failSignal is closed, then returns a
// plain error; later calls succeed. It lets a test queue a wake and then make
// the in-flight turn fail.
type failProvider struct {
	mu         sync.Mutex
	calls      int
	failSignal chan struct{}
}

func (p *failProvider) ID() string { return "fail" }

func (p *failProvider) Chat(ctx context.Context, _ llm.Request) (llm.Response, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		select {
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		case <-p.failSignal:
			return llm.Response{}, errors.New("boom")
		}
	}
	return llm.Response{Content: "失败后恢复"}, nil
}

func (p *failProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func waitForAssistantMessage(t *testing.T, st *store.Store, runID, want string) {
	t.Helper()
	sess, err := st.EnsureSession(runID)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		msgs, _ := st.ListMessages(sess.ID, 0)
		for _, m := range msgs {
			if m.Role == "assistant" && m.Content == want {
				return true
			}
		}
		return false
	})
}

// A Stop that lands while the turn is still preparing must not be lost: the
// provider must never be called and the run must stay paused.
func TestLoopStopBeforeTurnStartsDoesNotRunTurn(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &scriptedProvider{responses: []llm.Response{{Content: "不应执行"}}}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	loop.ExecutorFor = func(store.Run, string) (Executor, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return &fakeExecutor{}, nil
	}

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := loop.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitFor(t, 3*time.Second, func() bool { return !loop.Running(run.ID) })
	if calls := prov.Calls(); calls != 0 {
		t.Fatalf("provider called %d times after stop", calls)
	}
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "paused" || got.PauseReason != store.PauseReasonUserStop {
		t.Fatalf("status=%s reason=%q, want paused/user_stop", got.Status, got.PauseReason)
	}
}

func TestLoopWakeDoesNotStartStoppedRun(t *testing.T) {
	st, run, _ := newTestStore(t)
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, blockingProvider{}, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return loop.Running(run.ID) })
	if err := loop.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return !loop.Running(run.ID) })

	loop.Wake(run.ID)
	if loop.Running(run.ID) {
		t.Fatal("Wake started a turn for a user-stopped run")
	}
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "paused" {
		t.Fatalf("status = %s, want paused", got.Status)
	}
}

func TestLoopSubmitAfterStopRunsAgain(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &switchableProvider{block: true}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return loop.Running(run.ID) })
	waitFor(t, 2*time.Second, func() bool { return prov.Calls() > 0 })
	if err := loop.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return !loop.Running(run.ID) })

	prov.unblock()
	if err := loop.Submit(run.ID, "继续"); err != nil {
		t.Fatal(err)
	}
	waitForAssistantMessage(t, st, run.ID, "再次运行完成")
}

func TestLoopResumeAfterStopRunsAgain(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &switchableProvider{block: true}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return loop.Running(run.ID) })
	waitFor(t, 2*time.Second, func() bool { return prov.Calls() > 0 })
	if err := loop.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return !loop.Running(run.ID) })

	prov.unblock()
	if err := loop.Resume(run.ID); err != nil {
		t.Fatal(err)
	}
	waitForAssistantMessage(t, st, run.ID, "再次运行完成")
}

// Wake must resume an error-paused run even when no turn is running: a report
// that lands after the pause is exactly this case.
func TestLoopWakeResumesErrorPausedRun(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &scriptedProvider{responses: []llm.Response{{Content: "恢复后完成"}}}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := st.PauseRun(run.ID, store.PauseReasonError); err != nil {
		t.Fatal(err)
	}
	loop.Wake(run.ID)
	waitForAssistantMessage(t, st, run.ID, "恢复后完成")
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" || got.PauseReason != "" {
		t.Fatalf("status=%s reason=%q, want active", got.Status, got.PauseReason)
	}
}

// A wake queued while a turn is still unwinding must revive an error pause the
// failing turn just recorded, instead of parking the run with the wake pending.
func TestLoopWakeQueuedDuringTurnRevivesErrorPause(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &gateProvider{release: make(chan struct{})}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return loop.Running(run.ID) })
	waitFor(t, 2*time.Second, func() bool { return prov.Calls() > 0 })
	if err := st.PauseRun(run.ID, store.PauseReasonError); err != nil {
		t.Fatal(err)
	}
	loop.Wake(run.ID)
	close(prov.release)
	waitFor(t, 3*time.Second, func() bool {
		got, err := st.GetRun(run.ID)
		return err == nil && got.Status == "active" && prov.Calls() >= 2
	})
}

// A wake already queued when the turn errors must survive the error pause:
// fail() revives the paused/error state while pending work exists, so the loop
// continues and consumes the report instead of parking.
func TestLoopFailKeepsQueuedWakeAlive(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &failProvider{failSignal: make(chan struct{})}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return loop.Running(run.ID) })
	waitFor(t, 2*time.Second, func() bool { return prov.Calls() > 0 })
	// Queue the wake first, then let the in-flight turn fail.
	loop.Wake(run.ID)
	close(prov.failSignal)
	waitForAssistantMessage(t, st, run.ID, "失败后恢复")
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == "paused" {
		t.Fatalf("queued wake was swallowed by the error pause: status=%s reason=%q", got.Status, got.PauseReason)
	}
}

func TestLoopWaitsForActiveTaskBeforeAchieved(t *testing.T) {
	st, run, goal := newTestStore(t)
	if _, err := st.ProveGoal(goal.ID, nil, "met"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTask(store.Task{RunID: run.ID, Role: "worker", Title: "pending"}); err != nil {
		t.Fatal(err)
	}
	prov := &scriptedProvider{responses: []llm.Response{{Content: "等待子代理"}}}
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, prov, cfg)
	defer loop.Shutdown()

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return !loop.Running(run.ID) })
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" {
		t.Fatalf("run with active subagent should stay active, got %s", got.Status)
	}
}

func TestLoopStopInvokesOnStop(t *testing.T) {
	st, run, _ := newTestStore(t)
	cfg := config.Default()
	cfg.Runtime.AutoContinue = false
	loop := newTestLoop(st, blockingProvider{}, cfg)
	defer loop.Shutdown()
	called := make(chan string, 1)
	loop.OnStop = func(runID string) { called <- runID }

	if err := loop.Submit(run.ID, "开始"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return loop.Running(run.ID) })
	if err := loop.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-called:
		if id != run.ID {
			t.Fatalf("OnStop got %s, want %s", id, run.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnStop was not called")
	}
}
