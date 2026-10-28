package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicThinkingRequestConfig(t *testing.T) {
	cases := []struct {
		name       string
		cfg        Config
		maxTokens  int
		wantThink  map[string]any
		wantEffort string
	}{
		{
			name:      "legacy model uses budget tokens",
			cfg:       Config{Model: "claude-sonnet-4-5", ReasoningMode: "on"},
			maxTokens: 32768,
			wantThink: map[string]any{"type": "enabled", "budget_tokens": 10000, "display": "summarized"},
		},
		{
			name:      "dated legacy model stays on budget shape",
			cfg:       Config{Model: "claude-sonnet-4-5-20250929", ReasoningMode: "on"},
			maxTokens: 32768,
			wantThink: map[string]any{"type": "enabled", "budget_tokens": 10000, "display": "summarized"},
		},
		{
			name:      "dated claude 3.5 stays on budget shape",
			cfg:       Config{Model: "claude-3-5-sonnet-20241022", ReasoningMode: "on"},
			maxTokens: 32768,
			wantThink: map[string]any{"type": "enabled", "budget_tokens": 10000, "display": "summarized"},
		},
		{
			name:      "adaptive model",
			cfg:       Config{Model: "claude-opus-4-7", ReasoningMode: "on"},
			maxTokens: 32768,
			wantThink: map[string]any{"type": "adaptive", "display": "summarized"},
		},
		{
			name:      "dated adaptive model",
			cfg:       Config{Model: "claude-opus-4-6-20260101", ReasoningMode: "on"},
			maxTokens: 32768,
			wantThink: map[string]any{"type": "adaptive", "display": "summarized"},
		},
		{
			name:      "sonnet 3.7 ignores effort",
			cfg:       Config{Model: "claude-3-7-sonnet-latest", ReasoningEffort: "high", ReasoningMode: "on"},
			maxTokens: 20000,
			wantThink: map[string]any{"type": "enabled", "budget_tokens": 10000, "display": "summarized"},
		},
		{
			name:      "small max tokens halves budget",
			cfg:       Config{Model: "claude-sonnet-4-5", ReasoningMode: "on"},
			maxTokens: 4000,
			wantThink: map[string]any{"type": "enabled", "budget_tokens": 2000, "display": "summarized"},
		},
		{
			name:      "max tokens too small disables thinking",
			cfg:       Config{Model: "claude-sonnet-4-5", ReasoningMode: "on"},
			maxTokens: 1000,
		},
		{
			name:      "off disables thinking",
			cfg:       Config{Model: "claude-sonnet-4-5", ReasoningMode: "off"},
			maxTokens: 32768,
		},
		{
			name:      "effort none disables thinking",
			cfg:       Config{Model: "claude-sonnet-4-5", ReasoningEffort: "none", ReasoningMode: "on"},
			maxTokens: 32768,
		},
		{
			name:       "effort maps to output config",
			cfg:        Config{Model: "claude-sonnet-4-5", ReasoningEffort: "high", ReasoningMode: "on"},
			maxTokens:  32768,
			wantThink:  map[string]any{"type": "enabled", "budget_tokens": 10000, "display": "summarized"},
			wantEffort: "high",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, bodies := recordingServer(t)
			cfg := tc.cfg
			cfg.BaseURL = baseURL
			provider := NewAnthropic(cfg, nil)
			if _, err := provider.Chat(context.Background(), Request{
				Messages:  []Message{{Role: "user", Content: "hi"}},
				MaxTokens: tc.maxTokens,
			}); err != nil {
				t.Fatal(err)
			}
			all := bodies()
			if len(all) != 1 {
				t.Fatalf("requests = %d, want 1", len(all))
			}
			got, present := all[0]["thinking"]
			if tc.wantThink == nil {
				if present {
					t.Fatalf("thinking unexpectedly present: %#v", got)
				}
				return
			}
			if !present || !equalJSON(got, tc.wantThink) {
				t.Fatalf("thinking = %#v, want %#v", got, tc.wantThink)
			}
			if tc.wantEffort != "" {
				output, _ := all[0]["output_config"].(map[string]any)
				if output["effort"] != tc.wantEffort {
					t.Fatalf("output_config = %#v, want effort %q", all[0]["output_config"], tc.wantEffort)
				}
			}
		})
	}
}

func TestAnthropicAutoThinkingOnlyOnAnthropicWire(t *testing.T) {
	cases := []struct {
		name      string
		baseURL   string
		model     string
		mode      string
		maxTokens int
		want      bool
	}{
		{name: "native auto", baseURL: "https://api.anthropic.com", model: "claude-sonnet-4-5", mode: "auto", maxTokens: 32768, want: true},
		{name: "native auto bare host", baseURL: "api.anthropic.com", model: "claude-sonnet-4-5", mode: "auto", maxTokens: 32768, want: true},
		{name: "native auto 3.7", baseURL: "https://api.anthropic.com", model: "claude-3-7-sonnet-latest", mode: "auto", maxTokens: 32768, want: true},
		{name: "native auto old model", baseURL: "https://api.anthropic.com", model: "claude-3-5-sonnet-20241022", mode: "auto", maxTokens: 32768},
		{name: "native explicit on old model", baseURL: "https://api.anthropic.com", model: "claude-3-5-sonnet-20241022", mode: "on", maxTokens: 32768, want: true},
		{name: "deepseek anthropic facade auto", baseURL: "https://api.deepseek.com/anthropic", model: "claude-sonnet-4-5", mode: "auto", maxTokens: 32768},
		{name: "generic proxy auto", baseURL: "https://proxy.example.com", model: "claude-sonnet-4-5", mode: "auto", maxTokens: 32768},
		{name: "deepseek anthropic facade explicit on", baseURL: "https://api.deepseek.com/anthropic", model: "claude-sonnet-4-5", mode: "on", maxTokens: 32768, want: true},
		{name: "compaction budget", baseURL: "https://api.anthropic.com", model: "claude-sonnet-4-5", mode: "auto", maxTokens: 700},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewAnthropic(Config{BaseURL: tc.baseURL, Model: tc.model, ReasoningMode: tc.mode}, nil)
			cfg, _ := provider.thinkingConfig(tc.model, tc.maxTokens)
			if (cfg != nil) != tc.want {
				t.Fatalf("thinking = %#v, want present=%v", cfg, tc.want)
			}
		})
	}
}

func TestAnthropicDisableReasoningSkipsThinking(t *testing.T) {
	baseURL, bodies := recordingServer(t)
	provider := NewAnthropic(Config{BaseURL: baseURL, Model: "claude-sonnet-4-5", ReasoningMode: "on"}, nil)
	if _, err := provider.Chat(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}}, DisableReasoning: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, present := bodies()[0]["thinking"]; present {
		t.Fatalf("thinking unexpectedly present: %#v", bodies()[0]["thinking"])
	}
}

func TestAnthropicMultipleSignedBlocksDropsSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[
			{"type":"thinking","thinking":"first","signature":"sig-1"},
			{"type":"thinking","thinking":"second","signature":"sig-2"},
			{"type":"text","text":"done"}
		],"stop_reason":"end_turn"}`)
	}))
	defer srv.Close()

	provider := NewAnthropic(Config{BaseURL: srv.URL, Model: "claude-sonnet-4-5", ReasoningMode: "on"}, srv.Client())
	resp, err := provider.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReasoningSignature != "" {
		t.Fatalf("multiple signed blocks must not keep a signature: %q", resp.ReasoningSignature)
	}
	if resp.Reasoning != "firstsecond" {
		t.Fatalf("reasoning display = %q", resp.Reasoning)
	}
}

func TestAnthropicSingleSignedBlockKeepsMatchingText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[
			{"type":"thinking","thinking":"unsigned"},
			{"type":"thinking","thinking":"signed","signature":"sig-2"},
			{"type":"text","text":"done"}
		],"stop_reason":"end_turn"}`)
	}))
	defer srv.Close()

	provider := NewAnthropic(Config{BaseURL: srv.URL, Model: "claude-sonnet-4-5", ReasoningMode: "on"}, srv.Client())
	resp, err := provider.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Reasoning != "signed" || resp.ReasoningSignature != "sig-2" {
		t.Fatalf("reasoning=%q signature=%q", resp.Reasoning, resp.ReasoningSignature)
	}
}

func TestAnthropicReplaysSignedThinkingBlock(t *testing.T) {
	baseURL, bodies := recordingServer(t)
	provider := NewAnthropic(Config{
		BaseURL: baseURL, Model: "claude-sonnet-4-5", ReasoningEffort: "high", ReasoningMode: "on",
	}, nil)
	_, err := provider.Chat(context.Background(), Request{
		Temperature: 0.5,
		Messages: []Message{
			{Role: "user", Content: "go"},
			{Role: "assistant", Reasoning: "plan", ReasoningSignature: "sig-1",
				ToolCalls: []ToolCall{{ID: "t1", Name: "shell", Arguments: json.RawMessage(`{"cmd":"id"}`)}}},
			{Role: "tool", ToolCallID: "t1", Name: "shell", Content: "uid=0"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := bodies()[0]
	if _, present := body["temperature"]; present {
		t.Fatal("temperature must be omitted on a thinking request")
	}
	messages, _ := body["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	assistant, _ := messages[1].(map[string]any)
	blocks, _ := assistant["content"].([]any)
	thinking, _ := blocks[0].(map[string]any)
	if thinking["type"] != "thinking" || thinking["thinking"] != "plan" || thinking["signature"] != "sig-1" {
		t.Fatalf("first block = %#v", thinking)
	}
	toolUse, _ := blocks[1].(map[string]any)
	if toolUse["type"] != "tool_use" || toolUse["id"] != "t1" {
		t.Fatalf("second block = %#v", toolUse)
	}
	output, _ := body["output_config"].(map[string]any)
	if output["effort"] != "high" {
		t.Fatalf("output_config = %#v", body["output_config"])
	}
}

func TestAnthropicSkipsUnsignedOrDisabledThinking(t *testing.T) {
	cases := []struct {
		name         string
		cfg          Config
		msg          Message
		wantThinking bool
	}{
		{
			name:         "unsigned reasoning is not replayed",
			cfg:          Config{Model: "claude-sonnet-4-5", ReasoningMode: "on"},
			msg:          Message{Role: "assistant", Reasoning: "plan", Content: "done"},
			wantThinking: true,
		},
		{
			name: "thinking off drops signed blocks",
			cfg:  Config{Model: "claude-sonnet-4-5", ReasoningMode: "off"},
			msg:  Message{Role: "assistant", Reasoning: "plan", ReasoningSignature: "sig-1", Content: "done"},
		},
		{
			name: "unsigned tool history disables thinking",
			cfg:  Config{Model: "claude-sonnet-4-5", ReasoningMode: "on"},
			msg: Message{Role: "assistant", Reasoning: "plan",
				ToolCalls: []ToolCall{{ID: "t1", Name: "shell", Arguments: json.RawMessage(`{}`)}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, bodies := recordingServer(t)
			cfg := tc.cfg
			cfg.BaseURL = baseURL
			provider := NewAnthropic(cfg, nil)
			if _, err := provider.Chat(context.Background(), Request{Messages: []Message{
				{Role: "user", Content: "go"},
				tc.msg,
			}}); err != nil {
				t.Fatal(err)
			}
			body := bodies()[0]
			if _, present := body["thinking"]; present != tc.wantThinking {
				t.Fatalf("thinking parameter present = %v, want %v (%#v)", present, tc.wantThinking, body["thinking"])
			}
			messages, _ := body["messages"].([]any)
			assistant, _ := messages[1].(map[string]any)
			blocks, _ := assistant["content"].([]any)
			for _, raw := range blocks {
				block, _ := raw.(map[string]any)
				if block["type"] == "thinking" {
					t.Fatalf("unexpected thinking block: %#v", block)
				}
			}
		})
	}
}

func TestAnthropicParsesThinkingSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[
			{"type":"thinking","thinking":"plan","signature":"sig-1"},
			{"type":"text","text":"done"}
		],"stop_reason":"end_turn"}`)
	}))
	defer srv.Close()

	provider := NewAnthropic(Config{BaseURL: srv.URL, Model: "claude-sonnet-4-5"}, srv.Client())
	resp, err := provider.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Reasoning != "plan" || resp.ReasoningSignature != "sig-1" || resp.Content != "done" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAnthropicStreamMultipleSignedBlocksDropsSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"first"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"thinking"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"second"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"sig-2"}}`,
			`{"type":"content_block_start","index":2,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"done"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		}
		for _, event := range events {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	defer srv.Close()

	provider := NewAnthropic(Config{BaseURL: srv.URL, Model: "claude-sonnet-4-5", ReasoningMode: "on"}, srv.Client())
	resp, err := provider.ChatStreamWithReasoning(context.Background(),
		Request{Messages: []Message{{Role: "user", Content: "hi"}}},
		func(string) {}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReasoningSignature != "" {
		t.Fatalf("multiple signed blocks must not keep a signature: %q", resp.ReasoningSignature)
	}
	if resp.Reasoning != "firstsecond" || resp.Content != "done" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAnthropicStreamGuardSkipsUnsignedToolThinking(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
	}))
	defer srv.Close()

	provider := NewAnthropic(Config{BaseURL: srv.URL, Model: "claude-sonnet-4-5", ReasoningMode: "on"}, srv.Client())
	_, err := provider.ChatStreamWithReasoning(context.Background(), Request{Messages: []Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", Reasoning: "plan",
			ToolCalls: []ToolCall{{ID: "t1", Name: "shell", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", ToolCallID: "t1", Name: "shell", Content: "ok"},
	}}, func(string) {}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := gotBody["thinking"]; present {
		t.Fatalf("thinking unexpectedly present: %#v", gotBody["thinking"])
	}
	messages, _ := gotBody["messages"].([]any)
	assistant, _ := messages[1].(map[string]any)
	blocks, _ := assistant["content"].([]any)
	for _, raw := range blocks {
		block, _ := raw.(map[string]any)
		if block["type"] == "thinking" {
			t.Fatalf("unexpected thinking block: %#v", block)
		}
	}
}

func TestAnthropicStreamCapturesThinkingSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"plan"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"done"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		}
		for _, event := range events {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	defer srv.Close()

	provider := NewAnthropic(Config{BaseURL: srv.URL, Model: "claude-sonnet-4-5"}, srv.Client())
	resp, err := provider.ChatStreamWithReasoning(context.Background(),
		Request{Messages: []Message{{Role: "user", Content: "hi"}}},
		func(string) {}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Reasoning != "plan" || resp.ReasoningSignature != "sig-1" || resp.Content != "done" {
		t.Fatalf("response = %+v", resp)
	}
}
