package server

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyConfigRebuildsSkillRoots(t *testing.T) {
	srv, _, _ := newTestServer(t, false)
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTestSkill(t, rootA, "skill-a", "from A")
	writeTestSkill(t, rootB, "skill-b", "from B")

	cfg := srv.Config()
	cfg.Skills = []string{rootA}
	if err := srv.ApplyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	names := skillNames(srv)
	if len(names) != 1 || names[0] != "skill-a" {
		t.Fatalf("after root A = %v", names)
	}

	cfg.Skills = []string{rootB}
	if err := srv.ApplyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	names = skillNames(srv)
	if len(names) != 1 || names[0] != "skill-b" {
		t.Fatalf("after root B = %v", names)
	}
}

func skillNames(srv *Server) []string {
	srv.mu.RLock()
	loader := srv.skills
	srv.mu.RUnlock()
	if loader == nil {
		return nil
	}
	out := []string{}
	for _, sk := range loader.List() {
		out = append(out, sk.Name)
	}
	return out
}
