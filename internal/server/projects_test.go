package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/store"
)

func TestProjectWorkspaceFlow(t *testing.T) {
	_, _, ts := newTestServer(t, true)

	status, body := request(t, ts, "POST", "/api/projects", map[string]any{
		"name": "Acme SRC", "description": "授权范围内的 SRC", "authorization": "self-owned", "scope": "acme.test",
	})
	if status != http.StatusCreated {
		t.Fatalf("create project: %d %s", status, body)
	}
	var project store.Project
	if err := json.Unmarshal(body, &project); err != nil || project.ID == "" {
		t.Fatalf("project payload: %s", body)
	}

	status, body = request(t, ts, "POST", "/api/projects/"+project.ID+"/conversations", map[string]any{
		"mode": "bugbounty", "target": "acme.test", "objective": "找可提交漏洞",
	})
	if status != http.StatusCreated {
		t.Fatalf("create conversation: %d %s", status, body)
	}

	status, body = request(t, ts, "POST", "/api/projects/"+project.ID+"/board", map[string]any{
		"kind": "decision", "title": "scope", "detail": "只测 acme.test", "pinned": true,
	})
	if status != http.StatusCreated {
		t.Fatalf("board write: %d %s", status, body)
	}

	status, body = request(t, ts, "GET", "/api/projects/"+project.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("get project: %d %s", status, body)
	}
	var detail struct {
		Project store.ProjectDetail `json:"project"`
		Running map[string]bool     `json:"running"`
	}
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Project.Project.ID != project.ID || len(detail.Project.Conversations) != 1 {
		t.Fatalf("detail = %+v", detail.Project)
	}
	if len(detail.Project.Memory) != 1 || detail.Project.Memory[0].Value != "只测 acme.test" {
		t.Fatalf("memory = %+v", detail.Project.Memory)
	}

	status, body = request(t, ts, "GET", "/api/projects/"+project.ID+"/board", nil)
	if status != http.StatusOK {
		t.Fatalf("board read: %d %s", status, body)
	}
	if !json.Valid(body) {
		t.Fatalf("invalid board json: %s", body)
	}

	status, body = request(t, ts, "GET", "/api/dashboard", nil)
	if status != http.StatusOK {
		t.Fatalf("dashboard: %d %s", status, body)
	}
	var dashboard store.Dashboard
	if err := json.Unmarshal(body, &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard.Projects < 1 || dashboard.Conversations < 1 {
		t.Fatalf("dashboard = %+v", dashboard)
	}

	status, body = request(t, ts, "GET", "/api/runs?projectId="+project.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("runs by project: %d %s", status, body)
	}
}
