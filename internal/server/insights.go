package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/store"
)

var findingStates = map[string]bool{
	"open": true, "confirmed": true, "fixed": true,
	"ignored": true, "false_positive": true,
}

// handleFindingStatus moves a finding through its triage state.
func (s *Server) handleFindingStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		State string `json:"state"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	state := strings.TrimSpace(req.State)
	if !findingStates[state] {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("未知漏洞状态 %q", state))
		return
	}
	node, err := s.store.GetNode(id)
	if err != nil || node.Kind != "finding" {
		writeErr(w, http.StatusNotFound, fmt.Errorf("漏洞不存在"))
		return
	}
	if err := s.store.SetNodeState(id, state); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	_, _ = s.store.AddEvent(store.Event{
		RunID: node.RunID, Kind: "finding_status",
		Summary: "漏洞状态更新", Detail: id + " → " + state,
	})
	s.hub.Broadcast(node.RunID, map[string]any{"type": "finding.status", "id": id, "state": state})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "state": state})
}

// handleAllApprovals lists pending approvals across every conversation.
func (s *Server) handleAllApprovals(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "pending"
	}
	approvals, err := s.store.ListApprovals("", status, queryInt(r, "limit", 100))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": approvals})
}

// handleTaskCancel stops one subagent task.
func (s *Server) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	task, err := s.store.GetTask(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err := s.subagents.Cancel(r.Context(), task.RunID, task.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": task.ID})
}
