package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type anthropicProvider struct {
	config   *ProviderConfig
	agentCfg *Config
}

func (p *anthropicProvider) initialMessages(userMessage string) []any {
	return []any{
		map[string]any{"role": "user", "content": userMessage},
	}
}

func (p *anthropicProvider) chat(_ context.Context, messages []any) (*chatResponse, error) {
	tools := p.toolDefs()

	reqBody := map[string]any{
		"model":      p.config.Model,
		"max_tokens": p.agentCfg.MaxTokens,
		"system":     p.agentCfg.SystemPrompt,
		"tools":      tools,
		"messages":   messages,
	}

	respBody, err := httpPostJSON("https://api.anthropic.com/v1/messages", map[string]string{
		"x-api-key":         p.config.APIKey,
		"anthropic-version": "2023-06-01",
	}, reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Content    []json.RawMessage `json:"content"`
		StopReason string            `json:"stop_reason"`
		Error      *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s: %s", resp.Error.Type, resp.Error.Message)
	}

	// Parse content blocks into tool calls and text
	var calls []toolCall
	var textParts []string

	for _, raw := range resp.Content {
		var block struct {
			Type  string         `json:"type"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
			Text  string         `json:"text"`
		}
		if err := json.Unmarshal(raw, &block); err != nil {
			continue
		}
		switch block.Type {
		case "tool_use":
			calls = append(calls, toolCall{
				id:    block.ID,
				name:  block.Name,
				input: block.Input,
			})
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		}
	}

	return &chatResponse{
		toolCalls:        calls,
		text:             strings.Join(textParts, ""),
		done:             resp.StopReason == "end_turn",
		assistantMessage: map[string]any{"role": "assistant", "content": resp.Content},
	}, nil
}

func (p *anthropicProvider) formatToolResults(results []toolResult) []any {
	var blocks []map[string]any
	for _, r := range results {
		block := map[string]any{
			"type":        "tool_result",
			"tool_use_id": r.toolCallID,
			"content":     r.content,
		}
		if r.isError {
			block["is_error"] = true
		}
		blocks = append(blocks, block)
	}
	return []any{
		map[string]any{"role": "user", "content": blocks},
	}
}

func (p *anthropicProvider) toolDefs() []map[string]any {
	var defs []map[string]any
	for _, t := range p.agentCfg.Tools {
		defs = append(defs, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.Parameters,
		})
	}
	return defs
}
