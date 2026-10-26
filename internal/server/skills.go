package server

import (
	"net/http"
	"strings"
)

type skillInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

func (s *Server) handleSkillGet(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	s.mu.RLock()
	loader := s.skills
	s.mu.RUnlock()
	if loader == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "skills 未启用"})
		return
	}
	sk, ok := loader.Get(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "技能不存在"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill": sk})
}

func (s *Server) handleSkillSave(w http.ResponseWriter, r *http.Request) {
	var in skillInput
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.mu.RLock()
	loader := s.skills
	s.mu.RUnlock()
	if loader == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "skills 未启用"})
		return
	}
	sk, err := loader.Save(in.Name, in.Description, in.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill": sk})
}
