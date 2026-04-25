// Package config handles parsing and validation of Takumi configuration files.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// WorkspaceConfig represents the top-level takumi.yaml file.
type WorkspaceConfig struct {
	Workspace Workspace `yaml:"workspace"`
}

// Workspace defines workspace-level settings and metadata.
type Workspace struct {
	Name       string            `yaml:"name"`
	Ignore     []string          `yaml:"ignore,omitempty"`
	Sources    map[string]Source `yaml:"sources,omitempty"`
	VersionSet VersionSetRef     `yaml:"version-set,omitempty"`
	Settings   WorkspaceSettings `yaml:"settings"`
	AI         WorkspaceAIRef    `yaml:"ai,omitempty"`
}

// Source tracks a git repository cloned into the workspace.
type Source struct {
	URL    string `yaml:"url"`
	Branch string `yaml:"branch"`
	Path   string `yaml:"path"`
}

// VersionSetRef points to the version-set file.
type VersionSetRef struct {
	File string `yaml:"file,omitempty"`
}

// WorkspaceSettings holds workspace-level build settings.
type WorkspaceSettings struct {
	Parallel bool `yaml:"parallel"`
}

// WorkspaceAIRef points to the AI instructions file and agent config.
type WorkspaceAIRef struct {
	Instructions string `yaml:"instructions,omitempty"`
	Agent        string `yaml:"agent,omitempty"`
}

// LoadWorkspaceConfig reads and parses a takumi.yaml file.
func LoadWorkspaceConfig(path string) (*WorkspaceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workspace config: %w", err)
	}

	var cfg WorkspaceConfig
	if err := decodeStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing workspace config: %w", err)
	}

	return &cfg, nil
}

// DefaultWorkspaceConfig returns a minimal workspace config for takumi init.
func DefaultWorkspaceConfig(name string) *WorkspaceConfig {
	return &WorkspaceConfig{
		Workspace: Workspace{
			Name:    name,
			Ignore:  []string{"vendor/", "node_modules/", ".git/"},
			Sources: make(map[string]Source),
			Settings: WorkspaceSettings{
				Parallel: true,
			},
		},
	}
}

// Marshal serializes the workspace config to YAML bytes.
func (c *WorkspaceConfig) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

// SaveWorkspaceConfig writes the workspace config to the given path.
func SaveWorkspaceConfig(path string, cfg *WorkspaceConfig) error {
	data, err := cfg.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling workspace config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing workspace config: %w", err)
	}
	return nil
}
