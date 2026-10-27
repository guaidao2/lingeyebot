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
	var found *store.Message
	for i := range msgs {
		if strings.Contains(msgs[i].Content, "[子代理") && strings.Contains(msgs[i].Content, "3 个子域") {
			found = &msgs[i]
		}
	}
	if found == nil {
		t.Fatalf("parent session missing announcement: %+v", msgs)
	}
	if found.Visible {
		t.Fatal("subagent announcement must be hidden from the user-facing conversation")
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

func TestSubagentFinishReactivatesAchievedParent(t *testing.T) {
	st, run, _ := newTestStore(t)
	task, err := st.CreateTask(store.Task{RunID: run.ID, Role: "worker", Title: "收尾"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRunStatus(run.ID, "achieved"); err != nil {
		t.Fatal(err)
	}
	mgr := &SubagentManager{Store: st}
	mgr.finish(run, task, "done", "子代理结果", "")
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" {
		t.Fatalf("achieved parent was not reactivated: %s", got.Status)
	}
	sess, err := st.EnsureSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ListMessages(sess.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "[子代理 worker done]") && !msg.Visible {
			found = true
		}
	}
	if !found {
		t.Fatalf("hidden subagent report missing: %+v", msgs)
	}
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
