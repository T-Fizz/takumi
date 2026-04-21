package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tfitz/takumi/src/executor"
	"github.com/tfitz/takumi/src/workspace"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupWorkspace creates a minimal takumi workspace in a temp dir and chdir to it.
// Returns the workspace root and a cleanup function that restores the original cwd.
func setupWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create .takumi/ marker
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))

	// Create takumi.yaml
	wsYAML := `workspace:
  name: test-ws
  ai:
    agent: claude
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(wsYAML), 0644))

	// Create a package
	pkgDir := filepath.Join(dir, "my-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	pkgYAML := `package:
  name: my-pkg
  version: 0.1.0
phases:
  build:
    commands:
      - echo "building"
  test:
    commands:
      - echo "testing"
`
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(pkgYAML), 0644))

	// chdir to workspace
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(origDir) })

	return dir
}

// setupWorkspaceWithDeps creates a workspace with two packages where api depends on lib.
func setupWorkspaceWithDeps(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	wsYAML := `workspace:
  name: dep-ws
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(wsYAML), 0644))

	// lib package (no deps)
	libDir := filepath.Join(dir, "lib")
	require.NoError(t, os.MkdirAll(libDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(libDir, "takumi-pkg.yaml"), []byte(`package:
  name: lib
  version: 1.0.0
phases:
  build:
    commands:
      - echo "lib build"
`), 0644))

	// api package (depends on lib)
	apiDir := filepath.Join(dir, "api")
	require.NoError(t, os.MkdirAll(apiDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(apiDir, "takumi-pkg.yaml"), []byte(`package:
  name: api
  version: 0.2.0
dependencies:
  - lib
phases:
  build:
    commands:
      - echo "api build"
`), 0644))

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(origDir) })

	return dir
}

func makeRequest(args map[string]any) gomcp.CallToolRequest {
	return gomcp.CallToolRequest{
		Params: gomcp.CallToolParams{
			Arguments: args,
		},
	}
}

// ---------------------------------------------------------------------------
// loadWorkspace tests
// ---------------------------------------------------------------------------

func TestLoadWorkspace_Success(t *testing.T) {
	setupWorkspace(t)
	ws, err := loadWorkspace()
	require.NoError(t, err)
	assert.Equal(t, "test-ws", ws.Config.Workspace.Name)
	assert.Contains(t, ws.Packages, "my-pkg")
}

func TestLoadWorkspace_NoWorkspace(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	ws, err := loadWorkspace()
	assert.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "no takumi workspace")
}

// ---------------------------------------------------------------------------
// handleStatus tests
// ---------------------------------------------------------------------------

func TestHandleStatus_Basic(t *testing.T) {
	setupWorkspace(t)
	result, err := handleStatus(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Workspace: test-ws")
	assert.Contains(t, text, "my-pkg")
	assert.Contains(t, text, "Packages (1)")
	assert.Contains(t, text, "AI Agent: claude")
}

func TestHandleStatus_WithMetrics(t *testing.T) {
	dir := setupWorkspace(t)

	// Write metrics
	metrics := executor.MetricsFile{
		Runs: []executor.MetricsEntry{
			{Package: "my-pkg", Phase: "build", ExitCode: 0, DurationMs: 150},
		},
	}
	data, _ := json.Marshal(metrics)
	os.WriteFile(filepath.Join(dir, ".takumi", "metrics.json"), data, 0644)

	result, err := handleStatus(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Recent Builds")
	assert.Contains(t, text, "my-pkg")
	assert.Contains(t, text, "150ms")
}

func TestHandleStatus_NoWorkspace(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleStatus(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	// Outside a workspace, status returns a discovery message (not an error)
	assert.False(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "takumi init")
	assert.Contains(t, text, "not a Takumi workspace")
}

// ---------------------------------------------------------------------------
// handlePhase tests (build/test)
// ---------------------------------------------------------------------------

func TestHandleBuild_Success(t *testing.T) {
	setupWorkspace(t)
	result, err := handleBuild(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Build completed")
	assert.Contains(t, text, "my-pkg")
}

func TestHandleTest_Success(t *testing.T) {
	setupWorkspace(t)
	result, err := handleTest(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Test completed")
	assert.Contains(t, text, "my-pkg")
}

func TestHandleBuild_SpecificPackage(t *testing.T) {
	setupWorkspace(t)
	result, err := handleBuild(context.Background(), makeRequest(map[string]any{
		"packages": "my-pkg",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "my-pkg")
}

func TestHandleBuild_NoWorkspace(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleBuild(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleBuild_WritesLogFiles(t *testing.T) {
	dir := setupWorkspace(t)
	_, err := handleBuild(context.Background(), makeRequest(nil))
	require.NoError(t, err)

	logPath := filepath.Join(dir, ".takumi", "logs", "my-pkg.build.log")
	assert.FileExists(t, logPath)
}

func TestHandleBuild_RecordsMetrics(t *testing.T) {
	dir := setupWorkspace(t)
	_, err := handleBuild(context.Background(), makeRequest(nil))
	require.NoError(t, err)

	metricsPath := filepath.Join(dir, ".takumi", "metrics.json")
	assert.FileExists(t, metricsPath)

	data, _ := os.ReadFile(metricsPath)
	var metrics executor.MetricsFile
	require.NoError(t, json.Unmarshal(data, &metrics))
	assert.NotEmpty(t, metrics.Runs)
	assert.Equal(t, "my-pkg", metrics.Runs[0].Package)
	assert.Equal(t, "build", metrics.Runs[0].Phase)
}

// ---------------------------------------------------------------------------
// handleDiagnose tests
// ---------------------------------------------------------------------------

func TestHandleDiagnose_WithLogFile(t *testing.T) {
	dir := setupWorkspace(t)

	// Create a build log
	logContent := `# takumi build my-pkg
# started: 2026-01-01T00:00:00Z
# cwd: /test

$ echo "building"
building

# exit code: 1
# duration: 500ms
`
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.build.log"), []byte(logContent), 0644)

	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "my-pkg",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Diagnosis for my-pkg")
	assert.Contains(t, text, "Phase: build")
	assert.Contains(t, text, "Exit code: 1")
	assert.Contains(t, text, "my-pkg.build.log")
}

func TestHandleDiagnose_NoLogFiles(t *testing.T) {
	setupWorkspace(t)
	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "my-pkg",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "No log files found")
}

func TestHandleDiagnose_PackageNotFound(t *testing.T) {
	setupWorkspace(t)
	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "nonexistent",
	}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "package not found")
}

func TestHandleDiagnose_MissingParam(t *testing.T) {
	setupWorkspace(t)
	result, err := handleDiagnose(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleDiagnose_PrefersFailingLog(t *testing.T) {
	dir := setupWorkspace(t)

	// Build failed, test passed — diagnose should pick the failing build log
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.build.log"), []byte("build error output\n# exit code: 1\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.test.log"), []byte("tests passed\n# exit code: 0\n"), 0644)

	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "my-pkg",
	}))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Phase: build")
	assert.Contains(t, text, "Exit code: 1")
}

func TestHandleDiagnose_PrefersFailingTest(t *testing.T) {
	dir := setupWorkspace(t)

	// Build passed, test failed — diagnose should pick the failing test log
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.build.log"), []byte("build ok\n# exit code: 0\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.test.log"), []byte("FAIL\n# exit code: 2\n"), 0644)

	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "my-pkg",
	}))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Phase: test")
	assert.Contains(t, text, "Exit code: 2")
}

func TestHandleDiagnose_BothPassingFallsBackToNewest(t *testing.T) {
	dir := setupWorkspace(t)

	// Both passed — should fall back to the newest log
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.build.log"), []byte("build ok\n# exit code: 0\n"), 0644)
	// Touch test log to be newer
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.test.log"), []byte("tests ok\n# exit code: 0\n"), 0644)

	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "my-pkg",
	}))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	// Should return something — doesn't matter which phase, just shouldn't fail
	assert.Contains(t, text, "Phase:")
	assert.Contains(t, text, "Exit code: 0")
}

func TestHandleDiagnose_ExplicitPhase(t *testing.T) {
	dir := setupWorkspace(t)

	// Both exist, test is failing — but agent explicitly asks for build
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.build.log"), []byte("build ok\n# exit code: 0\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "my-pkg.test.log"), []byte("FAIL\n# exit code: 1\n"), 0644)

	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "my-pkg",
		"phase":   "build",
	}))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Phase: build")
	assert.Contains(t, text, "Exit code: 0")
}

// ---------------------------------------------------------------------------
// handleValidate tests
// ---------------------------------------------------------------------------

func TestHandleValidate_AllValid(t *testing.T) {
	setupWorkspace(t)
	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "All configurations valid")
}

func TestHandleValidate_WithErrors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: ""
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "workspace.name")
}

// ---------------------------------------------------------------------------
// handleGraph tests
// ---------------------------------------------------------------------------

func TestHandleGraph_SinglePackage(t *testing.T) {
	setupWorkspace(t)
	result, err := handleGraph(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Dependency Graph")
	assert.Contains(t, text, "1 packages")
	assert.Contains(t, text, "my-pkg")
}

func TestHandleGraph_WithDeps(t *testing.T) {
	setupWorkspaceWithDeps(t)
	result, err := handleGraph(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "2 packages")
	assert.Contains(t, text, "2 levels")
	assert.Contains(t, text, "Level 0")
	assert.Contains(t, text, "lib")
	assert.Contains(t, text, "api → lib")
}

func TestHandleGraph_NoWorkspace(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleGraph(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// ---------------------------------------------------------------------------
// handleAffected tests
// ---------------------------------------------------------------------------

func TestHandleAffected_NoWorkspace(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleAffected(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestCapitalize(t *testing.T) {
	assert.Equal(t, "Build", capitalize("build"))
	assert.Equal(t, "Test", capitalize("test"))
	assert.Equal(t, "", capitalize(""))
	assert.Equal(t, "A", capitalize("a"))
}

func TestSortedKeys(t *testing.T) {
	m := map[string]bool{"c": true, "a": true, "b": true}
	assert.Equal(t, []string{"a", "b", "c"}, sortedKeys(m))
}

func TestSortedPackageNames(t *testing.T) {
	setupWorkspaceWithDeps(t)
	ws, err := loadWorkspace()
	require.NoError(t, err)
	names := sortedPackageNames(ws)
	assert.Equal(t, []string{"api", "lib"}, names)
}

func TestNewGraph(t *testing.T) {
	setupWorkspaceWithDeps(t)
	ws, err := loadWorkspace()
	require.NoError(t, err)
	g := newGraph(ws)
	deps := g.DepsOf("api")
	assert.Equal(t, []string{"lib"}, deps)
}

func TestMapFilesToPackages(t *testing.T) {
	setupWorkspaceWithDeps(t)
	ws, err := loadWorkspace()
	require.NoError(t, err)

	// Use the workspace-resolved path to avoid symlink mismatches on macOS
	files := []string{filepath.Join(ws.Packages["lib"].Dir, "main.go")}
	affected := workspace.MapFilesToPackages(ws, files)
	assert.True(t, affected["lib"])
	assert.False(t, affected["api"])
}

func TestMapFilesToPackages_RelativePaths(t *testing.T) {
	setupWorkspaceWithDeps(t)
	ws, err := loadWorkspace()
	require.NoError(t, err)

	// Relative paths (as git diff would return)
	files := []string{"api/handler.go"}
	affected := workspace.MapFilesToPackages(ws, files)
	assert.True(t, affected["api"])
	assert.False(t, affected["lib"])
}

func TestMapFilesToPackages_NoMatch(t *testing.T) {
	setupWorkspaceWithDeps(t)
	ws, err := loadWorkspace()
	require.NoError(t, err)

	files := []string{"README.md"}
	affected := workspace.MapFilesToPackages(ws, files)
	assert.Empty(t, affected)
}

// ---------------------------------------------------------------------------
// handleStatus with sources
// ---------------------------------------------------------------------------

func TestHandleStatus_WithSources(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))

	// Source that exists
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "my-lib"), 0755))

	wsYAML := `workspace:
  name: src-ws
  sources:
    my-lib:
      url: "git@example.com:my-lib.git"
      path: "my-lib"
    missing-lib:
      url: "git@example.com:missing.git"
      path: "missing-lib"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(wsYAML), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleStatus(context.Background(), makeRequest(nil))
	require.NoError(t, err)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Sources (2)")
	assert.Contains(t, text, "✓")
	assert.Contains(t, text, "missing")
}

// ---------------------------------------------------------------------------
// Tool definition tests
// ---------------------------------------------------------------------------

func TestToolDefinitions(t *testing.T) {
	tools := []struct {
		tool gomcp.Tool
		name string
	}{
		{statusTool, "takumi_status"},
		{buildTool, "takumi_build"},
		{testTool, "takumi_test"},
		{diagnoseTool, "takumi_diagnose"},
		{affectedTool, "takumi_affected"},
		{validateTool, "takumi_validate"},
		{graphTool, "takumi_graph"},
	}

	for _, tt := range tools {
		assert.Equal(t, tt.name, tt.tool.Name, "tool name mismatch")
		assert.NotEmpty(t, tt.tool.Description, "tool %s should have description", tt.name)
	}
}

func TestDiagnoseToolRequiresPackage(t *testing.T) {
	assert.Contains(t, diagnoseTool.InputSchema.Required, "package")
}

// ---------------------------------------------------------------------------
// handleDiagnose with runtime
// ---------------------------------------------------------------------------

func TestHandleDiagnose_WithRuntime(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: rt-ws
`), 0644))

	pkgDir := filepath.Join(dir, "svc")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(`package:
  name: svc
  version: 0.1.0
runtime:
  setup:
    - echo setup
  env:
    PATH: "{{env_dir}}/bin:$PATH"
`), 0644))

	// Write a log file
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "svc.build.log"), []byte("# exit code: 1\n"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "svc",
	}))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Runtime:")
	assert.Contains(t, text, "not set up")
}

// ---------------------------------------------------------------------------
// handleValidate with unresolved deps
// ---------------------------------------------------------------------------

func TestHandleValidate_UnresolvedDeps(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: bad-deps
`), 0644))

	pkgDir := filepath.Join(dir, "svc")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(`package:
  name: svc
  version: 0.1.0
dependencies:
  - nonexistent
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "nonexistent")
}

// ---------------------------------------------------------------------------
// Git-backed workspace helper
// ---------------------------------------------------------------------------

// setupGitWorkspace creates a workspace inside a git repo with an initial commit.
func setupGitWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Init git repo
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, out)
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")

	// Create workspace
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	wsYAML := `workspace:
  name: git-ws
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(wsYAML), 0644))

	// Create two packages
	for _, pkg := range []struct{ name, yaml string }{
		{"lib", `package:
  name: lib
  version: 1.0.0
phases:
  build:
    commands:
      - echo "lib build"
  test:
    commands:
      - echo "lib test"
`},
		{"api", `package:
  name: api
  version: 0.2.0
dependencies:
  - lib
phases:
  build:
    commands:
      - echo "api build"
`},
	} {
		pkgDir := filepath.Join(dir, pkg.name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(pkg.yaml), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte("package "+pkg.name+"\n"), 0644))
	}

	// Commit everything
	run("add", "-A")
	run("commit", "-m", "initial")

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(origDir) })

	return dir
}

// ---------------------------------------------------------------------------
// handleAffected with git
// ---------------------------------------------------------------------------

func TestHandleAffected_NoChanges(t *testing.T) {
	setupGitWorkspace(t)
	result, err := handleAffected(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "No changed files")
}

func TestHandleAffected_WithChanges(t *testing.T) {
	dir := setupGitWorkspace(t)

	// Modify a tracked file in lib
	os.WriteFile(filepath.Join(dir, "lib", "main.go"), []byte("package lib\n// changed\n"), 0644)

	result, err := handleAffected(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Directly affected")
	assert.Contains(t, text, "lib")
}

func TestHandleAffected_TransitiveDeps(t *testing.T) {
	dir := setupGitWorkspace(t)

	// Modify lib — api depends on lib so should be transitively affected
	os.WriteFile(filepath.Join(dir, "lib", "main.go"), []byte("package lib\n// changed\n"), 0644)

	result, err := handleAffected(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Transitively affected")
	assert.Contains(t, text, "api")
	assert.Contains(t, text, "Total affected: 2")
}

func TestHandleAffected_CustomSince(t *testing.T) {
	setupGitWorkspace(t)

	result, err := handleAffected(context.Background(), makeRequest(map[string]any{
		"since": "HEAD",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "No changed files since HEAD")
}

// ---------------------------------------------------------------------------
// handlePhase — affected flag with git
// ---------------------------------------------------------------------------

func TestHandleBuild_AffectedFlag(t *testing.T) {
	dir := setupGitWorkspace(t)

	// Modify lib
	os.WriteFile(filepath.Join(dir, "lib", "main.go"), []byte("package lib\n// changed\n"), 0644)

	result, err := handleBuild(context.Background(), makeRequest(map[string]any{
		"affected": true,
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Build completed")
	assert.Contains(t, text, "lib")
}

func TestHandleBuild_AffectedNoChanges(t *testing.T) {
	setupGitWorkspace(t)

	result, err := handleBuild(context.Background(), makeRequest(map[string]any{
		"affected": true,
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "No affected packages")
}

func TestHandleBuild_NoCacheFlag(t *testing.T) {
	setupWorkspace(t)
	result, err := handleBuild(context.Background(), makeRequest(map[string]any{
		"no_cache": true,
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Build completed")
}

// ---------------------------------------------------------------------------
// handlePhase — build failure
// ---------------------------------------------------------------------------

func TestHandleBuild_Failure(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: fail-ws
`), 0644))

	pkgDir := filepath.Join(dir, "bad-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(`package:
  name: bad-pkg
  version: 0.1.0
phases:
  build:
    commands:
      - exit 1
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleBuild(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "failed")
	assert.Contains(t, text, "bad-pkg")
	assert.Contains(t, text, "Failed package logs")
}

func TestHandleBuild_MultiplePackages(t *testing.T) {
	setupWorkspaceWithDeps(t)
	result, err := handleBuild(context.Background(), makeRequest(map[string]any{
		"packages": "lib,api",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "lib")
	assert.Contains(t, text, "api")
}

// ---------------------------------------------------------------------------
// handleValidate — version set, parse errors, warnings-only
// ---------------------------------------------------------------------------

func TestHandleValidate_VersionSet(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: vs-ws
  version-set:
    file: versions.yaml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "versions.yaml"), []byte(`version-set:
  name: release
  strategy: strict
  packages:
    react: "18.0.0"
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "All configurations valid")
}

func TestHandleValidate_InvalidVersionSet(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: vs-ws
  version-set:
    file: versions.yaml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "versions.yaml"), []byte(`version-set:
  name: test
  strategy: yolo
  packages:
    a: "1.0.0"
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "yolo")
}

func TestHandleValidate_CorruptVersionSet(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: vs-ws
  version-set:
    file: versions.yaml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "versions.yaml"), []byte(`not: valid: yaml: [[[`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "version-set")
}

func TestHandleValidate_WarningsOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: warn-ws
`), 0644))

	pkgDir := filepath.Join(dir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(`package:
  name: pkg
  version: ""
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	// Warnings don't make it an error result
	require.False(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Validation:")
	assert.Contains(t, text, "0 errors")
}

func TestHandleValidate_CyclicDeps(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: cycle-ws
`), 0644))

	for _, pkg := range []struct{ name, dep string }{
		{"a", "b"},
		{"b", "a"},
	} {
		pkgDir := filepath.Join(dir, pkg.name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(`package:
  name: `+pkg.name+`
  version: 0.1.0
dependencies:
  - `+pkg.dep+`
`), 0644))
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "cycle")
}

// ---------------------------------------------------------------------------
// gitChangedFiles tests
// ---------------------------------------------------------------------------

func TestGitChangedFiles_NoRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := workspace.ChangedFiles(dir, "HEAD")
	assert.Error(t, err)
}

func TestGitChangedFiles_CleanRepo(t *testing.T) {
	dir := setupGitWorkspace(t)
	files, err := workspace.ChangedFiles(dir, "HEAD")
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestGitChangedFiles_WithChanges(t *testing.T) {
	dir := setupGitWorkspace(t)
	os.WriteFile(filepath.Join(dir, "lib", "main.go"), []byte("package lib\n// changed\n"), 0644)

	files, err := workspace.ChangedFiles(dir, "HEAD")
	require.NoError(t, err)
	assert.Contains(t, files, "lib/main.go")
}

// ---------------------------------------------------------------------------
// handleDiagnose — runtime configured and setup
// ---------------------------------------------------------------------------

func TestHandleDiagnose_WithRuntimeSetUp(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: rt-ws
`), 0644))

	pkgDir := filepath.Join(dir, "svc")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(`package:
  name: svc
  version: 0.1.0
runtime:
  setup:
    - echo setup
`), 0644))

	// Create env dir (simulating that env setup was run)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "envs", "svc"), 0755))

	// Write a log file
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "svc.build.log"), []byte("# exit code: 0\n# duration: 100ms\n"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "svc",
	}))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Runtime: configured")
	assert.Contains(t, text, "env dir:")
	assert.Contains(t, text, "Duration: 100ms")
}

// ---------------------------------------------------------------------------
// handlePhase — affected with git failure
// ---------------------------------------------------------------------------

func TestHandleBuild_AffectedNoGit(t *testing.T) {
	setupWorkspace(t) // no git repo
	result, err := handleBuild(context.Background(), makeRequest(map[string]any{
		"affected": true,
	}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "failed to determine affected")
}

// ---------------------------------------------------------------------------
// handleAffected — git error
// ---------------------------------------------------------------------------

func TestHandleAffected_GitError(t *testing.T) {
	setupWorkspace(t) // no git repo
	result, err := handleAffected(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "failed to get changed files")
}

// ---------------------------------------------------------------------------
// handleDiagnose — with changed files (git-backed)
// ---------------------------------------------------------------------------

func TestHandleDiagnose_WithChangedFiles(t *testing.T) {
	dir := setupGitWorkspace(t)

	// Modify a file
	os.WriteFile(filepath.Join(dir, "lib", "main.go"), []byte("package lib\n// changed\n"), 0644)

	// Create a log file
	os.WriteFile(filepath.Join(dir, ".takumi", "logs", "lib.build.log"), []byte("# exit code: 1\n"), 0644)

	result, err := handleDiagnose(context.Background(), makeRequest(map[string]any{
		"package": "lib",
	}))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Changed files:")
	assert.Contains(t, text, "lib/main.go")
}

// ---------------------------------------------------------------------------
// handleGraph — cycle detection
// ---------------------------------------------------------------------------

func TestHandleGraph_CycleDetection(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: cycle-ws
`), 0644))

	for _, pkg := range []struct{ name, dep string }{
		{"a", "b"},
		{"b", "a"},
	} {
		pkgDir := filepath.Join(dir, pkg.name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(`package:
  name: `+pkg.name+`
  version: 0.1.0
dependencies:
  - `+pkg.dep+`
`), 0644))
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleGraph(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "cycle")
}

// ---------------------------------------------------------------------------
// handleStatus — no packages, no sources, no agent
// ---------------------------------------------------------------------------

func TestHandleStatus_MinimalWorkspace(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: empty-ws
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleStatus(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "Packages (0)")
	assert.NotContains(t, text, "Sources")
	assert.NotContains(t, text, "AI Agent")
}

// ---------------------------------------------------------------------------
// handlePhase — cached results
// ---------------------------------------------------------------------------

func TestLoadWorkspace_LoadError(t *testing.T) {
	// A corrupt takumi.yaml should make workspace.Load return an error
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`not: valid: [[[`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	ws, err := loadWorkspace()
	assert.Error(t, err)
	assert.Nil(t, ws)
}

func TestHandleValidate_ParseErrors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: parse-err-ws
`), 0644))

	// Create a package with invalid YAML that will fail to parse
	pkgDir := filepath.Join(dir, "bad")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(`this is: not: valid: [[[`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "parse error")
}

func TestHandleValidate_VersionSetMissingFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: vs-ws
  version-set:
    file: nonexistent.yaml
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Should succeed — missing file is just skipped
	result, err := handleValidate(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError)
}

func TestHandleStatus_SourceDefaultPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi"), 0755))
	// Source without explicit path — should use name as path
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "my-src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: src-ws
  sources:
    my-src:
      url: "git@example.com:my-src.git"
`), 0644))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	result, err := handleStatus(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	text := result.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text, "✓ my-src")
}

func TestHandleBuild_CachedOnSecondRun(t *testing.T) {
	setupWorkspace(t)

	// First run — builds
	result1, err := handleBuild(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	text1 := result1.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text1, "Build completed: 1 passed")

	// Second run — should be cached
	result2, err := handleBuild(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	text2 := result2.Content[0].(gomcp.TextContent).Text
	assert.Contains(t, text2, "cached")
}
