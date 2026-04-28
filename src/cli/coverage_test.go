package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/executor"
	"github.com/tfitz/takumi/src/workspace"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupWorkspaceWithPackages creates a workspace with one or more packages
// that have build and test phases defined.
func setupWorkspaceWithPackages(t *testing.T, dir string) {
	t.Helper()
	setupWorkspace(t, dir)

	// svc-a: no deps
	pkgDirA := filepath.Join(dir, "svc-a")
	require.NoError(t, os.MkdirAll(pkgDirA, 0755))
	cfgA := `package:
  name: svc-a
  version: 0.1.0
phases:
  build:
    pre:
      - echo pre-build
    commands:
      - echo building-a
    post:
      - echo post-build
  test:
    commands:
      - echo testing-a
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDirA, "takumi-pkg.yaml"), []byte(cfgA), 0644))

	// svc-b: depends on svc-a
	pkgDirB := filepath.Join(dir, "svc-b")
	require.NoError(t, os.MkdirAll(pkgDirB, 0755))
	cfgB := `package:
  name: svc-b
  version: 0.2.0
dependencies:
  - svc-a
phases:
  build:
    commands:
      - echo building-b
  test:
    commands:
      - echo testing-b
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDirB, "takumi-pkg.yaml"), []byte(cfgB), 0644))
}

// setupWorkspaceWithRuntime creates a workspace containing a package that
// declares a runtime section with setup commands and env vars.
func setupWorkspaceWithRuntime(t *testing.T, dir string) {
	t.Helper()
	setupWorkspace(t, dir)

	pkgDir := filepath.Join(dir, "rt-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	cfg := `package:
  name: rt-pkg
  version: 0.1.0
runtime:
  setup:
    - mkdir -p {{env_dir}}
  env:
    MY_VAR: hello
phases:
  build:
    commands:
      - echo building
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
}

// setupGitWorkspace initialises a git repo on top of a workspace with
// packages so that gitChangedFiles and related helpers can operate.
func setupGitWorkspace(t *testing.T, dir string) {
	t.Helper()
	setupWorkspaceWithPackages(t, dir)

	cmds := [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "test"},
		{"git", "-C", dir, "add", "."},
		{"git", "-C", dir, "commit", "-m", "init"},
	}
	for _, c := range cmds {
		require.NoError(t, exec.Command(c[0], c[1:]...).Run(), "git command %v failed", c)
	}
}

// setupVersionSet writes a takumi-versions.yaml file into the workspace.
func setupVersionSet(t *testing.T, dir string) {
	t.Helper()
	vs := `version-set:
  name: test-versions
  strategy: strict
  packages:
    go: "1.22"
    node: "20.11"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, workspace.VersionsFile), []byte(vs), 0644))
}

// writeMetrics writes a metrics.json file with the given entries.
func writeMetrics(t *testing.T, dir string, runs []executor.MetricsEntry) {
	t.Helper()
	data, err := json.MarshalIndent(executor.MetricsFile{Runs: runs}, "", "  ")
	require.NoError(t, err)
	metricsDir := filepath.Join(dir, workspace.MarkerDir)
	require.NoError(t, os.MkdirAll(metricsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(metricsDir, "metrics.json"), data, 0644))
}

// ---------------------------------------------------------------------------
// contains()
// ---------------------------------------------------------------------------

func TestContains(t *testing.T) {
	assert.True(t, contains([]string{"a", "b", "c"}, "b"))
	assert.False(t, contains([]string{"a", "b", "c"}, "d"))
	assert.False(t, contains(nil, "x"))
	assert.False(t, contains([]string{}, "x"))
}

// ---------------------------------------------------------------------------
// joinParts()
// ---------------------------------------------------------------------------

func TestJoinParts(t *testing.T) {
	assert.Equal(t, "nothing to do", joinParts(nil))
	assert.Equal(t, "nothing to do", joinParts([]string{}))
	assert.Equal(t, "alpha", joinParts([]string{"alpha"}))
	assert.Equal(t, "alpha, beta", joinParts([]string{"alpha", "beta"}))
	assert.Equal(t, "a, b, c", joinParts([]string{"a", "b", "c"}))
}

// ---------------------------------------------------------------------------
// runGraph
// ---------------------------------------------------------------------------

func TestRunGraph_EmptyWorkspace(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runGraph(graphCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages found")
}

func TestRunGraph_WithPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runGraph(graphCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Dependency Graph")
	assert.Contains(t, out, "svc-a")
	assert.Contains(t, out, "svc-b")
	assert.Contains(t, out, "2 packages")
}

// ---------------------------------------------------------------------------
// runStatus
// ---------------------------------------------------------------------------

func TestRunStatus_EmptyWorkspace(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Workspace: test-ws")
	assert.Contains(t, out, "No packages found")
}

func TestRunStatus_WithPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "svc-a")
	assert.Contains(t, out, "svc-b")
	assert.Contains(t, out, "2 packages")
}

func TestRunStatus_WithRuntime(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Environments")
	assert.Contains(t, out, "rt-pkg")
	assert.Contains(t, out, "not set up")
}

func TestRunStatus_WithRuntimeReady(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	// Pre-create the env dir so status shows "ready"
	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "rt-pkg")
	require.NoError(t, os.MkdirAll(envDir, 0755))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "ready")
}

func TestRunStatus_WithMetrics(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	writeMetrics(t, dir, []executor.MetricsEntry{
		{Timestamp: "2025-01-01T00:00:00Z", Phase: "build", Package: "svc-a", DurationMs: 100, ExitCode: 0},
		{Timestamp: "2025-01-01T00:00:01Z", Phase: "test", Package: "svc-a", DurationMs: 50, ExitCode: 1},
	})
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Recent Builds")
	assert.Contains(t, out, "svc-a")
}

func TestRunStatus_WithSources(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "envs"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.Sources["my-lib"] = config.Source{
		URL:    "https://example.com/lib.git",
		Branch: "main",
		Path:   "./my-lib",
	}
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, workspace.WorkspaceFile), cfg))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Tracked Sources")
	assert.Contains(t, out, "my-lib")
	assert.Contains(t, out, "missing")
}

func TestRunStatus_WithSourcePresent(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "envs"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.Sources["my-lib"] = config.Source{
		URL:    "https://example.com/lib.git",
		Branch: "main",
		Path:   "./my-lib",
	}
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, workspace.WorkspaceFile), cfg))
	// Actually create the source directory so it shows as "present"
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "my-lib"), 0755))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "present")
}

func TestRunStatus_WithAIAgent(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.AI.Agent = "claude"
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "AI Agent")
	assert.Contains(t, out, "claude")
}

func TestRunStatus_MetricsMore5(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	var runs []executor.MetricsEntry
	for i := 0; i < 8; i++ {
		runs = append(runs, executor.MetricsEntry{
			Timestamp: "2025-01-01T00:00:00Z", Phase: "build", Package: "svc-a", DurationMs: 100, ExitCode: 0,
		})
	}
	writeMetrics(t, dir, runs)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Recent Builds")
}

// ---------------------------------------------------------------------------
// runEnvSetup / runEnvClean / runEnvList
// ---------------------------------------------------------------------------

func TestRunEnvSetup_NoRuntime(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir) // packages have no runtime
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvSetup(envSetupCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages define a runtime section")
}

func TestRunEnvSetup_WithRuntime(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvSetup(envSetupCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Setting up")
	assert.Contains(t, out, "rt-pkg")
	assert.Contains(t, out, "Environment ready")

	// Verify the env dir was actually created by the setup command
	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "rt-pkg")
	assert.DirExists(t, envDir)
}

func TestRunEnvSetup_FilteredByArgs(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	// Ask to set up a non-existent package name; runtime package should be skipped
	out := captureStdout(t, func() {
		err := runEnvSetup(envSetupCmd, []string{"nonexistent"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages define a runtime section")
}

func TestRunEnvClean_NothingToClean(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvClean(envCleanCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No environments to clean")
}

func TestRunEnvClean_WithExistingEnv(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "rt-pkg")
	require.NoError(t, os.MkdirAll(envDir, 0755))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvClean(envCleanCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaned")
	assert.Contains(t, out, "rt-pkg")
	assert.NoDirExists(t, envDir)
}

func TestRunEnvClean_FilteredByArgs(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "rt-pkg")
	require.NoError(t, os.MkdirAll(envDir, 0755))
	chdirClean(t, dir)

	// Clean with a different package name; should not clean rt-pkg
	out := captureStdout(t, func() {
		err := runEnvClean(envCleanCmd, []string{"other-pkg"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No environments to clean")
	assert.DirExists(t, envDir)
}

func TestRunEnvList_NoRuntime(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvList(envListCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages define a runtime section")
}

func TestRunEnvList_WithRuntimeNotSetUp(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvList(envListCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Environment Status")
	assert.Contains(t, out, "not set up")
}

func TestRunEnvList_WithRuntimeReady(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "rt-pkg")
	require.NoError(t, os.MkdirAll(envDir, 0755))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvList(envListCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "ready")
}

// ---------------------------------------------------------------------------
// runVersionSetCheck
// ---------------------------------------------------------------------------

func TestRunVersionSetCheck_NoVersionFile(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runVersionSetCheck(versionSetCheckCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No version-set file found")
}

func TestRunVersionSetCheck_WithVersionFile(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	setupVersionSet(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runVersionSetCheck(versionSetCheckCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Version Set: test-versions")
	assert.Contains(t, out, "Strategy: strict")
	assert.Contains(t, out, "go")
	assert.Contains(t, out, "1.22")
	assert.Contains(t, out, "node")
	assert.Contains(t, out, "20.11")
	assert.Contains(t, out, "2 pinned dependencies")
}

func TestRunVersionSetCheck_WithCustomFile(t *testing.T) {
	dir := realPath(t, t.TempDir())
	// Set up workspace with custom version-set file path
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.VersionSet.File = "custom-versions.yaml"
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))

	vs := `version-set:
  name: custom-vs
  packages:
    python: "3.12"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "custom-versions.yaml"), []byte(vs), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runVersionSetCheck(versionSetCheckCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Version Set: custom-vs")
	assert.Contains(t, out, "python")
}

// ---------------------------------------------------------------------------
// runValidate
// ---------------------------------------------------------------------------

func TestRunValidate_CleanWorkspace(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "All configs valid")
	assert.Contains(t, out, "no cycles")
}

func TestRunValidate_MissingDependency(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	pkgDir := filepath.Join(dir, "broken-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	cfg := `package:
  name: broken-pkg
  version: 0.1.0
dependencies:
  - nonexistent-dep
phases:
  build:
    commands:
      - echo hi
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		// Missing dep is a warning, not an error
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "nonexistent-dep")
	assert.Contains(t, out, "not in the workspace")
}

func TestRunValidate_WithVersionSetFileMissing(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.VersionSet.File = "nonexistent-versions.yaml"
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err) // file not found is an error
	})
	assert.Contains(t, out, "file not found")
}

func TestRunValidate_WithVersionSetFilePresent(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.VersionSet.File = "takumi-versions.yaml"
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))
	setupVersionSet(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "valid")
}

func TestRunValidate_VersionSetInvalid(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.VersionSet.File = "takumi-versions.yaml"
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))
	// Write invalid YAML for the version set file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi-versions.yaml"), []byte("{{bad}}"), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err)
	})
	_ = out
}

// ---------------------------------------------------------------------------
// printFindings
// ---------------------------------------------------------------------------

func TestPrintFindings_NoFindings(t *testing.T) {
	out := captureStdout(t, func() {
		e, w := printFindings("test.yaml", nil, 0, 0)
		assert.Equal(t, 0, e)
		assert.Equal(t, 0, w)
	})
	assert.Contains(t, out, "valid")
}

func TestPrintFindings_WithErrors(t *testing.T) {
	findings := []config.Finding{
		{Severity: config.SeverityError, Field: "package.name", Message: "must not be empty"},
	}
	out := captureStdout(t, func() {
		e, w := printFindings("pkg.yaml", findings, 0, 0)
		assert.Equal(t, 1, e)
		assert.Equal(t, 0, w)
	})
	assert.Contains(t, out, "must not be empty")
}

func TestPrintFindings_WithWarnings(t *testing.T) {
	findings := []config.Finding{
		{Severity: config.SeverityWarning, Field: "package.version", Message: "not set"},
	}
	out := captureStdout(t, func() {
		e, w := printFindings("pkg.yaml", findings, 0, 0)
		assert.Equal(t, 0, e)
		assert.Equal(t, 1, w)
	})
	assert.Contains(t, out, "not set")
}

func TestPrintFindings_MixedErrorsAndWarnings(t *testing.T) {
	findings := []config.Finding{
		{Severity: config.SeverityError, Field: "package.name", Message: "must not be empty"},
		{Severity: config.SeverityWarning, Field: "package.version", Message: "not set"},
	}
	out := captureStdout(t, func() {
		e, w := printFindings("pkg.yaml", findings, 2, 3)
		assert.Equal(t, 3, e)
		assert.Equal(t, 4, w)
	})
	assert.Contains(t, out, "must not be empty")
	assert.Contains(t, out, "not set")
}

// ---------------------------------------------------------------------------
// printDryRun and printCmdGroup
// ---------------------------------------------------------------------------

func TestPrintCmdGroup_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		printCmdGroup("pre", nil)
	})
	assert.Empty(t, out)
}

func TestPrintCmdGroup_WithCommands(t *testing.T) {
	out := captureStdout(t, func() {
		printCmdGroup("cmd", []string{"echo hello", "echo world"})
	})
	assert.Contains(t, out, "echo hello")
	assert.Contains(t, out, "echo world")
	assert.Contains(t, out, "cmd:")
}

func TestPrintDryRun_WithPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	out := captureStdout(t, func() {
		err := printDryRun(ws, nil, "build", false)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Dry Run: build")
	assert.Contains(t, out, "svc-a")
	assert.Contains(t, out, "svc-b")
	assert.Contains(t, out, "will run")
}

func TestPrintDryRun_FilteredPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	out := captureStdout(t, func() {
		err := printDryRun(ws, []string{"svc-a"}, "build", false)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "svc-a")
	assert.Contains(t, out, "1 package")
}

func TestPrintDryRun_NoCache(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	out := captureStdout(t, func() {
		err := printDryRun(ws, nil, "build", true)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "will run")
}

func TestPrintDryRun_WithRuntimeEnv(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	out := captureStdout(t, func() {
		err := printDryRun(ws, nil, "build", false)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "MY_VAR")
	assert.Contains(t, out, "env:")
}

func TestPrintDryRun_PhaseNotDefined(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	out := captureStdout(t, func() {
		err := printDryRun(ws, nil, "nonexistent-phase", false)
		assert.NoError(t, err)
	})
	// No packages should be shown since none define this phase
	assert.Contains(t, out, "Dry Run")
	assert.Contains(t, out, "0 packages")
}

// ---------------------------------------------------------------------------
// mapFilesToPackages
// ---------------------------------------------------------------------------

func TestMapFilesToPackages_NoMatch(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := workspace.MapFilesToPackages(ws, []string{"some/random/file.go"})
	assert.Empty(t, result)
}

func TestMapFilesToPackages_DirectMatch(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := workspace.MapFilesToPackages(ws, []string{"svc-a/main.go"})
	assert.True(t, result["svc-a"])
	assert.False(t, result["svc-b"])
}

func TestMapFilesToPackages_MultipleMatches(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := workspace.MapFilesToPackages(ws, []string{"svc-a/main.go", "svc-b/handler.go"})
	assert.True(t, result["svc-a"])
	assert.True(t, result["svc-b"])
}

func TestMapFilesToPackages_EmptyFiles(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := workspace.MapFilesToPackages(ws, nil)
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// gitChangedFiles
// ---------------------------------------------------------------------------

func TestGitChangedFiles_CleanRepo(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)

	files, err := workspace.ChangedFiles(dir, "HEAD")
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestGitChangedFiles_WithChanges(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)

	// Create a new file — unstaged, so it appears in "git diff" of working tree
	// but "git diff HEAD" only sees staged changes, so add it to the index.
	newFile := filepath.Join(dir, "svc-a", "new.go")
	require.NoError(t, os.WriteFile(newFile, []byte("package svc"), 0644))
	require.NoError(t, exec.Command("git", "-C", dir, "add", newFile).Run())

	files, err := workspace.ChangedFiles(dir, "HEAD")
	require.NoError(t, err)
	assert.NotEmpty(t, files)
	assert.Contains(t, files, "svc-a/new.go")
}

func TestGitChangedFiles_NotAGitRepo(t *testing.T) {
	dir := realPath(t, t.TempDir())

	_, err := workspace.ChangedFiles(dir, "HEAD")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// runAffected
// ---------------------------------------------------------------------------

func TestRunAffected_NotGitRepo(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	err := runAffected(affectedCmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git diff")
}

func TestRunAffected_NoChanges(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAffected(affectedCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No changes detected")
}

func TestRunAffected_WithChanges(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)

	// Create a change in svc-a and stage it so git diff HEAD sees it
	newFile := filepath.Join(dir, "svc-a", "new.go")
	require.NoError(t, os.WriteFile(newFile, []byte("package svc"), 0644))
	require.NoError(t, exec.Command("git", "-C", dir, "add", newFile).Run())

	out := captureStdout(t, func() {
		err := runAffected(affectedCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Affected Packages")
	assert.Contains(t, out, "svc-a")
	// svc-b depends on svc-a, so it should be in downstream
	assert.Contains(t, out, "svc-b")
	assert.Contains(t, out, "Downstream")
}

func TestRunAffected_ChangesOutsidePackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)

	// Create a change outside any package and stage it
	readme := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("hello"), 0644))
	require.NoError(t, exec.Command("git", "-C", dir, "add", readme).Run())

	out := captureStdout(t, func() {
		err := runAffected(affectedCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "don't belong to any tracked package")
}

// ---------------------------------------------------------------------------
// buildGraph
// ---------------------------------------------------------------------------

func TestBuildGraph_Empty(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	g := buildGraph(ws)
	assert.Empty(t, g.Nodes())
}

func TestBuildGraph_WithDeps(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	g := buildGraph(ws)
	nodes := g.Nodes()
	assert.Len(t, nodes, 2)

	// svc-b depends on svc-a
	deps := g.DepsOf("svc-b")
	assert.Contains(t, deps, "svc-a")

	// svc-a has no in-workspace deps
	depsA := g.DepsOf("svc-a")
	assert.Empty(t, depsA)
}

// ---------------------------------------------------------------------------
// runBuild / runPhaseCommand
// ---------------------------------------------------------------------------

func TestRunBuild_DryRun(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "--dry-run"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Dry Run: build")
	assert.Contains(t, out, "svc-a")
	assert.Contains(t, out, "svc-b")
}

func TestRunBuild_SpecificPackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "svc-a"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "svc-a")
}

func TestRunBuild_AllPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Running")
	assert.Contains(t, out, "svc-a")
}

func TestRunBuild_NoCache(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "--no-cache"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Running")
}

func TestRunBuild_AffectedNoGit(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	rootCmd.SetArgs([]string{"build", "--affected"})
	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestRunBuild_AffectedNoChanges(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "--affected"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No affected packages")
}

func TestRunBuild_AffectedWithChanges(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	// Create a change and stage it so git diff HEAD sees it
	newFile := filepath.Join(dir, "svc-a", "new.go")
	require.NoError(t, os.WriteFile(newFile, []byte("package svc"), 0644))
	require.NoError(t, exec.Command("git", "-C", dir, "add", newFile).Run())

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "--affected"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Running")
}

func TestRunBuildClean(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	buildDir := filepath.Join(dir, "build")
	require.NoError(t, os.MkdirAll(buildDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(buildDir, "artifact"), []byte("data"), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runBuildClean(buildCleanCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaned")
	assert.NoDirExists(t, buildDir)
}

// ---------------------------------------------------------------------------
// runPhaseCommand — test phase
// ---------------------------------------------------------------------------

func TestRunTest_DryRun(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"test", "--dry-run"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Dry Run: test")
}

func TestRunTest_SpecificPackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"test", "svc-a"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "svc-a")
}

// ---------------------------------------------------------------------------
// runGraph via rootCmd (integration)
// ---------------------------------------------------------------------------

func TestRunGraph_ViaRootCmd(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"graph"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Dependency Graph")
	assert.Contains(t, out, "Level 0")
	assert.Contains(t, out, "no deps")
}

// ---------------------------------------------------------------------------
// runStatus via rootCmd (integration)
// ---------------------------------------------------------------------------

func TestRunStatus_ViaRootCmd(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"status"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Workspace: test-ws")
}

// ---------------------------------------------------------------------------
// env commands via rootCmd (integration)
// ---------------------------------------------------------------------------

func TestEnvSetup_ViaRootCmd(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"env", "setup"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Setting up environments")
}

func TestEnvClean_ViaRootCmd(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"env", "clean"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaning environments")
}

func TestEnvList_ViaRootCmd(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"env", "list"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Environment Status")
}

// ---------------------------------------------------------------------------
// version-set check via rootCmd (integration)
// ---------------------------------------------------------------------------

func TestVersionSetCheck_ViaRootCmd(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	setupVersionSet(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"version-set", "check"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Version Set")
}

func TestVersionSetCheck_ViaAlias(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	setupVersionSet(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"vs", "check"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Version Set")
}

// ---------------------------------------------------------------------------
// affected via rootCmd (integration)
// ---------------------------------------------------------------------------

func TestAffected_ViaRootCmd(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"affected"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No changes detected")
}

// ---------------------------------------------------------------------------
// validate via rootCmd (integration)
// ---------------------------------------------------------------------------

func TestValidate_ViaRootCmd(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"validate"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Validating workspace")
}

// resetBuildFlags resets the bool flags on the build command so that state
// from a prior rootCmd.Execute() call does not leak into the next test.
func resetBuildFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"affected", "no-cache", "dry-run"} {
		f := buildCmd.Flags().Lookup(name)
		if f != nil {
			f.Value.Set("false")
			f.Changed = false
		}
	}
	for _, name := range []string{"affected", "no-cache", "dry-run"} {
		f := testCmd.Flags().Lookup(name)
		if f != nil {
			f.Value.Set("false")
			f.Changed = false
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases and additional coverage
// ---------------------------------------------------------------------------

func TestRunEnvSetup_FailingCommand(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	pkgDir := filepath.Join(dir, "fail-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	cfg := `package:
  name: fail-pkg
  version: 0.1.0
runtime:
  setup:
    - exit 1
  env:
    X: y
phases:
  build:
    commands:
      - echo hi
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvSetup(envSetupCmd, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "env setup for fail-pkg failed")
	})
	_ = out
}

func TestRunGraph_WithMultipleLevels(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	// Create 3 packages in a chain: c -> b -> a
	for _, pkg := range []struct {
		name string
		deps string
	}{
		{"pkg-a", ""},
		{"pkg-b", "  - pkg-a"},
		{"pkg-c", "  - pkg-b"},
	} {
		pkgDir := filepath.Join(dir, pkg.name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		depSection := ""
		if pkg.deps != "" {
			depSection = "dependencies:\n" + pkg.deps + "\n"
		}
		cfg := "package:\n  name: " + pkg.name + "\n  version: 0.1.0\n" + depSection +
			"phases:\n  build:\n    commands:\n      - echo building\n"
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	}
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runGraph(graphCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Level 0")
	assert.Contains(t, out, "Level 1")
	assert.Contains(t, out, "Level 2")
	assert.Contains(t, out, "3 packages")
	assert.Contains(t, out, "3 levels")
}

func TestRunStatus_PackageWithDepsAndRuntime(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	pkgDir := filepath.Join(dir, "full-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	cfg := `package:
  name: full-pkg
  version: 1.0.0
dependencies:
  - some-dep
runtime:
  setup:
    - mkdir -p {{env_dir}}
  env:
    FOO: bar
phases:
  build:
    commands:
      - echo building
  test:
    commands:
      - echo testing
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	// Should show deps, phases, and runtime in the package info line
	assert.Contains(t, out, "full-pkg")
	assert.Contains(t, out, "deps: some-dep")
	assert.Contains(t, out, "phases: build, test")
	assert.Contains(t, out, "runtime")
}

func TestRunBuild_BuildThenCachedRun(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	// First build
	out1 := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "svc-a"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out1, "svc-a")

	resetBuildFlags(t)
	// Second build should hit cache
	out2 := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "svc-a"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out2, "cached")
}

func TestRunBuild_DryRunShowsPre(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir) // svc-a has pre commands
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "--dry-run", "svc-a"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "pre:")
	assert.Contains(t, out, "echo pre-build")
	assert.Contains(t, out, "post:")
	assert.Contains(t, out, "echo post-build")
	assert.Contains(t, out, "cmd:")
	assert.Contains(t, out, "echo building-a")
}

func TestVersionSetCheck_NoStrategy(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	vs := `version-set:
  name: minimal-vs
  packages:
    go: "1.22"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, workspace.VersionsFile), []byte(vs), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runVersionSetCheck(versionSetCheckCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Version Set: minimal-vs")
	assert.NotContains(t, out, "Strategy:")
	assert.Contains(t, out, "1 pinned dependency")
}

func TestGitChangedFiles_FallbackToWorkingTree(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)

	// Use an invalid ref that will fail and trigger fallback
	files, err := workspace.ChangedFiles(dir, "nonexistent-ref-xyz")
	require.NoError(t, err)
	// Should fall back to working tree diff which is clean
	assert.Empty(t, files)
}

// ---------------------------------------------------------------------------
// docs commands — additional coverage
// ---------------------------------------------------------------------------

func TestRunDocsGenerate_WithPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runDocsGenerate(docsGenerateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Generating Documentation")
	assert.Contains(t, out, "commands.md")
	assert.Contains(t, out, "config-reference.md")
	assert.Contains(t, out, "config-reference.md")
	assert.Contains(t, out, "packages.md")
	// Verify files were created
	assert.FileExists(t, filepath.Join(dir, "docs", "user", "commands.md"))
	assert.FileExists(t, filepath.Join(dir, "docs", "user", "packages.md"))
}

func TestRunDocsHookInstall_WithGitRepo(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runDocsHookInstall(docsHookInstallCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Installed pre-commit hook")
	assert.FileExists(t, filepath.Join(dir, ".git", "hooks", "pre-commit"))
}

func TestRunDocsHookInstall_NoGitDir(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	err := runDocsHookInstall(docsHookInstallCmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestRunDocsHookRemove_NoHook(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runDocsHookRemove(docsHookRemoveCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No pre-commit hook")
}

func TestRunDocsHookRemove_WithHook(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)

	// Install hook first
	hookDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hookDir, "pre-commit"), []byte("#!/bin/sh\n"), 0755))

	out := captureStdout(t, func() {
		err := runDocsHookRemove(docsHookRemoveCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Removed pre-commit hook")
	assert.NoFileExists(t, filepath.Join(hookDir, "pre-commit"))
}

// ---------------------------------------------------------------------------
// agent helpers — additional coverage
// ---------------------------------------------------------------------------

func TestSetupAgentConfig_NoneAgent_CreatesNoFiles(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("none")
	require.NotNil(t, agent)

	require.NoError(t, setupAgentConfig(dir, agent))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "none agent must not create any files")
}

func TestSetupAgentConfig_CreateNew(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("claude")
	require.NotNil(t, agent)

	err := setupAgentConfig(dir, agent)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "CLAUDE.md"))

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), includeLine)
}

func TestSetupAgentConfig_AppendToExisting(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("claude")
	require.NotNil(t, agent)

	// Create existing file without the include line
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Existing content\n"), 0644))

	err := setupAgentConfig(dir, agent)
	assert.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Existing content")
	assert.Contains(t, string(data), includeLine)
}

func TestSetupAgentConfig_AlreadyIncluded(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("claude")
	require.NotNil(t, agent)

	// Create existing file with the include line already present
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(includeLine+"\n"), 0644))

	err := setupAgentConfig(dir, agent)
	assert.NoError(t, err)

	// File should not be modified (no duplicate)
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, includeLine+"\n", string(data))
}

func TestSetupAgentConfig_CursorCreatesSubdir(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("cursor")
	require.NotNil(t, agent)

	err := setupAgentConfig(dir, agent)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, ".cursor", "rules"))
}

func TestWriteTakumiMD_WritesFileWithWorkspaceName(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))

	require.NoError(t, writeTakumiMD(dir, "my-workspace"))

	data, err := os.ReadFile(filepath.Join(dir, ".takumi", "TAKUMI.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "my-workspace")
}

func TestAgentByName_ReturnsCorrectFilePath(t *testing.T) {
	cases := map[string]string{
		"claude":   "CLAUDE.md",
		"cursor":   ".cursor/rules",
		"copilot":  ".github/copilot-instructions.md",
		"windsurf": ".windsurfrules",
		"cline":    ".clinerules",
		"kiro":     "AGENTS.md",
		"none":     "",
	}
	for name, wantPath := range cases {
		got := AgentByName(name)
		require.NotNil(t, got, "AgentByName(%q) returned nil", name)
		assert.Equal(t, name, got.Name)
		assert.Equal(t, wantPath, got.FilePath, "FilePath for %q", name)
	}
	assert.Nil(t, AgentByName("nonexistent"))
}

func TestAgentNames_ReturnsAllSupportedAgents(t *testing.T) {
	got := agentNames()
	assert.Equal(t, "claude, cursor, copilot, windsurf, cline, kiro, none", got)
}

// ---------------------------------------------------------------------------
// runEnvClean — error path (read-only dir)
// ---------------------------------------------------------------------------

func TestRunEnvClean_AllPackagesNoFilter(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	// Set up multiple env dirs
	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "rt-pkg")
	require.NoError(t, os.MkdirAll(envDir, 0755))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvClean(envCleanCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaned")
}

// ---------------------------------------------------------------------------
// runPhaseCommand with failed build
// ---------------------------------------------------------------------------

func TestRunBuild_FailingCommand(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	pkgDir := filepath.Join(dir, "fail-build")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	cfg := `package:
  name: fail-build
  version: 0.1.0
phases:
  build:
    commands:
      - exit 1
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build"})
		err := rootCmd.Execute()
		assert.Error(t, err)
	})
	assert.Contains(t, out, "fail-build")
}

// ---------------------------------------------------------------------------
// runValidate with warnings-only findings
// ---------------------------------------------------------------------------

func TestRunValidate_WarningsOnly(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	// Create a package with a warning (empty commands)
	pkgDir := filepath.Join(dir, "warn-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	cfg := `package:
  name: warn-pkg
  version: 0.1.0
phases:
  build:
    commands: []
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.NoError(t, err) // warnings are not errors
	})
	assert.Contains(t, out, "warning")
}

// ---------------------------------------------------------------------------
// validate — cycle detection
// ---------------------------------------------------------------------------

func TestRunValidate_CycleDetection(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	// Create two packages that depend on each other → cycle
	for _, spec := range []struct {
		name, dep string
	}{
		{"cycle-a", "cycle-b"},
		{"cycle-b", "cycle-a"},
	} {
		pkgDir := filepath.Join(dir, spec.name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		cfg := "package:\n  name: " + spec.name + "\n  version: 0.1.0\n" +
			"dependencies:\n  - " + spec.dep + "\n" +
			"phases:\n  build:\n    commands:\n      - echo hi\n"
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	}
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err)
	})
	assert.Contains(t, out, "cycle detected")
}

func TestRunValidate_ErrorsOnly(t *testing.T) {
	dir := realPath(t, t.TempDir())
	// Workspace with a bad agent name should produce an error
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.AI.Agent = "bad-agent"
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err) // Errors cause failure
	})
	assert.Contains(t, out, "error")
}

func TestRunValidate_ErrorsAndWarnings(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.AI.Agent = "bad-agent"
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))

	// Also create a package with warnings (empty commands)
	pkgDir := filepath.Join(dir, "warn-pkg2")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	pkgCfg := `package:
  name: warn-pkg2
  version: 0.1.0
phases:
  build:
    commands: []
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(pkgCfg), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err)
	})
	assert.Contains(t, out, "error")
	assert.Contains(t, out, "warning")
}

// ---------------------------------------------------------------------------
// printDryRun — cached entry path
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// runCmd (run.go) — custom phase execution
// ---------------------------------------------------------------------------

func TestRunCmd_CustomPhase(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	// Reset run cmd flags too
	for _, name := range []string{"affected", "no-cache", "dry-run"} {
		f := runCmd.Flags().Lookup(name)
		if f != nil {
			f.Value.Set("false")
			f.Changed = false
		}
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"run", "build"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Running")
}

func TestRunCmd_DryRun(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	for _, name := range []string{"affected", "no-cache", "dry-run"} {
		f := runCmd.Flags().Lookup(name)
		if f != nil {
			f.Value.Set("false")
			f.Changed = false
		}
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"run", "--dry-run", "build"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Dry Run")
}

func TestRunCmd_WithPackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	for _, name := range []string{"affected", "no-cache", "dry-run"} {
		f := runCmd.Flags().Lookup(name)
		if f != nil {
			f.Value.Set("false")
			f.Changed = false
		}
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"run", "build", "svc-a"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "svc-a")
}

// ---------------------------------------------------------------------------
// printDryRun — cached entry path
// ---------------------------------------------------------------------------

func TestPrintDryRun_WithCachedEntry(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	// First, do a real build to populate cache
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build", "svc-a"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})

	resetBuildFlags(t)
	ws, err := loadWorkspace()
	require.NoError(t, err)

	// Now dry-run should show "cached"
	out := captureStdout(t, func() {
		err := printDryRun(ws, []string{"svc-a"}, "build", false)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "cached")
}

// ---------------------------------------------------------------------------
// genCommandIndex — branch coverage
// ---------------------------------------------------------------------------

func TestGenCommandIndex_HiddenCommand(t *testing.T) {
	var buf strings.Builder
	cmd := &cobra.Command{Use: "secret", Short: "hidden cmd", Hidden: true}
	genCommandIndex(&buf, cmd, "")
	assert.Empty(t, buf.String(), "hidden command should produce no output")
}

func TestGenCommandIndex_RunnableCommand(t *testing.T) {
	var buf strings.Builder
	cmd := &cobra.Command{
		Use:   "flagged",
		Short: "does things with flags",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	genCommandIndex(&buf, cmd, "takumi")
	out := buf.String()
	assert.Contains(t, out, "`takumi flagged`")
	assert.Contains(t, out, "does things with flags")
	assert.Contains(t, out, "commands/takumi_flagged.md")
}

func TestGenCommandIndex_NonRunnableParent(t *testing.T) {
	var buf strings.Builder
	parent := &cobra.Command{Use: "group", Short: "a group of commands"}
	child := &cobra.Command{
		Use:   "action",
		Short: "does the action",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	parent.AddCommand(child)
	genCommandIndex(&buf, parent, "takumi")
	out := buf.String()
	// Child should appear, parent should not (it's not runnable)
	assert.Contains(t, out, "`takumi group action`")
	assert.NotContains(t, out, "`takumi group`](")
}

func TestGenCommandIndex_NoPrefix(t *testing.T) {
	var buf strings.Builder
	cmd := &cobra.Command{
		Use:   "standalone",
		Short: "a standalone command",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	genCommandIndex(&buf, cmd, "")
	out := buf.String()
	assert.Contains(t, out, "`standalone`")
	assert.Contains(t, out, "a standalone command")
}

func TestGenCommandIndex_SkipsHelp(t *testing.T) {
	var buf strings.Builder
	cmd := &cobra.Command{
		Use:   "help",
		Short: "Help about any command",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	genCommandIndex(&buf, cmd, "takumi")
	assert.Empty(t, buf.String(), "help command should not appear in index")
}

// ---------------------------------------------------------------------------
// setupAgentConfig — error paths
// ---------------------------------------------------------------------------

func TestSetupAgentConfig_AppendReadOnly(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("claude")
	require.NotNil(t, agent)

	// Create existing file without include, then make it read-only
	filePath := filepath.Join(dir, agent.FilePath)
	require.NoError(t, os.WriteFile(filePath, []byte("# Existing\n"), 0444))
	t.Cleanup(func() { os.Chmod(filePath, 0644) })

	err := setupAgentConfig(dir, agent)
	assert.Error(t, err, "should fail when file is read-only and needs appending")
}

func TestWriteTakumiMD_DirNotWritable(t *testing.T) {
	dir := t.TempDir()
	takumiDir := filepath.Join(dir, ".takumi")
	require.NoError(t, os.MkdirAll(takumiDir, 0555))
	t.Cleanup(func() { os.Chmod(takumiDir, 0755) })

	err := writeTakumiMD(dir, "test")
	assert.Error(t, err, "should fail when .takumi/ is not writable")
}

// ---------------------------------------------------------------------------
// repoNameFromURL — SSH-style URL with colon
// ---------------------------------------------------------------------------

func TestRepoNameFromURL_SSHStyle(t *testing.T) {
	assert.Equal(t, "my-repo", repoNameFromURL("git@github.com:org/my-repo.git"))
}

func TestRepoNameFromURL_PlainName(t *testing.T) {
	// URL with no path separators or colons
	assert.Equal(t, "bare", repoNameFromURL("bare"))
}

// ---------------------------------------------------------------------------
// runRemove — delete with missing path + env cleanup warning
// ---------------------------------------------------------------------------

func TestRunRemove_DeleteMissingPath(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	// Add a source with a path that doesn't exist on disk
	ws, err := loadWorkspace()
	require.NoError(t, err)
	if ws.Config.Workspace.Sources == nil {
		ws.Config.Workspace.Sources = make(map[string]config.Source)
	}
	ws.Config.Workspace.Sources["phantom"] = config.Source{
		URL:  "https://example.com/phantom.git",
		Path: "./phantom",
	}
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, "takumi.yaml"), ws.Config))

	removeCmd.Flags().Set("delete", "true")
	t.Cleanup(func() { removeCmd.Flags().Set("delete", "false") })

	out := captureStdout(t, func() {
		err := runRemove(removeCmd, []string{"phantom"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Removed")
	// Should not contain "Deleted" since the path doesn't exist
	assert.NotContains(t, out, "Deleted")
}

// ---------------------------------------------------------------------------
// runEnvClean — with filter arg
// ---------------------------------------------------------------------------

func TestRunEnvClean_FilteredPackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	envsDir := filepath.Join(dir, workspace.MarkerDir, "envs")
	require.NoError(t, os.MkdirAll(filepath.Join(envsDir, "rt-pkg"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(envsDir, "other"), 0755))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvClean(envCleanCmd, []string{"rt-pkg"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaned")
}

// ---------------------------------------------------------------------------
// validate — version-set file not found + parse error paths
// ---------------------------------------------------------------------------

func TestRunValidate_VersionSetFileNotFound(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	// Add version-set config pointing to a nonexistent file
	ws, err := loadWorkspace()
	require.NoError(t, err)
	ws.Config.Workspace.VersionSet.File = "takumi-versions.yaml"
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, "takumi.yaml"), ws.Config))

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		// validate returns error when there are errors
		assert.Error(t, err)
	})
	assert.Contains(t, out, "takumi-versions.yaml")
	assert.Contains(t, out, "file not found")
}

func TestRunValidate_VersionSetParseError(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)
	ws.Config.Workspace.VersionSet.File = "takumi-versions.yaml"
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, "takumi.yaml"), ws.Config))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi-versions.yaml"), []byte("{{broken yaml}}"), 0644))

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err)
	})
	assert.Contains(t, out, "takumi-versions.yaml")
}

func TestRunValidate_VersionSetWithWarnings(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)
	ws.Config.Workspace.VersionSet.File = "takumi-versions.yaml"
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, "takumi.yaml"), ws.Config))
	// Valid but with warnings (empty packages, invalid strategy)
	vsYAML := "version-set:\n  name: release\n  strategy: invalid-strat\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi-versions.yaml"), []byte(vsYAML), 0644))

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err, "invalid strategy should produce an error")
	})
	assert.Contains(t, out, "takumi-versions.yaml")
}

func TestRunValidate_VersionSetValid(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)
	ws.Config.Workspace.VersionSet.File = "takumi-versions.yaml"
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, "takumi.yaml"), ws.Config))
	vsYAML := "version-set:\n  name: release\n  strategy: strict\n  packages:\n    react: \"18.0.0\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi-versions.yaml"), []byte(vsYAML), 0644))

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "valid")
}

// ---------------------------------------------------------------------------
// initPackageInDir — already exists
// ---------------------------------------------------------------------------

func TestInitPackageInDir_AlreadyExists(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	pkgDir := filepath.Join(dir, "existing")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte("package:\n  name: existing\n"), 0644))

	err := initPackageInDir(pkgDir, "existing", dir, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// ---------------------------------------------------------------------------
// env commands — additional branches
// ---------------------------------------------------------------------------

func TestRunEnvList_WithRuntime(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvList(envListCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Environment Status")
	assert.Contains(t, out, "not set up")
}

func TestRunEnvList_WithReadyEnv(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "rt-pkg")
	require.NoError(t, os.MkdirAll(envDir, 0755))

	out := captureStdout(t, func() {
		err := runEnvList(envListCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "ready")
}

func TestRunEnvClean_NoEnvs(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvClean(envCleanCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No environments to clean")
}

// ---------------------------------------------------------------------------
// runBuildClean — non-existent build dir (still succeeds)
// ---------------------------------------------------------------------------

func TestRunBuildClean_NoBuildDir(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runBuildClean(buildCleanCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaned")
}

// ---------------------------------------------------------------------------
// runPhaseCommand — skipped packages (no phase defined)
// ---------------------------------------------------------------------------

func TestRunPhaseCommand_SkippedPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	pkgDir := filepath.Join(dir, "no-build")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	cfg := "package:\n  name: no-build\n  version: 0.1.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "passed")
}

// ---------------------------------------------------------------------------
// runPhaseCommand — all cached (second run)
// ---------------------------------------------------------------------------

func TestRunPhaseCommand_AllCached(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})

	resetBuildFlags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build"})
		err := rootCmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "cached")
}

// ---------------------------------------------------------------------------
// joinParts
// ---------------------------------------------------------------------------

func TestJoinParts_Empty(t *testing.T) {
	assert.Equal(t, "nothing to do", joinParts(nil))
}

func TestJoinParts_Multiple(t *testing.T) {
	assert.Equal(t, "a, b, c", joinParts([]string{"a", "b", "c"}))
}

// ---------------------------------------------------------------------------
// runRemove — env cleanup when env dir exists
// ---------------------------------------------------------------------------

func TestRunRemove_CleansEnvDir(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)
	if ws.Config.Workspace.Sources == nil {
		ws.Config.Workspace.Sources = make(map[string]config.Source)
	}
	ws.Config.Workspace.Sources["with-env"] = config.Source{
		URL:  "https://example.com/repo.git",
		Path: "./with-env",
	}
	require.NoError(t, config.SaveWorkspaceConfig(filepath.Join(dir, "takumi.yaml"), ws.Config))

	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "with-env")
	require.NoError(t, os.MkdirAll(envDir, 0755))

	removeCmd.Flags().Set("delete", "false")

	out := captureStdout(t, func() {
		err := runRemove(removeCmd, []string{"with-env"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaned up")
	assert.Contains(t, out, "Removed")
}

// ---------------------------------------------------------------------------
// runDocsGenerate — minimal workspace
// ---------------------------------------------------------------------------

func TestRunDocsGenerate_MinimalWorkspace(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runDocsGenerate(docsGenerateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Generating Documentation")
	assert.Contains(t, out, "commands.md")
}

// ---------------------------------------------------------------------------
// repoNameFromURL — additional patterns
// ---------------------------------------------------------------------------

func TestRepoNameFromURL_SingleWord(t *testing.T) {
	assert.Equal(t, "myrepo", repoNameFromURL("myrepo"))
}

func TestRepoNameFromURL_WithGitSuffix(t *testing.T) {
	assert.Equal(t, "myrepo", repoNameFromURL("myrepo.git"))
}

func TestRepoNameFromURL_SSHWithPath(t *testing.T) {
	// git@github.com:org/my-repo.git → last "/" gives "my-repo", then ":" is not present
	assert.Equal(t, "my-repo", repoNameFromURL("git@github.com:org/my-repo.git"))
}

func TestRepoNameFromURL_SSHNoSlash(t *testing.T) {
	// git@host:myrepo.git → no "/", so colon handling gives "myrepo"
	assert.Equal(t, "myrepo", repoNameFromURL("git@host:myrepo.git"))
}

// ---------------------------------------------------------------------------
// setupAgentConfig — all branches
// ---------------------------------------------------------------------------

func TestSetupAgentConfig_NewFile(t *testing.T) {
	dir := realPath(t, t.TempDir())
	agent := AgentByName("claude")
	require.NotNil(t, agent)

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, agent.FilePath))
	require.NoError(t, err)
	assert.Contains(t, string(data), includeLine)
}

func TestSetupAgentConfig_ExistingWithoutInclude(t *testing.T) {
	dir := realPath(t, t.TempDir())
	agent := AgentByName("claude")

	// Write an existing file without the include line
	require.NoError(t, os.WriteFile(filepath.Join(dir, agent.FilePath), []byte("# My project\n"), 0644))

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, agent.FilePath))
	require.NoError(t, err)
	assert.Contains(t, string(data), "# My project")
	assert.Contains(t, string(data), includeLine)
}

func TestSetupAgentConfig_ExistingWithInclude(t *testing.T) {
	dir := realPath(t, t.TempDir())
	agent := AgentByName("claude")

	// Write file that already has the include line
	require.NoError(t, os.WriteFile(filepath.Join(dir, agent.FilePath), []byte(includeLine+"\n"), 0644))

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	// Should not duplicate the line
	data, _ := os.ReadFile(filepath.Join(dir, agent.FilePath))
	assert.Equal(t, 1, strings.Count(string(data), includeLine))
}
func TestSetupAgentConfig_SubdirAgent(t *testing.T) {
	// copilot agent has .github/ subdir
	dir := realPath(t, t.TempDir())
	agent := AgentByName("copilot")
	require.NotNil(t, agent)

	err := setupAgentConfig(dir, agent)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, agent.FilePath))
	require.NoError(t, err)
	assert.Contains(t, string(data), includeLine)
}

// ---------------------------------------------------------------------------
// runDocsHookInstall / runDocsHookRemove — branches
// ---------------------------------------------------------------------------

func TestRunDocsHookInstall_Success(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	// Create .git directory so it's recognized as git repo
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0755))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runDocsHookInstall(docsHookInstallCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Installed pre-commit hook")

	// Verify hook file exists and is executable
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	info, err := os.Stat(hookPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0111 != 0, "hook should be executable")
}

func TestRunDocsHookInstall_NotGitRepo(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	err := runDocsHookInstall(docsHookInstallCmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}
// ---------------------------------------------------------------------------

func TestRunEnvSetup_NoRuntimePackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir) // no runtime defined
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvSetup(envSetupCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages define a runtime section")
}
func TestRunEnvSetup_FilteredPackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvSetup(envSetupCmd, []string{"nonexistent"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages define a runtime section")
}

// ---------------------------------------------------------------------------
// runEnvClean — env dir exists with content
// ---------------------------------------------------------------------------

func TestRunEnvClean_WithEnvDir(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "svc-a")
	require.NoError(t, os.MkdirAll(envDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(envDir, "state"), []byte("x"), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runEnvClean(envCleanCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaned")
	assert.NoDirExists(t, envDir)
}
// ---------------------------------------------------------------------------
// runBuildClean — with existing build dir
// ---------------------------------------------------------------------------

func TestRunBuildClean_WithBuildDir(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	buildPath := filepath.Join(dir, "build")
	require.NoError(t, os.MkdirAll(buildPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(buildPath, "out.bin"), []byte("binary"), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runBuildClean(buildCleanCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Cleaned")
	assert.NoDirExists(t, buildPath)
}

// ---------------------------------------------------------------------------
// runPhaseCommand — dry-run flag
// ---------------------------------------------------------------------------

func TestRunPhaseCommand_DryRun(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	buildCmd.Flags().Set("dry-run", "true")
	t.Cleanup(func() { buildCmd.Flags().Set("dry-run", "false") })

	out := captureStdout(t, func() {
		err := runPhaseCommand(buildCmd, nil, "build")
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Dry Run")
	assert.Contains(t, out, "svc-a")
	assert.Contains(t, out, "will run")
}

func TestRunPhaseCommand_DryRunCached(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	// First run to populate cache
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"build"})
		rootCmd.Execute()
	})

	resetBuildFlags(t)
	buildCmd.Flags().Set("dry-run", "true")
	t.Cleanup(func() { buildCmd.Flags().Set("dry-run", "false") })

	out := captureStdout(t, func() {
		err := runPhaseCommand(buildCmd, nil, "build")
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Dry Run")
	assert.Contains(t, out, "cached")
}

func TestRunPhaseCommand_NoCache(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	buildCmd.Flags().Set("no-cache", "true")
	t.Cleanup(func() { buildCmd.Flags().Set("no-cache", "false") })

	out := captureStdout(t, func() {
		err := runPhaseCommand(buildCmd, nil, "build")
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Running build")
}

func TestRunPhaseCommand_WithSpecificPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	out := captureStdout(t, func() {
		err := runPhaseCommand(buildCmd, []string{"svc-a"}, "build")
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "1 package")
}

// ---------------------------------------------------------------------------
// runPhaseCommand — affected flag (in git workspace)
// ---------------------------------------------------------------------------

func TestRunPhaseCommand_AffectedNoChanges(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)
	resetBuildFlags(t)

	buildCmd.Flags().Set("affected", "true")
	t.Cleanup(func() { buildCmd.Flags().Set("affected", "false") })

	out := captureStdout(t, func() {
		err := runPhaseCommand(buildCmd, nil, "build")
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No affected packages")
}

func TestRunPhaseCommand_AffectedWithChanges(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)

	// Modify an existing tracked file in svc-a (new untracked files don't show in git diff HEAD)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "svc-a", "takumi-pkg.yaml"),
		[]byte("package:\n  name: svc-a\n  version: 0.1.1\nphases:\n  build:\n    commands:\n      - echo building-a\n"), 0644))

	chdirClean(t, dir)
	resetBuildFlags(t)

	buildCmd.Flags().Set("affected", "true")
	t.Cleanup(func() { buildCmd.Flags().Set("affected", "false") })

	out := captureStdout(t, func() {
		err := runPhaseCommand(buildCmd, nil, "build")
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Running build")
}

// ---------------------------------------------------------------------------
// runValidate — all branches
// ---------------------------------------------------------------------------

func TestRunValidate_AllValid(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "valid")
	assert.Contains(t, out, "no cycles")
}

func TestRunValidate_WithVersionSet(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	setupVersionSet(t, dir)

	// Point workspace config to version-set file
	ws, _ := workspace.Load(dir)
	ws.Config.Workspace.VersionSet.File = workspace.VersionsFile
	config.SaveWorkspaceConfig(filepath.Join(dir, "takumi.yaml"), ws.Config)

	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "valid")
}

func TestRunValidate_UnresolvedDep(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	pkgDir := filepath.Join(dir, "broken")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	cfg := "package:\n  name: broken\n  version: 0.1.0\ndependencies:\n  - nonexistent\n"
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.NoError(t, err) // warnings don't cause error
	})
	assert.Contains(t, out, "nonexistent")
}

func TestRunValidate_CyclicDeps(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	// a depends on b, b depends on a
	for _, pkg := range []struct{ name, dep string }{
		{"pkg-a", "pkg-b"},
		{"pkg-b", "pkg-a"},
	} {
		pkgDir := filepath.Join(dir, pkg.name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		cfg := "package:\n  name: " + pkg.name + "\n  version: 0.1.0\ndependencies:\n  - " + pkg.dep + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(cfg), 0644))
	}
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err)
	})
	assert.Contains(t, out, "cycle")
}

func TestRunValidate_WithParseErrors(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	// Create a malformed takumi-pkg.yaml
	pkgDir := filepath.Join(dir, "bad")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte("{{bad yaml"), 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runValidate(validateCmd, nil)
		assert.Error(t, err)
	})
	_ = out // parse errors are printed
}

// ---------------------------------------------------------------------------
// runStatus — all branches
// ---------------------------------------------------------------------------

func TestRunStatus_FullDashboard(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)

	// Set up agent
	ws, _ := workspace.Load(dir)
	ws.Config.Workspace.AI.Agent = "claude"
	config.SaveWorkspaceConfig(filepath.Join(dir, "takumi.yaml"), ws.Config)

	// Write metrics
	writeMetrics(t, dir, []executor.MetricsEntry{
		{Timestamp: "2025-01-01T00:00:00Z", Phase: "build", Package: "rt-pkg", DurationMs: 250, ExitCode: 0},
		{Timestamp: "2025-01-01T00:01:00Z", Phase: "test", Package: "rt-pkg", DurationMs: 100, ExitCode: 1},
	})

	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Workspace: test-ws")
	assert.Contains(t, out, "rt-pkg")
	assert.Contains(t, out, "Environments")
	assert.Contains(t, out, "not set up")
	assert.Contains(t, out, "Recent Builds")
	assert.Contains(t, out, "claude")
}
func TestRunStatus_MissingSource(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithSource(t, dir, "missing-repo", config.Source{
		URL:    "https://example.com/repo.git",
		Branch: "main",
		Path:   "./missing-repo",
	})
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "missing")
}

func TestRunStatus_NoPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runStatus(statusCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages found")
}

// ---------------------------------------------------------------------------
// runInit — branches
// ---------------------------------------------------------------------------

func TestRunInit_WithRootFlag(t *testing.T) {
	dir := realPath(t, t.TempDir())
	chdirClean(t, dir)

	// Mock agent selection
	orig := promptAgentSelection
	promptAgentSelection = func() (*AgentType, error) { return AgentByName("none"), nil }
	t.Cleanup(func() { promptAgentSelection = orig })

	initCmd.Flags().Set("root", "my-project")
	t.Cleanup(func() { initCmd.Flags().Set("root", "") })

	out := captureStdout(t, func() {
		err := runInit(initCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Initializing workspace")
	assert.FileExists(t, filepath.Join(dir, "my-project", "takumi.yaml"))
	assert.FileExists(t, filepath.Join(dir, "my-project", "takumi-pkg.yaml"))
}

func TestRunInit_WithRootAndPackageName(t *testing.T) {
	dir := realPath(t, t.TempDir())
	chdirClean(t, dir)

	orig := promptAgentSelection
	promptAgentSelection = func() (*AgentType, error) { return AgentByName("none"), nil }
	t.Cleanup(func() { promptAgentSelection = orig })

	initCmd.Flags().Set("root", "proj2")
	t.Cleanup(func() { initCmd.Flags().Set("root", "") })

	out := captureStdout(t, func() {
		err := runInit(initCmd, []string{"my-svc"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Initializing workspace")
	assert.FileExists(t, filepath.Join(dir, "proj2", "my-svc", "takumi-pkg.yaml"))
}
func TestRunInit_InvalidAgent(t *testing.T) {
	dir := realPath(t, t.TempDir())
	chdirClean(t, dir)

	initCmd.Flags().Set("root", "bad-agent")
	initCmd.Flags().Set("agent", "invalid-agent")
	t.Cleanup(func() {
		initCmd.Flags().Set("root", "")
		initCmd.Flags().Set("agent", "")
	})

	err := runInit(initCmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestRunInit_NamedSubdir(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runInit(initCmd, []string{"new-pkg"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "new-pkg")
	assert.FileExists(t, filepath.Join(dir, "new-pkg", "takumi-pkg.yaml"))
}

func TestRunInit_InCurrentDir(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runInit(initCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Initialized package")
	assert.FileExists(t, filepath.Join(dir, "takumi-pkg.yaml"))
}

func TestRunInit_NoWorkspaceCreatesOne(t *testing.T) {
	dir := realPath(t, t.TempDir())
	chdirClean(t, dir)

	orig := promptAgentSelection
	promptAgentSelection = func() (*AgentType, error) { return AgentByName("none"), nil }
	t.Cleanup(func() { promptAgentSelection = orig })

	out := captureStdout(t, func() {
		err := runInit(initCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Initializing workspace")
	assert.FileExists(t, filepath.Join(dir, "takumi.yaml"))
	assert.FileExists(t, filepath.Join(dir, "takumi-pkg.yaml"))
}

// ---------------------------------------------------------------------------
// resolveAgent branches
// ---------------------------------------------------------------------------

func TestResolveAgent_ValidFlag(t *testing.T) {
	initCmd.Flags().Set("agent", "cursor")
	t.Cleanup(func() { initCmd.Flags().Set("agent", "") })

	agent, err := resolveAgent(initCmd)
	require.NoError(t, err)
	assert.Equal(t, "cursor", agent.Name)
}

func TestResolveAgent_InvalidFlag(t *testing.T) {
	initCmd.Flags().Set("agent", "bad")
	t.Cleanup(func() { initCmd.Flags().Set("agent", "") })

	_, err := resolveAgent(initCmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestResolveAgent_Interactive(t *testing.T) {
	initCmd.Flags().Set("agent", "")

	orig := promptAgentSelection
	promptAgentSelection = func() (*AgentType, error) { return AgentByName("windsurf"), nil }
	t.Cleanup(func() { promptAgentSelection = orig })

	agent, err := resolveAgent(initCmd)
	require.NoError(t, err)
	assert.Equal(t, "windsurf", agent.Name)
}
func TestPrintDryRun_TargetedPackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	out := captureStdout(t, func() {
		err := printDryRun(ws, []string{"svc-a"}, "build", false)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "svc-a")
	assert.NotContains(t, out, "svc-b")
}
// ---------------------------------------------------------------------------
// runRemove — delete flag with existing disk path
// ---------------------------------------------------------------------------

func TestRunRemove_DeleteExistingPath(t *testing.T) {
	dir := realPath(t, t.TempDir())
	repoDir := filepath.Join(dir, "my-repo")
	require.NoError(t, os.MkdirAll(repoDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("x"), 0644))

	setupWorkspaceWithSource(t, dir, "my-repo", config.Source{
		URL:    "https://example.com/repo.git",
		Branch: "main",
		Path:   "./my-repo",
	})
	chdirClean(t, dir)

	removeCmd.Flags().Set("delete", "true")
	t.Cleanup(func() { removeCmd.Flags().Set("delete", "false") })

	out := captureStdout(t, func() {
		err := runRemove(removeCmd, []string{"my-repo"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Deleted")
	assert.Contains(t, out, "Removed")
	assert.NoDirExists(t, repoDir)
}

func TestRunRemove_DeleteAbsPath(t *testing.T) {
	dir := realPath(t, t.TempDir())
	absDir := filepath.Join(dir, "abs-repo")
	require.NoError(t, os.MkdirAll(absDir, 0755))

	setupWorkspaceWithSource(t, dir, "abs-repo", config.Source{
		URL:    "https://example.com/repo.git",
		Branch: "main",
		Path:   absDir, // absolute path
	})
	chdirClean(t, dir)

	removeCmd.Flags().Set("delete", "true")
	t.Cleanup(func() { removeCmd.Flags().Set("delete", "false") })

	out := captureStdout(t, func() {
		err := runRemove(removeCmd, []string{"abs-repo"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Deleted")
}

func TestAgentByName_NotFound(t *testing.T) {
	assert.Nil(t, AgentByName("nonexistent"))
}

func TestAgentByName_AllAgents(t *testing.T) {
	for _, agent := range SupportedAgents {
		found := AgentByName(agent.Name)
		require.NotNil(t, found, "agent %q should be found", agent.Name)
		assert.Equal(t, agent.Name, found.Name)
	}
}

// ---------------------------------------------------------------------------
// agentNames
// ---------------------------------------------------------------------------
func TestPrintFindings_Errors(t *testing.T) {
	findings := []config.Finding{
		{Severity: config.SeverityError, Field: "name", Message: "required"},
	}
	out := captureStdout(t, func() {
		e, w := printFindings("test.yaml", findings, 0, 0)
		assert.Equal(t, 1, e)
		assert.Equal(t, 0, w)
	})
	assert.Contains(t, out, "required")
}

func TestPrintFindings_WarningsOnly(t *testing.T) {
	findings := []config.Finding{
		{Severity: config.SeverityWarning, Field: "version", Message: "empty"},
	}
	out := captureStdout(t, func() {
		e, w := printFindings("test.yaml", findings, 0, 0)
		assert.Equal(t, 0, e)
		assert.Equal(t, 1, w)
	})
	assert.Contains(t, out, "empty")
}

// ---------------------------------------------------------------------------
// takumiMDContent
// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------
// writeTakumiMD
// ---------------------------------------------------------------------------

func TestWriteTakumiMD_Success(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))

	out := captureStdout(t, func() {
		err := writeTakumiMD(dir, "test-ws")
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "TAKUMI.md")

	data, err := os.ReadFile(filepath.Join(dir, ".takumi", "TAKUMI.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "test-ws")
}

// ---------------------------------------------------------------------------
// mapFilesToPackages — file outside any package
// ---------------------------------------------------------------------------

func TestMapFilesToPackages_OutsidePackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	// File at workspace root, outside any package
	result := workspace.MapFilesToPackages(ws, []string{"takumi.yaml"})
	assert.Empty(t, result)
}

func TestMapFilesToPackages_InsidePackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := workspace.MapFilesToPackages(ws, []string{"svc-a/main.go"})
	assert.True(t, result["svc-a"])
}

// ---------------------------------------------------------------------------
// runCheckout — path flag (relative)
// ---------------------------------------------------------------------------

func TestRunCheckout_RelativePathFlag(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	// Create a bare repo to clone from
	bareRepo := filepath.Join(dir, "bare-repo.git")
	require.NoError(t, exec.Command("git", "init", "--bare", bareRepo).Run())

	chdirClean(t, dir)

	checkoutCmd.Flags().Set("path", "custom-dir")
	t.Cleanup(func() { checkoutCmd.Flags().Set("path", "") })

	out := captureStdout(t, func() {
		err := runCheckout(checkoutCmd, []string{bareRepo})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Registered")
	assert.DirExists(t, filepath.Join(dir, "custom-dir"))
}

// ---------------------------------------------------------------------------
// detectGitBranch
// ---------------------------------------------------------------------------

func TestDetectGitBranch_NoGit(t *testing.T) {
	dir := realPath(t, t.TempDir())
	assert.Equal(t, "main", detectGitBranch(dir))
}

// ---------------------------------------------------------------------------
// initWorkspace — with agent
// ---------------------------------------------------------------------------

func TestInitWorkspace_WithClaudeAgent(t *testing.T) {
	dir := realPath(t, t.TempDir())
	agent := AgentByName("claude")

	out := captureStdout(t, func() {
		err := initWorkspace(dir, "test", agent)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Initializing workspace")
	assert.Contains(t, out, "CLAUDE.md")
	assert.FileExists(t, filepath.Join(dir, "CLAUDE.md"))
}

func TestInitWorkspace_NilAgent(t *testing.T) {
	dir := realPath(t, t.TempDir())

	out := captureStdout(t, func() {
		err := initWorkspace(dir, "test", nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Initializing workspace")
	assert.NotContains(t, out, "CLAUDE.md")
}

func TestInitWorkspace_NoneAgent(t *testing.T) {
	dir := realPath(t, t.TempDir())
	agent := AgentByName("none")

	out := captureStdout(t, func() {
		err := initWorkspace(dir, "test", agent)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Initializing workspace")
	// none agent should not create any config file
	assert.NotContains(t, out, "CLAUDE.md")
}

// ---------------------------------------------------------------------------
// initPackageInDir — branches
// ---------------------------------------------------------------------------

func TestInitPackageInDir_SubdirCreation(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	target := filepath.Join(dir, "new-subpkg")

	out := captureStdout(t, func() {
		err := initPackageInDir(target, "new-subpkg", dir, true)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "new-subpkg")
	assert.Contains(t, out, "new-subpkg/")
	assert.FileExists(t, filepath.Join(target, "takumi-pkg.yaml"))
}

func TestInitPackageInDir_InRoot(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)

	out := captureStdout(t, func() {
		err := initPackageInDir(dir, "root-pkg", dir, false)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Initialized package")
	assert.FileExists(t, filepath.Join(dir, "takumi-pkg.yaml"))
}

// ---------------------------------------------------------------------------
// genCommandIndex — more patterns
// ---------------------------------------------------------------------------

func TestGenCommandIndex_NestedSubcommands(t *testing.T) {
	var buf strings.Builder
	genCommandIndex(&buf, rootCmd, "")
	result := buf.String()
	// Should contain nested commands like "takumi docs generate"
	assert.Contains(t, result, "generate")
	assert.Contains(t, result, "build")
}

func TestRunAffected_WithDownstream(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)

	// svc-b depends on svc-a, modifying a tracked file in svc-a triggers downstream
	require.NoError(t, os.WriteFile(filepath.Join(dir, "svc-a", "takumi-pkg.yaml"),
		[]byte("package:\n  name: svc-a\n  version: 0.1.1\nphases:\n  build:\n    commands:\n      - echo building-a\n  test:\n    commands:\n      - echo testing-a\n"), 0644))

	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAffected(affectedCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "svc-a")
	assert.Contains(t, out, "Downstream")
	assert.Contains(t, out, "svc-b")
}
