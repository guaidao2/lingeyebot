// Package mcp implements a minimal Model Context Protocol client.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/guaidao2/lingeyebot/internal/config"
)

// Tool is one tool exposed by an MCP server.
type Tool struct {
	Server      string          `json:"server"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

// Status reports one server's connection state.
type Status struct {
	ID        string   `json:"id"`
	Transport string   `json:"transport"`
	State     string   `json:"state"`
	Error     string   `json:"error,omitempty"`
	Tools     int      `json:"tools"`
	ToolNames []string `json:"toolNames,omitempty"`
}

type client interface {
	initialize(ctx context.Context) error
	listTools(ctx context.Context) ([]Tool, error)
	call(ctx context.Context, name string, args json.RawMessage) (string, error)
	close() error
}

type server struct {
	id      string
	cfg     config.MCPServer
	client  client
	tools   []Tool
	state   string
	errText string
}

// Manager owns all configured MCP servers.
type Manager struct {
	mu        sync.RWMutex
	connectMu sync.Mutex
	servers   []*server
	byID      map[string]*server
	workspace string
}

// NewManager builds a manager from configuration.
func NewManager(cfgs []config.MCPServer, workspace string) *Manager {
	m := &Manager{byID: map[string]*server{}, workspace: workspace}
	for _, cfg := range cfgs {
		cfg := cfg
		s := &server{id: cfg.ID, cfg: cfg, state: "disabled"}
		if cfg.Enabled {
			c, err := m.newClient(cfg)
			if err != nil {
				s.state = "error"
				s.errText = err.Error()
			} else {
				s.client = c
			}
		}
		m.servers = append(m.servers, s)
		m.byID[cfg.ID] = s
	}
	sort.Slice(m.servers, func(i, j int) bool { return m.servers[i].id < m.servers[j].id })
	return m
}

func (m *Manager) newClient(cfg config.MCPServer) (client, error) {
	if cfg.Transport == "http" {
		return newHTTPClient(cfg)
	}
	return newStdioClient(cfg, m.workspace)
}

// Reconnect disposes every client, rebuilds it from configuration, and
// initializes again. This is the recovery path after a transport failure.
func (m *Manager) Reconnect(ctx context.Context) error {
	m.connectMu.Lock()
	defer m.connectMu.Unlock()
	m.mu.Lock()
	for _, s := range m.servers {
		if s.client != nil {
			_ = s.client.close()
			s.client = nil
		}
		s.tools = nil
		s.errText = ""
		s.state = "disabled"
		if !s.cfg.Enabled {
			continue
		}
		c, err := m.newClient(s.cfg)
		if err != nil {
			s.state, s.errText = "error", err.Error()
			continue
		}
		s.client = c
	}
	m.mu.Unlock()
	return m.connect(ctx)
}

// Connect initializes every enabled server. Failures are recorded per server
// and do not prevent the others from loading.
func (m *Manager) Connect(ctx context.Context) error {
	m.connectMu.Lock()
	defer m.connectMu.Unlock()
	return m.connect(ctx)
}

func (m *Manager) connect(ctx context.Context) error {
	var errs []string
	for _, s := range m.servers {
		m.mu.RLock()
		c := s.client
		m.mu.RUnlock()
		if c == nil {
			continue
		}
		if err := c.initialize(ctx); err != nil {
			m.mu.Lock()
			s.state, s.errText = "error", err.Error()
			m.mu.Unlock()
			errs = append(errs, s.id+": "+err.Error())
			continue
		}
		tools, err := c.listTools(ctx)
		if err != nil {
			m.mu.Lock()
			s.state, s.errText = "error", err.Error()
			m.mu.Unlock()
			errs = append(errs, s.id+": "+err.Error())
			continue
		}
		m.mu.Lock()
		s.tools = tools
		s.state = "ready"
		m.mu.Unlock()
	}
	if len(errs) > 0 {
		return fmt.Errorf("mcp: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Tools returns every discovered tool.
func (m *Manager) Tools() []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []Tool{}
	for _, s := range m.servers {
		out = append(out, s.tools...)
	}
	return out
}

// Call invokes one tool.
func (m *Manager) Call(ctx context.Context, serverID, toolName string, args json.RawMessage) (string, error) {
	m.mu.RLock()
	s := m.byID[serverID]
	state, errText := "", ""
	var c client
	if s != nil {
		state, errText = s.state, s.errText
		c = s.client
	}
	m.mu.RUnlock()
	if s == nil {
		return "", fmt.Errorf("unknown MCP server %q", serverID)
	}
	if c == nil || state != "ready" {
		return "", fmt.Errorf("MCP server %q is not ready: %s", serverID, errText)
	}
	return c.call(ctx, toolName, args)
}

// Status returns connection diagnostics.
func (m *Manager) Status() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.servers))
	for _, s := range m.servers {
		names := make([]string, 0, len(s.tools))
		for _, t := range s.tools {
			names = append(names, t.Name)
		}
		out = append(out, Status{ID: s.id, Transport: s.cfg.Transport, State: s.state, Error: s.errText, Tools: len(s.tools), ToolNames: names})
	}
	return out
}

// Close shuts down every client.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.servers {
		if s.client != nil {
			_ = s.client.close()
		}
	}
}
