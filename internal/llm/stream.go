package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// ChatStream performs an OpenAI-compatible streaming call. Content deltas are
// delivered through onDelta; reasoning deltas are available via
// ChatStreamWithReasoning.
func (p *OpenAI) ChatStream(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
	return p.chatStream(ctx, req, onDelta, nil)
}

// ChatStreamWithReasoning is the streaming variant that also reports
// reasoning_content/reasoning deltas while the model is thinking.
func (p *OpenAI) ChatStreamWithReasoning(ctx context.Context, req Request, onContent, onReasoning func(string)) (Response, error) {
	return p.chatStream(ctx, req, onContent, onReasoning)
}

func (p *OpenAI) chatStream(ctx context.Context, req Request, onDelta, onReasoning func(string)) (Response, error) {
	streamModel := firstNonEmpty(req.Model, p.model)
	deepseekWire := p.usesDeepSeekWire(streamModel)
	messages := make([]openAIMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		om := openAIMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID, Name: m.Name}
		if m.Role == "assistant" {
			if m.Reasoning != "" {
				reasoning := m.Reasoning
				om.ReasoningContent = &reasoning
			} else if deepseekWire {
				empty := ""
				om.ReasoningContent = &empty
			}
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
	body := map[string]any{"model": streamModel, "messages": messages, "stream": true}
	p.applyReasoning(body, streamModel, req.DisableReasoning)
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
	body[openAITokenKey(firstNonEmpty(req.Model, p.model), p.maxTokensField)] = maxTokens
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
		httpReq.Header.Set("Accept", "text/event-stream")
		if p.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		return httpReq, nil
	})
	if err != nil {
		return Response{}, fmt.Errorf("openai stream request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Response{}, fmt.Errorf("openai %s: %s", resp.Status, truncate(string(raw), 2000))
	}

	var content strings.Builder
	toolCalls := map[int]*ToolCall{}
	result := Response{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          *string `json:"content"`
					ReasoningContent *string `json:"reasoning_content"`
					Reasoning        *string `json:"reasoning"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		result.Usage = Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}
		for _, choice := range chunk.Choices {
			if choice.Delta.ReasoningContent != nil {
				result.Reasoning += *choice.Delta.ReasoningContent
				if onReasoning != nil && *choice.Delta.ReasoningContent != "" {
					onReasoning(*choice.Delta.ReasoningContent)
				}
			}
			if choice.Delta.Reasoning != nil {
				result.Reasoning += *choice.Delta.Reasoning
				if onReasoning != nil && *choice.Delta.Reasoning != "" {
					onReasoning(*choice.Delta.Reasoning)
				}
			}
			if choice.Delta.Content != nil && *choice.Delta.Content != "" {
				content.WriteString(*choice.Delta.Content)
				if onDelta != nil {
					onDelta(*choice.Delta.Content)
				}
			}
			for _, tc := range choice.Delta.ToolCalls {
				call := toolCalls[tc.Index]
				if call == nil {
					call = &ToolCall{}
					toolCalls[tc.Index] = call
				}
				if tc.ID != "" {
					call.ID = tc.ID
				}
				if tc.Function.Name != "" {
					call.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					call.Arguments = append(call.Arguments, []byte(tc.Function.Arguments)...)
				}
			}
			if choice.FinishReason != "" {
				result.StopReason = choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	result.Content = content.String()
	indices := make([]int, 0, len(toolCalls))
	for index := range toolCalls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		call := toolCalls[index]
		if len(call.Arguments) == 0 {
			call.Arguments = json.RawMessage("{}")
		}
		result.ToolCalls = append(result.ToolCalls, *call)
	}
	return result, nil
}

// ChatStream performs an Anthropic Messages streaming call.
func (p *Anthropic) ChatStream(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
	return p.chatStream(ctx, req, onDelta, nil)
}

// ChatStreamWithReasoning reports thinking deltas separately from text.
func (p *Anthropic) ChatStreamWithReasoning(ctx context.Context, req Request, onContent, onReasoning func(string)) (Response, error) {
	return p.chatStream(ctx, req, onContent, onReasoning)
}

func (p *Anthropic) chatStream(ctx context.Context, req Request, onDelta, onReasoning func(string)) (Response, error) {
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
		"model": model, "max_tokens": maxTokens,
		"messages": messages, "stream": true,
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
			tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Parameters})
		}
		body["tools"] = tools
	}
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
		httpReq.Header.Set("Accept", "text/event-stream")
		httpReq.Header.Set("anthropic-version", anthropicVersion)
		if p.apiKey != "" {
			httpReq.Header.Set("x-api-key", p.apiKey)
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		return httpReq, nil
	})
	if err != nil {
		return Response{}, fmt.Errorf("anthropic stream request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Response{}, fmt.Errorf("anthropic %s: %s", resp.Status, truncate(string(raw), 2000))
	}

	type blockState struct {
		kind      string
		text      strings.Builder
		signature strings.Builder
		id        string
		name      string
		args      strings.Builder
	}
	blocks := map[int]*blockState{}
	result := Response{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var event struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				Name      string `json:"name"`
				Text      string `json:"text"`
				Signature string `json:"signature"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if event.Type == "error" {
			return result, fmt.Errorf("anthropic stream: %s", event.Error.Message)
		}
		if event.Type == "content_block_start" {
			state := &blockState{kind: event.ContentBlock.Type, id: event.ContentBlock.ID, name: event.ContentBlock.Name}
			state.text.WriteString(event.ContentBlock.Text)
			if event.ContentBlock.Signature != "" {
				state.signature.WriteString(event.ContentBlock.Signature)
			}
			blocks[event.Index] = state
			continue
		}
		if event.Type == "content_block_delta" {
			state := blocks[event.Index]
			if state == nil {
				state = &blockState{}
				blocks[event.Index] = state
			}
			switch event.Delta.Type {
			case "text_delta":
				state.kind = "text"
				state.text.WriteString(event.Delta.Text)
				if onDelta != nil && event.Delta.Text != "" {
					onDelta(event.Delta.Text)
				}
			case "thinking_delta":
				state.kind = "thinking"
				state.text.WriteString(event.Delta.Thinking)
				result.Reasoning += event.Delta.Thinking
				if onReasoning != nil && event.Delta.Thinking != "" {
					onReasoning(event.Delta.Thinking)
				}
			case "signature_delta":
				// Anthropic sends the opaque signature for the thinking block
				// after its text; it must be replayed verbatim on continuation.
				state.kind = "thinking"
				state.signature.WriteString(event.Delta.Signature)
			case "input_json_delta":
				state.kind = "tool_use"
				state.args.WriteString(event.Delta.PartialJSON)
			}
			continue
		}
		if event.Type == "message_delta" {
			result.StopReason = event.Delta.StopReason
			if event.Usage.OutputTokens > 0 {
				result.Usage.OutputTokens = event.Usage.OutputTokens
			}
			continue
		}
		if event.Type == "message_start" && event.Usage.InputTokens > 0 {
			result.Usage.InputTokens = event.Usage.InputTokens
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	indices := make([]int, 0, len(blocks))
	for index := range blocks {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	var text strings.Builder
	var signed *blockState
	signedBlocks := 0
	for _, index := range indices {
		state := blocks[index]
		switch state.kind {
		case "text":
			text.WriteString(state.text.String())
		case "thinking":
			if state.signature.Len() > 0 {
				signedBlocks++
				signed = state
			}
		case "tool_use":
			args := state.args.String()
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{ID: state.id, Name: state.name, Arguments: json.RawMessage(args)})
		}
	}
	// Replay only a single signed thinking block: with several we cannot
	// represent their text/signature pairing in one persisted message.
	if signedBlocks == 1 && signed != nil {
		result.Reasoning = signed.text.String()
		result.ReasoningSignature = signed.signature.String()
	}
	result.Content = text.String()
	return result, nil
}
