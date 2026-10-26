package policy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/store"
)

func newTest(t *testing.T) (*Engine, store.Run) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	run, _, err := st.CreateRun(store.Run{Mode: "redteam", Target: "example.com", Objective: "o"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	// Approval is off by default; these tests exercise the ask path explicitly.
	cfg.Approval.Mode = "ask"
	return New(st, cfg), run
}

func shellCall(command string) llm.ToolCall {
	args, _ := json.Marshal(map[string]string{"command": command})
	return llm.ToolCall{Name: "shell", Arguments: args}
}

func TestAllowsOrdinaryCommand(t *testing.T) {
	eng, run := newTest(t)
	dec, err := eng.Authorize(context.Background(), run, "", shellCall("ls -la"))
	if err != nil || !dec.Allowed {
		t.Fatalf("decision = %+v err=%v", dec, err)
	}
	audit, _ := eng.Store.ListAudit(run.ID, 10)
	if len(audit) != 1 || audit[0].Action != "allow" {
		t.Fatalf("audit = %+v", audit)
	}
}

func TestAsksAndApprovesDangerousCommand(t *testing.T) {
	eng, run := newTest(t)
	asked := false
	eng.OnAsk = func(_ context.Context, a store.Approval) (bool, error) {
		asked = true
		_, err := eng.Store.DecideApproval(a.ID, true)
		return true, err
	}
	dec, err := eng.Authorize(context.Background(), run, "", shellCall("rm -rf / --no-preserve-root"))
	if err != nil || !dec.Allowed || !asked {
		t.Fatalf("decision = %+v asked=%v err=%v", dec, asked, err)
	}
}

func TestDeniesDangerousCommandWhenUserRefuses(t *testing.T) {
	eng, run := newTest(t)
	eng.OnAsk = func(_ context.Context, a store.Approval) (bool, error) {
		_, err := eng.Store.DecideApproval(a.ID, false)
		return false, err
	}
	dec, err := eng.Authorize(context.Background(), run, "", shellCall("shutdown -h now"))
	if err != nil || dec.Allowed {
		t.Fatalf("decision = %+v err=%v", dec, err)
	}
}

func TestDenyConstraintWins(t *testing.T) {
	eng, run := newTest(t)
	if _, err := eng.Store.AddConstraint(store.Constraint{RunID: run.ID, Kind: "deny", Text: "prod-db"}); err != nil {
		t.Fatal(err)
	}
	asked := false
	eng.OnAsk = func(context.Context, store.Approval) (bool, error) { asked = true; return true, nil }
	dec, err := eng.Authorize(context.Background(), run, "", shellCall("psql prod-db.example.com"))
	if err != nil || dec.Allowed || asked {
		t.Fatalf("decision = %+v asked=%v err=%v", dec, asked, err)
	}
}

func TestAutoModeSkipsApproval(t *testing.T) {
	eng, run := newTest(t)
	eng.Config.Approval.Mode = "auto"
	dec, err := eng.Authorize(context.Background(), run, "", shellCall("nmap -sV example.com"))
	if err != nil || !dec.Allowed {
		t.Fatalf("decision = %+v err=%v", dec, err)
	}
}

func TestAutoModeBlocksDestructiveCommand(t *testing.T) {
	eng, run := newTest(t)
	eng.Config.Approval.Mode = "auto"
	asked := false
	eng.OnAsk = func(context.Context, store.Approval) (bool, error) { asked = true; return true, nil }
	dec, err := eng.Authorize(context.Background(), run, "", shellCall("rm -rf / --no-preserve-root"))
	if err != nil || dec.Allowed || asked {
		t.Fatalf("auto must hard-deny destructive: decision=%+v asked=%v err=%v", dec, asked, err)
	}
}

func TestAskListForcesApproval(t *testing.T) {
	eng, run := newTest(t)
	eng.Config.Approval.AskTools = []string{"http_request"}
	eng.OnAsk = func(_ context.Context, a store.Approval) (bool, error) {
		_, err := eng.Store.DecideApproval(a.ID, true)
		return true, err
	}
	dec, err := eng.Authorize(context.Background(), run, "", llm.ToolCall{Name: "http_request", Arguments: json.RawMessage(`{"url":"https://example.com"}`)})
	if err != nil || !dec.Allowed {
		t.Fatalf("decision = %+v err=%v", dec, err)
	}
	pending, _ := eng.Store.ListApprovals(run.ID, "", 10)
	if len(pending) != 1 || pending[0].Status != "approved" {
		t.Fatalf("approvals = %+v", pending)
	}
}

func TestDenyModeDeniesDangerousCommand(t *testing.T) {
	eng, run := newTest(t)
	eng.Config.Approval.Mode = "deny"
	asked := false
	eng.OnAsk = func(context.Context, store.Approval) (bool, error) { asked = true; return true, nil }
	dec, err := eng.Authorize(context.Background(), run, "", shellCall("rm -rf / --no-preserve-root"))
	if err != nil || dec.Allowed || asked {
		t.Fatalf("deny mode must not ask: decision=%+v asked=%v err=%v", dec, asked, err)
	}
}
