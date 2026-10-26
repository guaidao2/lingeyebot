package mode

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/model"
	"github.com/guaidao2/lingeyebot/internal/store"
)

func setupRun(t *testing.T) (*store.Store, store.Run, store.Snapshot, store.Coverage) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	run, _, err := st.CreateRun(store.Run{Mode: "ctf", Target: "chal", Objective: "o"})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := st.Snapshot(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	cov, err := st.Coverage(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return st, run, snap, cov
}

func TestValidateGoalCoverage(t *testing.T) {
	st, run, snap, cov := setupRun(t)
	a1, _, _ := st.UpsertAsset(store.Asset{Type: "subdomain", Value: "a.example.com", FirstSeenRun: run.ID})
	a2, _, _ := st.UpsertAsset(store.Asset{Type: "subdomain", Value: "b.example.com", FirstSeenRun: run.ID})
	_ = a2
	ev, _ := st.AddEvidence(store.Evidence{RunID: run.ID, Kind: "http", Label: "a"}, []byte("200"))
	payload, _ := json.Marshal(model.Fact{Detail: "a 可访问", Basis: "observed", EvidenceIDs: []string{ev.ID}, AssetIDs: []string{a1.ID}})
	_, _ = st.AddNode(store.Node{RunID: run.ID, Kind: model.KindFact, Payload: payload})
	snap, _ = st.Snapshot(run.ID)
	cov, _ = st.Coverage(run.ID)

	d, _ := Get("redteam")
	goal := store.Goal{Text: "覆盖率达到 100%"}
	if err := d.ValidateGoal(st, run, snap, cov, goal); err == nil || !strings.Contains(err.Error(), "覆盖率") {
		t.Fatalf("expected coverage failure, got %v", err)
	}
	okGoal := store.Goal{Text: "确认对外暴露面", Acceptance: "覆盖率 50%"}
	if err := d.ValidateGoal(st, run, snap, cov, okGoal); err != nil {
		t.Fatalf("50%% should pass at 1/2: %v", err)
	}
}

func TestValidateGoalFlag(t *testing.T) {
	st, run, snap, cov := setupRun(t)
	ctf, _ := Get("ctf")
	goal := store.Goal{Text: "拿到 flag"}
	if err := ctf.ValidateGoal(st, run, snap, cov, goal); err == nil {
		t.Fatal("flag goal without evidence must fail")
	}
	_, _ = st.AddEvidence(store.Evidence{RunID: run.ID, Kind: "text", Label: "输出"}, []byte("done: flag{demo_value}"))
	snap, _ = st.Snapshot(run.ID)
	if err := ctf.ValidateGoal(st, run, snap, cov, goal); err != nil {
		t.Fatalf("flag evidence should pass: %v", err)
	}
}

func TestValidateGoalUnquantified(t *testing.T) {
	st, run, snap, cov := setupRun(t)
	d, _ := Get("daily")
	if err := d.ValidateGoal(st, run, snap, cov, store.Goal{Text: "给出加固建议"}); err != nil {
		t.Fatalf("unquantified goal should pass: %v", err)
	}
}
