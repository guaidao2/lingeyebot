package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIChatParsesToolCalls(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"我来看看","tool_calls":[
				{"id":"call_1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"id\"}"}}
			]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":12,"completion_tokens":8}
		}`))
	}))
	defer srv.Close()

	p := NewOpenAI(Config{ID: "p1", BaseURL: srv.URL, APIKey: "sk-test", Model: "gpt-x"}, srv.Client())
	resp, err := p.Chat(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools: []ToolSpec{{
			Name: "shell", Description: "run", Parameters: json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %s", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth = %s", gotAuth)
	}
	if gotBody["model"] != "gpt-x" {
		t.Errorf("model = %v", gotBody["model"])
	}
	if _, ok := gotBody["tools"]; !ok {
		t.Errorf("tools missing from request")
	}
	if resp.Content != "我来看看" || len(resp.ToolCalls) != 1 {
		t.Fatalf("response = %+v", resp)
	}
	if resp.ToolCalls[0].Name != "shell" || string(resp.ToolCalls[0].Arguments) != `{"command":"id"}` {
		t.Errorf("tool call = %+v", resp.ToolCalls[0])
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 8 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestAnthropicChatParsesToolUse(t *testing.T) {
	var gotPath, gotKey, gotVersion string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content":[
				{"type":"text","text":"先侦察"},
				{"type":"tool_use","id":"tu_1","name":"http_request","input":{"url":"https://example.com"}}
			],
			"stop_reason":"tool_use",
			"usage":{"input_tokens":21,"output_tokens":9}
		}`))
	}))
	defer srv.Close()

	p := NewAnthropic(Config{ID: "p2", BaseURL: srv.URL, APIKey: "ant-test", Model: "claude-x"}, srv.Client())
	resp, err := p.Chat(context.Background(), Request{
		System:   "system prompt",
		Messages: []Message{{Role: "user", Content: "开始"}},
		Tools: []ToolSpec{{
			Name: "http_request", Description: "http", Parameters: json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path = %s", gotPath)
	}
	if gotKey != "ant-test" || gotVersion != anthropicVersion {
		t.Errorf("headers: key=%s version=%s", gotKey, gotVersion)
	}
	if gotBody["system"] != "system prompt" {
		t.Errorf("system = %v", gotBody["system"])
	}
	if resp.Content != "先侦察" || len(resp.ToolCalls) != 1 {
		t.Fatalf("response = %+v", resp)
	}
	if resp.ToolCalls[0].Name != "http_request" || string(resp.ToolCalls[0].Arguments) != `{"url":"https://example.com"}` {
		t.Errorf("tool call = %+v", resp.ToolCalls[0])
	}
}

func TestEndpointNormalization(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com":                        "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/":                       "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/v1":                     "https://api.openai.com/v1/chat/completions",
		"https://proxy.example.com/v1/chat/completions": "https://proxy.example.com/v1/chat/completions",
	}
	for in, want := range cases {
		if got := OpenAIEndpoint(in); got != want {
			t.Errorf("OpenAIEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
	anth := map[string]string{
		"https://api.anthropic.com":             "https://api.anthropic.com/v1/messages",
		"https://api.anthropic.com/v1":          "https://api.anthropic.com/v1/messages",
		"https://proxy.example.com/v1/messages": "https://proxy.example.com/v1/messages",
	}
	for in, want := range anth {
		if got := AnthropicEndpoint(in); got != want {
			t.Errorf("AnthropicEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProviderErrorsSurfaceBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()
	p := NewOpenAI(Config{BaseURL: srv.URL, APIKey: "x"}, srv.Client())
	_, err := p.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected error body, got %v", err)
	}
}

func TestAnthropicCoalescesConsecutiveRoles(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()
	p := NewAnthropic(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, srv.Client())
	_, err := p.Chat(context.Background(), Request{Messages: []Message{
		{Role: "user", Content: "first"},
		{Role: "tool", ToolCallID: "t1", Content: "result"},
		{Role: "user", Content: "second"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("consecutive user turns were not coalesced: %s", mustJSON(t, gotBody))
	}
	first, _ := msgs[0].(map[string]any)
	blocks, _ := first["content"].([]any)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks in one user turn, got %s", mustJSON(t, gotBody))
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOpenAIOutputCapAndField(t *testing.T) {
	cases := []struct {
		model    string
		field    string
		wantKey  string
		otherKey string
	}{
		{"gpt-4.1", "", "max_tokens", "max_completion_tokens"},
		{"gpt-5", "", "max_completion_tokens", "max_tokens"},
		{"gpt-4.1", "max_completion_tokens", "max_completion_tokens", "max_tokens"},
	}
	for _, tc := range cases {
		t.Run(tc.model+"-"+tc.field, func(t *testing.T) {
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			}))
			defer srv.Close()
			p := NewOpenAI(Config{BaseURL: srv.URL, Model: tc.model, MaxTokens: 32768, MaxTokensField: tc.field}, srv.Client())
			if _, err := p.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}); err != nil {
				t.Fatal(err)
			}
			if got := body[tc.wantKey]; got != float64(32768) {
				t.Fatalf("%s = %v, want 32768; body=%v", tc.wantKey, got, body)
			}
			if _, exists := body[tc.otherKey]; exists {
				t.Fatalf("%s must not be sent: body=%v", tc.otherKey, body)
			}
		})
	}
}

func TestOpenAIReasoningProfiles(t *testing.T) {
	cases := []struct {
		name    string
		model   string
		mode    string
		effort  string
		profile string
		wantKey string
		wantVal string
	}{
		{"deepseek-off", "deepseek-chat", "off", "", "deepseek_compat", "thinking.type", "disabled"},
		{"deepseek-on", "deepseek-chat", "on", "", "deepseek_compat", "thinking.type", "enabled"},
		{"gpt5-off", "gpt-5", "off", "", "openai_compat", "reasoning_effort", "none"},
		{"gpt5-high", "gpt-5", "auto", "high", "openai_compat", "reasoning_effort", "high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			}))
			defer srv.Close()
			p := NewOpenAI(Config{
				BaseURL: srv.URL, Model: tc.model, MaxTokens: 32768,
				ReasoningMode: tc.mode, ReasoningEffort: tc.effort, ReasoningProfile: tc.profile,
			}, srv.Client())
			if _, err := p.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}); err != nil {
				t.Fatal(err)
			}
			var got string
			if tc.wantKey == "thinking.type" {
				thinking, _ := body["thinking"].(map[string]any)
				got, _ = thinking["type"].(string)
			} else {
				got, _ = body[tc.wantKey].(string)
			}
			if got != tc.wantVal {
				t.Fatalf("%s = %q, want %q; body=%v", tc.wantKey, got, tc.wantVal, body)
			}
		})
	}
}
