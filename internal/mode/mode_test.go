package mode

import (
	"strings"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/store"
)

func TestFiveBuiltinModes(t *testing.T) {
	list := List()
	if len(list) != 5 {
		t.Fatalf("expected 5 modes, got %d", len(list))
	}
	want := map[string]bool{"daily": false, "bugbounty": false, "audit": false, "redteam": false, "ctf": false}
	for _, s := range list {
		if _, ok := want[s.ID]; !ok {
			t.Errorf("unexpected mode %s", s.ID)
		}
		want[s.ID] = true
		if s.Name == "" || s.Tagline == "" || len(s.Roles) == 0 {
			t.Errorf("incomplete mode summary: %+v", s)
		}
	}
	for id, ok := range want {
		if !ok {
			t.Errorf("missing mode %s", id)
		}
	}
}

func TestModePromptsAndTools(t *testing.T) {
	run := store.Run{ID: "r1", Mode: "bugbounty", Target: "example.com", Objective: "找洞", Scope: "example.com"}
	snap := store.Snapshot{Run: &run, Constraints: []store.Constraint{{Kind: "deny", Text: "不要打支付"}}}
	for _, d := range All() {
		prompt := d.MainSystem(run, snap, "- web-recon: recon\n")
		for _, needle := range []string{"example.com", "不要打支付", "证据纪律", "工具"} {
			if !strings.Contains(prompt, needle) {
				t.Errorf("%s prompt missing %q", d.ID, needle)
			}
		}
		tools := d.ToolsFor("main")
		if len(tools) < 10 {
			t.Errorf("%s has too few tools: %v", d.ID, tools)
		}
		for _, required := range []string{"state_overview", "subagent", "report_render", "goal_prove"} {
			if !contains(tools, required) {
				t.Errorf("%s missing tool %s", d.ID, required)
			}
		}
		for _, banned := range []string{"shell", "http_request", "read_file", "write_file", "edit_file", "list_dir", "grep"} {
			if contains(tools, banned) {
				t.Errorf("%s main agent must not have execution tool %s", d.ID, banned)
			}
		}
		for _, role := range d.Roles {
			if d.RoleSystem(role, run, snap, "") == "" {
				t.Errorf("%s role %s has no prompt", d.ID, role)
			}
			if len(d.ToolsFor(role)) == 0 {
				t.Errorf("%s role %s has no tools", d.ID, role)
			}
		}
	}
}

func TestRedTeamHasAllRolesAndCTFIsExhaustive(t *testing.T) {
	red, _ := Get("redteam")
	for _, role := range []string{"intel-collector", "enumerator", "threat-modeler", "pre-pentester", "post-pentester", "evasion", "worker"} {
		if !contains(red.Roles, role) {
			t.Errorf("redteam missing role %s", role)
		}
	}
	ctf, _ := Get("ctf")
	if !strings.Contains(ctf.Prompt, "必定有解") || !strings.Contains(ctf.Prompt, "flag") {
		t.Error("ctf prompt must encode the always-solvable premise and flag goal")
	}
}

func TestSubagentsCannotSpawnSubagents(t *testing.T) {
	d, _ := Get("redteam")
	pre := d.ToolsFor("pre-pentester")
	if contains(pre, "subagent") {
		t.Error("subagent tool must not be available to subagents")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestCommanderDelegatesExecution(t *testing.T) {
	got, _ := Get("redteam")
	main := got.ToolsFor("main")
	for _, banned := range []string{"shell", "http_request", "read_file", "write_file", "edit_file", "list_dir", "grep"} {
		if contains(main, banned) {
			t.Fatalf("main agent must not execute %s", banned)
		}
	}
	for _, required := range []string{"subagent", "state_overview", "report_render", "board_write", "board_read"} {
		if !contains(main, required) {
			t.Fatalf("main agent missing %s", required)
		}
	}
	pre := got.ToolsFor("pre-pentester")
	if !contains(pre, "shell") || !contains(pre, "http_request") {
		t.Fatalf("pre-pentester must keep execution tools: %v", pre)
	}
	if contains(pre, "subagent") {
		t.Fatal("subagent roles must not delegate further")
	}
}
