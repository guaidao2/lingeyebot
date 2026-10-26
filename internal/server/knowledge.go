package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/knowledge"
	"github.com/guaidao2/lingeyebot/internal/mode"
)

func validKnowledgeRole(role string) bool {
	return role == "main" || mode.HasRole(role)
}

func (s *Server) handleKnowledgeList(w http.ResponseWriter, r *http.Request) {
	role := r.PathValue("role")
	if !validKnowledgeRole(role) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("未知角色 %q", role))
		return
	}
	s.mu.RLock()
	kstore := s.knowledge
	s.mu.RUnlock()
	if kstore == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("知识库未初始化"))
		return
	}
	docs, err := kstore.List(role)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"docs": docs})
}

func (s *Server) handleKnowledgeGet(w http.ResponseWriter, r *http.Request) {
	role, name := r.PathValue("role"), r.PathValue("name")
	if !validKnowledgeRole(role) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("未知角色 %q", role))
		return
	}
	s.mu.RLock()
	kstore := s.knowledge
	s.mu.RUnlock()
	if kstore == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("知识库未初始化"))
		return
	}
	doc, err := kstore.Get(role, name)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleKnowledgeSave(w http.ResponseWriter, r *http.Request) {
	role := r.PathValue("role")
	if !validKnowledgeRole(role) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("未知角色 %q", role))
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Body        string `json:"body"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Description) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("知识名称与描述必填"))
		return
	}
	s.mu.RLock()
	kstore := s.knowledge
	s.mu.RUnlock()
	if kstore == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("知识库未初始化"))
		return
	}
	doc, err := kstore.Save(role, req.Name, req.Description, req.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

var _ = knowledge.MaxBody
