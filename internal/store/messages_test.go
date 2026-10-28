package store

import "testing"

func TestMessageReasoningSignatureRoundTrip(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	run, _, err := st.CreateRun(Run{Mode: "ctf", Target: "chal", Objective: "flag"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.EnsureSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(Message{
		SessionID: sess.ID, RunID: run.ID, Role: "assistant",
		Reasoning: "plan", ReasoningSignature: "sig-1", Content: "done",
	}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ListMessages(sess.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %+v", msgs)
	}
	if msgs[0].Reasoning != "plan" || msgs[0].ReasoningSignature != "sig-1" {
		t.Fatalf("reasoning = %q signature = %q", msgs[0].Reasoning, msgs[0].ReasoningSignature)
	}
}
