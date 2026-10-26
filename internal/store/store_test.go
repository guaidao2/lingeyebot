package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/model"
)

func TestRunGraphCoverageReport(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	run, goal, err := st.CreateRun(Run{
		Mode: "daily", Target: "example.com", Objective: "确认对外暴露面",
		Authorization: "self-owned", Scope: "example.com",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	asset, created, err := st.UpsertAsset(Asset{Type: "root-domain", Value: "example.com", FirstSeenRun: run.ID})
	if err != nil || !created {
		t.Fatalf("upsert asset: created=%v err=%v", created, err)
	}
	if _, createdAgain, err := st.UpsertAsset(Asset{Type: "root-domain", Value: "Example.COM", FirstSeenRun: run.ID}); err != nil || createdAgain {
		t.Fatalf("asset should deduplicate: created=%v err=%v", createdAgain, err)
	}

	intentPayload, _ := json.Marshal(model.Intent{
		Title: "探测首页与响应头", ExpectedObservation: "返回 200 与版本指纹", GoalID: goal.ID,
	})
	intent, err := st.AddNode(Node{RunID: run.ID, Kind: model.KindIntent, Payload: intentPayload, Priority: 5})
	if err != nil {
		t.Fatalf("add intent: %v", err)
	}

	ev, err := st.AddEvidence(Evidence{RunID: run.ID, Kind: "http", Label: "GET /", Channel: "http"}, []byte("HTTP/1.1 200 OK"))
	if err != nil {
		t.Fatalf("add evidence: %v", err)
	}
	if ev.SHA256 == "" || ev.Size == 0 {
		t.Fatalf("evidence hash/size not set: %+v", ev)
	}

	factPayload, _ := json.Marshal(model.Fact{
		Kind: "http", Detail: "首页返回 200", Basis: "observed", Confidence: 0.9,
		EvidenceIDs: []string{ev.ID}, AssetIDs: []string{asset.ID},
	})
	fact, err := st.AddNode(Node{RunID: run.ID, Kind: model.KindFact, Payload: factPayload, Owner: "worker"})
	if err != nil {
		t.Fatalf("add fact: %v", err)
	}
	if _, err := st.AddEdge(run.ID, intent.ID, fact.ID, "yields"); err != nil {
		t.Fatalf("add edge: %v", err)
	}
	if err := st.Anchor(fact.ID, asset.ID); err != nil {
		t.Fatalf("anchor: %v", err)
	}

	cov, err := st.Coverage(run.ID)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if cov.Total != 1 || cov.Tested != 1 {
		t.Fatalf("expected 1/1 tested, got %d/%d", cov.Tested, cov.Total)
	}

	if _, err := st.ProveGoal(goal.ID, []string{ev.ID}, "met"); err != nil {
		t.Fatalf("prove goal: %v", err)
	}
	settled, err := st.GoalsSettled(run.ID)
	if err != nil || !settled {
		t.Fatalf("goal should be settled: %v %v", settled, err)
	}

	report, err := st.Report(run.ID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	for _, section := range []string{"## 摘要", "## 探索链路", "## 信念缺口", "首页返回 200", "example.com"} {
		if !strings.Contains(report, section) {
			t.Errorf("report missing %q", section)
		}
	}

	snap, err := st.Snapshot(run.ID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Nodes) != 2 || len(snap.Edges) != 1 || len(snap.Evidence) != 1 {
		t.Fatalf("unexpected snapshot sizes: nodes=%d edges=%d evidence=%d", len(snap.Nodes), len(snap.Edges), len(snap.Evidence))
	}
}

func TestFactWithoutEvidenceDoesNotCountAsTested(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	run, _, _ := st.CreateRun(Run{Mode: "daily", Target: "t", Objective: "o"})
	asset, _, _ := st.UpsertAsset(Asset{Type: "subdomain", Value: "a.example.com", FirstSeenRun: run.ID})
	payload, _ := json.Marshal(model.Fact{Detail: "看起来存在", Basis: "assumed", AssetIDs: []string{asset.ID}})
	if _, err := st.AddNode(Node{RunID: run.ID, Kind: model.KindFact, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	cov, err := st.Coverage(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cov.Tested != 0 {
		t.Fatalf("a fact without evidence must not count as tested, got %d", cov.Tested)
	}
}

func TestApprovalsAndTasks(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	run, _, _ := st.CreateRun(Run{Mode: "redteam", Target: "t", Objective: "o"})

	task, err := st.CreateTask(Task{RunID: run.ID, Role: "worker", Title: "测试"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendInbox(task.ID, "收窄范围"); err != nil {
		t.Fatal(err)
	}
	inbox, err := st.TakeInbox(task.ID)
	if err != nil || len(inbox) != 1 {
		t.Fatalf("inbox: %+v %v", inbox, err)
	}

	appr, err := st.CreateApproval(Approval{RunID: run.ID, Tool: "shell", Input: `{"command":"id"}`, Reason: "high risk"})
	if err != nil {
		t.Fatal(err)
	}
	decided, err := st.DecideApproval(appr.ID, true)
	if err != nil || decided.Status != "approved" {
		t.Fatalf("decide: %+v %v", decided, err)
	}
	if err := st.AddAudit(AuditEntry{RunID: run.ID, Tool: "shell", Action: "allow", Command: "id"}); err != nil {
		t.Fatal(err)
	}
	entries, err := st.ListAudit(run.ID, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit: %+v %v", entries, err)
	}
}
