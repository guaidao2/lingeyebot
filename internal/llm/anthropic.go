package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const anthropicVersion = "2023-06-01"

// Anthropic implements the Anthropic Messages protocol.
type Anthropic struct {
	id            string
	baseURL       string
	apiKey        string
	model         string
	maxTokens     int
	contextWindow int
	client        *http.Client
}

// NewAnthropic builds an Anthropic adapter.
func NewAnthropic(c Config, client *http.Client) *Anthropic {
	if client == nil {
		client = defaultLLMClient()
	}
	return &Anthropic{id: c.ID, baseURL: c.BaseURL, apiKey: c.APIKey, model: c.Model, maxTokens: c.MaxTokens, contextWindow: c.ContextWindow, client: client}
}

// ID returns the provider id.
func (p *Anthropic) ID() string { return p.id }

// OutputCap returns the configured per-reply output cap, or 0 when unset.
func (p *Anthropic) OutputCap() int { return p.maxTokens }

// ContextWindow returns this channel's context budget, or 0 to inherit.
func (p *Anthropic) ContextWindow() int { return p.contextWindow }

// Endpoint resolves the messages URL from a user-supplied base URL.
func (p *Anthropic) Endpoint() string { return AnthropicEndpoint(p.baseURL) }

// AnthropicEndpoint accepts a bare host, a /v1 base, or a full endpoint.
func AnthropicEndpoint(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	switch {
	case strings.HasSuffix(b, "/messages"):
		return b
	case strings.HasSuffix(b, "/v1"):
		return b + "/messages"
	default:
		return b + "/v1/messages"
	}
}

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

// Chat performs one Anthropic Messages call.
func (p *Anthropic) Chat(ctx context.Context, req Request) (Response, error) {
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			appendAnthropic(&messages, "user", []anthropicBlock{{Type: "text", Text: m.Content}})
		case "assistant":
			blocks := []anthropicBlock{}
			if m.Content != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Arguments
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
			}
			appendAnthropic(&messages, "assistant", blocks)
		case "tool":
			appendAnthropic(&messages, "user", []anthropicBlock{{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content}})
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	body := map[string]any{
		"model":      firstNonEmpty(req.Model, p.model),
		"max_tokens": maxTokens,
		"messages":   messages,
	}
	if strings.TrimSpace(req.System) != "" {
		body["system"] = req.System
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.Parameters,
			})
		}
		body["tools"] = tools
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	resp, err := doWithRetry(ctx, p.client, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint(), bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("anthropic-version", anthropicVersion)
		if p.apiKey != "" {
			httpReq.Header.Set("x-api-key", p.apiKey)
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		return httpReq, nil
	})
	if err != nil {
		return Response{}, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("anthropic %s: %s", resp.Status, truncate(string(raw), 2000))
	}
	var out struct {
		Content []anthropicBlock `json:"content"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, fmt.Errorf("anthropic decode: %w: %s", err, truncate(string(raw), 500))
	}
	result := Response{
		StopReason: out.StopReason,
		Usage:      Usage{InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens},
	}
	for _, block := range out.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "thinking":
			result.Reasoning += block.Thinking
		case "tool_use":
			input := block.Input
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Arguments: input})
		}
	}
	return result, nil
}

// appendAnthropic merges consecutive turns with the same role. Anthropic
// requires alternating user/assistant messages; a tool result followed by an
// auto-continue user message would otherwise produce two user turns.
func appendAnthropic(messages *[]anthropicMessage, role string, blocks []anthropicBlock) {
	if len(blocks) == 0 {
		return
	}
	if n := len(*messages); n > 0 && (*messages)[n-1].Role == role {
		(*messages)[n-1].Content = append((*messages)[n-1].Content, blocks...)
		return
	}
	*messages = append(*messages, anthropicMessage{Role: role, Content: blocks})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
