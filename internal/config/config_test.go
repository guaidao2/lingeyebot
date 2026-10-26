package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultUsesWorkingDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	if cfg.DataDir != filepath.Join(cwd, "data") {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.Workspace != filepath.Join(cwd, "workspace") {
		t.Fatalf("Workspace = %q", cfg.Workspace)
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0] != filepath.Join(cwd, "skills") {
		t.Fatalf("Skills = %v", cfg.Skills)
	}
	t.Setenv("LINGEYE_CONFIG", "")
	if got := Path(); got != filepath.Join(cwd, "config.json") {
		t.Fatalf("Path = %q", got)
	}
}

func TestLoadMovesLegacyWorkspaceSkills(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	oldRoot := filepath.Join(workspace, "skills")
	if err := os.MkdirAll(filepath.Join(oldRoot, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldRoot, "demo", "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "config.json")
	body := fmt.Sprintf(`{"version":%d,"workspace":%q,"dataDir":%q,"skills":[%q]}`,
		SchemaVersion, workspace, filepath.Join(root, "data"), oldRoot)
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGEYE_CONFIG", cfgPath)
	t.Setenv("LINGEYE_DATA_DIR", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(root, "skills")
	if len(cfg.Skills) != 1 || cfg.Skills[0] != newRoot {
		t.Fatalf("skills = %v, want %s", cfg.Skills, newRoot)
	}
	if _, err := os.Stat(filepath.Join(newRoot, "demo", "SKILL.md")); err != nil {
		t.Fatalf("skill was not moved: %v", err)
	}
	if _, err := os.Stat(oldRoot); !os.IsNotExist(err) {
		t.Fatalf("legacy skills dir still exists: %v", err)
	}
}

func TestDefaultApprovalIsOff(t *testing.T) {
	cfg := Default()
	if cfg.Approval.Mode != "auto" {
		t.Fatalf("default approval mode = %q, want auto", cfg.Approval.Mode)
	}
	cfg.Approval.Mode = ""
	normalize(&cfg)
	if cfg.Approval.Mode != "auto" {
		t.Fatalf("empty approval mode normalized to %q, want auto", cfg.Approval.Mode)
	}
}

func TestModelTokenDefaults(t *testing.T) {
	cfg := Default()
	if cfg.Runtime.MaxOutputTokens != 32768 {
		t.Fatalf("MaxOutputTokens = %d, want 32768", cfg.Runtime.MaxOutputTokens)
	}
	if cfg.Runtime.ContextWindow != 1000000 {
		t.Fatalf("ContextWindow = %d, want 1000000", cfg.Runtime.ContextWindow)
	}
	cfg.Providers = []Provider{{Kind: "openai", MaxTokens: 0, MaxTokensField: "MAX_COMPLETION_TOKENS"}}
	normalize(&cfg)
	if cfg.Providers[0].MaxTokens != 32768 {
		t.Fatalf("provider MaxTokens = %d, want 32768", cfg.Providers[0].MaxTokens)
	}
	if cfg.Providers[0].MaxTokensField != "max_completion_tokens" {
		t.Fatalf("provider MaxTokensField = %q", cfg.Providers[0].MaxTokensField)
	}
}

func TestLegacyTokenDefaultsUpgrade(t *testing.T) {
	cfg := Default()
	cfg.Runtime.MaxOutputTokens = 8192
	cfg.Runtime.ContextWindow = 128000
	cfg.Providers = []Provider{{MaxTokens: 4096}}
	upgradeLegacyDefaults(&cfg)
	if cfg.Runtime.MaxOutputTokens != 32768 || cfg.Runtime.ContextWindow != 1000000 {
		t.Fatalf("runtime upgrade = %+v", cfg.Runtime)
	}
	if cfg.Providers[0].MaxTokens != 32768 {
		t.Fatalf("provider upgrade = %d", cfg.Providers[0].MaxTokens)
	}
}

func TestLoadUpgradesLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	legacy := `{
  "providers": [{"id":"p1","kind":"openai","maxTokens":4096}],
  "runtime": {"maxOutputTokens":8192,"contextWindow":128000}
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGEYE_CONFIG", path)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != SchemaVersion {
		t.Fatalf("version = %d, want %d", cfg.Version, SchemaVersion)
	}
	if cfg.Runtime.MaxOutputTokens != 32768 || cfg.Runtime.ContextWindow != 1000000 {
		t.Fatalf("runtime = %+v", cfg.Runtime)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].MaxTokens != 32768 {
		t.Fatalf("providers = %+v", cfg.Providers)
	}
}

func TestLoadMigratesLegacyBudgets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	legacy := `{
  "version": 2,
  "runtime": {"maxIterations": 60, "maxAutoTurns": 40}
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGEYE_CONFIG", path)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.MaxIterations != 0 || cfg.Runtime.MaxAutoTurns != 0 {
		t.Fatalf("legacy budgets were not removed: %+v", cfg.Runtime)
	}
}

func TestLoadKeepsExplicitBudgets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	explicit := `{
  "version": 3,
  "runtime": {"maxIterations": 60, "maxAutoTurns": 40}
}`
	if err := os.WriteFile(path, []byte(explicit), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGEYE_CONFIG", path)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.MaxIterations != 60 || cfg.Runtime.MaxAutoTurns != 40 {
		t.Fatalf("explicit budgets were changed: %+v", cfg.Runtime)
	}
}
