package agent

import (
	"context"
	"fmt"
)

// provider is the internal interface for LLM API implementations.
type provider interface {
	// chat sends the current message history and returns the response.
	chat(ctx context.Context, messages []any) (*chatResponse, error)
	// initialMessages builds the starting message list from the user's prompt.
	initialMessages(userMessage string) []any
	// formatToolResults builds provider-specific messages from tool execution results.
	formatToolResults(results []toolResult) []any
}

type chatResponse struct {
	toolCalls        []toolCall
	text             string
	done             bool
	assistantMessage any // raw message to append to conversation history
}

type toolCall struct {
	id    string
	name  string
	input map[string]any
}

type toolResult struct {
	toolCallID string
	content    string
	isError    bool
}

// Run executes a multi-turn agent loop. The agent sends the initial message
// to the LLM, then iterates: executing tool calls and feeding results back
// until the completion tool is called or max turns is reached.
func Run(ctx context.Context, pc *ProviderConfig, cfg *Config, userMessage string) (*Result, error) {
	p, err := newProvider(pc, cfg)
	if err != nil {
		return nil, err
	}

	messages := p.initialMessages(userMessage)
	toolMap := make(map[string]Tool, len(cfg.Tools))
	for _, t := range cfg.Tools {
		toolMap[t.Name] = t
	}

	for turn := 0; turn < cfg.MaxTurns; turn++ {
		resp, err := p.chat(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("turn %d: %w", turn+1, err)
		}

		messages = append(messages, resp.assistantMessage)

		// No tool calls — agent finished naturally
		if resp.done || len(resp.toolCalls) == 0 {
			return &Result{Output: resp.text, Turns: turn + 1}, nil
		}

		// Execute tool calls
		var results []toolResult
		var completionOutput string
		completed := false

		for _, tc := range resp.toolCalls {
			if cfg.OnToolCall != nil {
				cfg.OnToolCall(tc.name, tc.input)
			}

			// Check for completion tool
			if tc.name == cfg.CompletionTool {
				completionOutput = fmt.Sprintf("%v", tc.input[completionInputKey(cfg)])
				completed = true
				results = append(results, toolResult{
					toolCallID: tc.id,
					content:    "Submitted.",
				})
				continue
			}

			// Execute regular tool
			tool, ok := toolMap[tc.name]
			if !ok {
				results = append(results, toolResult{
					toolCallID: tc.id,
					content:    fmt.Sprintf("Unknown tool: %s", tc.name),
					isError:    true,
				})
				continue
			}

			output, isErr := tool.Execute(tc.input)
			results = append(results, toolResult{
				toolCallID: tc.id,
				content:    output,
				isError:    isErr,
			})
		}

		// Append tool results to conversation
		resultMsgs := p.formatToolResults(results)
		for _, m := range resultMsgs {
			messages = append(messages, m)
		}

		if completed {
			return &Result{Output: completionOutput, Turns: turn + 1}, nil
		}
	}

	return nil, fmt.Errorf("agent exceeded %d turns without completing", cfg.MaxTurns)
}

// completionInputKey returns the first required field of the completion tool,
// or "output" as a default. This is the field the runner reads as the result.
func completionInputKey(cfg *Config) string {
	for _, t := range cfg.Tools {
		if t.Name == cfg.CompletionTool {
			if req, ok := t.Parameters["required"].([]string); ok && len(req) > 0 {
				return req[0]
			}
			// Try []any (from JSON unmarshaling)
			if req, ok := t.Parameters["required"].([]any); ok && len(req) > 0 {
				return fmt.Sprintf("%v", req[0])
			}
		}
	}
	return "output"
}

func newProvider(pc *ProviderConfig, cfg *Config) (provider, error) {
	switch pc.Name {
	case "anthropic":
		return &anthropicProvider{config: pc, agentCfg: cfg}, nil
	case "openai":
		return &openaiProvider{config: pc, agentCfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pc.Name)
	}
}
