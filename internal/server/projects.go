package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/agent"
	"github.com/guaidao2/lingeyebot/internal/mode"
	"github.com/guaidao2/lingeyebot/internal/store"
)

func (s *Server) handleListProjects(w http.ResponseWriter, _ *http.Request) {
	projects, err := s.store.ListProjects()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		Authorization string `json:"authorization"`
		Scope         string `json:"scope"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	project, err := s.store.CreateProject(store.Project{
		Name: req.Name, Description: req.Description, Authorization: req.Authorization, Scope: req.Scope,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	detail, err := s.store.ProjectDetail(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	running := map[string]bool{}
	for _, conversation := range detail.Conversations {
		running[conversation.ID] = s.loop.Running(conversation.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": detail, "running": running})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          *string `json:"name"`
		Description   *string `json:"description"`
		Authorization *string `json:"authorization"`
		Scope         *string `json:"scope"`
		Status        *string `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	current, err := s.store.GetProject(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		current.Name = *req.Name
	}
	if req.Description != nil {
		current.Description = *req.Description
	}
	if req.Authorization != nil {
		current.Authorization = *req.Authorization
	}
	if req.Scope != nil {
		current.Scope = *req.Scope
	}
	if req.Status != nil && *req.Status != "" {
		current.Status = *req.Status
	}
	project, err := s.store.UpdateProject(current)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	project, err := s.store.GetProject(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if project.NKey == "default" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("默认项目不能删除"))
		return
	}
	runs, err := s.store.ListRunsByProject(id, 10000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	for _, run := range runs {
		if s.loop.Running(run.ID) {
			writeErr(w, http.StatusConflict, fmt.Errorf("项目内有运行中的任务，请先停止后再删除"))
			return
		}
	}
	if err := s.store.DeleteProject(id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) handleProjectConversations(w http.ResponseWriter, r *http.Request) {
	conversations, err := s.store.ListRunsByProject(r.PathValue("id"), 200)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	running := map[string]bool{}
	for _, conversation := range conversations {
		running[conversation.ID] = s.loop.Running(conversation.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": conversations, "running": running})
}

func (s *Server) handleCreateProjectConversation(w http.ResponseWriter, r *http.Request) {
	var req createRunRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.ProjectID = r.PathValue("id")
	run, goal, err := s.createConversation(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.hub.Broadcast(run.ID, map[string]any{"type": "run.created", "run": run})
	writeJSON(w, http.StatusCreated, map[string]any{"run": run, "goal": goal})
}

func (s *Server) createConversation(req createRunRequest) (store.Run, store.Goal, error) {
	if _, ok := mode.Get(req.Mode); !ok {
		return store.Run{}, store.Goal{}, fmt.Errorf("未知模式 %q", req.Mode)
	}
	goalMode := false
	if req.GoalMode != nil {
		goalMode = *req.GoalMode
	}
	if strings.TrimSpace(req.Target) == "" {
		return store.Run{}, store.Goal{}, fmt.Errorf("target 必填")
	}
	if goalMode && strings.TrimSpace(req.Objective) == "" {
		return store.Run{}, store.Goal{}, fmt.Errorf("启用目标模式时 objective/验收必填")
	}
	if req.ProjectID == "" {
		project, err := s.store.EnsureDefaultProject()
		if err != nil {
			return store.Run{}, store.Goal{}, err
		}
		req.ProjectID = project.ID
	}
	if _, err := s.store.GetProject(req.ProjectID); err != nil {
		return store.Run{}, store.Goal{}, fmt.Errorf("项目不存在：%s", req.ProjectID)
	}
	providerID, modelName := req.ProviderID, req.Model
	if providerID == "" {
		if p, ok := s.Config().Active(); ok {
			providerID = p.ID
			if modelName == "" {
				modelName = p.Model
			}
		}
	}
	run, goal, err := s.store.CreateRun(store.Run{
		ProjectID: req.ProjectID, Title: req.Title,
		Mode: req.Mode, Target: req.Target, Objective: req.Objective,
		Authorization: req.Authorization, Scope: req.Scope, ProviderID: providerID, Model: modelName,
		GoalMode: &goalMode,
	})
	if err != nil {
		return run, goal, err
	}
	for _, constraint := range req.Constraints {
		if strings.TrimSpace(constraint.Text) == "" {
			continue
		}
		if _, err := s.store.AddConstraint(store.Constraint{
			RunID: run.ID, Kind: constraint.Kind, Text: constraint.Text, Origin: "user",
		}); err != nil {
			return run, goal, err
		}
	}
	return run, goal, nil
}

func (s *Server) handleProjectBoard(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if r.Method == http.MethodGet {
		nodes, err := s.store.ListProjectNodes(projectID, 300)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		edges, _ := s.store.ListProjectEdges(projectID)
		memory, _ := s.store.ListProjectMemory(projectID, 100)
		writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "edges": edges, "memory": memory})
		return
	}
	var req struct {
		Kind         string          `json:"kind"`
		Title        string          `json:"title"`
		Detail       string          `json:"detail"`
		Pinned       bool            `json:"pinned"`
		SourceNodeID string          `json:"sourceNodeId"`
		Payload      json.RawMessage `json:"payload"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	payload := req.Payload
	if len(payload) == 0 {
		payload, _ = json.Marshal(map[string]string{"title": req.Title, "detail": req.Detail})
	}
	kind := req.Kind
	if kind == "" {
		kind = "note"
	}
	node, err := s.store.AddProjectNode(store.ProjectNode{
		ProjectID: projectID, ConversationID: "", Kind: kind, Payload: payload,
		Pinned: req.Pinned, Origin: "human", Owner: "user", SourceNodeID: req.SourceNodeID,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var memory *store.ProjectMemory
	if req.Pinned {
		key := req.Title
		if strings.TrimSpace(key) == "" {
			key = kind + "-" + node.ID
		}
		value := req.Detail
		if strings.TrimSpace(value) == "" {
			value = req.Title
		}
		saved, err := s.store.SetProjectMemory(projectID, key, value, kind, node.ID, "user")
		if err == nil {
			memory = &saved
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"node": node, "memory": memory})
}

func (s *Server) handleProjectMemory(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if r.Method == http.MethodGet {
		memory, err := s.store.ListProjectMemory(projectID, 200)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"memory": memory})
		return
	}
	var req struct {
		Key          string `json:"key"`
		Value        string `json:"value"`
		Kind         string `json:"kind"`
		SourceNodeID string `json:"sourceNodeId"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	memory, err := s.store.SetProjectMemory(projectID, req.Key, req.Value, req.Kind, req.SourceNodeID, "user")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, memory)
}

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	dashboard, err := s.store.Dashboard()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	dashboard.Capabilities.Modes = len(mode.All())
	dashboard.Capabilities.Agents = len(agent.BuiltinRoles)
	s.mu.RLock()
	skills := s.skills
	mcpMgr := s.mcp
	s.mu.RUnlock()
	if skills != nil {
		dashboard.Capabilities.Skills = len(skills.List())
	}
	if mcpMgr != nil {
		for _, status := range mcpMgr.Status() {
			dashboard.Capabilities.MCPTools += status.Tools
			if status.State == "ready" {
				dashboard.Capabilities.MCPOnline++
			}
		}
	}
	writeJSON(w, http.StatusOK, dashboard)
}

func (s *Server) handleAllFindings(w http.ResponseWriter, r *http.Request) {
	findings, err := s.store.ListAllFindings(queryInt(r, "limit", 200))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": findings})
}

func (s *Server) handleAllAssets(w http.ResponseWriter, _ *http.Request) {
	assets, err := s.store.ListAssets("", "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": assets})
}

func (s *Server) handleAllTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.ListAllTasks(queryInt(r, "limit", 200))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) handleTaskDetail(w http.ResponseWriter, r *http.Request) {
	task, err := s.store.GetTask(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	run, err := s.store.GetRun(task.RunID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	project, err := s.store.GetProject(run.ProjectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	events, err := s.store.ListTaskEvents(run.ID, task.ID, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"task": task, "run": run, "project": project, "events": events,
	})
}
