package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
