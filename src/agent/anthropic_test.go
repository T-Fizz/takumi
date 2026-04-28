package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAnthropicForTest() *anthropicProvider {
	return &anthropicProvider{
		config:   &ProviderConfig{Name: "anthropic", APIKey: "k", Model: "claude-test"},
		agentCfg: &Config{MaxTokens: 1024, SystemPrompt: "be helpful"},
	}
}

// TestAnthropicChat_ParsesTextResponse verifies a text-only response is
// returned as resp.text with done=true (stop_reason=end_turn).
func TestAnthropicChat_ParsesTextResponse(t *testing.T) {
	withMockHTTP(t, []byte(`{
		"content": [{"type": "text", "text": "Hello there"}],
		"stop_reason": "end_turn"
	}`))

	resp, err := newAnthropicForTest().chat(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "Hello there", resp.text)
	assert.True(t, resp.done)
	assert.Empty(t, resp.toolCalls)
}

// TestAnthropicChat_ParsesToolUseResponse verifies tool_use blocks become
// toolCall entries with id, name, and input wired through.
func TestAnthropicChat_ParsesToolUseResponse(t *testing.T) {
	withMockHTTP(t, []byte(`{
		"content": [
			{"type": "text", "text": "calling now"},
			{"type": "tool_use", "id": "tu_1", "name": "read_file", "input": {"path": "/tmp/x"}}
		],
		"stop_reason": "tool_use"
	}`))

	resp, err := newAnthropicForTest().chat(context.Background(), nil)
	require.NoError(t, err)
	assert.False(t, resp.done, "stop_reason=tool_use must not mark done")
	require.Len(t, resp.toolCalls, 1)
	assert.Equal(t, "tu_1", resp.toolCalls[0].id)
	assert.Equal(t, "read_file", resp.toolCalls[0].name)
	assert.Equal(t, map[string]any{"path": "/tmp/x"}, resp.toolCalls[0].input)
	assert.Equal(t, "calling now", resp.text)
}

// TestAnthropicChat_APIError_SurfacesTypeAndMessage verifies the error JSON
// from the API is parsed and bubbled up with both type and message.
func TestAnthropicChat_APIError_SurfacesTypeAndMessage(t *testing.T) {
	withMockHTTP(t, []byte(`{
		"error": {"type": "authentication_error", "message": "invalid x-api-key"}
	}`))

	_, err := newAnthropicForTest().chat(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication_error")
	assert.Contains(t, err.Error(), "invalid x-api-key")
}

// TestAnthropicChat_MalformedJSON_ReturnsParseError verifies invalid JSON
// from the API is reported as a parse error, not silently treated as empty.
func TestAnthropicChat_MalformedJSON_ReturnsParseError(t *testing.T) {
	withMockHTTP(t, []byte(`not json at all`))

	_, err := newAnthropicForTest().chat(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing response")
}

// TestAnthropicChat_SendsModelMaxTokensSystemAndTools verifies the request
// body carries the model, max_tokens, system prompt, and tool schema.
func TestAnthropicChat_SendsModelMaxTokensSystemAndTools(t *testing.T) {
	calls := withMockHTTP(t, []byte(`{"content":[],"stop_reason":"end_turn"}`))

	p := newAnthropicForTest()
	p.agentCfg.Tools = []Tool{{
		Name:        "noop",
		Description: "do nothing",
		Parameters:  map[string]any{"type": "object"},
	}}

	_, err := p.chat(context.Background(), []any{
		map[string]any{"role": "user", "content": "hi"},
	})
	require.NoError(t, err)

	require.Len(t, *calls, 1)
	body := (*calls)[0].body.(map[string]any)
	assert.Equal(t, "claude-test", body["model"])
	assert.Equal(t, 1024, body["max_tokens"])
	assert.Equal(t, "be helpful", body["system"])
	tools := body["tools"].([]map[string]any)
	require.Len(t, tools, 1)
	assert.Equal(t, "noop", tools[0]["name"])
	assert.Equal(t, "do nothing", tools[0]["description"])
}

// TestAnthropicChat_SendsAuthHeaders verifies x-api-key and anthropic-version
// headers are set on every request.
func TestAnthropicChat_SendsAuthHeaders(t *testing.T) {
	calls := withMockHTTP(t, []byte(`{"content":[],"stop_reason":"end_turn"}`))

	_, err := newAnthropicForTest().chat(context.Background(), nil)
	require.NoError(t, err)

	require.Len(t, *calls, 1)
	assert.Equal(t, "k", (*calls)[0].headers["x-api-key"])
	assert.Equal(t, "2023-06-01", (*calls)[0].headers["anthropic-version"])
	assert.Equal(t, "https://api.anthropic.com/v1/messages", (*calls)[0].url)
}

// TestAnthropicChat_UnknownContentBlocks_AreSilentlySkipped verifies that
// unknown block types don't fail parsing — they're ignored so future
// API additions don't crash the runner.
func TestAnthropicChat_UnknownContentBlocks_AreSilentlySkipped(t *testing.T) {
	withMockHTTP(t, []byte(`{
		"content": [
			{"type": "text", "text": "hi"},
			{"type": "future_block_type", "data": "whatever"}
		],
		"stop_reason": "end_turn"
	}`))

	resp, err := newAnthropicForTest().chat(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "hi", resp.text)
	assert.Empty(t, resp.toolCalls)
}
