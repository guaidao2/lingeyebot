package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// recordingServer captures every request body and answers both streamed and
// non-streamed calls with a minimal valid response.
func recordingServer(t *testing.T) (string, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(server.Close)
	return server.URL, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), bodies...)
	}
}

func messageAt(t *testing.T, body map[string]any, index int) map[string]any {
	t.Helper()
	messages, ok := body["messages"].([]any)
	if !ok || index >= len(messages) {
		t.Fatalf("messages = %#v", body["messages"])
	}
	message, ok := messages[index].(map[string]any)
	if !ok {
		t.Fatalf("message %d = %#v", index, messages[index])
	}
	return message
}

func TestDeepSeekReplaysAssistantReasoning(t *testing.T) {
	baseURL, bodies := recordingServer(t)
	provider := NewOpenAI(Config{
		BaseURL:          baseURL,
		Model:            "deepseek-chat",
		ReasoningProfile: "deepseek_compat",
	}, nil)
	request := Request{Messages: []Message{
		{Role: "assistant", Content: "previous", Reasoning: "thought"},
		{Role: "user", Content: "next"},
	}}
	if _, err := provider.Chat(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ChatStreamWithReasoning(context.Background(), request, func(string) {}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	all := bodies()
	if len(all) != 2 {
		t.Fatalf("requests = %d, want 2", len(all))
	}
	for i, body := range all {
		assistant := messageAt(t, body, 0)
		if got, _ := assistant["reasoning_content"].(string); got != "thought" {
			t.Fatalf("body %d reasoning_content = %q", i, got)
		}
		if _, present := messageAt(t, body, 1)["reasoning_content"]; present {
			t.Fatalf("body %d attached reasoning_content to a user message", i)
		}
	}
}

func TestDeepSeekBackfillsEmptyReasoning(t *testing.T) {
	baseURL, bodies := recordingServer(t)
	provider := NewOpenAI(Config{
		BaseURL:          baseURL,
		Model:            "deepseek-chat",
		ReasoningProfile: "deepseek_compat",
	}, nil)
	request := Request{Messages: []Message{
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Name: "echo", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", ToolCallID: "call_1", Name: "echo", Content: "ok"},
	}}
	if _, err := provider.Chat(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ChatStreamWithReasoning(context.Background(), request, func(string) {}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	for i, body := range bodies() {
		assistant := messageAt(t, body, 0)
		value, present := assistant["reasoning_content"]
		if !present {
			t.Fatalf("body %d omitted reasoning_content for an assistant message", i)
		}
		if value != "" {
			t.Fatalf("body %d reasoning_content = %#v, want empty string", i, value)
		}
	}
}

// CyberStrikeAI keeps reasoning_content in history for every provider; only
// DeepSeek backfills an empty value for turns that had no reasoning.
func TestReasoningContentReplayedForEveryOpenAICompatibleProvider(t *testing.T) {
	cases := []struct {
		name              string
		cfg               Config
		backfillWhenEmpty bool
	}{
		{name: "openai compat", cfg: Config{Model: "gpt-4o", ReasoningProfile: "openai_compat"}},
		{name: "auto gateway", cfg: Config{Model: "gpt-4o"}},
		{name: "output config", cfg: Config{Model: "gateway-model", ReasoningProfile: "output_config_effort"}},
		{name: "deepseek", cfg: Config{Model: "deepseek-chat", ReasoningProfile: "deepseek_compat"}, backfillWhenEmpty: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, bodies := recordingServer(t)
			cfg := tc.cfg
			cfg.BaseURL = baseURL
			provider := NewOpenAI(cfg, nil)
			request := Request{Messages: []Message{
				{Role: "assistant", Content: "answer", Reasoning: "thought"},
				{Role: "assistant", Content: "answer without reasoning"},
				{Role: "user", Content: "next"},
			}}
			if _, err := provider.Chat(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if _, err := provider.ChatStreamWithReasoning(context.Background(), request, func(string) {}, func(string) {}); err != nil {
				t.Fatal(err)
			}
			for i, body := range bodies() {
				if got, _ := messageAt(t, body, 0)["reasoning_content"].(string); got != "thought" {
					t.Fatalf("request %d reasoning_content = %q, want thought", i, got)
				}
				_, present := messageAt(t, body, 1)["reasoning_content"]
				if present != tc.backfillWhenEmpty {
					t.Fatalf("request %d empty reasoning present = %v, want %v", i, present, tc.backfillWhenEmpty)
				}
				if _, present := messageAt(t, body, 2)["reasoning_content"]; present {
					t.Fatalf("request %d attached reasoning_content to a user message", i)
				}
			}
		})
	}
}

func TestAutoProfileDetectsDeepSeekByEndpoint(t *testing.T) {
	cases := []struct {
		baseURL string
		model   string
		want    string
	}{
		{baseURL: "https://api.deepseek.com/v1", model: "any-model", want: "deepseek_compat"},
		{baseURL: "https://gateway.example.com/deepseek/v1", model: "any-model", want: "deepseek_compat"},
		{baseURL: "https://gateway.example.com/v1", model: "deepseek-chat", want: "openai_compat"},
	}
	for _, tc := range cases {
		provider := NewOpenAI(Config{BaseURL: tc.baseURL, Model: tc.model}, nil)
		if got := provider.reasoningProfileFor(tc.model); got != tc.want {
			t.Errorf("%s + %s profile = %q, want %q", tc.baseURL, tc.model, got, tc.want)
		}
	}
	explicit := NewOpenAI(Config{
		BaseURL: "https://gateway.example.com/deepseek/v1", Model: "m", ReasoningProfile: "openai_compat",
	}, nil)
	if got := explicit.reasoningProfileFor("m"); got != "openai_compat" {
		t.Errorf("explicit profile = %q, want openai_compat", got)
	}
}

func TestReasoningRequestFieldsPerProfile(t *testing.T) {
	cases := []struct {
		name       string
		cfg        Config
		disable    bool
		wantBody   map[string]any
		wantAbsent []string
	}{
		{
			name:     "deepseek auto enables thinking",
			cfg:      Config{Model: "deepseek-chat", ReasoningProfile: "deepseek_compat", ReasoningMode: "auto"},
			wantBody: map[string]any{"thinking": map[string]any{"type": "enabled"}},
		},
		{
			name:     "deepseek effort",
			cfg:      Config{Model: "deepseek-chat", ReasoningProfile: "deepseek_compat", ReasoningEffort: "high"},
			wantBody: map[string]any{"thinking": map[string]any{"type": "enabled"}, "reasoning_effort": "high"},
		},
		{
			name:       "deepseek off",
			cfg:        Config{Model: "deepseek-chat", ReasoningProfile: "deepseek_compat", ReasoningMode: "off"},
			wantBody:   map[string]any{"thinking": map[string]any{"type": "disabled"}},
			wantAbsent: []string{"reasoning_effort"},
		},
		{
			name:       "deepseek off ignores leftover effort",
			cfg:        Config{Model: "deepseek-chat", ReasoningProfile: "deepseek_compat", ReasoningMode: "off", ReasoningEffort: "high"},
			wantBody:   map[string]any{"thinking": map[string]any{"type": "disabled"}},
			wantAbsent: []string{"reasoning_effort"},
		},
		{
			name:     "deepseek disable reasoning for compaction",
			cfg:      Config{Model: "deepseek-chat", ReasoningProfile: "deepseek_compat", ReasoningMode: "on"},
			disable:  true,
			wantBody: map[string]any{"thinking": map[string]any{"type": "disabled"}},
		},
		{
			name:     "openai mode on without effort uses medium",
			cfg:      Config{Model: "gpt-4o", ReasoningMode: "on"},
			wantBody: map[string]any{"reasoning_effort": "medium"},
		},
		{
			name:       "openai auto leaves default",
			cfg:        Config{Model: "gpt-4o", ReasoningMode: "auto"},
			wantAbsent: []string{"reasoning_effort", "thinking", "output_config"},
		},
		{
			name:     "openai reasoning model off sends none",
			cfg:      Config{Model: "gpt-5.2", ReasoningMode: "off"},
			wantBody: map[string]any{"reasoning_effort": "none"},
		},
		{
			name:     "openai disable reasoning for compaction",
			cfg:      Config{Model: "gpt-5.2", ReasoningMode: "on"},
			disable:  true,
			wantBody: map[string]any{"reasoning_effort": "none"},
		},
		{
			name:     "output config effort",
			cfg:      Config{Model: "gateway", ReasoningProfile: "output_config_effort", ReasoningEffort: "high"},
			wantBody: map[string]any{"output_config": map[string]any{"effort": "high"}},
		},
		{
			name:     "output config on without effort uses high",
			cfg:      Config{Model: "gateway", ReasoningProfile: "output_config_effort", ReasoningMode: "on"},
			wantBody: map[string]any{"output_config": map[string]any{"effort": "high"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, bodies := recordingServer(t)
			cfg := tc.cfg
			cfg.BaseURL = baseURL
			provider := NewOpenAI(cfg, nil)
			if _, err := provider.Chat(context.Background(), Request{
				Messages: []Message{{Role: "user", Content: "hi"}}, DisableReasoning: tc.disable,
			}); err != nil {
				t.Fatal(err)
			}
			all := bodies()
			if len(all) != 1 {
				t.Fatalf("requests = %d, want 1", len(all))
			}
			for key, want := range tc.wantBody {
				got, ok := all[0][key]
				if !ok {
					t.Fatalf("body missing %s: %+v", key, all[0])
				}
				if !equalJSON(got, want) {
					t.Fatalf("body[%s] = %#v, want %#v", key, got, want)
				}
			}
			for _, key := range tc.wantAbsent {
				if _, ok := all[0][key]; ok {
					t.Fatalf("body unexpectedly has %s: %+v", key, all[0])
				}
			}
		})
	}
}

func equalJSON(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}
