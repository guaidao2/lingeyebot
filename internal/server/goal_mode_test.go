package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/store"
)

func TestGoalModeAPIContract(t *testing.T) {
	srv, st, _ := newTestServer(t, false)
	project, err := st.EnsureDefaultProject()
	if err != nil {
		t.Fatal(err)
	}

	// New tasks default to plain conversation: omitted goalMode creates no goal.
	plain, plainGoal, err := srv.createConversation(createRunRequest{
		ProjectID: project.ID, Mode: "daily", Target: "plain.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plain.GoalsEnabled() || plainGoal.ID != "" {
		t.Fatalf("omitted goalMode should keep plain conversation: run=%+v goal=%+v", plain, plainGoal)
	}

	// Explicit true enables goal mode and requires an objective.
	enabled := true
	run, goal, err := srv.createConversation(createRunRequest{
		ProjectID: project.ID, Mode: "daily", Target: "goal.test",
		Objective: "do it", GoalMode: &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !run.GoalsEnabled() || goal.ID == "" {
		t.Fatalf("explicit true should create a goal: run=%+v goal=%+v", run, goal)
	}

	// Adding a goal later turns a plain run back into goal mode.
	req := httptest.NewRequest(http.MethodPost, "/api/runs/"+plain.ID+"/goals", strings.NewReader(`{"text":"new goal","acceptance":"proved"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add goal: %d %s", rec.Code, rec.Body.String())
	}
	got, err := st.GetRun(plain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.GoalsEnabled() {
		t.Fatal("adding a goal should re-enable goal mode")
	}
}

func TestPostMessageReactivatesAchievedRun(t *testing.T) {
	_, st, ts := newTestServer(t, true)
	project, err := st.EnsureDefaultProject()
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := st.CreateRun(store.Run{
		ProjectID: project.ID, Mode: "daily", Target: "achieved.test", Objective: "done",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRunStatus(run.ID, "achieved"); err != nil {
		t.Fatal(err)
	}
	status, body := request(t, ts, http.MethodPost, "/api/runs/"+run.ID+"/messages", map[string]any{"text": "继续收尾"})
	if status != http.StatusOK {
		t.Fatalf("post to achieved: %d %s", status, body)
	}
	sess, err := st.EnsureSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ListMessages(sess.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, msg := range msgs {
		if msg.Role == "user" && strings.Contains(msg.Content, "继续收尾") {
			found = true
		}
	}
	if !found {
		t.Fatal("message to achieved run was not persisted")
	}
}
