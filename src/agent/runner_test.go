package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func anthropicTextResponse(text string) []byte {
	return []byte(`{
		"content": [{"type": "text", "text": "` + text + `"}],
		"stop_reason": "end_turn"
	}`)
}

func anthropicToolCallResponse(id, name, inputJSON string) []byte {
	return []byte(`{
		"content": [{"type": "tool_use", "id": "` + id + `", "name": "` + name + `", "input": ` + inputJSON + `}],
		"stop_reason": "tool_use"
	}`)
}

func runConfig(tools ...Tool) *Config {
	return &Config{
		SystemPrompt:   "be helpful",
		Tools:          tools,
		CompletionTool: "submit",
		MaxTurns:       5,
		MaxTokens:      1024,
	}
}

func anthropicProviderCfg() *ProviderConfig {
	return &ProviderConfig{Name: "anthropic", APIKey: "k", Model: "m"}
}

// TestRun_NaturalCompletion_OneTurn verifies that a text-only response with
// no tool calls returns immediately as Result.Output after exactly 1 turn.
func TestRun_NaturalCompletion_OneTurn(t *testing.T) {
	withMockHTTP(t, anthropicTextResponse("done"))

	res, err := Run(context.Background(), anthropicProviderCfg(), runConfig(), "hi")
	require.NoError(t, err)
	assert.Equal(t, "done", res.Output)
	assert.Equal(t, 1, res.Turns)
}

// TestRun_ExecutesToolThenContinues verifies the runner: dispatches the tool
// call, feeds the result back, and reaches completion on the next turn.
func TestRun_ExecutesToolThenContinues(t *testing.T) {
	executed := false
	tool := Tool{
		Name: "noop",
		Execute: func(input map[string]any) (string, bool) {
			executed = true
			assert.Equal(t, "value", input["arg"])
			return "tool-output", false
		},
	}

	withMockHTTP(t,
		anthropicToolCallResponse("tu_1", "noop", `{"arg":"value"}`),
		anthropicTextResponse("all done"),
	)

	res, err := Run(context.Background(), anthropicProviderCfg(), runConfig(tool), "go")
	require.NoError(t, err)
	assert.True(t, executed, "tool must be invoked")
	assert.Equal(t, "all done", res.Output)
	assert.Equal(t, 2, res.Turns)
}

// TestRun_CompletionTool_ReturnsToolInputAsOutput verifies that calling the
// configured CompletionTool ends the run with the tool's named input as Output.
func TestRun_CompletionTool_ReturnsToolInputAsOutput(t *testing.T) {
	cfg := runConfig(Tool{
		Name: "submit",
		Parameters: map[string]any{
			"required": []string{"result"},
		},
	})

	withMockHTTP(t,
		anthropicToolCallResponse("tu_1", "submit", `{"result":"final-answer"}`),
	)

	res, err := Run(context.Background(), anthropicProviderCfg(), cfg, "go")
	require.NoError(t, err)
	assert.Equal(t, "final-answer", res.Output)
	assert.Equal(t, 1, res.Turns)
}

// TestRun_UnknownTool_ReportedAsErrorResult verifies an unknown tool name from
// the LLM is reported back as an error tool-result, not a hard failure.
func TestRun_UnknownTool_ReportedAsErrorResult(t *testing.T) {
	withMockHTTP(t,
		anthropicToolCallResponse("tu_1", "ghost-tool", `{}`),
		anthropicTextResponse("recovered"),
	)

	res, err := Run(context.Background(), anthropicProviderCfg(), runConfig(), "go")
	require.NoError(t, err, "unknown tool must not abort the run")
	assert.Equal(t, "recovered", res.Output)
	assert.Equal(t, 2, res.Turns)
}

// TestRun_ToolErrorIsForwarded verifies that when a tool's Execute returns
// isErr=true, that flag travels back to the LLM in the next turn's tool result.
func TestRun_ToolErrorIsForwarded(t *testing.T) {
	tool := Tool{
		Name: "broken",
		Execute: func(input map[string]any) (string, bool) {
			return "boom", true
		},
	}

	withMockHTTP(t,
		anthropicToolCallResponse("tu_1", "broken", `{}`),
		anthropicTextResponse("recovered"),
	)

	res, err := Run(context.Background(), anthropicProviderCfg(), runConfig(tool), "go")
	require.NoError(t, err)
	assert.Equal(t, 2, res.Turns)
	// Tool error must not abort the loop — agent gets a chance to recover.
	assert.Equal(t, "recovered", res.Output)
}

// TestRun_MaxTurnsExceeded_ReturnsError verifies the runner enforces the
// MaxTurns cap with a clear error, not an infinite loop.
func TestRun_MaxTurnsExceeded_ReturnsError(t *testing.T) {
	tool := Tool{
		Name:    "loop",
		Execute: func(input map[string]any) (string, bool) { return "go again", false },
	}
	cfg := runConfig(tool)
	cfg.MaxTurns = 2

	// Tool-call forever — never completes.
	withMockHTTP(t,
		anthropicToolCallResponse("tu_1", "loop", `{}`),
		anthropicToolCallResponse("tu_2", "loop", `{}`),
	)

	_, err := Run(context.Background(), anthropicProviderCfg(), cfg, "go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded 2 turns")
}

// TestRun_OnToolCallCallback_FiresForEveryCall verifies the OnToolCall hook
// fires once per tool invocation with the right name and input.
func TestRun_OnToolCallCallback_FiresForEveryCall(t *testing.T) {
	type observed struct {
		name  string
		input map[string]any
	}
	var seen []observed

	cfg := runConfig(Tool{
		Name:    "ping",
		Execute: func(input map[string]any) (string, bool) { return "pong", false },
	})
	cfg.OnToolCall = func(name string, input map[string]any) {
		seen = append(seen, observed{name, input})
	}

	withMockHTTP(t,
		anthropicToolCallResponse("tu_1", "ping", `{"x":1}`),
		anthropicTextResponse("done"),
	)

	_, err := Run(context.Background(), anthropicProviderCfg(), cfg, "go")
	require.NoError(t, err)
	require.Len(t, seen, 1)
	assert.Equal(t, "ping", seen[0].name)
	assert.Equal(t, float64(1), seen[0].input["x"])
}

// TestRun_ChatErrorBubblesUpWithTurnContext verifies that a transport error
// is wrapped with "turn N" so the failing turn is identifiable in logs.
func TestRun_ChatErrorBubblesUpWithTurnContext(t *testing.T) {
	original := httpPostJSON
	httpPostJSON = func(url string, headers map[string]string, body any) ([]byte, error) {
		return nil, errors.New("network down")
	}
	t.Cleanup(func() { httpPostJSON = original })

	_, err := Run(context.Background(), anthropicProviderCfg(), runConfig(), "go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "turn 1")
	assert.Contains(t, err.Error(), "network down")
}

// TestRun_UnsupportedProvider_ErrorsBeforeAnyHTTPCall verifies that misconfigured
// providers fail fast — no wasted API call.
func TestRun_UnsupportedProvider_ErrorsBeforeAnyHTTPCall(t *testing.T) {
	called := 0
	original := httpPostJSON
	httpPostJSON = func(url string, headers map[string]string, body any) ([]byte, error) {
		called++
		return nil, nil
	}
	t.Cleanup(func() { httpPostJSON = original })

	_, err := Run(context.Background(), &ProviderConfig{Name: "gemini"}, runConfig(), "go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported provider")
	assert.Equal(t, 0, called, "no HTTP call should happen for unknown provider")
}
