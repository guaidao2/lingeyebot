package store

import "testing"

func TestOpenRecoversInterruptedState(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := st.CreateRun(Run{Mode: "daily", Target: "recovery.test", Objective: "recover"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.CreateTask(Task{RunID: run.ID, Role: "worker", Title: "interrupted", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	gotRun, err := reopened.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotRun.Status != "paused" {
		t.Fatalf("run status = %s, want paused", gotRun.Status)
	}
	gotTask, err := reopened.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTask.Status != "failed" {
		t.Fatalf("task status = %s, want failed", gotTask.Status)
	}
	events, err := reopened.ListEvents(run.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Kind == "control" && event.Summary == "进程重启，任务已暂停" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing restart control event: %+v", events)
	}
}
