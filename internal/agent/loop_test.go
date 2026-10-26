package agent

import (
	"context"
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
