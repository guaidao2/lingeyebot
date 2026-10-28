package agent

import (
	"testing"

	"github.com/guaidao2/lingeyebot/internal/store"
)

func TestMessagesFromStorePreservesReasoning(t *testing.T) {
	out := MessagesFromStore([]store.Message{
		{Role: "assistant", Content: "answer", Reasoning: "thought", ReasoningSignature: "sig-1"},
		{Role: "user", Content: "next"},
		{Role: "assistant", Content: "answer 2", Reasoning: "thought-2"},
	})
	if len(out) != 3 {
		t.Fatalf("messages = %+v", out)
	}
	if out[0].Reasoning != "thought" || out[2].Reasoning != "thought-2" {
		t.Fatalf("reasoning dropped: %+v", out)
	}
	if out[0].ReasoningSignature != "sig-1" {
		t.Fatalf("reasoning signature dropped: %+v", out)
	}
}

func TestMessagesFromStoreDropsReasoningOnlyAssistant(t *testing.T) {
	out := MessagesFromStore([]store.Message{
		{Role: "assistant", Reasoning: "thought without content"},
		{Role: "user", Content: "next"},
	})
	if len(out) != 1 || out[0].Role != "user" {
		t.Fatalf("reasoning-only assistant must be dropped: %+v", out)
	}
}
