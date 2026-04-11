// Package skills loads, renders, and manages AI skill templates.
package skills

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

// Skill represents a loaded AI skill definition.
type Skill struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	AutoContext  []string `yaml:"auto_context,omitempty"`
	Prompt       string   `yaml:"prompt"`
	OutputFormat string   `yaml:"output_format,omitempty"`
	MaxTokens    int      `yaml:"max_tokens,omitempty"`
}

// SkillFile wraps the YAML structure.
type SkillFile struct {
	Skill Skill `yaml:"skill"`
}

// Source indicates where a skill was loaded from.
type Source int

const (
	SourceBuiltin   Source = iota // shipped with binary
	SourceWorkspace               // from takumi-ai.yaml
	SourcePackage                 // from takumi-pkg.yaml
)

// LoadedSkill is a skill with its origin.
type LoadedSkill struct {
	Skill
	Source Source
	Path   string // file path (empty for embedded)
}

// LoadBuiltins returns all built-in skills embedded in the binary.
func LoadBuiltins() ([]LoadedSkill, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, fmt.Errorf("reading embedded skills: %w", err)
	}

	var skills []LoadedSkill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := builtinFS.ReadFile("builtin/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading embedded skill %s: %w", entry.Name(), err)
		}

		var sf SkillFile
		if err := yaml.Unmarshal(data, &sf); err != nil {
			return nil, fmt.Errorf("parsing embedded skill %s: %w", entry.Name(), err)
		}

		skills = append(skills, LoadedSkill{
			Skill:  sf.Skill,
			Source: SourceBuiltin,
		})
	}
	return skills, nil
}

// LoadFromDir loads all .yaml skill files from a directory.
func LoadFromDir(dir string, source Source) ([]LoadedSkill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []LoadedSkill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var sf SkillFile
		if err := yaml.Unmarshal(data, &sf); err != nil {
			continue
		}
		if sf.Skill.Name == "" {
			continue
		}

		skills = append(skills, LoadedSkill{
			Skill:  sf.Skill,
			Source: source,
			Path:   path,
		})
	}
	return skills, nil
}

// Render substitutes {{variables}} in the skill prompt with the given values.
func Render(prompt string, vars map[string]string) string {
	result := prompt
	for key, val := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", val)
	}
	return result
}
