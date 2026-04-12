package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PackageConfig represents a takumi-pkg.yaml file.
type PackageConfig struct {
	Package      PackageMeta       `yaml:"package"`
	Dependencies []string          `yaml:"dependencies,omitempty"`
	Runtime      *Runtime          `yaml:"runtime,omitempty"`
	Phases       map[string]*Phase `yaml:"phases,omitempty"`
	AI           *PackageAI        `yaml:"ai,omitempty"`
}

// PackageMeta holds basic package identity.
type PackageMeta struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// Runtime defines optional per-package environment isolation.
type Runtime struct {
	Setup []string          `yaml:"setup,omitempty"`
	Env   map[string]string `yaml:"env,omitempty"`
}

// Phase defines the commands for a build phase.
type Phase struct {
	Pre      []string `yaml:"pre,omitempty"`
	Commands []string `yaml:"commands"`
	Post     []string `yaml:"post,omitempty"`
}

// PackageAI holds AI-related metadata for a package.
type PackageAI struct {
	Description string            `yaml:"description,omitempty"`
	Notes       []string          `yaml:"notes,omitempty"`
	Tasks       map[string]AITask `yaml:"tasks,omitempty"`
}

// AITask is a named recipe an AI assistant can follow.
type AITask struct {
	Description string   `yaml:"description"`
	Steps       []string `yaml:"steps"`
}

// LoadPackageConfig reads and parses a takumi-pkg.yaml file.
func LoadPackageConfig(path string) (*PackageConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading package config: %w", err)
	}

	var cfg PackageConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing package config: %w", err)
	}

	return &cfg, nil
}

// DefaultPackageConfig returns a minimal package config for takumi init.
func DefaultPackageConfig(name string) *PackageConfig {
	return &PackageConfig{
		Package: PackageMeta{
			Name:    name,
			Version: "0.1.0",
		},
		Phases: map[string]*Phase{
			"build": {
				Commands: []string{"echo 'TODO: add build commands'"},
			},
			"test": {
				Commands: []string{"echo 'TODO: add test commands'"},
			},
		},
	}
}

// Marshal serializes the package config to YAML bytes.
func (c *PackageConfig) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}
