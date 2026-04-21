package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/workspace"
)

// withMockAgentPrompt replaces the interactive agent prompt with one that
// returns the "none" agent (skip), restoring the original on cleanup.
func withMockAgentPrompt(t *testing.T) {
	t.Helper()
	original := promptAgentSelection
	promptAgentSelection = func() (*AgentType, error) {
		return AgentByName("none"), nil
	}
	t.Cleanup(func() { promptAgentSelection = original })
}

func TestRunInit_CreatesWorkspaceAndPackage(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)
	withMockAgentPrompt(t)

	err := runInit(initCmd, nil)
	require.NoError(t, err)

	// Workspace marker created
	assert.DirExists(t, filepath.Join(dir, ".takumi"))
	assert.DirExists(t, filepath.Join(dir, ".takumi", "envs"))
	assert.DirExists(t, filepath.Join(dir, ".takumi", "logs"))

	// Workspace config created and valid
	wsCfg, err := config.LoadWorkspaceConfig(filepath.Join(dir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(dir), wsCfg.Workspace.Name)
	assert.True(t, wsCfg.Workspace.Settings.Parallel)

	// Package config created and valid
	pkgCfg, err := config.LoadPackageConfig(filepath.Join(dir, "takumi-pkg.yaml"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(dir), pkgCfg.Package.Name)
	assert.Equal(t, "0.1.0", pkgCfg.Package.Version)
}

func TestRunInit_NamedSubpackage(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)

	// First create a workspace
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".takumi"), 0755))
	wsCfg := config.DefaultWorkspaceConfig("test")
	data, _ := wsCfg.Marshal()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))

	// Init named subpackage
	err := runInit(initCmd, []string{"my-service"})
	require.NoError(t, err)

	// Subdir created with package config
	pkgPath := filepath.Join(dir, "my-service", "takumi-pkg.yaml")
	assert.FileExists(t, pkgPath)

	pkgCfg, err := config.LoadPackageConfig(pkgPath)
	require.NoError(t, err)
	assert.Equal(t, "my-service", pkgCfg.Package.Name)
}

func TestRunInit_NamedSubpackage_CreatesWorkspaceIfMissing(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)
	withMockAgentPrompt(t)

	err := runInit(initCmd, []string{"new-svc"})
	require.NoError(t, err)

	// Should have created workspace at cwd
	assert.DirExists(t, filepath.Join(dir, ".takumi"))
	assert.FileExists(t, filepath.Join(dir, "takumi.yaml"))

	// And the subpackage
	assert.FileExists(t, filepath.Join(dir, "new-svc", "takumi-pkg.yaml"))
}

func TestRunInit_DoesNotDuplicateWorkspace(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)
	withMockAgentPrompt(t)

	// Create workspace first
	require.NoError(t, runInit(initCmd, nil))

	// Create a subdirectory and init from there
	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(subDir, 0755))
	require.NoError(t, os.Chdir(subDir))

	require.NoError(t, runInit(initCmd, []string{"svc"}))

	// Should NOT create a second .takumi/ in sub/
	assert.NoDirExists(t, filepath.Join(subDir, ".takumi"))
	// But should create the package
	assert.FileExists(t, filepath.Join(subDir, "svc", "takumi-pkg.yaml"))
}

func TestRunInit_ErrorsOnExistingPackageConfig(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)
	withMockAgentPrompt(t)

	// Create workspace + package
	require.NoError(t, runInit(initCmd, nil))

	// Try again — should error
	err := runInit(initCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRunInit_ErrorCreatingSubdir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests unreliable on Windows")
	}

	dir := t.TempDir()
	chdirClean(t, dir)

	// Create workspace
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".takumi"), 0755))
	wsCfg := config.DefaultWorkspaceConfig("test")
	data, _ := wsCfg.Marshal()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))

	// Make cwd read-only so MkdirAll fails
	require.NoError(t, os.Chmod(dir, 0555))
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	err := runInit(initCmd, []string{"should-fail"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating directory")
}

func TestRunInit_ErrorWritingPackageConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests unreliable on Windows")
	}

	dir := t.TempDir()
	chdirClean(t, dir)

	// Create workspace
	require.NoError(t, initWorkspace(dir, "test", nil))

	// Create target dir that's read-only
	targetDir := filepath.Join(dir, "locked")
	require.NoError(t, os.Mkdir(targetDir, 0555))
	t.Cleanup(func() { os.Chmod(targetDir, 0755) })

	err := runInit(initCmd, []string{"locked"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing takumi-pkg.yaml")
}

func TestInitWorkspace_CreatesAllDirectories(t *testing.T) {
	dir := t.TempDir()
	err := initWorkspace(dir, "test-ws", nil)
	require.NoError(t, err)

	assert.DirExists(t, filepath.Join(dir, ".takumi"))
	assert.DirExists(t, filepath.Join(dir, ".takumi", "envs"))
	assert.DirExists(t, filepath.Join(dir, ".takumi", "logs"))
}

func TestInitWorkspace_WritesValidConfig(t *testing.T) {
	dir := t.TempDir()
	err := initWorkspace(dir, "my-ws", nil)
	require.NoError(t, err)

	cfg, err := config.LoadWorkspaceConfig(filepath.Join(dir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "my-ws", cfg.Workspace.Name)
}

func TestInitWorkspace_ErrorOnReadOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests unreliable on Windows")
	}

	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0555))
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	err := initWorkspace(dir, "fail", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating .takumi")
}

func TestInitWorkspace_ErrorCreatingSubdirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests unreliable on Windows")
	}

	dir := t.TempDir()
	// Pre-create .takumi so MkdirAll on line 103 is a no-op
	markerDir := filepath.Join(dir, ".takumi")
	require.NoError(t, os.Mkdir(markerDir, 0755))

	// Make it read-only so subdirectory creation fails
	require.NoError(t, os.Chmod(markerDir, 0555))
	t.Cleanup(func() { os.Chmod(markerDir, 0755) })

	err := initWorkspace(dir, "fail", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".takumi/envs")
}

func TestInitWorkspace_ErrorWritingConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests unreliable on Windows")
	}

	dir := t.TempDir()

	// Create .takumi dir but make root read-only so takumi.yaml write fails
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "envs"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	require.NoError(t, os.Chmod(dir, 0555))
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	err := initWorkspace(dir, "fail", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing workspace config")
}

func TestRunInit_RootFlag_CreatesProjectDir(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)
	withMockAgentPrompt(t)

	initCmd.Flags().Set("root", "my-project")
	t.Cleanup(func() { initCmd.Flags().Set("root", "") })

	err := runInit(initCmd, nil)
	require.NoError(t, err)

	projectDir := filepath.Join(dir, "my-project")
	assert.DirExists(t, filepath.Join(projectDir, ".takumi"))
	assert.FileExists(t, filepath.Join(projectDir, "takumi.yaml"))
	assert.FileExists(t, filepath.Join(projectDir, "takumi-pkg.yaml"))

	// Workspace name should be the root flag value
	cfg, err := config.LoadWorkspaceConfig(filepath.Join(projectDir, "takumi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "my-project", cfg.Workspace.Name)

	// Package name should also be root flag value
	pkgCfg, err := config.LoadPackageConfig(filepath.Join(projectDir, "takumi-pkg.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "my-project", pkgCfg.Package.Name)
}

func TestRunInit_RootFlag_WithPackageName(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)
	withMockAgentPrompt(t)

	initCmd.Flags().Set("root", "my-project")
	t.Cleanup(func() { initCmd.Flags().Set("root", "") })

	err := runInit(initCmd, []string{"svc-a"})
	require.NoError(t, err)

	projectDir := filepath.Join(dir, "my-project")
	assert.DirExists(t, filepath.Join(projectDir, ".takumi"))
	assert.FileExists(t, filepath.Join(projectDir, "takumi.yaml"))
	// Package should be in a subdir
	assert.FileExists(t, filepath.Join(projectDir, "svc-a", "takumi-pkg.yaml"))
	// Root should NOT have a takumi-pkg.yaml
	assert.NoFileExists(t, filepath.Join(projectDir, "takumi-pkg.yaml"))
}

func TestRunInit_PackageDiscoverableAfterInit(t *testing.T) {
	dir := t.TempDir()
	chdirClean(t, dir)
	withMockAgentPrompt(t)

	// Init workspace + root package
	require.NoError(t, runInit(initCmd, nil))

	// Init two sub-packages
	require.NoError(t, runInit(initCmd, []string{"svc-a"}))
	require.NoError(t, runInit(initCmd, []string{"svc-b"}))

	// Scan should find all three
	pkgs, _, err := workspace.ScanPackages(dir, nil)
	require.NoError(t, err)
	assert.Len(t, pkgs, 3)
	assert.Contains(t, pkgs, filepath.Base(dir))
	assert.Contains(t, pkgs, "svc-a")
	assert.Contains(t, pkgs, "svc-b")
}
