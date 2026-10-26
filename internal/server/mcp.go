package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/mcp"
)

// handleMCPTest connects a draft server once and returns its tool inventory.
// It never mutates the running manager; the user still has to save to apply it.
func (s *Server) handleMCPReconnect(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	manager := s.mcp
	s.mu.RUnlock()
	if manager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "MCP 未初始化", "servers": []mcp.Status{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	err := manager.Reconnect(ctx)
	payload := map[string]any{"ok": err == nil, "servers": manager.Status()}
	if err != nil {
		payload["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleMCPTest(w http.ResponseWriter, r *http.Request) {
	var one config.MCPServer
	if err := readJSON(r, &one); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(one.ID) == "" {
		one.ID = "test"
	}
	s.mu.RLock()
	workspace := s.cfg.Workspace
	prev := s.cfg
	s.mu.RUnlock()
	old, ok := findMCPServer(prev.MCP, one.ID)
	if !ok {
		old, ok = findMCPServerByShape(prev.MCP, one)
	}
	if ok {
		one.Env = restoreMaskedMap(one.Env, old.Env)
		one.Headers = restoreMaskedMap(one.Headers, old.Headers)
	} else {
		one.Env = clearMaskedMap(one.Env)
		one.Headers = clearMaskedMap(one.Headers)
	}

	manager := mcp.NewManager([]config.MCPServer{one}, workspace)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	err := manager.Connect(ctx)
	statuses := manager.Status()
	manager.Close()
	status := mcp.Status{ID: one.ID, Transport: one.Transport, State: "error"}
	if len(statuses) > 0 {
		status = statuses[0]
	}
	result := map[string]any{"ok": err == nil && status.State == "ready", "status": status}
	if err != nil {
		result["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, result)
}
