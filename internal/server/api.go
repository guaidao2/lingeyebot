package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/mode"
	"github.com/guaidao2/lingeyebot/internal/model"
	"github.com/guaidao2/lingeyebot/internal/store"
)

type runSummary struct {
	store.Run
	Goals     int  `json:"goals"`
	OpenGoals int  `json:"openGoals"`
	Intents   int  `json:"intents"`
	Facts     int  `json:"facts"`
	Findings  int  `json:"findings"`
	Running   bool `json:"running"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "version": s.version, "uptimeSeconds": int(time.Since(s.startedAt).Seconds()),
	})
}

func (s *Server) handleModes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"modes": mode.MetadataList()})
}

func (s *Server) handleConfigGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Config().Public())
}

func (s *Server) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	var in config.Config
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.ApplyConfig(in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.Config().Public())
}

func (s *Server) handleSkills(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	skills := s.skills
	s.mu.RUnlock()
	out := []map[string]string{}
	if skills != nil {
		for _, sk := range skills.List() {
			out = append(out, map[string]string{"name": sk.Name, "description": sk.Description, "path": sk.Path})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": out})
}

func (s *Server) handleMCP(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	mcpMgr := s.mcp
	s.mu.RUnlock()
	if mcpMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"servers": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": mcpMgr.Status()})
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("projectId")
	var (
		runs []store.Run
		err  error
	)
	if projectID != "" {
		runs, err = s.store.ListRunsByProject(projectID, 200)
	} else {
		runs, err = s.store.ListRuns(100)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]runSummary, 0, len(runs))
	for _, run := range runs {
		summary := runSummary{Run: run}
		if goals, err := s.store.ListGoals(run.ID); err == nil {
			summary.Goals = len(goals)
			for _, g := range goals {
				if g.Status == "open" {
					summary.OpenGoals++
				}
			}
		}
		if nodes, err := s.store.ListNodes(run.ID, ""); err == nil {
			for _, n := range nodes {
				switch n.Kind {
				case model.KindIntent:
					summary.Intents++
				case model.KindFact:
					summary.Facts++
				case model.KindFinding:
					summary.Findings++
				}
			}
		}
		summary.Running = s.loop.Running(run.ID)
		out = append(out, summary)
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": out})
}

type createRunRequest struct {
	ProjectID     string             `json:"projectId"`
	Title         string             `json:"title"`
	Mode          string             `json:"mode"`
	Target        string             `json:"target"`
	Objective     string             `json:"objective"`
	Authorization string             `json:"authorization"`
	Scope         string             `json:"scope"`
	ProviderID    string             `json:"providerId"`
	Model         string             `json:"model"`
	GoalMode      *bool              `json:"goalMode,omitempty"`
	Constraints   []model.Constraint `json:"constraints"`
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req createRunRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	run, goal, err := s.createConversation(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.hub.Broadcast(run.ID, map[string]any{"type": "run.created", "run": run})
	writeJSON(w, http.StatusCreated, map[string]any{"run": run, "goal": goal})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetRun(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	goals, _ := s.store.ListGoals(run.ID)
	constraints, _ := s.store.ListConstraints(run.ID)
	coverage, _ := s.store.Coverage(run.ID)
	nodes, _ := s.store.ListNodes(run.ID, "")
	counts := map[string]int{}
	for _, n := range nodes {
		counts[n.Kind]++
	}
	definition, _ := mode.Get(run.Mode)
	project, _ := s.store.GetProject(run.ProjectID)
	writeJSON(w, http.StatusOK, map[string]any{
		"run": run, "project": project, "goals": goals, "constraints": constraints,
		"counts": counts, "coverage": coverage,
		"running": s.loop.Running(run.ID),
		"mode": map[string]any{
			"id": definition.ID, "name": definition.Name, "tagline": definition.Tagline, "roles": definition.Roles,
		},
	})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	sess, err := s.store.EnsureSession(runID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	limit := queryInt(r, "limit", 300)
	all := r.URL.Query().Get("all") == "1"
	msgs, err := s.store.ListMessages(sess.ID, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]store.Message, 0, len(msgs))
	for _, m := range msgs {
		if !all && !m.Visible {
			continue
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out, "running": s.loop.Running(runID)})
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("消息不能为空"))
		return
	}
	run, err := s.store.GetRun(runID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if run.Status == "achieved" {
		writeErr(w, http.StatusConflict, fmt.Errorf("任务已完成；如需继续，请新增目标或新建任务"))
		return
	}
	if _, _, ok := s.providerFor(run); !ok {
		writeErr(w, http.StatusConflict, errNotConfigured)
		return
	}
	if err := s.loop.Submit(runID, req.Text); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.hub.Broadcast(runID, map[string]any{"type": "message"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if err := s.loop.Stop(runID); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	s.hub.Broadcast(runID, map[string]any{"type": "run.status", "status": "paused"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if err := s.loop.Resume(runID); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	s.hub.Broadcast(runID, map[string]any{"type": "run.status", "status": "active"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	snap, err := s.store.Snapshot(runID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	cov, _ := s.store.Coverage(runID)
	review, _ := s.store.Review(runID)
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot": snap, "coverage": cov, "review": review, "running": s.loop.Running(runID),
	})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	snap, err := s.store.Snapshot(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": snap.Nodes, "edges": snap.Edges, "anchors": snap.Anchors, "assets": snap.Assets,
	})
}

func (s *Server) handleFindings(w http.ResponseWriter, r *http.Request) {
	snap, err := s.store.Snapshot(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	evidenceByID := map[string]store.Evidence{}
	for _, e := range snap.Evidence {
		evidenceByID[e.ID] = e
	}
	type findingView struct {
		ID        string           `json:"id"`
		State     string           `json:"state"`
		CreatedAt string           `json:"createdAt"`
		Finding   model.Finding    `json:"finding"`
		Evidence  []store.Evidence `json:"evidence"`
	}
	out := []findingView{}
	for _, n := range snap.Nodes {
		if n.Kind != model.KindFinding {
			continue
		}
		f, err := store.ParseFinding(n)
		if err != nil {
			continue
		}
		view := findingView{ID: n.ID, State: n.State, CreatedAt: n.CreatedAt, Finding: f}
		for _, id := range f.EvidenceIDs {
			if e, ok := evidenceByID[id]; ok {
				view.Evidence = append(view.Evidence, e)
			}
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": out})
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	cov, err := s.store.Coverage(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	assets := make([]store.Asset, 0, len(cov.Assets))
	for _, a := range cov.Assets {
		assets = append(assets, a.Asset)
	}
	writeJSON(w, http.StatusOK, map[string]any{"coverage": cov, "assets": store.SortedAssets(assets)})
}

func (s *Server) handleEvidence(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	list, err := s.store.ListEvidence(runID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	type evidenceView struct {
		store.Evidence
		Preview string `json:"preview"`
	}
	out := []evidenceView{}
	for i, e := range list {
		if i >= 200 {
			break
		}
		preview := ""
		if body, err := s.store.ReadEvidence(e.ID); err == nil {
			preview = agentShort(string(body), 4000)
		}
		out = append(out, evidenceView{Evidence: e, Preview: preview})
	}
	writeJSON(w, http.StatusOK, map[string]any{"evidence": out})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	md, err := s.store.Report(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"markdown": md})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.ListEvents(r.PathValue("id"), queryInt(r, "limit", 300))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.ListTasks(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	pending, err := s.store.ListApprovals(runID, "pending", 50)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	recent, _ := s.store.ListApprovals(runID, "", 20)
	writeJSON(w, http.StatusOK, map[string]any{"pending": pending, "recent": recent})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListAudit(r.PathValue("id"), queryInt(r, "limit", 200))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit": entries})
}

func (s *Server) handleAddGoal(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	var req struct {
		Text       string `json:"text"`
		Acceptance string `json:"acceptance"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("目标不能为空"))
		return
	}
	goal, err := s.store.AddGoal(store.Goal{RunID: runID, Text: req.Text, Acceptance: req.Acceptance})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.store.SetGoalMode(runID, true); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if run, err := s.store.GetRun(runID); err == nil && (run.Status == "achieved" || run.Status == "stalled") {
		_ = s.store.UpdateRunStatus(runID, "active")
	}
	s.loop.Wake(runID)
	s.hub.Broadcast(runID, map[string]any{"type": "goal", "goal": goal})
	writeJSON(w, http.StatusCreated, goal)
}

func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Approve bool `json:"approve"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	approval, err := s.store.DecideApproval(r.PathValue("id"), req.Approve)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.hub.Broadcast(approval.RunID, map[string]any{"type": "approval.decided", "approval": approval})
	writeJSON(w, http.StatusOK, approval)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	runID := r.PathValue("id")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := s.hub.Subscribe(runID)
	defer s.hub.Unsubscribe(runID, ch)
	_, _ = io.WriteString(w, "retry: 3000\n: connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", time.Now().UnixNano(), data)
			flusher.Flush()
		case <-ticker.C:
			_, _ = io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		return fmt.Errorf("Content-Type 必须是 application/json")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("请求体为空")
	}
	return json.Unmarshal(data, v)
}

// sameOrigin rejects cross-origin state-changing requests. Combined with the
// JSON-only body check above, plain HTML form CSRF cannot reach these handlers.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					http.Error(w, "cross-origin request rejected", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func agentShort(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
