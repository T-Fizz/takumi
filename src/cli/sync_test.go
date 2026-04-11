package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/config"
)

// chdirClean changes to dir and restores the original cwd on cleanup.
func chdirClean(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestRunSync_NoSources(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspace(t, wsDir)
	chdirClean(t, wsDir)

	exitCode := withFakeExit(t)
	out := captureStdout(t, func() {
		err := runSync(syncCmd, nil)
		require.NoError(t, err)
	})
	assert.Equal(t, -1, *exitCode)
	assert.Contains(t, out, "No tracked sources")
}

func TestRunSync_PullsExistingRepo(t *testing.T) {
	wsDir := realPath(t, t.TempDir())

	// Create a bare repo and clone it into the workspace
	bareRepo := createBareRepo(t, wsDir)
	clonedDir := filepath.Join(wsDir, "test-repo")
	require.NoError(t, exec.Command("git", "clone", bareRepo, clonedDir).Run())

	// Detect the actual default branch name
	branch := detectGitBranch(clonedDir)

	setupWorkspaceWithSource(t, wsDir, "test-repo", config.Source{
		URL:    bareRepo,
		Branch: branch,
		Path:   "./test-repo",
	})
	chdirClean(t, wsDir)

	exitCode := withFakeExit(t)
	err := runSync(syncCmd, nil)
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)
}

func TestRunSync_ClonesMissingRepo(t *testing.T) {
	wsDir := realPath(t, t.TempDir())

	bareRepo := createBareRepo(t, wsDir)

	setupWorkspaceWithSource(t, wsDir, "test-repo", config.Source{
		URL:    bareRepo,
		Branch: "main",
		Path:   "./test-repo",
	})
	chdirClean(t, wsDir)

	// The directory does not exist, sync should clone it
	exitCode := withFakeExit(t)
	err := runSync(syncCmd, nil)
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)

	assert.DirExists(t, filepath.Join(wsDir, "test-repo"))
}

func TestRunSync_HandlesBadURL(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspaceWithSource(t, wsDir, "bad-repo", config.Source{
		URL:    "https://example.com/nonexistent-repo.git",
		Branch: "main",
		Path:   "./bad-repo",
	})
	chdirClean(t, wsDir)

	exitCode := withFakeExit(t)
	out := captureStdout(t, func() {
		err := runSync(syncCmd, nil)
		// sync continues past failures, returns nil
		assert.NoError(t, err)
	})
	_ = out
	assert.Equal(t, -1, *exitCode)
}
