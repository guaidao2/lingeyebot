package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderDiscoversFrontmatterSkills(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "web-recon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: web-recon\ndescription: Enumerate a web target before touching payloads.\n---\n\n# Web recon\n\n1. Fingerprint.\n2. Enumerate.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	l := New([]string{root})
	list := l.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(list))
	}
	if list[0].Name != "web-recon" || list[0].Description == "" {
		t.Fatalf("unexpected skill: %+v", list[0])
	}
	if got, ok := l.Get("web-recon"); !ok || got.Body == "" {
		t.Fatalf("get failed: %+v %v", got, ok)
	}
	if l.Index() == "" {
		t.Fatal("index should not be empty")
	}
}

func TestLoaderFallsBackToFirstParagraph(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "audit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Audit\n\nTrace sources to sinks.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := New([]string{root})
	s, ok := l.Get("audit")
	if !ok {
		t.Fatal("skill not found by directory name")
	}
	if s.Description != "Trace sources to sinks." {
		t.Fatalf("description = %q", s.Description)
	}
}

func TestLoaderSkipsMissingRoots(t *testing.T) {
	l := New([]string{filepath.Join(t.TempDir(), "nope")})
	if len(l.List()) != 0 {
		t.Fatal("missing roots should be skipped")
	}
}

func TestSaveSkillRoundTrip(t *testing.T) {
	root := t.TempDir()
	loader := New([]string{root})
	saved, err := loader.Save("ssrf-testing", "SSRF 测试流程", "# SSRF\n\n先确认回连。")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "ssrf-testing" || saved.Description != "SSRF 测试流程" || !strings.Contains(saved.Body, "先确认回连") {
		t.Fatalf("saved = %+v", saved)
	}
	if _, err := os.Stat(saved.Path); err != nil {
		t.Fatal(err)
	}
	updated, err := loader.Save("ssrf-testing", "更新后的描述", "# SSRF v2")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "更新后的描述" || !strings.Contains(updated.Body, "v2") {
		t.Fatalf("updated = %+v", updated)
	}
}
