package server

import (
	"net/http"

	"github.com/guaidao2/lingeyebot/internal/mode"
)

// handleAgentCatalog exposes the built-in role and main-agent prompts without
// requiring a run. This is a read-only catalog: the UI can show methodology,
// tools and participating modes before assigning any task.
func (s *Server) handleAgentCatalog(w http.ResponseWriter, r *http.Request) {
	model := ""
	if provider, ok := s.Config().Active(); ok {
		model = provider.Model
	}
	roles := mode.RoleCatalog()
	for i := range roles {
		roles[i].Model = model
	}
	main := mode.MainCatalog()
	for i := range main {
		main[i].Model = model
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles, "modes": main})
}
