package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOpenAIForTest() *openaiProvider {
	return &openaiProvider{
		config:   &ProviderConfig{Name: "openai", APIKey: "k", Model: "gpt-test"},
		agentCfg: &Config{MaxTokens: 1024, SystemPrompt: "you are a bot"},
	}
}

// TestOpenAIChat_ParsesTextResponse verifies a text response with no tool calls
// becomes resp.text with done=true (no tool calls -> done).
func TestOpenAIChat_ParsesTextResponse(t *testing.T) {
	withMockHTTP(t, []byte(`{
		"choices": [{
			"message": {"content": "All done."},
			"finish_reason": "stop"
		}]
	}`))

	resp, err := newOpenAIForTest().chat(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "All done.", resp.text)
	assert.True(t, resp.done, "no tool calls means done")
	assert.Empty(t, resp.toolCalls)
}

// TestOpenAIChat_ParsesToolCalls verifies tool_calls in the message become
// toolCall entries with arguments JSON-decoded into the input map.
func TestOpenAIChat_ParsesToolCalls(t *testing.T) {
	withMockHTTP(t, []byte(`{
		"choices": [{
			"message": {
				"content": null,
				"tool_calls": [{
					"id": "call_42",
					"function": {"name": "read_file", "arguments": "{\"path\":\"/tmp/x\"}"}
				}]
			},
			"finish_reason": "tool_calls"
		}]
	}`))

	resp, err := newOpenAIForTest().chat(context.Background(), nil)
	require.NoError(t, err)
	assert.False(t, resp.done, "tool calls present must not mark done")
	require.Len(t, resp.toolCalls, 1)
	assert.Equal(t, "call_42", resp.toolCalls[0].id)
	assert.Equal(t, "read_file", resp.toolCalls[0].name)
	assert.Equal(t, map[string]any{"path": "/tmp/x"}, resp.toolCalls[0].input)
}

// TestOpenAIChat_APIError_SurfacesMessage verifies the error JSON from the API
// is parsed and bubbled up with the message.
func TestOpenAIChat_APIError_SurfacesMessage(t *testing.T) {
	withMockHTTP(t, []byte(`{"error": {"message": "rate limit exceeded"}}`))

	_, err := newOpenAIForTest().chat(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
}

// TestOpenAIChat_NoChoices_ReturnsError verifies an empty choices array
// is treated as an explicit error, not as a successful empty response.
func TestOpenAIChat_NoChoices_ReturnsError(t *testing.T) {
	withMockHTTP(t, []byte(`{"choices": []}`))

	_, err := newOpenAIForTest().chat(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

// TestOpenAIChat_MalformedJSON_ReturnsParseError verifies invalid JSON is
// reported as a parse error.
func TestOpenAIChat_MalformedJSON_ReturnsParseError(t *testing.T) {
	withMockHTTP(t, []byte(`not json`))

	_, err := newOpenAIForTest().chat(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing response")
}

// TestOpenAIChat_SendsBearerTokenAndModel verifies auth header and model.
func TestOpenAIChat_SendsBearerTokenAndModel(t *testing.T) {
	calls := withMockHTTP(t, []byte(`{
		"choices": [{"message": {"content": "x"}, "finish_reason": "stop"}]
	}`))

	_, err := newOpenAIForTest().chat(context.Background(), nil)
	require.NoError(t, err)

	require.Len(t, *calls, 1)
	assert.Equal(t, "Bearer k", (*calls)[0].headers["Authorization"])
	assert.Equal(t, "https://api.openai.com/v1/chat/completions", (*calls)[0].url)
	body := (*calls)[0].body.(map[string]any)
	assert.Equal(t, "gpt-test", body["model"])
}

// TestOpenAIChat_AssistantMessageRoundtrip verifies the assistant message
// captured for history includes content (when present) and tool_calls.
func TestOpenAIChat_AssistantMessageRoundtrip(t *testing.T) {
	withMockHTTP(t, []byte(`{
		"choices": [{
			"message": {
				"content": "thinking...",
				"tool_calls": [{
					"id": "c1",
					"function": {"name": "noop", "arguments": "{}"}
				}]
			},
			"finish_reason": "tool_calls"
		}]
	}`))

	resp, err := newOpenAIForTest().chat(context.Background(), nil)
	require.NoError(t, err)
	msg := resp.assistantMessage.(map[string]any)
	assert.Equal(t, "assistant", msg["role"])
	assert.Equal(t, "thinking...", msg["content"])
	assert.NotNil(t, msg["tool_calls"], "tool_calls must be carried in assistant message for history")
}
