package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
)

func newEnv(t *testing.T) Env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	run, _, err := st.CreateRun(store.Run{Mode: "daily", Target: "example.com", Objective: "验证", Scope: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return Env{Store: st, Config: config.Default(), Run: run, Workspace: workspace, Agent: "test"}
}

func call(reg *Registry, name string, args any) (string, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return reg.Execute(context.Background(), llm.ToolCall{Name: name, Arguments: data})
}

func TestFileTools(t *testing.T) {
	env := newEnv(t)
	reg := Build(env).Filter([]string{"write_file", "read_file", "edit_file", "list_dir", "grep"})

	if _, err := call(reg, "write_file", map[string]any{"path": "notes/a.txt", "content": "alpha\nbeta\n"}); err != nil {
		t.Fatal(err)
	}
	body, err := call(reg, "read_file", map[string]any{"path": "notes/a.txt"})
	if err != nil || !strings.Contains(body, "alpha") {
		t.Fatalf("read: %q %v", body, err)
	}
	if _, err := call(reg, "edit_file", map[string]any{"path": "notes/a.txt", "old_string": "beta", "new_string": "gamma"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(reg, "edit_file", map[string]any{"path": "notes/a.txt", "old_string": "zzz", "new_string": "x"}); err == nil {
		t.Fatal("editing a missing string should fail")
	}
	list, err := call(reg, "list_dir", map[string]any{"path": "notes"})
	if err != nil || !strings.Contains(list, "a.txt") {
		t.Fatalf("list: %q %v", list, err)
	}
	matches, err := call(reg, "grep", map[string]any{"pattern": "gamma", "path": "notes"})
	if err != nil || !strings.Contains(matches, "a.txt") {
		t.Fatalf("grep: %q %v", matches, err)
	}
}

func TestShellTool(t *testing.T) {
	env := newEnv(t)
	reg := Build(env).Filter([]string{"shell"})
	out, err := call(reg, "shell", map[string]any{"command": "echo lingeyebot"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "exit=0") || !strings.Contains(out, "lingeyebot") {
		t.Fatalf("shell output: %s", out)
	}
}

func TestFactDisciplineAndCoverage(t *testing.T) {
	env := newEnv(t)
	reg := Build(env)
	assetOut, err := call(reg, "asset_add", map[string]any{"type": "root-domain", "value": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	var asset struct {
		Asset store.Asset `json:"asset"`
	}
	_ = json.Unmarshal([]byte(assetOut), &asset)

	if _, err := call(reg, "fact_add", map[string]any{
		"detail": "首页返回 200", "basis": "observed", "assetIds": []string{asset.Asset.ID},
	}); err == nil {
		t.Fatal("observed fact without evidence must be rejected")
	}

	evOut, err := call(reg, "evidence_add", map[string]any{
		"kind": "http", "label": "GET /", "content": "HTTP/1.1 200 OK", "channel": "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	var ev store.Evidence
	_ = json.Unmarshal([]byte(evOut), &ev)

	factOut, err := call(reg, "fact_add", map[string]any{
		"detail": "首页返回 200", "basis": "observed", "assetIds": []string{asset.Asset.ID},
		"evidenceIds": []string{ev.ID},
	})
	if err != nil {
		t.Fatalf("fact add: %v", err)
	}
	if !strings.Contains(factOut, `"covered": true`) {
		t.Fatalf("fact output: %s", factOut)
	}
	covOut, err := call(reg, "coverage", map[string]any{})
	if err != nil || !strings.Contains(covOut, `"tested": 1`) {
		t.Fatalf("coverage: %s %v", covOut, err)
	}
}

func TestIntentDeduplicationAndGoalState(t *testing.T) {
	env := newEnv(t)
	reg := Build(env)
	goalOut, err := call(reg, "goal_add", map[string]any{"text": "拿到 flag", "acceptance": "flag 文件存在"})
	if err != nil {
		t.Fatal(err)
	}
	var goal store.Goal
	_ = json.Unmarshal([]byte(goalOut), &goal)

	args := map[string]any{"intents": []map[string]any{{
		"title": "探测首页", "expectedObservation": "返回 200", "goalId": goal.ID, "priority": 5,
	}}}
	if _, err := call(reg, "intent_add", args); err != nil {
		t.Fatal(err)
	}
	dupOut, err := call(reg, "intent_add", args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dupOut, "探测首页") || !strings.Contains(dupOut, "skippedDuplicate") {
		t.Fatalf("duplicate intent should be skipped: %s", dupOut)
	}
}

func TestFindingRequiresStepsAndEvidence(t *testing.T) {
	env := newEnv(t)
	reg := Build(env)
	if _, err := call(reg, "finding_add", map[string]any{
		"title": "SQLi", "severity": "high", "reproducibleSteps": []string{},
	}); err == nil {
		t.Fatal("finding without steps must be rejected")
	}
	if _, err := call(reg, "finding_add", map[string]any{
		"title": "SQLi", "severity": "high", "reproducibleSteps": []string{"curl ..."},
	}); err == nil {
		t.Fatal("finding without evidence must be rejected")
	}
	out, err := call(reg, "finding_add", map[string]any{
		"title": "SQLi", "severity": "high", "reproducibleSteps": []string{"curl ..."},
		"evidence": []map[string]any{{"kind": "http", "label": "payload", "content": "200 and delay"}},
	})
	if err != nil {
		t.Fatalf("finding add: %v", err)
	}
	if !strings.Contains(out, "SQLi") {
		t.Fatalf("finding output: %s", out)
	}
}

func TestReportExport(t *testing.T) {
	env := newEnv(t)
	reg := Build(env)
	if _, err := call(reg, "report_export", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(env.Workspace, "reports", env.Run.ID)
	for _, name := range []string{"report.md", "findings.json", "assets.json", "graph.json", "coverage.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

type fakeSubagents struct {
	spawned []SpawnInput
	sent    string
}

func (f *fakeSubagents) Spawn(_ context.Context, _ string, in SpawnInput) (store.Task, error) {
	f.spawned = append(f.spawned, in)
	return store.Task{ID: "task-1", Role: in.Role, Title: in.Task, Status: "queued"}, nil
}
func (f *fakeSubagents) Send(_ context.Context, _, _, msg string) error { f.sent = msg; return nil }
func (f *fakeSubagents) Cancel(context.Context, string, string) error   { return nil }
func (f *fakeSubagents) Check(context.Context, string, string) (store.Task, error) {
	return store.Task{ID: "task-1", Status: "running"}, nil
}
func (f *fakeSubagents) List(context.Context, string) ([]store.Task, error) {
	return []store.Task{{ID: "task-1", Status: "running"}}, nil
}

func TestSubagentTool(t *testing.T) {
	env := newEnv(t)
	fake := &fakeSubagents{}
	env.Subagents = fake
	reg := Build(env).Filter([]string{"subagent"})
	out, err := call(reg, "subagent", map[string]any{"action": "create", "role": "enumerator", "task": "枚举子域"})
	if err != nil || !strings.Contains(out, "task-1") {
		t.Fatalf("create: %s %v", out, err)
	}
	if len(fake.spawned) != 1 || fake.spawned[0].Role != "enumerator" {
		t.Fatalf("spawned = %+v", fake.spawned)
	}
	if _, err := call(reg, "subagent", map[string]any{"action": "send", "task_id": "task-1", "message": "收窄到 a.com"}); err != nil {
		t.Fatal(err)
	}
	if fake.sent != "收窄到 a.com" {
		t.Fatalf("sent = %q", fake.sent)
	}
	if _, err := call(reg, "subagent", map[string]any{"action": "create", "task": ""}); err == nil {
		t.Fatal("empty task must fail")
	}
}

func TestGraphAutoLinkWithoutIntent(t *testing.T) {
	env := newEnv(t)
	reg := Build(env)

	evOut, err := call(reg, "evidence_add", map[string]any{
		"kind": "http", "label": "GET /", "content": "HTTP/1.1 200 OK", "channel": "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	var ev store.Evidence
	if err := json.Unmarshal([]byte(evOut), &ev); err != nil {
		t.Fatal(err)
	}
	if _, err := call(reg, "fact_add", map[string]any{
		"detail": "首页返回 200", "basis": "observed", "evidenceIds": []string{ev.ID},
	}); err != nil {
		t.Fatalf("fact add: %v", err)
	}
	if _, err := call(reg, "finding_add", map[string]any{
		"title": "测试漏洞", "severity": "high", "reproducibleSteps": []string{"GET /"},
		"evidenceIds": []string{ev.ID},
	}); err != nil {
		t.Fatalf("finding add: %v", err)
	}
	edges, err := env.Store.ListEdges(env.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) < 2 {
		t.Fatalf("expected auto edges from goal, got %+v", edges)
	}
}

func TestScopeChecksRejectForeignRun(t *testing.T) {
	env := newEnv(t)
	other, _, err := env.Store.CreateRun(store.Run{Mode: "daily", Target: "other.example", Objective: "other"})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := env.Store.Snapshot(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Goals) == 0 {
		t.Fatal("other run has no goal")
	}
	otherGoal := snap.Goals[0]
	ownSnap, err := env.Store.Snapshot(env.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownSnap.Goals) == 0 {
		t.Fatal("current run has no goal")
	}
	ownGoal := ownSnap.Goals[0]
	reg := Build(env).Filter([]string{"goal_prove", "intent_add", "retest_finish"})

	if _, err := call(reg, "goal_prove", map[string]any{
		"goalId": otherGoal.ID, "status": "met", "reason": "foreign",
	}); err == nil {
		t.Fatal("goal_prove accepted a goal from another run")
	}

	if _, err := call(reg, "intent_add", map[string]any{
		"intents": []map[string]any{{"title": "foreign goal intent", "goalId": otherGoal.ID}},
	}); err == nil {
		t.Fatal("intent_add accepted a goal from another run")
	}

	otherFact, err := env.Store.AddNode(store.Node{
		RunID: other.ID, Kind: "fact", State: "done",
		Payload: []byte(`{"detail":"foreign fact","basis":"observed"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(reg, "intent_add", map[string]any{
		"intents": []map[string]any{{"title": "foreign fact intent", "derivedFromFactIds": []string{otherFact.ID}}},
	}); err == nil {
		t.Fatal("intent_add accepted a fact from another run")
	}

	before, err := env.Store.ListNodes(env.Run.ID, "intent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(reg, "intent_add", map[string]any{
		"intents": []map[string]any{
			{"title": "valid before failure", "goalId": ownGoal.ID},
			{"title": "foreign after valid", "goalId": otherGoal.ID},
		},
	}); err == nil {
		t.Fatal("intent_add accepted a mixed batch with a foreign goal")
	}
	after, err := env.Store.ListNodes(env.Run.ID, "intent")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("intent_add wrote a partial batch: before=%d after=%d", len(before), len(after))
	}

	foreignAsset, _, err := env.Store.UpsertAsset(store.Asset{
		Type: "domain", Value: "other.example", FirstSeenRun: other.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(reg, "intent_add", map[string]any{
		"intents": []map[string]any{{
			"title": "existing asset intent", "goalId": ownGoal.ID,
			"assetIds": []string{foreignAsset.ID},
		}},
	}); err != nil {
		t.Fatalf("intent_add rejected an existing global asset: %v", err)
	}
	if _, err := call(reg, "intent_add", map[string]any{
		"intents": []map[string]any{{
			"title": "missing asset intent", "goalId": ownGoal.ID,
			"assetIds": []string{"asset-does-not-exist"},
		}},
	}); err == nil {
		t.Fatal("intent_add accepted a missing asset")
	}

	otherFinding, err := env.Store.AddNode(store.Node{
		RunID: other.ID, Kind: "finding", State: "open",
		Payload: []byte(`{"title":"foreign finding","severity":"high"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	retest, err := env.Store.AddRetest(store.Retest{RunID: other.ID, FindingID: otherFinding.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(reg, "retest_finish", map[string]any{
		"retestId": retest.ID, "verdict": "fixed", "notes": "foreign",
	}); err == nil {
		t.Fatal("retest_finish accepted a retest from another run")
	}
}

var errUnused = errors.New("unused")

func TestFilterMcpWildcard(t *testing.T) {
	reg := NewRegistry([]Spec{
		{Name: "shell", Fn: func(context.Context, []byte) (string, error) { return "", nil }},
		{Name: "mcp__a__b", Fn: func(context.Context, []byte) (string, error) { return "", nil }},
	})
	filtered := reg.Filter([]string{"mcp__*"})
	if len(filtered.Names()) != 1 || filtered.Names()[0] != "mcp__a__b" {
		t.Fatalf("names = %v", filtered.Names())
	}
}
