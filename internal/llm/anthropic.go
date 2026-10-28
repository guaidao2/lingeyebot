package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const anthropicVersion = "2023-06-01"

// Anthropic implements the Anthropic Messages protocol.
type Anthropic struct {
	id              string
	baseURL         string
	apiKey          string
	model           string
	maxTokens       int
	contextWindow   int
	reasoningMode   string
	reasoningEffort string
	client          *http.Client
}

// NewAnthropic builds an Anthropic adapter.
func NewAnthropic(c Config, client *http.Client) *Anthropic {
	if client == nil {
		client = defaultLLMClient()
	}
	return &Anthropic{
		id: c.ID, baseURL: c.BaseURL, apiKey: c.APIKey, model: c.Model, maxTokens: c.MaxTokens,
		contextWindow: c.ContextWindow, reasoningMode: c.ReasoningMode, reasoningEffort: c.ReasoningEffort,
		client: client,
	}
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
	Signature string          `json:"signature,omitempty"`
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
	model := firstNonEmpty(req.Model, p.model)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	thinkingCfg, allowEffort := p.thinkingConfigForMessages(req, model, maxTokens)

	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			appendAnthropic(&messages, "user", []anthropicBlock{{Type: "text", Text: m.Content}})
		case "assistant":
			blocks := []anthropicBlock{}
			// Replay the signed thinking block verbatim. Anthropic requires it
			// when continuing a tool-use turn; an unsigned reasoning string must
			// never be sent as a thinking block.
			if thinkingCfg != nil && m.ReasoningSignature != "" {
				blocks = append(blocks, anthropicBlock{
					Type: "thinking", Thinking: m.Reasoning, Signature: m.ReasoningSignature,
				})
			}
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

	body := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   messages,
	}
	if thinkingCfg != nil {
		body["thinking"] = thinkingCfg
		if effort := strings.ToLower(strings.TrimSpace(p.reasoningEffort)); allowEffort && effort != "" && effort != "none" {
			body["output_config"] = map[string]any{"effort": effort}
		}
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
	// Extended thinking samples deterministically; Anthropic rejects a custom
	// temperature on a thinking request.
	if req.Temperature > 0 && thinkingCfg == nil {
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
	signedBlocks := 0
	signedText, signedSig := "", ""
	for _, block := range out.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "thinking":
			result.Reasoning += block.Thinking
			if block.Signature != "" {
				signedBlocks++
				signedText, signedSig = block.Thinking, block.Signature
			}
		case "tool_use":
			input := block.Input
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Arguments: input})
		}
	}
	// A single signed thinking block can be replayed verbatim. With more than
	// one we cannot represent the text/signature pairing in one message, so drop
	// the signature rather than replaying a mismatched block.
	if signedBlocks == 1 {
		result.Reasoning = signedText
		result.ReasoningSignature = signedSig
	}
	return result, nil
}

// minThinkingTokens is the smallest output budget worth spending on extended
// thinking. Below it (internal compaction uses 700), thinking is skipped so the
// call still produces text.
const minThinkingTokens = 2048

// thinkingConfig mirrors CyberStrikeAI's applyClaudeExtendedThinking: thinking
// is on unless the mode is off. Adaptive models get {"type":"adaptive"}; older
// models use the documented {"type":"enabled","budget_tokens":...} shape. The
// second return value reports whether output_config.effort may be sent (the
// reference never maps effort for Sonnet 3.7).
func (p *Anthropic) thinkingConfig(model string, maxTokens int) (map[string]any, bool) {
	mode := strings.ToLower(strings.TrimSpace(p.reasoningMode))
	if mode == "" {
		mode = "auto"
	}
	effort := strings.ToLower(strings.TrimSpace(p.reasoningEffort))
	if mode == "off" || effort == "none" {
		return nil, false
	}
	// Auto only enables thinking on the native Anthropic wire and only for
	// models that support extended thinking. Anthropic-format gateways (DeepSeek
	// /anthropic and friends) have their own reasoning dials and may reject
	// Anthropic's thinking shape; they must opt in with mode=on. mode=on is also
	// the escape hatch for an older/unlisted model on the native wire.
	if mode == "auto" && (!p.anthropicWire() || !supportsClaudeThinking(model)) {
		return nil, false
	}
	if maxTokens > 0 && maxTokens < minThinkingTokens {
		return nil, false
	}
	m := strings.ToLower(model)
	sonnet37 := strings.Contains(m, "3-7-sonnet") || strings.Contains(m, "3.7") || strings.Contains(m, "sonnet-3-7")
	if !sonnet37 && isAdaptiveClaude(m) {
		return map[string]any{"type": "adaptive", "display": "summarized"}, true
	}
	budget := 10000
	if maxTokens > 0 && maxTokens <= budget {
		budget = maxTokens / 2
	}
	return map[string]any{"type": "enabled", "budget_tokens": budget, "display": "summarized"}, !sonnet37
}

// anthropicWire reports whether the channel targets Anthropic's native API or a
// proxy on an anthropic.com host. A path such as DeepSeek's /anthropic facade is
// not the native wire.
func (p *Anthropic) anthropicWire() bool {
	base := strings.TrimSpace(p.baseURL)
	if base == "" {
		return true
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		u, err = url.Parse("https://" + strings.TrimLeft(base, "/"))
		if err != nil {
			return false
		}
	}
	host := strings.ToLower(u.Hostname())
	return host == "anthropic.com" || strings.HasSuffix(host, ".anthropic.com")
}

// thinkingConfigForMessages disables thinking when the newest assistant turn
// contains tool calls but no signed thinking block (history persisted before
// thinking was enabled). Anthropic rejects a thinking request whose tool_use is
// not preceded by its thinking block.
func (p *Anthropic) thinkingConfigForMessages(req Request, model string, maxTokens int) (map[string]any, bool) {
	if req.DisableReasoning {
		return nil, false
	}
	cfg, allowEffort := p.thinkingConfig(model, maxTokens)
	if cfg == nil {
		return nil, false
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != "assistant" {
			continue
		}
		if len(req.Messages[i].ToolCalls) > 0 && req.Messages[i].ReasoningSignature == "" {
			return nil, false
		}
		break
	}
	return cfg, allowEffort
}

// supportsClaudeThinking reports whether the model family supports extended
// thinking (Claude 3.7 and newer). Unknown names stay off under auto; mode=on
// remains an explicit override.
func supportsClaudeThinking(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "3-7") || strings.Contains(m, "3.7") {
		return true
	}
	if strings.Contains(m, "claude-2") || strings.Contains(m, "claude-3-") || strings.Contains(m, "claude-3.") {
		return false
	}
	for _, marker := range []string{"claude-4", "claude-5", "sonnet-4", "sonnet-5", "opus-4", "opus-5", "haiku-4", "haiku-5"} {
		if strings.Contains(m, marker) {
			return true
		}
	}
	return false
}

// isAdaptiveClaude lists the models CyberStrikeAI treats as adaptive-capable.
// Older Claude 3.x/4.0-4.5 (including dated IDs such as
// claude-sonnet-4-5-20250929) stay on the enabled+budget shape.
func isAdaptiveClaude(model string) bool {
	for _, marker := range []string{"4-6", "4.6", "4-7", "4.7", "opus-5", "sonnet-5", "claude-5"} {
		if strings.Contains(model, marker) {
			return true
		}
	}
	return false
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
