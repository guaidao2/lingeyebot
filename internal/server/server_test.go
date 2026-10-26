package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/skill"
	"github.com/guaidao2/lingeyebot/internal/store"
)

func newTestServer(t *testing.T, withProvider bool) (*Server, *store.Store, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LINGEYE_CONFIG", filepath.Join(dir, "config.json"))
	cfg := config.Default()
	cfg.DataDir = filepath.Join(dir, "data")
	cfg.Workspace = filepath.Join(dir, "workspace")
	cfg.Skills = []string{filepath.Join(cfg.Workspace, "skills")}
	cfg.Listen = "127.0.0.1:0"
	if withProvider {
		cfg.Providers = []config.Provider{{
			ID: "p1", Kind: "openai", BaseURL: "https://llm.invalid/v1",
			APIKey: "sk-secret-1234", Model: "test-model", MaxTokens: 1024,
		}}
		cfg.ActiveProvider = "p1"
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(cfg, st, skill.New(cfg.Skills), "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		srv.Shutdown(context.Background())
	})
	return srv, st, ts
}

func request(t *testing.T, ts *httptest.Server, method, path string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func TestHealthModesAndStatic(t *testing.T) {
	_, _, ts := newTestServer(t, true)

	status, body := request(t, ts, "GET", "/api/health", nil)
	if status != http.StatusOK || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("health: %d %s", status, body)
	}
	status, body = request(t, ts, "GET", "/api/modes", nil)
	if status != http.StatusOK {
		t.Fatalf("modes: %d %s", status, body)
	}
	var modes struct {
		Modes []struct {
			ID string `json:"id"`
		} `json:"modes"`
	}
	if err := json.Unmarshal(body, &modes); err != nil || len(modes.Modes) != 5 {
		t.Fatalf("modes = %s", body)
	}
	status, body = request(t, ts, "GET", "/", nil)
	if status != http.StatusOK || !strings.Contains(string(body), "lingeyebot") {
		t.Fatalf("static: %d %s", status, body)
	}
}

func TestCreateRunAndReadViews(t *testing.T) {
	_, _, ts := newTestServer(t, true)
	status, body := request(t, ts, "POST", "/api/runs", map[string]any{
		"mode": "bugbounty", "target": "example.com", "objective": "找可提交漏洞",
		"authorization": "self-owned", "scope": "example.com",
		"constraints": []map[string]string{{"kind": "deny", "text": "不要碰支付"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create run: %d %s", status, body)
	}
	var created struct {
		Run  store.Run  `json:"run"`
		Goal store.Goal `json:"goal"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.Run.ID == "" {
		t.Fatalf("create payload: %s", body)
	}
	runID := created.Run.ID

	status, body = request(t, ts, "GET", "/api/runs", nil)
	if status != http.StatusOK || !strings.Contains(string(body), runID) {
		t.Fatalf("list runs: %d %s", status, body)
	}
	for _, path := range []string{
		"/api/runs/" + runID,
		"/api/runs/" + runID + "/snapshot",
		"/api/runs/" + runID + "/graph",
		"/api/runs/" + runID + "/findings",
		"/api/runs/" + runID + "/assets",
		"/api/runs/" + runID + "/evidence",
		"/api/runs/" + runID + "/events",
		"/api/runs/" + runID + "/tasks",
		"/api/runs/" + runID + "/approvals",
		"/api/runs/" + runID + "/audit",
		"/api/runs/" + runID + "/messages",
	} {
		status, body = request(t, ts, "GET", path, nil)
		if status != http.StatusOK {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
	status, body = request(t, ts, "GET", "/api/runs/"+runID+"/report", nil)
	if status != http.StatusOK || !strings.Contains(string(body), "报告") {
		t.Fatalf("report: %d %s", status, body)
	}
}

func TestConfigMasksAndPreservesKeys(t *testing.T) {
	_, _, ts := newTestServer(t, true)
	status, body := request(t, ts, "GET", "/api/config", nil)
	if status != http.StatusOK || !strings.Contains(string(body), "****1234") {
		t.Fatalf("config get: %d %s", status, body)
	}
	var cfg config.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Providers[0].APIKey != "****1234" {
		t.Fatalf("key not masked: %s", cfg.Providers[0].APIKey)
	}
	cfg.ActiveProvider = "p1"
	status, body = request(t, ts, "PUT", "/api/config", cfg)
	if status != http.StatusOK {
		t.Fatalf("config put: %d %s", status, body)
	}
	saved, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "sk-secret-1234") {
		t.Fatalf("saved config lost the real key: %s", saved)
	}
}

func TestPostMessageWithoutProviderFailsFast(t *testing.T) {
	_, _, ts := newTestServer(t, false)
	status, body := request(t, ts, "POST", "/api/runs", map[string]any{
		"mode": "daily", "target": "t", "objective": "o",
	})
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var created struct {
		Run store.Run `json:"run"`
	}
	_ = json.Unmarshal(body, &created)
	status, body = request(t, ts, "POST", "/api/runs/"+created.Run.ID+"/messages", map[string]any{"text": "你好"})
	if status != http.StatusConflict || !strings.Contains(string(body), "LLM") {
		t.Fatalf("expected 409 with LLM hint, got %d %s", status, body)
	}
}

func TestApprovalDecisionEndpoint(t *testing.T) {
	srv, st, ts := newTestServer(t, true)
	run, _, err := st.CreateRun(store.Run{Mode: "redteam", Target: "t", Objective: "o"})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := st.CreateApproval(store.Approval{RunID: run.ID, Tool: "shell", Input: `{"command":"id"}`, Reason: "test"})
	if err != nil {
		t.Fatal(err)
	}
	status, body := request(t, ts, "POST", "/api/approvals/"+approval.ID+"/decide", map[string]any{"approve": true})
	if status != http.StatusOK {
		t.Fatalf("decide: %d %s", status, body)
	}
	got, _ := st.GetApproval(approval.ID)
	if got.Status != "approved" {
		t.Fatalf("approval status = %s", got.Status)
	}
	_ = srv
}

func TestHubBroadcasts(t *testing.T) {
	h := NewHub()
	ch := h.Subscribe("r1")
	defer h.Unsubscribe("r1", ch)
	h.Broadcast("r1", map[string]any{"type": "event"})
	select {
	case data := <-ch:
		if !strings.Contains(string(data), "event") {
			t.Fatalf("frame = %s", data)
		}
	default:
		t.Fatal("expected a frame")
	}
}

func TestSSEStreamDeliversEvents(t *testing.T) {
	srv, _, ts := newTestServer(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/runs/run-x/events/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	srv.hub.Broadcast("run-x", map[string]any{"type": "event", "hello": "world"})
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "hello") && !strings.Contains(string(buf[:n]), "connected") {
		t.Fatalf("unexpected stream payload: %q", string(buf[:n]))
	}
}

func TestRejectsNonJSONAndCrossOrigin(t *testing.T) {
	_, _, ts := newTestServer(t, true)
	body := `{"mode":"daily","target":"t","objective":"o"}`

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("text/plain POST should be rejected, got %d", resp.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Origin", "http://evil.example")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin POST should be forbidden, got %d", resp2.StatusCode)
	}
}
