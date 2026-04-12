package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadWorkspaceConfig_FullConfig(t *testing.T) {
	yaml := `
workspace:
  name: my-platform
  ignore:
    - vendor/
    - archived/
  sources:
    auth-service:
      url: https://github.com/org/auth-service.git
      branch: main
      path: ./auth-service
    shared-protos:
      url: https://github.com/org/shared-protos.git
      branch: v2
      path: ./shared-protos
  version-set:
    file: takumi-versions.yaml
  settings:
    parallel: true
  ai:
    instructions: takumi-ai.yaml
`
	path := writeTempFile(t, "takumi.yaml", yaml)
	cfg, err := LoadWorkspaceConfig(path)

	require.NoError(t, err)
	assert.Equal(t, "my-platform", cfg.Workspace.Name)
	assert.Equal(t, []string{"vendor/", "archived/"}, cfg.Workspace.Ignore)
	assert.True(t, cfg.Workspace.Settings.Parallel)
	assert.Equal(t, "takumi-versions.yaml", cfg.Workspace.VersionSet.File)
	assert.Equal(t, "takumi-ai.yaml", cfg.Workspace.AI.Instructions)

	// Sources
	require.Len(t, cfg.Workspace.Sources, 2)
	auth := cfg.Workspace.Sources["auth-service"]
	assert.Equal(t, "https://github.com/org/auth-service.git", auth.URL)
	assert.Equal(t, "main", auth.Branch)
	assert.Equal(t, "./auth-service", auth.Path)

	protos := cfg.Workspace.Sources["shared-protos"]
	assert.Equal(t, "v2", protos.Branch)
}

func TestLoadWorkspaceConfig_MinimalConfig(t *testing.T) {
	yaml := `
workspace:
  name: tiny
`
	path := writeTempFile(t, "takumi.yaml", yaml)
	cfg, err := LoadWorkspaceConfig(path)

	require.NoError(t, err)
	assert.Equal(t, "tiny", cfg.Workspace.Name)
	assert.Nil(t, cfg.Workspace.Ignore)
	assert.Nil(t, cfg.Workspace.Sources)
	assert.False(t, cfg.Workspace.Settings.Parallel)
}

func TestLoadWorkspaceConfig_FileNotFound(t *testing.T) {
	_, err := LoadWorkspaceConfig("/nonexistent/takumi.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading workspace config")
}

func TestLoadWorkspaceConfig_InvalidYAML(t *testing.T) {
	path := writeTempFile(t, "takumi.yaml", "{{not yaml}}")
	_, err := LoadWorkspaceConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing workspace config")
}

func TestDefaultWorkspaceConfig(t *testing.T) {
	cfg := DefaultWorkspaceConfig("test-ws")

	assert.Equal(t, "test-ws", cfg.Workspace.Name)
	assert.Equal(t, []string{"vendor/", "node_modules/", ".git/"}, cfg.Workspace.Ignore)
	assert.True(t, cfg.Workspace.Settings.Parallel)
	assert.NotNil(t, cfg.Workspace.Sources)
	assert.Empty(t, cfg.Workspace.Sources)
}

func TestWorkspaceConfig_MarshalRoundtrip(t *testing.T) {
	original := DefaultWorkspaceConfig("roundtrip")
	original.Workspace.Sources["svc"] = Source{
		URL:    "https://example.com/svc.git",
		Branch: "main",
		Path:   "./svc",
	}

	data, err := original.Marshal()
	require.NoError(t, err)

	path := writeTempFile(t, "takumi.yaml", string(data))
	loaded, err := LoadWorkspaceConfig(path)
	require.NoError(t, err)

	assert.Equal(t, original.Workspace.Name, loaded.Workspace.Name)
	assert.Equal(t, original.Workspace.Settings.Parallel, loaded.Workspace.Settings.Parallel)
	assert.Equal(t, original.Workspace.Sources["svc"].URL, loaded.Workspace.Sources["svc"].URL)
}

func TestWorkspaceConfig_MarshalOmitsEmptyFields(t *testing.T) {
	cfg := &WorkspaceConfig{
		Workspace: Workspace{
			Name: "sparse",
			Settings: WorkspaceSettings{
				Parallel: true,
			},
		},
	}

	data, err := cfg.Marshal()
	require.NoError(t, err)

	yaml := string(data)
	assert.NotContains(t, yaml, "version-set")
	assert.NotContains(t, yaml, "instructions")
	assert.NotContains(t, yaml, "sources")
}

func TestSaveWorkspaceConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "takumi.yaml")
	cfg := DefaultWorkspaceConfig("saved-ws")
	err := SaveWorkspaceConfig(path, cfg)
	require.NoError(t, err)

	loaded, err := LoadWorkspaceConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "saved-ws", loaded.Workspace.Name)
	assert.True(t, loaded.Workspace.Settings.Parallel)
}

func TestSaveWorkspaceConfig_BadPath(t *testing.T) {
	err := SaveWorkspaceConfig("/nonexistent/dir/takumi.yaml", DefaultWorkspaceConfig("x"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "writing workspace config")
}

func TestLoadVersionSetConfig_Valid(t *testing.T) {
	yaml := `version-set:
  name: release-q1
  strategy: strict
  packages:
    react: "18.0.0"
    typescript: "5.3.0"
`
	path := writeTempFile(t, "takumi-versions.yaml", yaml)
	cfg, err := LoadVersionSetConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "release-q1", cfg.VersionSet.Name)
	assert.Equal(t, "strict", cfg.VersionSet.Strategy)
	assert.Equal(t, "18.0.0", cfg.VersionSet.Packages["react"])
	assert.Len(t, cfg.VersionSet.Packages, 2)
}

func TestLoadVersionSetConfig_NotFound(t *testing.T) {
	_, err := LoadVersionSetConfig("/nonexistent/file.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading version-set config")
}

func TestLoadVersionSetConfig_InvalidYAML(t *testing.T) {
	path := writeTempFile(t, "bad.yaml", "{{not yaml}}")
	_, err := LoadVersionSetConfig(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing version-set config")
}

// writeTempFile creates a temp file with the given content and returns its path.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}
