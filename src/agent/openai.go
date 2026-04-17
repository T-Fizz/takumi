package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

type openaiProvider struct {
	config   *ProviderConfig
	agentCfg *Config
}

func (p *openaiProvider) initialMessages(userMessage string) []any {
	return []any{
		map[string]any{"role": "system", "content": p.agentCfg.SystemPrompt},
		map[string]any{"role": "user", "content": userMessage},
	}
}

func (p *openaiProvider) chat(_ context.Context, messages []any) (*chatResponse, error) {
	reqBody := map[string]any{
		"model":      p.config.Model,
		"max_tokens": p.agentCfg.MaxTokens,
		"tools":      p.toolDefs(),
		"messages":   messages,
	}

	respBody, err := httpPostJSON("https://api.openai.com/v1/chat/completions", map[string]string{
		"Authorization": "Bearer " + p.config.APIKey,
	}, reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content   *string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("openai error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	choice := resp.Choices[0]

	// Parse tool calls
	var calls []toolCall
	for _, tc := range choice.Message.ToolCalls {
		var input map[string]any
		json.Unmarshal([]byte(tc.Function.Arguments), &input)
		calls = append(calls, toolCall{
			id:    tc.ID,
			name:  tc.Function.Name,
			input: input,
		})
	}

	// Build assistant message for history
	assistantMsg := map[string]any{"role": "assistant"}
	if choice.Message.Content != nil {
		assistantMsg["content"] = *choice.Message.Content
	}
	if len(choice.Message.ToolCalls) > 0 {
		assistantMsg["tool_calls"] = choice.Message.ToolCalls
	}

	var text string
	if choice.Message.Content != nil {
		text = *choice.Message.Content
	}

	return &chatResponse{
		toolCalls:        calls,
		text:             text,
		done:             len(calls) == 0,
		assistantMessage: assistantMsg,
	}, nil
}

func (p *openaiProvider) formatToolResults(results []toolResult) []any {
	var msgs []any
	for _, r := range results {
		msgs = append(msgs, map[string]any{
			"role":         "tool",
			"tool_call_id": r.toolCallID,
			"content":      r.content,
		})
	}
	return msgs
}

func (p *openaiProvider) toolDefs() []map[string]any {
	var defs []map[string]any
	for _, t := range p.agentCfg.Tools {
		defs = append(defs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}
	return defs
}
