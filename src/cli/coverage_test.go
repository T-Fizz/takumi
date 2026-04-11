package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
// envStatus (ai.go helper)
// ---------------------------------------------------------------------------

func TestEnvStatus_NoPackage(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := envStatus(ws, "nonexistent")
	assert.Equal(t, "no runtime defined", result)
}

func TestEnvStatus_NoRuntime(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := envStatus(ws, "svc-a")
	assert.Equal(t, "no runtime defined", result)
}

func TestEnvStatus_NotSetUp(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := envStatus(ws, "rt-pkg")
	assert.Equal(t, "not set up", result)
}

func TestEnvStatus_Ready(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithRuntime(t, dir)
	envDir := filepath.Join(dir, workspace.MarkerDir, "envs", "rt-pkg")
	require.NoError(t, os.MkdirAll(envDir, 0755))
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := envStatus(ws, "rt-pkg")
	assert.Contains(t, result, "ready")
	assert.Contains(t, result, envDir)
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

	result := mapFilesToPackages(ws, []string{"some/random/file.go"})
	assert.Empty(t, result)
}

func TestMapFilesToPackages_DirectMatch(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := mapFilesToPackages(ws, []string{"svc-a/main.go"})
	assert.True(t, result["svc-a"])
	assert.False(t, result["svc-b"])
}

func TestMapFilesToPackages_MultipleMatches(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := mapFilesToPackages(ws, []string{"svc-a/main.go", "svc-b/handler.go"})
	assert.True(t, result["svc-a"])
	assert.True(t, result["svc-b"])
}

func TestMapFilesToPackages_EmptyFiles(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	ws, err := loadWorkspace()
	require.NoError(t, err)

	result := mapFilesToPackages(ws, nil)
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// gitChangedFiles
// ---------------------------------------------------------------------------

func TestGitChangedFiles_CleanRepo(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)

	files, err := gitChangedFiles(dir, "HEAD")
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

	files, err := gitChangedFiles(dir, "HEAD")
	require.NoError(t, err)
	assert.NotEmpty(t, files)
	assert.Contains(t, files, "svc-a/new.go")
}

func TestGitChangedFiles_NotAGitRepo(t *testing.T) {
	dir := realPath(t, t.TempDir())

	_, err := gitChangedFiles(dir, "HEAD")
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
	assert.Contains(t, out, "1 dep")
	assert.Contains(t, out, "2 phases")
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
	files, err := gitChangedFiles(dir, "nonexistent-ref-xyz")
	require.NoError(t, err)
	// Should fall back to working tree diff which is clean
	assert.Empty(t, files)
}

// ---------------------------------------------------------------------------
// AI commands — additional coverage
// ---------------------------------------------------------------------------

func TestRunAIDiagnose_WithLogFile(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	// Create a log file for svc-a build
	logDir := filepath.Join(dir, workspace.MarkerDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))
	logContent := "ERROR: build failed at line 42\n"
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "svc-a.build.log"), []byte(logContent), 0644))

	out := captureStdout(t, func() {
		err := runAIDiagnose(aiDiagnoseCmd, []string{"svc-a"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Diagnostic: svc-a")
}

func TestRunAIDiagnose_NoLogFile(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAIDiagnose(aiDiagnoseCmd, []string{"svc-a"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No log file found")
}

func TestRunAIDiagnose_PackageNotFound(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	err := runAIDiagnose(aiDiagnoseCmd, []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in workspace")
}

func TestRunAIDiagnose_TestLogFallback(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	// Create only a test log (no build log)
	logDir := filepath.Join(dir, workspace.MarkerDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "svc-a.test.log"), []byte("test failed\n"), 0644))

	out := captureStdout(t, func() {
		err := runAIDiagnose(aiDiagnoseCmd, []string{"svc-a"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Diagnostic: svc-a")
}

func TestRunAISkillRun_Diagnose(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	err := runAISkillRun(aiSkillRunCmd, []string{"diagnose"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "use 'takumi ai diagnose <package>' instead")
}

func TestRunAISkillRun_Review(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAISkillRun(aiSkillRunCmd, []string{"review"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Review Prompt")
}

func TestRunAISkillRun_Optimize(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAISkillRun(aiSkillRunCmd, []string{"optimize"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Optimization Prompt")
}

func TestRunAISkillRun_Onboard(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAISkillRun(aiSkillRunCmd, []string{"onboard"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Onboarding Prompt")
}

func TestRunAISkillRun_NotFound(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	err := runAISkillRun(aiSkillRunCmd, []string{"totally-fake-skill"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunAISkillShow_WithAutoContext(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	// "review" skill has auto-context defined
	out := captureStdout(t, func() {
		err := runAISkillShow(aiSkillShowCmd, []string{"review"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Skill: review")
	assert.Contains(t, out, "Auto-context")
	assert.Contains(t, out, "Prompt template")
}

func TestRunAISkillShow_WithoutAutoContext(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	// "operator" skill has no auto-context
	out := captureStdout(t, func() {
		err := runAISkillShow(aiSkillShowCmd, []string{"operator"})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Skill: operator")
	assert.Contains(t, out, "Prompt template")
	assert.NotContains(t, out, "Auto-context")
}

func TestRunAISkillShow_NotFound(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	err := runAISkillShow(aiSkillShowCmd, []string{"no-such-skill"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunAIContext_WithAgent(t *testing.T) {
	dir := realPath(t, t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	cfg := config.DefaultWorkspaceConfig("test-ws")
	cfg.Workspace.AI.Agent = "claude"
	data, err := cfg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), data, 0644))
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAIContext(aiContextCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Regenerated AI context")
	// Should have created CLAUDE.md
	assert.FileExists(t, filepath.Join(dir, "CLAUDE.md"))
	// Should have created .takumi/TAKUMI.md
	assert.FileExists(t, filepath.Join(dir, ".takumi", "TAKUMI.md"))
}

func TestRunAIContext_NoAgent(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAIContext(aiContextCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Regenerated AI context")
}

func TestFindSkill_NotFound(t *testing.T) {
	result := findSkill("completely-nonexistent-skill-xyz")
	assert.Nil(t, result)
}

func TestFindSkill_Found(t *testing.T) {
	result := findSkill("operator")
	assert.NotNil(t, result)
	assert.Equal(t, "operator", result.Name)
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
	assert.Contains(t, out, "skills-reference.md")
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

func TestSetupAgentConfig_NoneAgent_Cov(t *testing.T) {
	dir := t.TempDir()
	agent := AgentByName("none")
	require.NotNil(t, agent)

	err := setupAgentConfig(dir, agent)
	assert.NoError(t, err)
	// "none" should not create any file
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

func TestWriteTakumiMD_Cov(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))

	out := captureStdout(t, func() {
		err := writeTakumiMD(dir, "my-workspace")
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "TAKUMI.md")

	data, err := os.ReadFile(filepath.Join(dir, ".takumi", "TAKUMI.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "my-workspace")
}

func TestTakumiMDContent_Cov(t *testing.T) {
	content := takumiMDContent("test-ws")
	assert.Contains(t, content, "test-ws")
	assert.Contains(t, content, "takumi status")
	assert.Contains(t, content, "takumi build")
}

func TestAgentByName_Cov(t *testing.T) {
	assert.NotNil(t, AgentByName("claude"))
	assert.NotNil(t, AgentByName("cursor"))
	assert.NotNil(t, AgentByName("copilot"))
	assert.NotNil(t, AgentByName("windsurf"))
	assert.NotNil(t, AgentByName("cline"))
	assert.NotNil(t, AgentByName("none"))
	assert.Nil(t, AgentByName("nonexistent"))
}

func TestAgentNames_Cov(t *testing.T) {
	names := agentNames()
	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "cursor")
	assert.Contains(t, names, "none")
}

// ---------------------------------------------------------------------------
// gitDiffOutput
// ---------------------------------------------------------------------------

func TestGitDiffOutput_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	result := gitDiffOutput(dir)
	assert.Contains(t, result, "git diff unavailable")
}

func TestGitDiffOutput_CleanRepo(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)

	result := gitDiffOutput(dir)
	assert.Empty(t, result)
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
// runAIReview and runAIOptimize in git workspace
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

func TestRunDocsGenerate_WithAIFlag(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspace(t, dir)
	chdirClean(t, dir)

	// Reset the --ai flag
	f := docsGenerateCmd.Flags().Lookup("ai")
	if f != nil {
		f.Value.Set("false")
		f.Changed = false
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"docs", "generate", "--ai"})
		err := rootCmd.Execute()
		// may fail (doc-writer skill may or may not produce errors), but should execute
		_ = err
	})
	assert.Contains(t, out, "Generating Documentation")
	assert.Contains(t, out, "doc-writer")
}

func TestRunAIReview_InGitWorkspace(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupGitWorkspace(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAIReview(aiReviewCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Review Prompt")
}

func TestRunAIOptimize_WithMetrics(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	writeMetrics(t, dir, []executor.MetricsEntry{
		{Timestamp: "2025-01-01T00:00:00Z", Phase: "build", Package: "svc-a", DurationMs: 100, ExitCode: 0},
	})
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAIOptimize(aiOptimizeCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Optimization Prompt")
}

func TestRunAIOnboard_WithPackages(t *testing.T) {
	dir := realPath(t, t.TempDir())
	setupWorkspaceWithPackages(t, dir)
	chdirClean(t, dir)

	out := captureStdout(t, func() {
		err := runAIOnboard(aiOnboardCmd, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Onboarding Prompt")
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
