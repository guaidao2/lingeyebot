// Package config loads and persists lingeyebot's instance configuration.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SchemaVersion is bumped when legacy config defaults need a one-time upgrade.
const SchemaVersion = 3

// Provider describes one LLM endpoint. Both OpenAI-compatible and Anthropic
// protocols are supported; the user supplies base URL, key and model.
type Provider struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // openai | anthropic
	BaseURL   string `json:"baseUrl"`
	APIKey    string `json:"apiKey"`
	Model     string `json:"model"`
	MaxTokens int    `json:"maxTokens,omitempty"`
	// MaxTokensField selects the OpenAI request key for the output cap.
	// "" = auto (reasoning models use max_completion_tokens), or explicit
	// "max_tokens" / "max_completion_tokens".
	MaxTokensField string `json:"maxTokensField,omitempty"`
	// ContextWindow is this channel's total context budget. 0 = inherit runtime.
	ContextWindow int `json:"contextWindow,omitempty"`
	// ReasoningMode is auto|on|off for reasoning-capable channels.
	ReasoningMode string `json:"reasoningMode,omitempty"`
	// ReasoningEffort is low|medium|high|max|xhigh|none; empty = model default.
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	// ReasoningProfile is auto|openai_compat|deepseek_compat.
	ReasoningProfile string  `json:"reasoningProfile,omitempty"`
	Temperature      float64 `json:"temperature,omitempty"`
}

// MCPServer describes one MCP endpoint. Transport is stdio or http.
type MCPServer struct {
	ID        string            `json:"id"`
	Transport string            `json:"transport"` // stdio | http
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Enabled   bool              `json:"enabled"`
}

// ApprovalConfig controls the tool-call gate.
type ApprovalConfig struct {
	// Mode is "auto" (default), "ask" or "deny".
	Mode string `json:"mode"`
	// AskTools lists tools that always require confirmation.
	AskTools []string `json:"askTools,omitempty"`
}

// AuthConfig is the single personal account. An empty password hash means
// authentication is disabled (local-only default).
type AuthConfig struct {
	Enabled      bool   `json:"enabled"`
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	Secret       string `json:"secret"`
	SessionDays  int    `json:"sessionDays"`
}

// RuntimeConfig holds agent loop limits.
type RuntimeConfig struct {
	MaxIterations    int  `json:"maxIterations"`
	MaxOutputTokens  int  `json:"maxOutputTokens"`
	MaxAutoTurns     int  `json:"maxAutoTurns"`
	ContextMessages  int  `json:"contextMessages"`
	ContextWindow    int  `json:"contextWindow"`
	ToolResultChars  int  `json:"toolResultChars"`
	AutoContinue     bool `json:"autoContinue"`
	AutoContinueWait int  `json:"autoContinueWaitSeconds"`
}

// Config is the whole instance configuration.
type Config struct {
	Version        int            `json:"version,omitempty"`
	Listen         string         `json:"listen"`
	DataDir        string         `json:"dataDir"`
	Workspace      string         `json:"workspace"`
	Skills         []string       `json:"skills"`
	Providers      []Provider     `json:"providers"`
	ActiveProvider string         `json:"activeProvider"`
	MCP            []MCPServer    `json:"mcp"`
	Approval       ApprovalConfig `json:"approval"`
	Runtime        RuntimeConfig  `json:"runtime"`
	Auth           AuthConfig     `json:"auth"`
}

// Default returns a usable configuration with no providers configured yet.
// All runtime files live under the process working directory so the binary is
// portable and never spills data into the user's home directory.
func Default() Config {
	base := workingDir()
	return Config{
		Version:   SchemaVersion,
		Listen:    "127.0.0.1:8787",
		DataDir:   filepath.Join(base, "data"),
		Workspace: filepath.Join(base, "workspace"),
		Skills:    []string{filepath.Join(base, "skills")},
		Approval:  ApprovalConfig{Mode: "auto"},
		Auth:      AuthConfig{Username: "bot", SessionDays: 7},
		Runtime: RuntimeConfig{
			MaxIterations:    0,
			MaxOutputTokens:  32768,
			MaxAutoTurns:     0,
			ContextMessages:  80,
			ContextWindow:    1000000,
			ToolResultChars:  24000,
			AutoContinue:     true,
			AutoContinueWait: 2,
		},
	}
}

// Path resolves the config file location.
func Path() string {
	if p := strings.TrimSpace(os.Getenv("LINGEYE_CONFIG")); p != "" {
		return p
	}
	return filepath.Join(workingDir(), "config.json")
}

// migrateSkillsRoot moves the legacy <workspace>/skills directory to the
// sibling <workspace-parent>/skills location. Skills are global capabilities,
// not part of an individual workspace.
func migrateSkillsRoot(cfg *Config) bool {
	if strings.TrimSpace(cfg.Workspace) == "" {
		return false
	}
	oldRoot := filepath.Join(cfg.Workspace, "skills")
	newRoot := filepath.Join(filepath.Dir(cfg.Workspace), "skills")
	if filepath.Clean(oldRoot) == filepath.Clean(newRoot) {
		return false
	}
	oldExists := false
	if _, err := os.Stat(oldRoot); err == nil {
		oldExists = true
	}
	newExists := false
	if _, err := os.Stat(newRoot); err == nil {
		newExists = true
	}
	if oldExists && !newExists {
		if err := os.Rename(oldRoot, newRoot); err != nil {
			// Do not drop a skill root we could not move (EXDEV, permissions).
			if len(cfg.Skills) == 0 {
				cfg.Skills = []string{oldRoot}
			}
			return false
		}
	}
	changed := false
	for i, root := range cfg.Skills {
		if filepath.Clean(strings.TrimSpace(root)) != filepath.Clean(oldRoot) {
			continue
		}
		cfg.Skills[i] = newRoot
		changed = true
	}
	if len(cfg.Skills) == 0 && oldExists {
		cfg.Skills = []string{newRoot}
		changed = true
	}
	return changed
}

func workingDir() string {
	if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		return wd
	}
	return "."
}

// Load reads the config file, creating a default one when missing.
func Load() (Config, error) {
	cfg := Default()
	path := Path()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		applyEnv(&cfg)
		normalize(&cfg)
		if saveErr := Save(cfg); saveErr != nil {
			return cfg, saveErr
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	// Default() already carries the current SchemaVersion. Reset it before
	// decoding so a config file without a version field is still seen as legacy.
	cfg.Version = 0
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	needSave := cfg.Auth.Secret == ""
	if cfg.Version < SchemaVersion {
		upgradeLegacyDefaults(&cfg)
		cfg.Version = SchemaVersion
		needSave = true
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join(workingDir(), "data")
	}
	if cfg.Workspace == "" {
		cfg.Workspace = filepath.Join(cfg.DataDir, "workspace")
	}
	if migrateSkillsRoot(&cfg) {
		needSave = true
	}
	if len(cfg.Skills) == 0 {
		cfg.Skills = []string{filepath.Join(filepath.Dir(cfg.Workspace), "skills")}
	}
	normalize(&cfg)
	applyEnv(&cfg)
	if needSave {
		_ = Save(cfg)
	}
	return cfg, nil
}

// upgradeLegacyDefaults lifts only the historical default values to the new
// baseline. The old 60-iteration / 40-auto-turn caps were defaults, not user
// choices; 0 now means unlimited and is the new default.
func upgradeLegacyDefaults(cfg *Config) {
	if cfg.Runtime.MaxOutputTokens == 8192 {
		cfg.Runtime.MaxOutputTokens = 32768
	}
	if cfg.Runtime.ContextWindow == 128000 {
		cfg.Runtime.ContextWindow = 1000000
	}
	if cfg.Runtime.MaxIterations == 60 {
		cfg.Runtime.MaxIterations = 0
	}
	if cfg.Runtime.MaxAutoTurns == 40 {
		cfg.Runtime.MaxAutoTurns = 0
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].MaxTokens == 4096 {
			cfg.Providers[i].MaxTokens = 32768
		}
	}
}

// Save writes the config atomically with 0600 permissions.
func Save(cfg Config) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Active returns the selected provider, or false when none is configured.
func (c Config) Active() (Provider, bool) {
	if c.ActiveProvider != "" {
		for _, p := range c.Providers {
			if p.ID == c.ActiveProvider {
				return p, true
			}
		}
	}
	if len(c.Providers) > 0 {
		return c.Providers[0], true
	}
	return Provider{}, false
}

// Public returns a copy with API keys masked, safe to send to the browser.
func (c Config) Public() Config {
	out := c
	out.Providers = make([]Provider, len(c.Providers))
	for i, p := range c.Providers {
		p.APIKey = MaskKey(p.APIKey)
		out.Providers[i] = p
	}
	out.MCP = make([]MCPServer, len(c.MCP))
	for i, m := range c.MCP {
		m.Env = maskMap(m.Env)
		m.Headers = maskMap(m.Headers)
		out.MCP[i] = m
	}
	out.Auth.PasswordHash = ""
	out.Auth.Secret = ""
	return out
}

func maskMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = MaskKey(value)
	}
	return out
}

// Normalize applies defaults and validates enum-like fields after a merge.
func Normalize(cfg *Config) { normalize(cfg) }

// MaskKey keeps the last four characters for recognition.
func MaskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

func normalize(cfg *Config) {
	roots := make([]string, 0, len(cfg.Skills))
	seen := map[string]bool{}
	for _, root := range cfg.Skills {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	if len(roots) == 0 {
		roots = []string{filepath.Join(filepath.Dir(cfg.Workspace), "skills")}
	}
	cfg.Skills = roots

	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		p.Kind = strings.ToLower(strings.TrimSpace(p.Kind))
		if p.Kind != "anthropic" {
			p.Kind = "openai"
		}
		p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		if p.BaseURL == "" {
			if p.Kind == "anthropic" {
				p.BaseURL = "https://api.anthropic.com"
			} else {
				p.BaseURL = "https://api.openai.com/v1"
			}
		}
		p.ID = strings.TrimSpace(p.ID)
		if p.ID == "" {
			p.ID = "p-" + shortID(len(cfg.Providers)+1)
		}
		if p.MaxTokens <= 0 {
			p.MaxTokens = 32768
		}
		p.MaxTokensField = strings.ToLower(strings.TrimSpace(p.MaxTokensField))
		switch p.MaxTokensField {
		case "max_tokens", "max_completion_tokens":
		default:
			p.MaxTokensField = ""
		}
		p.ReasoningMode = strings.ToLower(strings.TrimSpace(p.ReasoningMode))
		switch p.ReasoningMode {
		case "on", "off":
		default:
			p.ReasoningMode = "auto"
		}
		p.ReasoningEffort = strings.ToLower(strings.TrimSpace(p.ReasoningEffort))
		switch p.ReasoningEffort {
		case "none", "low", "medium", "high", "max", "xhigh":
		default:
			p.ReasoningEffort = ""
		}
		p.ReasoningProfile = strings.ToLower(strings.TrimSpace(p.ReasoningProfile))
		switch p.ReasoningProfile {
		case "openai_compat", "deepseek_compat":
		default:
			p.ReasoningProfile = "auto"
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Approval.Mode)) {
	case "auto", "ask", "deny":
		cfg.Approval.Mode = strings.ToLower(strings.TrimSpace(cfg.Approval.Mode))
	default:
		cfg.Approval.Mode = "auto"
	}
	if cfg.Runtime.MaxIterations < 0 {
		cfg.Runtime.MaxIterations = 0
	}
	if cfg.Runtime.MaxAutoTurns < 0 {
		cfg.Runtime.MaxAutoTurns = 0
	}
	if cfg.Runtime.MaxOutputTokens <= 0 {
		cfg.Runtime.MaxOutputTokens = 32768
	}
	if cfg.Runtime.ContextMessages <= 0 {
		cfg.Runtime.ContextMessages = 80
	}
	if cfg.Runtime.ContextWindow <= 0 {
		cfg.Runtime.ContextWindow = 1000000
	}
	if cfg.Runtime.ToolResultChars <= 0 {
		cfg.Runtime.ToolResultChars = 24000
	}
	if cfg.Runtime.AutoContinueWait <= 0 {
		cfg.Runtime.AutoContinueWait = 2
	}
	if cfg.Auth.Username == "" {
		cfg.Auth.Username = "bot"
	}
	if cfg.Auth.SessionDays <= 0 {
		cfg.Auth.SessionDays = 7
	}
	if cfg.Auth.Secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err == nil {
			cfg.Auth.Secret = base64.RawURLEncoding.EncodeToString(buf)
		}
	}
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8787"
	}
	for i := range cfg.MCP {
		m := &cfg.MCP[i]
		m.Transport = strings.ToLower(strings.TrimSpace(m.Transport))
		if m.Transport == "" {
			m.Transport = "stdio"
		}
		if m.ID == "" {
			m.ID = "mcp-" + shortID(i+1)
		}
	}
}

func applyEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("LINGEYE_DATA_DIR")); v != "" {
		cfg.DataDir = v
		cfg.Workspace = filepath.Join(v, "workspace")
		oldSkills := filepath.Join(cfg.Workspace, "skills")
		newSkills := filepath.Join(v, "skills")
		if _, err := os.Stat(newSkills); errors.Is(err, os.ErrNotExist) {
			if _, oldErr := os.Stat(oldSkills); oldErr == nil {
				if err := os.Rename(oldSkills, newSkills); err != nil {
					newSkills = oldSkills
				}
			}
		}
		cfg.Skills = []string{newSkills}
	}
	if v := strings.TrimSpace(os.Getenv("LINGEYE_LISTEN")); v != "" {
		cfg.Listen = v
	}
}

func shortID(n int) string {
	return fmt.Sprintf("%04x", n)
}

// PlatformShell returns the default shell command for this OS.
func PlatformShell() string {
	if runtime.GOOS == "windows" {
		return "pwsh"
	}
	return "bash"
}
