package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/workspace"
)

func TestRunRemove_RemovesSource(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspaceWithSource(t, wsDir, "my-pkg", config.Source{
		URL:    "https://example.com/my-pkg.git",
		Branch: "main",
		Path:   "./my-pkg",
	})
	chdirClean(t, wsDir)

	exitCode := withFakeExit(t)
	err := runRemove(removeCmd, []string{"my-pkg"})
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)

	// Verify removed from config
	cfg, err := config.LoadWorkspaceConfig(filepath.Join(wsDir, "takumi.yaml"))
	require.NoError(t, err)
	_, exists := cfg.Workspace.Sources["my-pkg"]
	assert.False(t, exists)
}

func TestRunRemove_CleansUpEnvDir(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspaceWithSource(t, wsDir, "my-pkg", config.Source{
		URL:    "https://example.com/my-pkg.git",
		Branch: "main",
		Path:   "./my-pkg",
	})

	// Create an env directory
	envDir := filepath.Join(wsDir, ".takumi", "envs", "my-pkg")
	require.NoError(t, os.MkdirAll(envDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(envDir, "marker"), []byte("x"), 0644))

	chdirClean(t, wsDir)
	exitCode := withFakeExit(t)
	err := runRemove(removeCmd, []string{"my-pkg"})
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)

	assert.NoDirExists(t, envDir)
}

func TestRunRemove_WithDeleteFlag(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	pkgDir := filepath.Join(wsDir, "my-pkg")
	require.NoError(t, os.Mkdir(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "file.txt"), []byte("data"), 0644))

	setupWorkspaceWithSource(t, wsDir, "my-pkg", config.Source{
		URL:    "https://example.com/my-pkg.git",
		Branch: "main",
		Path:   "./my-pkg",
	})
	chdirClean(t, wsDir)

	removeCmd.Flags().Set("delete", "true")
	t.Cleanup(func() { removeCmd.Flags().Set("delete", "false") })

	exitCode := withFakeExit(t)
	err := runRemove(removeCmd, []string{"my-pkg"})
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)

	// Directory should be deleted
	assert.NoDirExists(t, pkgDir)
}

func TestRunRemove_ErrorIfNotTracked(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspace(t, wsDir)
	chdirClean(t, wsDir)

	exitCode := withFakeExit(t)
	err := runRemove(removeCmd, []string{"nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a tracked source")
	assert.Equal(t, -1, *exitCode)
}

// setupWorkspaceWithSource creates a workspace with a single tracked source.
func setupWorkspaceWithSource(t *testing.T, dir, name string, source config.Source) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "envs"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.Sources[name] = source
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, workspace.WorkspaceFile), cfg))
}
