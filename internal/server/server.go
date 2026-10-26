package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/guaidao2/lingeyebot/internal/agent"
	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/knowledge"
	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/mcp"
	"github.com/guaidao2/lingeyebot/internal/mode"
	"github.com/guaidao2/lingeyebot/internal/policy"
	"github.com/guaidao2/lingeyebot/internal/skill"
	"github.com/guaidao2/lingeyebot/internal/store"
	"github.com/guaidao2/lingeyebot/internal/tool"
)

// Server wires configuration, storage, agents, tools and the HTTP surface.
type Server struct {
	mu        sync.RWMutex
	cfg       config.Config
	store     *store.Store
	skills    *skill.Loader
	knowledge *knowledge.Store
	mcp       *mcp.Manager
	providers map[string]llm.Provider
	hub       *Hub
	loop      *agent.Loop
	subagents *agent.SubagentManager
	gate      *policy.Engine
	http      *http.Server
	version   string
	startedAt time.Time
}

// New builds a server and connects configured MCP servers.
func New(cfg config.Config, st *store.Store, skills *skill.Loader, version string) (*Server, error) {
	s := &Server{
		cfg: cfg, store: st, skills: skills,
		knowledge: knowledge.New(filepath.Join(cfg.DataDir, "agents")),
		providers: buildProviders(cfg), hub: NewHub(),
		version: version, startedAt: time.Now(),
	}
	mcpMgr := mcp.NewManager(cfg.MCP, cfg.Workspace)
	connectCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	if err := mcpMgr.Connect(connectCtx); err != nil {
		log.Printf("mcp: %v", err)
	}
	cancel()
	s.mcp = mcpMgr

	gate := policy.New(st, cfg)
	gate.OnPending = func(a store.Approval) {
		s.hub.Broadcast(a.RunID, map[string]any{"type": "approval", "approval": a})
	}
	s.gate = gate

	s.loop = &agent.Loop{
		Store: st, Config: cfg,
		ProviderFor: s.providerFor,
		ExecutorFor: s.executorFor,
		GateFor:     s.gateFor,
		PromptFor:   s.promptFor,
		Sink:        serverSink{s.hub},
	}
	s.subagents = &agent.SubagentManager{
		Store: st, Config: cfg, Loop: s.loop,
		ProviderFor: s.providerFor,
		ExecutorFor: s.executorForRole,
		GateFor:     s.gateFor,
		RolePrompt:  s.rolePrompt,
		Sink:        serverSink{s.hub},
	}
	s.loop.OnStop = s.subagents.CancelRun
	return s, nil
}

func buildProviders(cfg config.Config) map[string]llm.Provider {
	out := map[string]llm.Provider{}
	for _, p := range cfg.Providers {
		prov, err := llm.New(llm.Config{
			ID: p.ID, Kind: p.Kind, BaseURL: p.BaseURL, APIKey: p.APIKey,
			Model: p.Model, MaxTokens: p.MaxTokens, MaxTokensField: p.MaxTokensField,
			ContextWindow: p.ContextWindow, ReasoningMode: p.ReasoningMode,
			ReasoningEffort: p.ReasoningEffort, ReasoningProfile: p.ReasoningProfile,
			Temperature: p.Temperature,
		}, nil)
		if err != nil {
			log.Printf("provider %s: %v", p.ID, err)
			continue
		}
		out[p.ID] = prov
	}
	return out
}

type serverSink struct{ hub *Hub }

func (s serverSink) Emit(runID string, ev agent.Event) {
	s.hub.Broadcast(runID, map[string]any{"type": "event", "runId": runID, "event": ev})
}

func (s *Server) providerFor(run store.Run) (llm.Provider, string, bool) {
	s.mu.RLock()
	cfg := s.cfg
	providers := s.providers
	s.mu.RUnlock()

	var chosen config.Provider
	ok := false
	if run.ProviderID != "" {
		for _, p := range cfg.Providers {
			if p.ID == run.ProviderID {
				chosen, ok = p, true
				break
			}
		}
	}
	if !ok {
		chosen, ok = cfg.Active()
	}
	if !ok {
		return nil, "", false
	}
	prov := providers[chosen.ID]
	if prov == nil {
		built, err := llm.New(llm.Config{
			ID: chosen.ID, Kind: chosen.Kind, BaseURL: chosen.BaseURL, APIKey: chosen.APIKey,
			Model: chosen.Model, MaxTokens: chosen.MaxTokens, MaxTokensField: chosen.MaxTokensField,
			ContextWindow: chosen.ContextWindow, ReasoningMode: chosen.ReasoningMode,
			ReasoningEffort: chosen.ReasoningEffort, ReasoningProfile: chosen.ReasoningProfile,
			Temperature: chosen.Temperature,
		}, nil)
		if err != nil {
			return nil, "", false
		}
		prov = built
	}
	model := run.Model
	if model == "" {
		model = chosen.Model
	}
	if model == "" {
		return nil, "", false
	}
	return prov, model, true
}

func (s *Server) gateFor(store.Run, string) agent.Gate { return s.gate }

func (s *Server) promptFor(run store.Run, snap store.Snapshot, role string) string {
	d, ok := mode.Get(run.Mode)
	if !ok {
		d, _ = mode.Get("daily")
	}
	index := ""
	s.mu.RLock()
	skills := s.skills
	s.mu.RUnlock()
	if skills != nil {
		index = skills.Index()
	}
	roleKey := role
	if roleKey == "" || roleKey == "main" {
		roleKey = "main"
	}
	if roleKey == "main" {
		return d.MainSystem(run, snap, index) + s.knowledgeIndex(roleKey) + s.projectContext(run)
	}
	return d.RoleSystem(role, run, snap, index) + s.knowledgeIndex(roleKey) + s.projectContext(run)
}

func (s *Server) rolePrompt(run store.Run, role string) string {
	d, ok := mode.Get(run.Mode)
	if !ok {
		d, _ = mode.Get("daily")
	}
	index := ""
	s.mu.RLock()
	skills := s.skills
	s.mu.RUnlock()
	if skills != nil {
		index = skills.Index()
	}
	roleKey := role
	if roleKey == "" {
		roleKey = "main"
	}
	snap, err := s.store.Snapshot(run.ID)
	if err != nil {
		return d.RoleSystem(role, run, store.Snapshot{Run: &run}, index) + s.knowledgeIndex(roleKey) + s.projectContext(run)
	}
	return d.RoleSystem(role, run, snap, index) + s.knowledgeIndex(roleKey) + s.projectContext(run)
}

func (s *Server) knowledgeIndex(role string) string {
	s.mu.RLock()
	store := s.knowledge
	s.mu.RUnlock()
	if store == nil {
		return ""
	}
	index := store.Index(role)
	if index == "" {
		return ""
	}
	return "\n\n【角色知识库】\n以下是你自己的方法论索引；需要全文用 knowledge_load。\n" + index
}

// projectContext injects the shared project scope, memory and pinned blackboard.
func (s *Server) projectContext(run store.Run) string {
	if run.ProjectID == "" {
		return ""
	}
	project, err := s.store.GetProject(run.ProjectID)
	if err != nil {
		return ""
	}
	memory, _ := s.store.ListProjectMemory(run.ProjectID, 20)
	board, _ := s.store.ListProjectNodes(run.ProjectID, 30)

	var b strings.Builder
	fmt.Fprintf(&b, "\n\n【项目】%s（%s）\n", project.Name, project.NKey)
	if project.Description != "" {
		fmt.Fprintf(&b, "【项目说明】%s\n", project.Description)
	}
	if project.Authorization != "" {
		fmt.Fprintf(&b, "【项目授权】%s\n", project.Authorization)
	}
	if project.Scope != "" {
		fmt.Fprintf(&b, "【项目范围】%s\n", project.Scope)
	}
	if len(memory) > 0 {
		b.WriteString("【项目记忆】跨会话共享，写入前先核对；用 board_write 追加：\n")
		for i, m := range memory {
			if i >= 20 {
				break
			}
			fmt.Fprintf(&b, "- [%s] %s: %s\n", m.Kind, m.Key, m.Value)
		}
	}
	pinned := 0
	for _, n := range board {
		if !n.Pinned || pinned >= 12 {
			continue
		}
		var payload struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(n.Payload, &payload)
		text := payload.Detail
		if text == "" {
			text = payload.Title
		}
		if text != "" {
			fmt.Fprintf(&b, "- [%s] %s\n", n.Kind, text)
			pinned++
		}
	}
	b.WriteString("共享黑板：board_read 读取项目记忆/节点；board_write 写入跨会话事实、决策或凭据（pinned 会进入项目记忆）。\n")
	return b.String()
}

func (s *Server) executorFor(run store.Run, taskID string) (agent.Executor, error) {
	role := "main"
	if taskID != "" {
		if task, err := s.store.GetTask(taskID); err == nil {
			role = task.Role
		}
	}
	return s.executorForRole(run, taskID, role)
}

func (s *Server) executorForRole(run store.Run, taskID, role string) (agent.Executor, error) {
	d, ok := mode.Get(run.Mode)
	if !ok {
		d, _ = mode.Get("daily")
	}
	s.mu.RLock()
	cfg := s.cfg
	skills := s.skills
	mcpMgr := s.mcp
	knowledgeStore := s.knowledge
	s.mu.RUnlock()

	sessionID := ""
	if taskID == "" {
		if sess, err := s.store.EnsureSession(run.ID); err == nil {
			sessionID = sess.ID
		}
	} else if sess, err := s.store.EnsureSessionKey(run.ID, "task:"+taskID); err == nil {
		sessionID = sess.ID
	}

	roleKey := role
	if roleKey == "" {
		roleKey = "main"
	}
	env := tool.Env{
		Store: s.store, Config: cfg, Run: run, TaskID: taskID, SessionID: sessionID,
		Agent: role, Role: roleKey, Workspace: cfg.Workspace, Subagents: s.subagents,
		Knowledge:    knowledgeStore,
		ValidateGoal: s.validateGoal(run),
	}
	if skills != nil {
		env.Skills = skillsAdapter{skills}
	}
	if mcpMgr != nil {
		env.MCP = mcpAdapter{mcpMgr}
	}
	return tool.Build(env).Filter(d.ToolsFor(role)), nil
}

func (s *Server) validateGoal(run store.Run) func(store.Goal) error {
	d, ok := mode.Get(run.Mode)
	if !ok {
		return nil
	}
	return func(g store.Goal) error {
		snap, err := s.store.Snapshot(run.ID)
		if err != nil {
			return err
		}
		cov, err := s.store.Coverage(run.ID)
		if err != nil {
			return err
		}
		return d.ValidateGoal(s.store, run, snap, cov, g)
	}
}

// Config returns a copy of the current configuration.
func (s *Server) Config() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// ApplyConfig persists and applies a new configuration.
func (s *Server) ApplyConfig(in config.Config) error {
	s.mu.RLock()
	prev := s.cfg
	s.mu.RUnlock()

	for i := range in.Providers {
		p := &in.Providers[i]
		if strings.Contains(p.APIKey, "****") || p.APIKey == "" {
			matched := false
			for _, old := range prev.Providers {
				if old.ID == p.ID {
					p.APIKey = old.APIKey
					matched = true
					break
				}
			}
			if !matched {
				// The provider id is editable. Recover a masked key by its
				// visible suffix only when exactly one old key matches; two
				// providers can share the same last four characters.
				candidate := ""
				matches := 0
				for _, old := range prev.Providers {
					if old.APIKey != "" && config.MaskKey(old.APIKey) == p.APIKey {
						candidate = old.APIKey
						matches++
					}
				}
				if matches == 1 {
					p.APIKey = candidate
					matched = true
				}
			}
			if !matched {
				p.APIKey = ""
			}
		}
	}
	for i := range in.MCP {
		old, ok := findMCPServer(prev.MCP, in.MCP[i].ID)
		if !ok {
			old, ok = findMCPServerByShape(prev.MCP, in.MCP[i])
		}
		if !ok {
			// If the server was renamed and re-pointed in the same save there
			// is no previous secret to restore; clear masked placeholders so a
			// literal "****abcd" is never persisted as a credential.
			in.MCP[i].Env = clearMaskedMap(in.MCP[i].Env)
			in.MCP[i].Headers = clearMaskedMap(in.MCP[i].Headers)
			continue
		}
		in.MCP[i].Env = restoreMaskedMap(in.MCP[i].Env, old.Env)
		in.MCP[i].Headers = restoreMaskedMap(in.MCP[i].Headers, old.Headers)
	}
	if in.Auth.Secret == "" {
		in.Auth.Secret = prev.Auth.Secret
	}
	if in.Auth.PasswordHash == "" {
		in.Auth.PasswordHash = prev.Auth.PasswordHash
	}
	if in.Auth.Username == "" {
		in.Auth.Username = prev.Auth.Username
	}
	if in.Auth.SessionDays <= 0 {
		in.Auth.SessionDays = prev.Auth.SessionDays
	}
	if in.Auth.PasswordHash != "" {
		in.Auth.Enabled = true
	}
	if in.Version < config.SchemaVersion {
		in.Version = config.SchemaVersion
	}
	if err := config.Save(in); err != nil {
		return err
	}
	providers := buildProviders(in)
	mcpMgr := mcp.NewManager(in.MCP, in.Workspace)
	newSkills := skill.New(in.Skills)
	newKnowledge := knowledge.New(filepath.Join(in.DataDir, "agents"))
	connectCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	if err := mcpMgr.Connect(connectCtx); err != nil {
		log.Printf("mcp reload: %v", err)
	}
	cancel()

	s.mu.Lock()
	oldMCP := s.mcp
	s.cfg = in
	s.providers = providers
	s.mcp = mcpMgr
	s.skills = newSkills
	s.knowledge = newKnowledge
	s.mu.Unlock()

	if oldMCP != nil {
		oldMCP.Close()
	}
	s.loop.SetConfig(in)
	s.gate.SetConfig(in)
	s.subagents.SetConfig(in)
	return nil
}

type skillsAdapter struct{ l *skill.Loader }

func (a skillsAdapter) List() []tool.SkillInfo {
	var out []tool.SkillInfo
	for _, s := range a.l.List() {
		out = append(out, tool.SkillInfo{Name: s.Name, Description: s.Description, Path: s.Path, Body: s.Body})
	}
	return out
}

func (a skillsAdapter) Get(name string) (tool.SkillInfo, bool) {
	s, ok := a.l.Get(name)
	return tool.SkillInfo{Name: s.Name, Description: s.Description, Path: s.Path, Body: s.Body}, ok
}

type mcpAdapter struct{ m *mcp.Manager }

func (a mcpAdapter) Tools() []tool.MCPTool {
	var out []tool.MCPTool
	for _, t := range a.m.Tools() {
		out = append(out, tool.MCPTool{Server: t.Server, Name: t.Name, Description: t.Description, Schema: t.Schema})
	}
	return out
}

func (a mcpAdapter) Call(ctx context.Context, server, toolName string, args json.RawMessage) (string, error) {
	return a.m.Call(ctx, server, toolName, args)
}

// Handler builds the HTTP router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("POST /api/auth/password", s.handleSetPassword)
	mux.HandleFunc("GET /api/modes", s.handleModes)
	mux.HandleFunc("GET /api/agents/catalog", s.handleAgentCatalog)
	mux.HandleFunc("GET /api/agents/{role}/knowledge", s.handleKnowledgeList)
	mux.HandleFunc("POST /api/agents/{role}/knowledge", s.handleKnowledgeSave)
	mux.HandleFunc("GET /api/agents/{role}/knowledge/{name}", s.handleKnowledgeGet)
	mux.HandleFunc("GET /api/config", s.handleConfigGet)
	mux.HandleFunc("PUT /api/config", s.handleConfigPut)
	mux.HandleFunc("GET /api/skills", s.handleSkills)
	mux.HandleFunc("GET /api/skills/{name}", s.handleSkillGet)
	mux.HandleFunc("POST /api/skills", s.handleSkillSave)
	mux.HandleFunc("GET /api/mcp", s.handleMCP)
	mux.HandleFunc("POST /api/mcp/test", s.handleMCPTest)
	mux.HandleFunc("POST /api/mcp/reconnect", s.handleMCPReconnect)
	mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	mux.HandleFunc("GET /api/findings", s.handleAllFindings)
	mux.HandleFunc("POST /api/findings/{id}/status", s.handleFindingStatus)
	mux.HandleFunc("GET /api/approvals", s.handleAllApprovals)
	mux.HandleFunc("GET /api/assets", s.handleAllAssets)
	mux.HandleFunc("GET /api/tasks", s.handleAllTasks)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleTaskDetail)
	mux.HandleFunc("POST /api/tasks/{id}/cancel", s.handleTaskCancel)
	mux.HandleFunc("GET /api/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("GET /api/projects/{id}", s.handleGetProject)
	mux.HandleFunc("PUT /api/projects/{id}", s.handleUpdateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	mux.HandleFunc("GET /api/projects/{id}/conversations", s.handleProjectConversations)
	mux.HandleFunc("POST /api/projects/{id}/conversations", s.handleCreateProjectConversation)
	mux.HandleFunc("GET /api/projects/{id}/board", s.handleProjectBoard)
	mux.HandleFunc("POST /api/projects/{id}/board", s.handleProjectBoard)
	mux.HandleFunc("GET /api/projects/{id}/memory", s.handleProjectMemory)
	mux.HandleFunc("POST /api/projects/{id}/memory", s.handleProjectMemory)

	mux.HandleFunc("GET /api/runs", s.handleListRuns)
	mux.HandleFunc("POST /api/runs", s.handleCreateRun)
	mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	mux.HandleFunc("GET /api/runs/{id}/messages", s.handleMessages)
	mux.HandleFunc("POST /api/runs/{id}/messages", s.handlePostMessage)
	mux.HandleFunc("POST /api/runs/{id}/stop", s.handleStop)
	mux.HandleFunc("POST /api/runs/{id}/resume", s.handleResume)
	mux.HandleFunc("GET /api/runs/{id}/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /api/runs/{id}/graph", s.handleGraph)
	mux.HandleFunc("GET /api/runs/{id}/findings", s.handleFindings)
	mux.HandleFunc("GET /api/runs/{id}/assets", s.handleAssets)
	mux.HandleFunc("GET /api/runs/{id}/evidence", s.handleEvidence)
	mux.HandleFunc("GET /api/runs/{id}/report", s.handleReport)
	mux.HandleFunc("GET /api/runs/{id}/events", s.handleEvents)
	mux.HandleFunc("GET /api/runs/{id}/events/stream", s.handleStream)
	mux.HandleFunc("GET /api/runs/{id}/tasks", s.handleTasks)
	mux.HandleFunc("GET /api/runs/{id}/approvals", s.handleApprovals)
	mux.HandleFunc("GET /api/runs/{id}/audit", s.handleAudit)
	mux.HandleFunc("POST /api/runs/{id}/goals", s.handleAddGoal)
	mux.HandleFunc("POST /api/approvals/{id}/decide", s.handleDecide)

	mux.Handle("/", s.staticHandler())
	return recoverMiddleware(s.hostGuard(sameOrigin(s.authMiddleware(mux))))
}

// hostGuard rejects requests whose Host is not the configured listen host.
// This closes the DNS-rebinding path to unauthenticated local endpoints such
// as /api/mcp/test while keeping localhost and the bound address usable.
func (s *Server) hostGuard(next http.Handler) http.Handler {
	cfg := s.Config()
	raw := cfg.Listen
	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Non-browser clients (CLI/tests) do not send Origin/Referer; the
		// DNS-rebinding threat only applies to browser-originated requests.
		if r.Header.Get("Origin") == "" && r.Header.Get("Referer") == "" {
			next.ServeHTTP(w, r)
			return
		}
		reqHost := r.Host
		if h, _, err := net.SplitHostPort(reqHost); err == nil {
			reqHost = h
		}
		reqHost = strings.Trim(reqHost, "[]")
		if reqHost != host && reqHost != "localhost" && reqHost != "127.0.0.1" && reqHost != "::1" {
			http.Error(w, "host rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	s.mu.RLock()
	addr := s.cfg.Listen
	s.mu.RUnlock()
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("lingeyebot %s listening on http://%s", s.version, addr)
	return s.http.ListenAndServe()
}

// Shutdown stops agents, MCP clients, the HTTP server and the database.
func (s *Server) Shutdown(ctx context.Context) {
	if s.loop != nil {
		s.loop.Shutdown()
	}
	if s.subagents != nil {
		s.subagents.Shutdown()
	}
	if s.mcp != nil {
		s.mcp.Close()
	}
	if s.http != nil {
		_ = s.http.Shutdown(ctx)
	}
	if s.store != nil {
		_ = s.store.Close()
	}
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic serving %s: %v", r.URL.Path, rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// staticHandler serves the embedded SPA with an index fallback.
func (s *Server) staticHandler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, placeholderHTML)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			if strings.Contains(path, ".") {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

var errNotConfigured = errors.New("LLM 端点尚未配置：请在设置里填写 base URL、API Key 与模型名")

const placeholderHTML = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>lingeyebot</title>
<body style="font-family:system-ui,sans-serif;max-width:680px;margin:12vh auto;line-height:1.6;background:#0a0d13;color:#dbe4f3">
<h1>lingeyebot</h1><p>Web UI 尚未构建。运行 <code>./scripts/build.sh</code>（需要 Node.js），或把 <code>web/dist</code> 复制到 <code>internal/server/dist</code> 后重新编译。</p>
<p>API 已可用：<a style="color:#8bbcff" href="/api/health">/api/health</a></p></body></html>`

func findMCPServer(servers []config.MCPServer, id string) (config.MCPServer, bool) {
	for _, server := range servers {
		if server.ID == id {
			return server, true
		}
	}
	return config.MCPServer{}, false
}

// findMCPServerByShape recovers the previous secret map after the user renames
// a server: match the transport plus its command/URL, not the editable id.
func findMCPServerByShape(servers []config.MCPServer, target config.MCPServer) (config.MCPServer, bool) {
	for _, server := range servers {
		if server.Transport != target.Transport {
			continue
		}
		if target.Transport == "http" && target.URL != "" && server.URL == target.URL {
			return server, true
		}
		if target.Transport == "stdio" && target.Command != "" && server.Command == target.Command && strings.Join(server.Args, "\x00") == strings.Join(target.Args, "\x00") {
			return server, true
		}
	}
	return config.MCPServer{}, false
}

func restoreMaskedMap(current, previous map[string]string) map[string]string {
	if len(current) == 0 || len(previous) == 0 {
		return current
	}
	for key, value := range current {
		if strings.Contains(value, "****") {
			if old, ok := previous[key]; ok {
				current[key] = old
			}
		}
	}
	return current
}

func clearMaskedMap(current map[string]string) map[string]string {
	for key, value := range current {
		if strings.Contains(value, "****") {
			current[key] = ""
		}
	}
	return current
}
