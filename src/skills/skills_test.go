package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBuiltins(t *testing.T) {
	skills, err := LoadBuiltins()
	require.NoError(t, err)
	assert.NotEmpty(t, skills)

	// Verify known built-in skills
	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
		assert.Equal(t, SourceBuiltin, s.Source)
		assert.Empty(t, s.Path, "embedded skills should have empty path")
		assert.NotEmpty(t, s.Description)
		assert.NotEmpty(t, s.Prompt)
	}

	for _, expected := range []string{"operator", "diagnose", "review", "optimize", "onboard", "doc-writer"} {
		assert.True(t, names[expected], "should include built-in skill %q", expected)
	}
}

func TestLoadBuiltins_SkillFields(t *testing.T) {
	skills, err := LoadBuiltins()
	require.NoError(t, err)

	for _, s := range skills {
		if s.Name == "diagnose" {
			assert.NotEmpty(t, s.AutoContext)
			assert.Greater(t, s.MaxTokens, 0)
			assert.Contains(t, s.Prompt, "{{package_name}}")
			return
		}
	}
	t.Fatal("diagnose skill not found in builtins")
}

func TestLoadFromDir_ValidSkills(t *testing.T) {
	dir := t.TempDir()

	skillYAML := `skill:
  name: custom-lint
  description: "Run custom linting"
  prompt: |
    Lint the code in {{package_name}}.
  max_tokens: 200
`
	os.WriteFile(filepath.Join(dir, "lint.yaml"), []byte(skillYAML), 0644)

	skills, err := LoadFromDir(dir, SourceWorkspace)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "custom-lint", skills[0].Name)
	assert.Equal(t, SourceWorkspace, skills[0].Source)
	assert.Contains(t, skills[0].Path, "lint.yaml")
	assert.Equal(t, 200, skills[0].MaxTokens)
}

func TestLoadFromDir_NonexistentDir(t *testing.T) {
	skills, err := LoadFromDir("/nonexistent/path", SourceWorkspace)
	require.NoError(t, err)
	assert.Nil(t, skills)
}

func TestLoadFromDir_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	skills, err := LoadFromDir(dir, SourceWorkspace)
	require.NoError(t, err)
	assert.Empty(t, skills)
}

func TestLoadFromDir_SkipsNonYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# not a skill"), 0644)

	skills, err := LoadFromDir(dir, SourceWorkspace)
	require.NoError(t, err)
	assert.Empty(t, skills)
}

func TestLoadFromDir_SkipsInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{not yaml}}"), 0644)

	skills, err := LoadFromDir(dir, SourceWorkspace)
	require.NoError(t, err)
	assert.Empty(t, skills)
}

func TestLoadFromDir_SkipsEmptyName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.yaml"), []byte("skill:\n  description: no name\n"), 0644)

	skills, err := LoadFromDir(dir, SourceWorkspace)
	require.NoError(t, err)
	assert.Empty(t, skills)
}

func TestLoadFromDir_MultipleSkills(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		yaml := "skill:\n  name: " + name + "\n  description: test\n  prompt: do stuff\n"
		os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(yaml), 0644)
	}

	skills, err := LoadFromDir(dir, SourcePackage)
	require.NoError(t, err)
	assert.Len(t, skills, 2)
	for _, s := range skills {
		assert.Equal(t, SourcePackage, s.Source)
	}
}

func TestRender_BasicSubstitution(t *testing.T) {
	prompt := "Build {{package_name}} in {{workspace}}"
	vars := map[string]string{
		"package_name": "my-service",
		"workspace":    "my-project",
	}
	result := Render(prompt, vars)
	assert.Equal(t, "Build my-service in my-project", result)
}

func TestRender_NoVars(t *testing.T) {
	prompt := "Build something"
	result := Render(prompt, nil)
	assert.Equal(t, "Build something", result)
}

func TestRender_UnmatchedVarsLeftAlone(t *testing.T) {
	prompt := "Build {{package_name}} with {{unknown}}"
	vars := map[string]string{"package_name": "svc"}
	result := Render(prompt, vars)
	assert.Equal(t, "Build svc with {{unknown}}", result)
}

func TestRender_EmptyPrompt(t *testing.T) {
	result := Render("", map[string]string{"x": "y"})
	assert.Equal(t, "", result)
}

func TestRender_RepeatedVars(t *testing.T) {
	prompt := "{{name}} and {{name}}"
	result := Render(prompt, map[string]string{"name": "svc"})
	assert.Equal(t, "svc and svc", result)
}

func TestSourceConstants(t *testing.T) {
	assert.Equal(t, Source(0), SourceBuiltin)
	assert.Equal(t, Source(1), SourceWorkspace)
	assert.Equal(t, Source(2), SourcePackage)
}

func TestLoadBuiltins_AllHaveRequiredFields(t *testing.T) {
	skills, err := LoadBuiltins()
	require.NoError(t, err)
	for _, s := range skills {
		assert.NotEmpty(t, s.Name, "every skill must have a name")
		assert.NotEmpty(t, s.Description, "skill %q must have a description", s.Name)
		assert.NotEmpty(t, s.Prompt, "skill %q must have a prompt", s.Name)
	}
}

func TestLoadBuiltins_Count(t *testing.T) {
	skills, err := LoadBuiltins()
	require.NoError(t, err)
	assert.Equal(t, 6, len(skills), "expected 6 built-in skills")
}

func TestLoadBuiltins_NoDuplicateNames(t *testing.T) {
	skills, err := LoadBuiltins()
	require.NoError(t, err)
	seen := make(map[string]bool)
	for _, s := range skills {
		assert.False(t, seen[s.Name], "duplicate skill name: %s", s.Name)
		seen[s.Name] = true
	}
}

func TestLoadFromDir_WithAutoContext(t *testing.T) {
	dir := t.TempDir()
	yaml := `skill:
  name: ctx-skill
  description: test
  prompt: do stuff
  auto_context:
    - git_diff
    - package_config
  max_tokens: 500
`
	os.WriteFile(filepath.Join(dir, "ctx.yaml"), []byte(yaml), 0644)
	skills, err := LoadFromDir(dir, SourceWorkspace)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, []string{"git_diff", "package_config"}, skills[0].AutoContext)
	assert.Equal(t, 500, skills[0].MaxTokens)
}

func TestLoadFromDir_ReadError(t *testing.T) {
	// A directory that exists but a file that can't be read
	dir := t.TempDir()
	badFile := filepath.Join(dir, "unreadable.yaml")
	os.WriteFile(badFile, []byte("skill:\n  name: x\n"), 0644)
	os.Chmod(badFile, 0000)
	t.Cleanup(func() { os.Chmod(badFile, 0644) })

	skills, err := LoadFromDir(dir, SourceWorkspace)
	require.NoError(t, err)
	// Should skip unreadable files gracefully
	assert.Empty(t, skills)
}
