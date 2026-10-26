package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIStreamDeltasAndToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"choices":[{"delta":{"content":"Hel"}}]}`,
			`{"choices":[{"delta":{"content":"lo"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"shell","arguments":"{\"command\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"id\"}"}}]},"finish_reason":"tool_calls"}]}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := NewOpenAI(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, srv.Client())
	var deltas strings.Builder
	resp, err := p.ChatStream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}, func(delta string) {
		deltas.WriteString(delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hello" || deltas.String() != "Hello" {
		t.Fatalf("content=%q deltas=%q", resp.Content, deltas.String())
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "shell" || string(resp.ToolCalls[0].Arguments) != `{"command":"id"}` {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
}

func TestAnthropicStreamDeltasAndToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"shell"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"id\"}"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		}
		for _, event := range events {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	defer srv.Close()

	p := NewAnthropic(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, srv.Client())
	var deltas strings.Builder
	resp, err := p.ChatStream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}, func(delta string) {
		deltas.WriteString(delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hi" || deltas.String() != "Hi" {
		t.Fatalf("content=%q deltas=%q", resp.Content, deltas.String())
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "t1" || string(resp.ToolCalls[0].Arguments) != `{"command":"id"}` {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
}
