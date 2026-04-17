// Package agent provides a multi-turn LLM agent loop with tool calling.
// Supports Anthropic and OpenAI providers.
package agent

import (
	"fmt"
	"os"
)

// Config defines the behavior of an agent run.
type Config struct {
	SystemPrompt   string
	Tools          []Tool
	CompletionTool string // Tool name that signals the agent is done
	MaxTurns       int
	MaxTokens      int
	OnToolCall     func(name string, input map[string]any) // Optional progress callback
}

// ProviderConfig identifies which LLM provider and model to use.
type ProviderConfig struct {
	Name   string // "anthropic" or "openai"
	APIKey string
	Model  string
}

// Tool defines a tool the agent can call.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any                            // JSON Schema for input
	Execute     func(input map[string]any) (string, bool) // (output, isError)
}

// Result is returned when the agent completes.
type Result struct {
	Output string // Content from the completion tool
	Turns  int    // Number of API round-trips
}

// DetectProvider auto-detects the LLM provider from environment variables.
// providerName and modelOverride are optional overrides (pass "" to auto-detect).
func DetectProvider(providerName, modelOverride string) (*ProviderConfig, error) {
	if providerName != "" {
		switch providerName {
		case "anthropic":
			key := os.Getenv("ANTHROPIC_API_KEY")
			if key == "" {
				return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
			}
			model := modelOverride
			if model == "" {
				model = "claude-sonnet-4-5-20250514"
			}
			return &ProviderConfig{Name: "anthropic", APIKey: key, Model: model}, nil
		case "openai":
			key := os.Getenv("OPENAI_API_KEY")
			if key == "" {
				return nil, fmt.Errorf("OPENAI_API_KEY not set")
			}
			model := modelOverride
			if model == "" {
				model = "gpt-4o"
			}
			return &ProviderConfig{Name: "openai", APIKey: key, Model: model}, nil
		default:
			return nil, fmt.Errorf("unknown provider %q (supported: anthropic, openai)", providerName)
		}
	}

	// Auto-detect from env
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := modelOverride
		if model == "" {
			model = "claude-sonnet-4-5-20250514"
		}
		return &ProviderConfig{Name: "anthropic", APIKey: key, Model: model}, nil
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model := modelOverride
		if model == "" {
			model = "gpt-4o"
		}
		return &ProviderConfig{Name: "openai", APIKey: key, Model: model}, nil
	}

	return nil, fmt.Errorf("no LLM API key found. Set ANTHROPIC_API_KEY or OPENAI_API_KEY")
}
