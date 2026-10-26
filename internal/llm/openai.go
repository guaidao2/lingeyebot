package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// OpenAI implements the OpenAI-compatible chat completions protocol.
type OpenAI struct {
	id               string
	baseURL          string
	apiKey           string
	model            string
	maxTokens        int
	maxTokensField   string
	contextWindow    int
	reasoningMode    string
	reasoningEffort  string
	reasoningProfile string
	client           *http.Client
}

// NewOpenAI builds an OpenAI-compatible adapter.
func NewOpenAI(c Config, client *http.Client) *OpenAI {
	if client == nil {
		client = defaultLLMClient()
	}
	return &OpenAI{
		id: c.ID, baseURL: c.BaseURL, apiKey: c.APIKey, model: c.Model,
		maxTokens: c.MaxTokens, maxTokensField: c.MaxTokensField,
		contextWindow: c.ContextWindow, reasoningMode: c.ReasoningMode,
		reasoningEffort: c.ReasoningEffort, reasoningProfile: c.ReasoningProfile,
		client: client,
	}
}

// ID returns the provider id.
func (p *OpenAI) ID() string { return p.id }

// OutputCap returns the configured per-reply output cap, or 0 when unset.
func (p *OpenAI) OutputCap() int { return p.maxTokens }

// ContextWindow returns this channel's context budget, or 0 to inherit.
func (p *OpenAI) ContextWindow() int { return p.contextWindow }

func (p *OpenAI) applyReasoning(body map[string]any, model string) {
	mode := p.reasoningMode
	if mode == "" {
		mode = "auto"
	}
	profile := p.reasoningProfile
	if profile == "" || profile == "auto" {
		if strings.Contains(strings.ToLower(model), "deepseek") {
			profile = "deepseek_compat"
		} else {
			profile = "openai_compat"
		}
	}
	effort := p.reasoningEffort
	if effort == "none" {
		mode = "off"
	}
	if profile == "deepseek_compat" {
		switch mode {
		case "off":
			body["thinking"] = map[string]any{"type": "disabled"}
		case "on":
			body["thinking"] = map[string]any{"type": "enabled"}
		default:
			if effort != "" {
				body["thinking"] = map[string]any{"type": "enabled"}
			}
		}
		return
	}
	if mode == "off" {
		if isOpenAIReasoningModel(model) || effort == "none" {
			body["reasoning_effort"] = "none"
		}
		return
	}
	if effort != "" && effort != "none" {
		body["reasoning_effort"] = effort
	}
}

func isOpenAIReasoningModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"o1", "o3", "o4", "gpt-5"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func openAITokenKey(model, field string) string {
	switch field {
	case "max_tokens", "max_completion_tokens":
		return field
	}
	if isOpenAIReasoningModel(model) {
		return "max_completion_tokens"
	}
	return "max_tokens"
}

// Endpoint resolves the chat-completions URL from a user-supplied base URL.
func (p *OpenAI) Endpoint() string { return OpenAIEndpoint(p.baseURL) }

// OpenAIEndpoint accepts a bare host, a /v1 base, or a full endpoint.
func OpenAIEndpoint(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	switch {
	case strings.HasSuffix(b, "/chat/completions"):
		return b
	case strings.HasSuffix(b, "/v1"):
		return b + "/chat/completions"
	default:
		return b + "/v1/chat/completions"
	}
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

// Chat performs one OpenAI-compatible call.
func (p *OpenAI) Chat(ctx context.Context, req Request) (Response, error) {
	messages := make([]openAIMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		om := openAIMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID, Name: m.Name}
		if m.Role == "tool" && m.Content == "" {
			om.Content = ""
		}
		for _, tc := range m.ToolCalls {
			args := string(tc.Arguments)
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			om.ToolCalls = append(om.ToolCalls, openAIToolCall{
				ID: tc.ID, Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: tc.Name, Arguments: args},
			})
		}
		messages = append(messages, om)
	}

	body := map[string]any{"model": firstNonEmpty(req.Model, p.model), "messages": messages}
	if len(req.Tools) > 0 {
		tools := make([]openAITool, 0, len(req.Tools))
		for _, t := range req.Tools {
			tool := openAITool{Type: "function"}
			tool.Function.Name = t.Name
			tool.Function.Description = t.Description
			tool.Function.Parameters = t.Parameters
			tools = append(tools, tool)
		}
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	model := firstNonEmpty(req.Model, p.model)
	body[openAITokenKey(model, p.maxTokensField)] = maxTokens
	p.applyReasoning(body, model)
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
		if p.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		return httpReq, nil
	})
	if err != nil {
		return Response{}, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("openai %s: %s", resp.Status, truncate(string(raw), 2000))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content          *string          `json:"content"`
				ReasoningContent *string          `json:"reasoning_content"`
				Reasoning        *string          `json:"reasoning"`
				ToolCalls        []openAIToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, fmt.Errorf("openai decode: %w: %s", err, truncate(string(raw), 500))
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("openai returned no choices")
	}
	choice := out.Choices[0]
	result := Response{
		StopReason: choice.FinishReason,
		Usage:      Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens},
	}
	if choice.Message.Content != nil {
		result.Content = *choice.Message.Content
	}
	if choice.Message.ReasoningContent != nil {
		result.Reasoning = *choice.Message.ReasoningContent
	} else if choice.Message.Reasoning != nil {
		result.Reasoning = *choice.Message.Reasoning
	}
	for _, tc := range choice.Message.ToolCalls {
		args := tc.Function.Arguments
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		result.ToolCalls = append(result.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: json.RawMessage(args)})
	}
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 0 {
		n = 0
	}
	s = s[:n]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s + "...(truncated)"
}
