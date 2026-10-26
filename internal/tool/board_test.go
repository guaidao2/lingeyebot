package tool

import (
	"strings"
	"testing"
)

func TestBoardWriteAndRead(t *testing.T) {
	env := newEnv(t)
	reg := Build(env)

	out, err := call(reg, "board_write", map[string]any{
		"kind": "decision", "title": "scope", "detail": "只测 example.com", "pinned": true,
	})
	if err != nil {
		t.Fatalf("board write: %v", err)
	}
	if !strings.Contains(out, "只测 example.com") {
		t.Fatalf("write output: %s", out)
	}

	out, err = call(reg, "board_read", map[string]any{})
	if err != nil {
		t.Fatalf("board read: %v", err)
	}
	if !strings.Contains(out, "只测 example.com") || !strings.Contains(out, "memory") {
		t.Fatalf("read output: %s", out)
	}
}

func TestAssetAddLinksProject(t *testing.T) {
	env := newEnv(t)
	reg := Build(env)
	out, err := call(reg, "asset_add", map[string]any{"type": "root-domain", "value": "linked.test"})
	if err != nil {
		t.Fatalf("asset add: %v", err)
	}
	if !strings.Contains(out, "linked.test") {
		t.Fatalf("asset output: %s", out)
	}
	assets, err := env.Store.ListProjectAssets(env.Run.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, asset := range assets {
		if asset.Value == "linked.test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("asset was not linked to project %s: %+v", env.Run.ProjectID, assets)
	}
}
