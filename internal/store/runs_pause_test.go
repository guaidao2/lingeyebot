package store

import "testing"

func newPauseTestRun(t *testing.T) (*Store, Run) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	run, _, err := st.CreateRun(Run{Mode: "ctf", Target: "chal", Objective: "flag"})
	if err != nil {
		t.Fatal(err)
	}
	return st, run
}

func TestRunPauseReasonRoundTrips(t *testing.T) {
	st, run := newPauseTestRun(t)
	if err := st.PauseRun(run.ID, PauseReasonUserStop); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "paused" || got.PauseReason != PauseReasonUserStop {
		t.Fatalf("run = %s/%q", got.Status, got.PauseReason)
	}
}

func TestUpdateRunStatusClearsPauseReason(t *testing.T) {
	st, run := newPauseTestRun(t)
	if err := st.PauseRun(run.ID, PauseReasonUserStop); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRunStatus(run.ID, "active"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PauseReason != "" {
		t.Fatalf("pause reason survived activation: %q", got.PauseReason)
	}
}

func TestPauseRunByErrorKeepsUserStop(t *testing.T) {
	st, run := newPauseTestRun(t)
	if err := st.PauseRun(run.ID, PauseReasonUserStop); err != nil {
		t.Fatal(err)
	}
	if err := st.PauseRunByError(run.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(run.ID)
	if got.Status != "paused" || got.PauseReason != PauseReasonUserStop {
		t.Fatalf("user stop overwritten: %s/%q", got.Status, got.PauseReason)
	}
}

func TestPauseRunByErrorRecordsErrorOnActiveRun(t *testing.T) {
	st, run := newPauseTestRun(t)
	if err := st.PauseRunByError(run.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(run.ID)
	if got.Status != "paused" || got.PauseReason != PauseReasonError {
		t.Fatalf("run = %s/%q, want paused/error", got.Status, got.PauseReason)
	}
}

func TestActivateRunIfPausedByErrorOnlyResumesErrorPauses(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		want   bool
	}{
		{name: "error", reason: PauseReasonError, want: true},
		{name: "user stop", reason: PauseReasonUserStop},
		{name: "restart", reason: PauseReasonRestart},
		{name: "manual pause", reason: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, run := newPauseTestRun(t)
			if err := st.PauseRun(run.ID, tc.reason); err != nil {
				t.Fatal(err)
			}
			revived, err := st.ActivateRunIfPausedByError(run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if revived != tc.want {
				t.Fatalf("revived = %v, want %v", revived, tc.want)
			}
			got, _ := st.GetRun(run.ID)
			wantStatus := "paused"
			if tc.want {
				wantStatus = "active"
			}
			if got.Status != wantStatus {
				t.Fatalf("status = %s, want %s", got.Status, wantStatus)
			}
			if !tc.want && got.PauseReason != tc.reason {
				t.Fatalf("pause reason = %q, want %q", got.PauseReason, tc.reason)
			}
		})
	}
}

func TestActivateRunIfPausedByErrorLeavesActiveRunAlone(t *testing.T) {
	st, run := newPauseTestRun(t)
	revived, err := st.ActivateRunIfPausedByError(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revived {
		t.Fatal("active run must not report a revive")
	}
}
