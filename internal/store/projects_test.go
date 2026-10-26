package store

import (
	"encoding/json"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/model"
)

func TestDefaultProjectAndProjectScopedRun(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	project, err := st.EnsureDefaultProject()
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := st.CreateRun(Run{Mode: "daily", Target: "t", Objective: "o"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ProjectID != project.ID {
		t.Fatalf("run project = %q, want %q", run.ProjectID, project.ID)
	}
	if run.Title != "o" {
		t.Fatalf("title = %q", run.Title)
	}
}

func TestProjectBoardAssetsMemoryAndFindings(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	project, err := st.CreateProject(Project{Name: "Acme SRC", Description: "example"})
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := st.CreateRun(Run{ProjectID: project.ID, Mode: "bugbounty", Target: "acme.test", Objective: "找洞"})
	if err != nil {
		t.Fatal(err)
	}
	asset, _, err := st.UpsertAsset(Asset{Type: "root-domain", Value: "acme.test", FirstSeenRun: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.LinkProjectAsset(project.ID, asset.ID, "agent"); err != nil {
		t.Fatal(err)
	}

	node, err := st.AddProjectNode(ProjectNode{
		ProjectID: project.ID, ConversationID: run.ID, Kind: "decision",
		Payload: json.RawMessage(`{"text":"scope 只到 acme.test"}`), Pinned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetProjectMemory(project.ID, "scope", "只测 acme.test", "decision", node.ID, "main"); err != nil {
		t.Fatal(err)
	}

	ev, err := st.AddEvidence(Evidence{RunID: run.ID, Kind: "http", Label: "GET /"}, []byte("200"))
	if err != nil {
		t.Fatal(err)
	}
	findingPayload, _ := json.Marshal(model.Finding{
		Title: "越权", Severity: "high", Steps: []string{"step"}, EvidenceIDs: []string{ev.ID}, AssetIDs: []string{asset.ID},
	})
	findingNode, err := st.AddNode(Node{RunID: run.ID, Kind: model.KindFinding, Payload: findingPayload})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Anchor(findingNode.ID, asset.ID); err != nil {
		t.Fatal(err)
	}

	detail, err := st.ProjectDetail(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Conversations) != 1 || len(detail.Board) != 1 || len(detail.Memory) != 1 || len(detail.Findings) != 1 {
		t.Fatalf("unexpected detail sizes: conv=%d board=%d memory=%d findings=%d",
			len(detail.Conversations), len(detail.Board), len(detail.Memory), len(detail.Findings))
	}
	if detail.Board[0].Pinned != true {
		t.Fatal("board node should be pinned")
	}
	if detail.Coverage.Total != 1 || detail.Coverage.Tested != 1 {
		t.Fatalf("coverage = %d/%d", detail.Coverage.Tested, detail.Coverage.Total)
	}

	projects, err := st.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	var acme *ProjectSummary
	for i := range projects {
		if projects[i].NKey == project.NKey {
			acme = &projects[i]
		}
	}
	if acme == nil || acme.Findings != 1 || acme.Conversations != 1 {
		t.Fatalf("summary = %+v", projects)
	}
}

func TestMigrateLegacyRunsToDefaultProject(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.DB.Exec(`INSERT INTO runs (id, mode, target, objective, created_at, updated_at)
		VALUES ('legacy-1','daily','t','o','2026-01-01T00:00:00.000000000Z','2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`UPDATE runs SET project_id='', title='' WHERE id='legacy-1'`); err != nil {
		t.Fatal(err)
	}
	if err := st.migrate(); err != nil {
		t.Fatal(err)
	}
	run, err := st.GetRun("legacy-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.ProjectID == "" || run.Title != "o" {
		t.Fatalf("legacy migration failed: %+v", run)
	}
}

func TestDeleteProjectCascadesAndProtectsDefault(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	project, err := st.CreateProject(Project{Name: "Disposable"})
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := st.CreateRun(Run{ProjectID: project.ID, Mode: "daily", Target: "example.test", Objective: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddNode(Node{RunID: run.ID, Kind: "note", Payload: json.RawMessage(`{"text":"x"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddEvent(Event{RunID: run.ID, Kind: "tool_call", Summary: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTask(Task{RunID: run.ID, Role: "worker", Title: "x"}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteProject(project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetProject(project.ID); err == nil {
		t.Fatal("project still exists after delete")
	}
	if _, err := st.GetRun(run.ID); err == nil {
		t.Fatal("run still exists after project delete")
	}
	if err := st.DeleteProject(defaultProjectID); err == nil {
		t.Fatal("default project must not be deletable")
	}
}

func TestGoalModeDisabled(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	disabled := false
	run, goal, err := st.CreateRun(Run{
		Mode: "daily", Target: "example.test", Objective: "", GoalMode: &disabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if goal.ID != "" {
		t.Fatalf("goal should not be created: %+v", goal)
	}
	if run.GoalsEnabled() {
		t.Fatal("run should not be goal-enabled")
	}
	if run.Title != "example.test" {
		t.Fatalf("title fallback = %q", run.Title)
	}
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GoalsEnabled() {
		t.Fatal("persisted run should not be goal-enabled")
	}
}
