package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/guaidao2/lingeyebot/internal/llm"
)

func TestCompactMessagesSummarizesOldest(t *testing.T) {
	provider := &scriptedProvider{responses: []llm.Response{{Content: "- decision: keep scope\n- fact: api is reachable"}}}
	messages := []llm.Message{}
	for i := 0; i < 20; i++ {
		messages = append(messages, llm.Message{Role: "user", Content: strings.Repeat("x", 4000)})
	}
	out, changed := compactMessages(context.Background(), provider, "m", messages, 1000, 4)
	if !changed {
		t.Fatal("long history should be compacted")
	}
	if len(out) != 5 {
		t.Fatalf("expected summary + 4 tail messages, got %d", len(out))
	}
	if !strings.Contains(out[0].Content, "keep scope") {
		t.Fatalf("summary missing: %q", out[0].Content)
	}
}

func TestCompactMessagesLeavesShortHistoryAlone(t *testing.T) {
	messages := []llm.Message{{Role: "user", Content: "short"}}
	out, changed := compactMessages(context.Background(), &scriptedProvider{}, "m", messages, 128000, 16)
	if changed || len(out) != 1 {
		t.Fatalf("short history should not compact: changed=%v len=%d", changed, len(out))
	}
}

func TestCompactMessagesKeepsToolPairIntact(t *testing.T) {
	provider := &scriptedProvider{responses: []llm.Response{{Content: "summary"}}}
	messages := []llm.Message{}
	for i := 0; i < 10; i++ {
		messages = append(messages, llm.Message{Role: "user", Content: strings.Repeat("y", 4000)})
	}
	messages = append(messages,
		llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "shell", Arguments: []byte(`{}`)}}},
		llm.Message{Role: "tool", ToolCallID: "c1", Content: "ok"},
	)
	out, changed := compactMessages(context.Background(), provider, "m", messages, 1000, 1)
	if !changed {
		t.Fatal("expected compaction")
	}
	if out[len(out)-1].Role != "tool" {
		t.Fatalf("tail should include the tool result, got %+v", out)
	}
	if out[len(out)-2].Role != "assistant" {
		t.Fatalf("assistant/tool pair was split: %+v", out)
	}
}
