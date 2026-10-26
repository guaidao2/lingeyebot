package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/store"
)

func TestFindingStatusGlobalApprovalsAndTaskCancel(t *testing.T) {
	_, st, ts := newTestServer(t, true)
	project, err := st.EnsureDefaultProject()
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := st.CreateRun(store.Run{
		ProjectID: project.ID, Mode: "daily", Target: "example.test", Objective: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	finding, err := st.AddNode(store.Node{
		RunID: run.ID, Kind: "finding", State: "open",
		Payload: []byte(`{"title":"BOLA","severity":"high","reproducibleSteps":["step"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	status, body := request(t, ts, "POST", "/api/findings/"+finding.ID+"/status", map[string]any{"state": "confirmed"})
	if status != http.StatusOK {
		t.Fatalf("finding status: %d %s", status, body)
	}
	updated, err := st.GetNode(finding.ID)
	if err != nil || updated.State != "confirmed" {
		t.Fatalf("finding state = %q err=%v", updated.State, err)
	}

	approval, err := st.CreateApproval(store.Approval{
		RunID: run.ID, Tool: "shell", Input: `{"command":"id"}`, Reason: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	status, body = request(t, ts, "GET", "/api/approvals", nil)
	if status != http.StatusOK || !strings.Contains(string(body), approval.ID) {
		t.Fatalf("global approvals: %d %s", status, body)
	}
	status, body = request(t, ts, "POST", "/api/approvals/"+approval.ID+"/decide", map[string]any{"approve": true})
	if status != http.StatusOK {
		t.Fatalf("decide: %d %s", status, body)
	}
	_, body = request(t, ts, "GET", "/api/approvals", nil)
	if strings.Contains(string(body), approval.ID) {
		t.Fatalf("decided approval still pending: %s", body)
	}

	task, err := st.CreateTask(store.Task{RunID: run.ID, Role: "worker", Title: "cancel me"})
	if err != nil {
		t.Fatal(err)
	}
	status, body = request(t, ts, "POST", "/api/tasks/"+task.ID+"/cancel", nil)
	if status != http.StatusOK {
		t.Fatalf("cancel task: %d %s", status, body)
	}
	cancelled, err := st.GetTask(task.ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("task status = %q err=%v", cancelled.Status, err)
	}
}
