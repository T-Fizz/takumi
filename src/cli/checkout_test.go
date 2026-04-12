package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/workspace"
)

func TestRepoNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/org/my-repo.git", "my-repo"},
		{"https://github.com/org/my-repo", "my-repo"},
		{"git@github.com:org/my-repo.git", "my-repo"},
		{"https://gitlab.com/group/subgroup/repo.git", "repo"},
		{"my-repo.git", "my-repo"},
		{"my-repo", "my-repo"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, repoNameFromURL(tt.url))
		})
	}
}

func TestRunCheckout_ClonesAndRegisters(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspace(t, wsDir)
	chdirClean(t, wsDir)

	// Create a local bare repo to clone from
	bareRepo := createBareRepo(t, wsDir)

	exitCode := withFakeExit(t)
	err := runCheckout(checkoutCmd, []string{bareRepo})
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)

	// Verify clone happened
	clonedDir := filepath.Join(wsDir, "test-repo")
	assert.DirExists(t, clonedDir)

	// Verify registered in workspace config
	cfg, err := config.LoadWorkspaceConfig(filepath.Join(wsDir, "takumi.yaml"))
	require.NoError(t, err)
	source, ok := cfg.Workspace.Sources["test-repo"]
	assert.True(t, ok)
	assert.Equal(t, bareRepo, source.URL)
	assert.Equal(t, "./test-repo", source.Path)
}

func TestRunCheckout_WithBranchFlag(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspace(t, wsDir)
	chdirClean(t, wsDir)

	bareRepo := createBareRepo(t, wsDir)

	exitCode := withFakeExit(t)
	checkoutCmd.Flags().Set("branch", "main")
	t.Cleanup(func() { checkoutCmd.Flags().Set("branch", "") })

	err := runCheckout(checkoutCmd, []string{bareRepo})
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)

	cfg, err := config.LoadWorkspaceConfig(filepath.Join(wsDir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "main", cfg.Workspace.Sources["test-repo"].Branch)
}

func TestRunCheckout_WithPathFlag(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspace(t, wsDir)
	chdirClean(t, wsDir)

	bareRepo := createBareRepo(t, wsDir)

	checkoutCmd.Flags().Set("path", "custom-dir")
	t.Cleanup(func() { checkoutCmd.Flags().Set("path", "") })

	exitCode := withFakeExit(t)
	err := runCheckout(checkoutCmd, []string{bareRepo})
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)

	assert.DirExists(t, filepath.Join(wsDir, "custom-dir"))

	cfg, err := config.LoadWorkspaceConfig(filepath.Join(wsDir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "./custom-dir", cfg.Workspace.Sources["test-repo"].Path)
}

func TestRunCheckout_ErrorIfDirExists(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspace(t, wsDir)
	chdirClean(t, wsDir)

	// Create a dir that would conflict
	require.NoError(t, os.Mkdir(filepath.Join(wsDir, "test-repo"), 0755))

	exitCode := withFakeExit(t)
	err := runCheckout(checkoutCmd, []string{"https://example.com/test-repo.git"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Equal(t, -1, *exitCode)
}

func TestRunCheckout_DetectsPackages(t *testing.T) {
	wsDir := realPath(t, t.TempDir())
	setupWorkspace(t, wsDir)
	chdirClean(t, wsDir)

	// Create a bare repo that contains a takumi-pkg.yaml
	bareRepo := createBareRepoWithPackage(t, wsDir, "my-service")

	exitCode := withFakeExit(t)
	err := runCheckout(checkoutCmd, []string{bareRepo})
	require.NoError(t, err)
	assert.Equal(t, -1, *exitCode)

	// Verify the package was detected
	pkgs, _, err := workspace.ScanPackages(filepath.Join(wsDir, "test-repo"), nil)
	require.NoError(t, err)
	assert.Contains(t, pkgs, "my-service")
}

func TestDetectGitBranch(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	require.NoError(t, cmd.Run())

	// Default branch
	branch := detectGitBranch(dir)
	assert.NotEmpty(t, branch)
}

func TestDetectGitBranch_FallbackOnError(t *testing.T) {
	branch := detectGitBranch("/nonexistent")
	assert.Equal(t, "main", branch)
}

// createBareRepo creates a bare git repo with one commit, returns the path.
func createBareRepo(t *testing.T, parentDir string) string {
	t.Helper()
	// Create a normal repo, make a commit, then clone as bare
	tmpRepo := filepath.Join(parentDir, "_tmp_repo")
	require.NoError(t, os.Mkdir(tmpRepo, 0755))

	cmds := [][]string{
		{"git", "init", tmpRepo},
		{"git", "-C", tmpRepo, "config", "user.email", "test@test.com"},
		{"git", "-C", tmpRepo, "config", "user.name", "test"},
	}
	for _, c := range cmds {
		require.NoError(t, exec.Command(c[0], c[1:]...).Run())
	}

	require.NoError(t, os.WriteFile(filepath.Join(tmpRepo, "README.md"), []byte("# test"), 0644))
	require.NoError(t, exec.Command("git", "-C", tmpRepo, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", tmpRepo, "commit", "-m", "init").Run())

	bareRepo := filepath.Join(parentDir, "test-repo.git")
	require.NoError(t, exec.Command("git", "clone", "--bare", tmpRepo, bareRepo).Run())
	require.NoError(t, os.RemoveAll(tmpRepo))

	return bareRepo
}

// createBareRepoWithPackage creates a bare repo that contains a takumi-pkg.yaml.
func createBareRepoWithPackage(t *testing.T, parentDir, pkgName string) string {
	t.Helper()
	tmpRepo := filepath.Join(parentDir, "_tmp_repo")
	require.NoError(t, os.Mkdir(tmpRepo, 0755))

	cmds := [][]string{
		{"git", "init", tmpRepo},
		{"git", "-C", tmpRepo, "config", "user.email", "test@test.com"},
		{"git", "-C", tmpRepo, "config", "user.name", "test"},
	}
	for _, c := range cmds {
		require.NoError(t, exec.Command(c[0], c[1:]...).Run())
	}

	pkgYAML := fmt.Sprintf("package:\n  name: %s\n  version: 0.1.0\n", pkgName)
	require.NoError(t, os.WriteFile(filepath.Join(tmpRepo, "takumi-pkg.yaml"), []byte(pkgYAML), 0644))
	require.NoError(t, exec.Command("git", "-C", tmpRepo, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", tmpRepo, "commit", "-m", "init").Run())

	bareRepo := filepath.Join(parentDir, "test-repo.git")
	require.NoError(t, exec.Command("git", "clone", "--bare", tmpRepo, bareRepo).Run())
	require.NoError(t, os.RemoveAll(tmpRepo))

	return bareRepo
}
