package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/llm"
)

// estimateTokens is a cheap byte-based estimate (about four bytes per token).
func estimateTokens(messages []llm.Message) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content)
		for _, call := range message.ToolCalls {
			total += len(call.Name) + len(call.Arguments)
		}
	}
	return total/4 + len(messages)*4
}

// compactMessages summarizes the oldest messages when the conversation exceeds
// 80% of the configured window. It never splits a tool_call from its results.
func compactMessages(ctx context.Context, provider llm.Provider, model string, messages []llm.Message, window, keep int) ([]llm.Message, bool) {
	if window <= 0 {
		window = 1000000
	}
	if estimateTokens(messages) <= window*8/10 {
		return messages, false
	}
	if keep <= 0 || keep > len(messages) {
		keep = 16
	}
	if len(messages) <= keep {
		return messages, false
	}
	start := len(messages) - keep
	for start > 0 && messages[start].Role == "tool" {
		start--
	}
	head, tail := messages[:start], messages[start:]
	if len(head) == 0 {
		return messages, false
	}

	summary, err := summarize(ctx, provider, model, head)
	if err != nil || strings.TrimSpace(summary) == "" {
		summary = fmt.Sprintf("较早的 %d 条消息因上下文预算被截断；需要细节时请重新查询证据或黑板。", len(head))
	}
	out := make([]llm.Message, 0, len(tail)+1)
	out = append(out, llm.Message{Role: "user", Content: "[较早历史摘要]\n" + summary})
	out = append(out, tail...)
	return out, true
}

func summarize(ctx context.Context, provider llm.Provider, model string, messages []llm.Message) (string, error) {
	prompt := []llm.Message{
		{Role: "user", Content: "Summarize the following conversation for continuity. Keep: decisions, confirmed facts (with evidence IDs when present), open questions, failed paths, and unfinished work. Use terse bullet points. Do not invent anything.\n\n" + renderForSummary(messages)},
	}
	response, err := provider.Chat(ctx, llm.Request{
		Model: model, System: "You compact a security agent's working memory. Be accurate and terse.",
		Messages: prompt, MaxTokens: 700,
	})
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

func renderForSummary(messages []llm.Message) string {
	var b strings.Builder
	for _, message := range messages {
		role := message.Role
		content := message.Content
		if len(content) > 4000 {
			content = content[:4000] + "…"
		}
		fmt.Fprintf(&b, "%s: %s\n", role, content)
		for _, call := range message.ToolCalls {
			fmt.Fprintf(&b, "  tool_call %s(%s)\n", call.Name, Short(string(call.Arguments), 300))
		}
	}
	return b.String()
}
