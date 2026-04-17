package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// DetectProvider
// ---------------------------------------------------------------------------

func TestDetectProvider_ExplicitAnthropic(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	p, err := DetectProvider("anthropic", "")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", p.Name)
	assert.Equal(t, "test-key", p.APIKey)
	assert.Equal(t, "claude-sonnet-4-5-20250514", p.Model)
}

func TestDetectProvider_ExplicitOpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	p, err := DetectProvider("openai", "")
	require.NoError(t, err)
	assert.Equal(t, "openai", p.Name)
	assert.Equal(t, "gpt-4o", p.Model)
}

func TestDetectProvider_ModelOverride(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "key")
	p, err := DetectProvider("anthropic", "claude-haiku-4-5-20251001")
	require.NoError(t, err)
	assert.Equal(t, "claude-haiku-4-5-20251001", p.Model)
}

func TestDetectProvider_AutoDetectAnthropic(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "auto-key")
	os.Unsetenv("OPENAI_API_KEY")
	p, err := DetectProvider("", "")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", p.Name)
}

func TestDetectProvider_AutoDetectOpenAI(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Setenv("OPENAI_API_KEY", "auto-key")
	p, err := DetectProvider("", "")
	require.NoError(t, err)
	assert.Equal(t, "openai", p.Name)
}

func TestDetectProvider_NoKeys(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	_, err := DetectProvider("", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no LLM API key found")
}

func TestDetectProvider_ExplicitMissingKey(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	_, err := DetectProvider("anthropic", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY not set")
}

func TestDetectProvider_UnknownProvider(t *testing.T) {
	_, err := DetectProvider("gemini", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

// ---------------------------------------------------------------------------
// newProvider
// ---------------------------------------------------------------------------

func TestNewProvider_Anthropic(t *testing.T) {
	pc := &ProviderConfig{Name: "anthropic", APIKey: "k", Model: "m"}
	cfg := &Config{MaxTokens: 1024}
	p, err := newProvider(pc, cfg)
	require.NoError(t, err)
	_, ok := p.(*anthropicProvider)
	assert.True(t, ok)
}

func TestNewProvider_OpenAI(t *testing.T) {
	pc := &ProviderConfig{Name: "openai", APIKey: "k", Model: "m"}
	cfg := &Config{MaxTokens: 1024}
	p, err := newProvider(pc, cfg)
	require.NoError(t, err)
	_, ok := p.(*openaiProvider)
	assert.True(t, ok)
}

func TestNewProvider_Unknown(t *testing.T) {
	pc := &ProviderConfig{Name: "unknown"}
	_, err := newProvider(pc, &Config{})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// completionInputKey
// ---------------------------------------------------------------------------

func TestCompletionInputKey_StringSlice(t *testing.T) {
	cfg := &Config{
		CompletionTool: "done",
		Tools: []Tool{
			{
				Name: "done",
				Parameters: map[string]any{
					"required": []string{"result"},
				},
			},
		},
	}
	assert.Equal(t, "result", completionInputKey(cfg))
}

func TestCompletionInputKey_AnySlice(t *testing.T) {
	cfg := &Config{
		CompletionTool: "done",
		Tools: []Tool{
			{
				Name: "done",
				Parameters: map[string]any{
					"required": []any{"output"},
				},
			},
		},
	}
	assert.Equal(t, "output", completionInputKey(cfg))
}

func TestCompletionInputKey_Default(t *testing.T) {
	cfg := &Config{CompletionTool: "done", Tools: []Tool{{Name: "done"}}}
	assert.Equal(t, "output", completionInputKey(cfg))
}

// ---------------------------------------------------------------------------
// Provider message formatting
// ---------------------------------------------------------------------------

func TestAnthropicInitialMessages(t *testing.T) {
	p := &anthropicProvider{agentCfg: &Config{}}
	msgs := p.initialMessages("hello")
	require.Len(t, msgs, 1)
	m := msgs[0].(map[string]any)
	assert.Equal(t, "user", m["role"])
	assert.Equal(t, "hello", m["content"])
}

func TestOpenAIInitialMessages(t *testing.T) {
	p := &openaiProvider{agentCfg: &Config{SystemPrompt: "you are a bot"}}
	msgs := p.initialMessages("hello")
	require.Len(t, msgs, 2)
	sys := msgs[0].(map[string]any)
	assert.Equal(t, "system", sys["role"])
	assert.Equal(t, "you are a bot", sys["content"])
	user := msgs[1].(map[string]any)
	assert.Equal(t, "user", user["role"])
}

func TestAnthropicFormatToolResults(t *testing.T) {
	p := &anthropicProvider{agentCfg: &Config{}}
	results := []toolResult{
		{toolCallID: "tc1", content: "ok"},
		{toolCallID: "tc2", content: "fail", isError: true},
	}
	msgs := p.formatToolResults(results)
	require.Len(t, msgs, 1) // Anthropic bundles all results into one user message
	m := msgs[0].(map[string]any)
	assert.Equal(t, "user", m["role"])
	blocks := m["content"].([]map[string]any)
	require.Len(t, blocks, 2)
	assert.Equal(t, "tool_result", blocks[0]["type"])
	assert.Equal(t, "tc1", blocks[0]["tool_use_id"])
	assert.Nil(t, blocks[0]["is_error"])
	assert.Equal(t, true, blocks[1]["is_error"])
}

func TestOpenAIFormatToolResults(t *testing.T) {
	p := &openaiProvider{agentCfg: &Config{}}
	results := []toolResult{
		{toolCallID: "tc1", content: "ok"},
		{toolCallID: "tc2", content: "fail", isError: true},
	}
	msgs := p.formatToolResults(results)
	require.Len(t, msgs, 2) // OpenAI uses separate messages per tool result
	m1 := msgs[0].(map[string]any)
	assert.Equal(t, "tool", m1["role"])
	assert.Equal(t, "tc1", m1["tool_call_id"])
}

// ---------------------------------------------------------------------------
// Tool defs
// ---------------------------------------------------------------------------

func TestAnthropicToolDefs(t *testing.T) {
	p := &anthropicProvider{agentCfg: &Config{
		Tools: []Tool{
			{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	}}
	defs := p.toolDefs()
	require.Len(t, defs, 1)
	assert.Equal(t, "read_file", defs[0]["name"])
	assert.Equal(t, "Read a file", defs[0]["description"])
	assert.Equal(t, map[string]any{"type": "object"}, defs[0]["input_schema"])
}

func TestOpenAIToolDefs(t *testing.T) {
	p := &openaiProvider{agentCfg: &Config{
		Tools: []Tool{
			{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	}}
	defs := p.toolDefs()
	require.Len(t, defs, 1)
	assert.Equal(t, "function", defs[0]["type"])
	fn := defs[0]["function"].(map[string]any)
	assert.Equal(t, "read_file", fn["name"])
	assert.Equal(t, map[string]any{"type": "object"}, fn["parameters"])
}
