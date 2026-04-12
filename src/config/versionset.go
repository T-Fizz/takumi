package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// VersionSetConfig represents the takumi-versions.yaml file.
type VersionSetConfig struct {
	VersionSet VersionSet `yaml:"version-set"`
}

// VersionSet defines centralized version pinning.
type VersionSet struct {
	Name     string            `yaml:"name"`
	Packages map[string]string `yaml:"packages"` // dependency → version
	Strategy string            `yaml:"strategy"` // strict | prefer-latest | prefer-pinned
}

// LoadVersionSetConfig reads and parses a takumi-versions.yaml file.
func LoadVersionSetConfig(path string) (*VersionSetConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading version-set config: %w", err)
	}

	var cfg VersionSetConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing version-set config: %w", err)
	}

	return &cfg, nil
}
