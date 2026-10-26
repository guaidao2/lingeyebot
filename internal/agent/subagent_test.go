package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
	"github.com/guaidao2/lingeyebot/internal/tool"
)

func TestSubagentRunsAndAnnounces(t *testing.T) {
	st, run, _ := newTestStore(t)
	prov := &scriptedProvider{responses: []llm.Response{{Content: "枚举完成，发现 3 个子域"}}}
	cfg := config.Default()
	mgr := &SubagentManager{
		Store: st, Config: cfg,
		ProviderFor: func(store.Run) (llm.Provider, string, bool) { return prov, "m", true },
		ExecutorFor: func(store.Run, string, string) (Executor, error) { return &fakeExecutor{}, nil },
		GateFor:     func(store.Run, string) Gate { return AllowAll{} },
		RolePrompt:  func(store.Run, string) string { return "role prompt" },
	}
	defer mgr.Shutdown()

	ctx := context.Background()
	task, err := mgr.Spawn(ctx, run.ID, tool.SpawnInput{Role: "enumerator", Task: "枚举子域", Label: "枚举"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		got, err := st.GetTask(task.ID)
		return err == nil && got.Status == "done"
	})
	got, _ := st.GetTask(task.ID)
	if !strings.Contains(got.Result, "3 个子域") {
		t.Fatalf("result = %q", got.Result)
	}
	sess, _ := st.EnsureSession(run.ID)
	msgs, _ := st.ListMessages(sess.ID, 0)
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "[子代理") && strings.Contains(msg.Content, "3 个子域") {
			found = true
		}
	}
	if !found {
		t.Fatalf("parent session missing announcement: %+v", msgs)
	}
}

func TestSubagentCancelKeepsPartialResult(t *testing.T) {
	st, run, _ := newTestStore(t)
	mgr := &SubagentManager{
		Store: st, Config: config.Default(),
		ProviderFor: func(store.Run) (llm.Provider, string, bool) { return blockingProvider{}, "m", true },
		ExecutorFor: func(store.Run, string, string) (Executor, error) { return &fakeExecutor{}, nil },
		GateFor:     func(store.Run, string) Gate { return AllowAll{} },
		RolePrompt:  func(store.Run, string) string { return "role" },
	}
	defer mgr.Shutdown()
	ctx := context.Background()
	task, err := mgr.Spawn(ctx, run.ID, tool.SpawnInput{Role: "worker", Task: "长任务"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		got, _ := st.GetTask(task.ID)
		return got.Status == "running"
	})
	if err := mgr.Send(ctx, run.ID, task.ID, "收窄范围"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Cancel(ctx, run.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		got, _ := st.GetTask(task.ID)
		return got.Status == "cancelled"
	})
	waitFor(t, 3*time.Second, func() bool {
		mgr.mu.Lock()
		_, stillRunning := mgr.running[task.ID]
		mgr.mu.Unlock()
		return !stillRunning
	})
}

func TestSubagentRejectsUnknownRole(t *testing.T) {
	st, run, _ := newTestStore(t)
	mgr := &SubagentManager{
		Store: st, Config: config.Default(),
		ProviderFor: func(store.Run) (llm.Provider, string, bool) { return &scriptedProvider{}, "m", true },
		ExecutorFor: func(store.Run, string, string) (Executor, error) { return &fakeExecutor{}, nil },
		GateFor:     func(store.Run, string) Gate { return AllowAll{} },
	}
	defer mgr.Shutdown()
	ctx := context.Background()
	if _, err := mgr.Spawn(ctx, run.ID, tool.SpawnInput{Role: "nope", Task: "x"}); err == nil {
		t.Fatal("unknown role must fail")
	}
}

func TestCancelRunStopsRunningTask(t *testing.T) {
	st, run, _ := newTestStore(t)
	mgr := &SubagentManager{
		Store: st, Config: config.Default(),
		ProviderFor: func(store.Run) (llm.Provider, string, bool) { return blockingProvider{}, "m", true },
		ExecutorFor: func(store.Run, string, string) (Executor, error) { return &fakeExecutor{}, nil },
		GateFor:     func(store.Run, string) Gate { return AllowAll{} },
		RolePrompt:  func(store.Run, string) string { return "role" },
	}
	defer mgr.Shutdown()
	ctx := context.Background()
	task, err := mgr.Spawn(ctx, run.ID, tool.SpawnInput{Role: "worker", Task: "长任务"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		got, _ := st.GetTask(task.ID)
		return got.Status == "running"
	})
	mgr.CancelRun(run.ID)
	waitFor(t, 3*time.Second, func() bool {
		got, _ := st.GetTask(task.ID)
		return got.Status == "cancelled"
	})
	waitFor(t, 3*time.Second, func() bool {
		mgr.mu.Lock()
		_, stillRunning := mgr.running[task.ID]
		mgr.mu.Unlock()
		return !stillRunning
	})
}
