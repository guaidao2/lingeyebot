// Package llm defines the provider contract and protocol adapters.
package llm

import (
	"context"
	"encoding/json"
)

// DefaultMaxTokens is the fallback per-reply output cap when no provider cap is set.
const DefaultMaxTokens = 32768

// Message is one provider-neutral conversation message.
type Message struct {
	Role      string `json:"role"` // system|user|assistant|tool
	Content   string `json:"content,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	// ReasoningSignature is the provider's opaque signature for the reasoning
	// block (Anthropic thinking/signature). It must be replayed verbatim when
	// continuing a tool-use turn; it is never shown to the model as text.
	ReasoningSignature string     `json:"reasoningSignature,omitempty"`
	ToolCalls          []ToolCall `json:"toolCalls,omitempty"`
	ToolCallID         string     `json:"toolCallId,omitempty"`
	Name               string     `json:"name,omitempty"`
}

// ToolCall is one model-requested tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolSpec describes a tool to the model.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request is one model call.
type Request struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolSpec
	MaxTokens   int
	Temperature float64
	// DisableReasoning forces this call to run without thinking. Internal
	// bookkeeping calls (compaction/summarization) use it so a thinking model
	// does not spend the whole output budget on reasoning and return no text.
	DisableReasoning bool
}

// Usage reports token consumption.
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// Response is one model reply.
type Response struct {
	Content            string
	Reasoning          string
	ReasoningSignature string
	ToolCalls          []ToolCall
	Usage              Usage
	StopReason         string
}

// Provider is one LLM backend.
type Provider interface {
	ID() string
	Chat(ctx context.Context, req Request) (Response, error)
}

// StreamProvider is an optional token-level streaming backend.
type StreamProvider interface {
	Provider
	ChatStream(ctx context.Context, req Request, onDelta func(string)) (Response, error)
}

// ReasoningStreamProvider is an optional streaming backend that reports
// reasoning_content/thinking deltas separately from visible content.
type ReasoningStreamProvider interface {
	Provider
	ChatStreamWithReasoning(ctx context.Context, req Request, onContent, onReasoning func(string)) (Response, error)
}

// Config configures a protocol adapter.
type Config struct {
	ID               string
	Kind             string
	BaseURL          string
	APIKey           string
	Model            string
	MaxTokens        int
	MaxTokensField   string
	ContextWindow    int
	ReasoningMode    string
	ReasoningEffort  string
	ReasoningProfile string
	Temperature      float64
}
