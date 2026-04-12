package mcp

// End-to-end tests simulating the full agent workflow through MCP tools.
// Each scenario mirrors what a real user + AI agent session looks like:
// setup repo → init workspace → build → test → break something → diagnose → fix → ship.

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
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toolText(t *testing.T, result *gomcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	return result.Content[0].(gomcp.TextContent).Text
}

func call(t *testing.T, handler func(context.Context, gomcp.CallToolRequest) (*gomcp.CallToolResult, error), args map[string]any) (string, bool) {
	t.Helper()
	result, err := handler(context.Background(), makeRequest(args))
	require.NoError(t, err, "handler returned Go error (infrastructure failure)")
	text := toolText(t, result)
	return text, result.IsError
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=dev", "GIT_AUTHOR_EMAIL=dev@example.com",
		"GIT_COMMITTER_NAME=dev", "GIT_COMMITTER_EMAIL=dev@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(origDir) })
}

// ---------------------------------------------------------------------------
// Scenario 1: Local project — new developer, fresh repo
//
// Simulates: developer has a local Go project, wants to set up takumi,
// build/test it, make a feature change, hit a test failure, diagnose,
// fix, and ship.
// ---------------------------------------------------------------------------

func TestE2E_LocalProject(t *testing.T) {
	dir := t.TempDir()

	// ── Step 1: Developer has a local Go project with git ──────────────
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	// Create project structure: a simple Go service
	svcDir := filepath.Join(dir, "user-svc")
	require.NoError(t, os.MkdirAll(svcDir, 0755))
	os.WriteFile(filepath.Join(svcDir, "main.go"), []byte(`package main

import "fmt"

func main() { fmt.Println("user-svc running") }
`), 0644)
	os.WriteFile(filepath.Join(svcDir, "main_test.go"), []byte(`package main

import "testing"

func TestMain_Runs(t *testing.T) {
	// placeholder — always passes
}
`), 0644)

	// ── Step 2: Agent runs "takumi init" equivalent — sets up workspace ─
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: my-app
  ai:
    agent: claude
`), 0644)
	os.WriteFile(filepath.Join(svcDir, "takumi-pkg.yaml"), []byte(`package:
  name: user-svc
  version: 0.1.0
phases:
  build:
    commands:
      - echo "compiling user-svc..."
  test:
    commands:
      - echo "running tests..."
      - echo "PASS"
`), 0644)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial project setup")

	chdir(t, dir)

	// ── Step 3: Agent asks "what does this workspace look like?" ────────
	t.Run("status_after_init", func(t *testing.T) {
		text, isErr := call(t, handleStatus, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: my-app")
		assert.Contains(t, text, "user-svc v0.1.0")
		assert.Contains(t, text, "Packages (1)")
		assert.Contains(t, text, "AI Agent: claude")
	})

	// ── Step 4: Agent validates the setup ──────────────────────────────
	t.Run("validate_clean", func(t *testing.T) {
		text, isErr := call(t, handleValidate, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 5: Agent checks the dependency graph ──────────────────────
	t.Run("graph_single_pkg", func(t *testing.T) {
		text, isErr := call(t, handleGraph, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "1 packages, 1 levels")
		assert.Contains(t, text, "user-svc")
	})

	// ── Step 6: Agent builds the project ───────────────────────────────
	t.Run("first_build", func(t *testing.T) {
		text, isErr := call(t, handleBuild, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 1 passed")
		assert.Contains(t, text, "user-svc")
		// Log file should exist
		assert.FileExists(t, filepath.Join(dir, ".takumi", "logs", "user-svc.build.log"))
	})

	// ── Step 7: Agent runs tests ───────────────────────────────────────
	t.Run("first_test", func(t *testing.T) {
		text, isErr := call(t, handleTest, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 1 passed")
	})

	// ── Step 8: Second build hits cache ────────────────────────────────
	t.Run("cached_build", func(t *testing.T) {
		text, isErr := call(t, handleBuild, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "1 cached")
	})

	// ── Step 9: Developer says "add a /health endpoint" ────────────────
	//    Agent modifies existing tracked source file
	os.WriteFile(filepath.Join(svcDir, "main.go"), []byte(`package main

import "fmt"

func main() { fmt.Println("user-svc running") }

func healthCheck() string { return "ok" }
`), 0644)

	// ── Step 10: Agent checks what's affected ──────────────────────────
	t.Run("affected_after_change", func(t *testing.T) {
		text, isErr := call(t, handleAffected, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Directly affected")
		assert.Contains(t, text, "user-svc")
	})

	// ── Step 11: Agent builds only affected packages ───────────────────
	t.Run("affected_build", func(t *testing.T) {
		text, isErr := call(t, handleBuild, map[string]any{"affected": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed")
		assert.Contains(t, text, "user-svc")
	})

	// ── Step 12: Agent runs tests — this time they fail ────────────────
	//    Simulate a failing test command
	os.WriteFile(filepath.Join(svcDir, "takumi-pkg.yaml"), []byte(`package:
  name: user-svc
  version: 0.1.0
phases:
  build:
    commands:
      - echo "compiling user-svc..."
  test:
    commands:
      - echo "running tests..."
      - "echo FAIL: TestHealthHandler && exit 1"
`), 0644)

	t.Run("test_failure", func(t *testing.T) {
		text, isErr := call(t, handleTest, map[string]any{"no_cache": true})
		assert.True(t, isErr, "test failure should be a tool error")
		assert.Contains(t, text, "failed")
		assert.Contains(t, text, "user-svc")
		assert.Contains(t, text, "Failed package logs")
	})

	// ── Step 13: Agent diagnoses the failure ───────────────────────────
	t.Run("diagnose_failure", func(t *testing.T) {
		text, isErr := call(t, handleDiagnose, map[string]any{"package": "user-svc"})
		assert.False(t, isErr) // diagnose itself doesn't fail
		assert.Contains(t, text, "Diagnosis for user-svc")
		assert.Contains(t, text, "Phase: test")
		assert.Contains(t, text, "user-svc.test.log")
		assert.Contains(t, text, "Changed files")
	})

	// ── Step 14: Agent fixes the test (restores passing command) ───────
	os.WriteFile(filepath.Join(svcDir, "takumi-pkg.yaml"), []byte(`package:
  name: user-svc
  version: 0.2.0
phases:
  build:
    commands:
      - echo "compiling user-svc..."
  test:
    commands:
      - echo "running tests... PASS"
`), 0644)

	// ── Step 15: Agent re-runs tests — now they pass ───────────────────
	t.Run("test_after_fix", func(t *testing.T) {
		text, isErr := call(t, handleTest, map[string]any{"no_cache": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 1 passed")
	})

	// ── Step 16: Final build before shipping ───────────────────────────
	t.Run("final_build", func(t *testing.T) {
		text, isErr := call(t, handleBuild, map[string]any{"no_cache": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 1 passed")
	})

	// ── Step 17: Verify metrics were recorded throughout ───────────────
	t.Run("metrics_recorded", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(dir, ".takumi", "metrics.json"))
		require.NoError(t, err)
		var metrics executor.MetricsFile
		require.NoError(t, json.Unmarshal(data, &metrics))
		assert.GreaterOrEqual(t, len(metrics.Runs), 4, "should have recorded multiple build/test runs")
	})

	// ── Step 18: Status shows the full picture at the end ──────────────
	t.Run("final_status", func(t *testing.T) {
		text, isErr := call(t, handleStatus, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Recent Builds")
	})
}

// ---------------------------------------------------------------------------
// Scenario 2: GitHub clone — multi-repo workspace with dependencies
//
// Simulates: developer clones a workspace from GitHub that has tracked
// sources (other repos) and multiple packages with dependencies.
// Agent onboards, explores the graph, builds, modifies a library,
// discovers downstream impact, and rebuilds.
// ---------------------------------------------------------------------------

func TestE2E_GitHubClone(t *testing.T) {
	dir := t.TempDir()

	// ── Step 1: Simulate a cloned workspace with sources ───────────────
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: platform
  settings:
    parallel: true
  sources:
    shared-utils:
      url: "git@github.com:acme/shared-utils.git"
      path: "shared-utils"
  ai:
    agent: claude
`), 0644)

	// shared-utils: foundation library (simulating a cloned source)
	utilsDir := filepath.Join(dir, "shared-utils")
	require.NoError(t, os.MkdirAll(utilsDir, 0755))
	os.WriteFile(filepath.Join(utilsDir, "takumi-pkg.yaml"), []byte(`package:
  name: shared-utils
  version: 1.0.0
phases:
  build:
    commands:
      - echo "building shared-utils..."
  test:
    commands:
      - echo "testing shared-utils... PASS"
`), 0644)
	os.WriteFile(filepath.Join(utilsDir, "utils.go"), []byte("package utils\n"), 0644)

	// auth-svc: depends on shared-utils
	authDir := filepath.Join(dir, "auth-svc")
	require.NoError(t, os.MkdirAll(authDir, 0755))
	os.WriteFile(filepath.Join(authDir, "takumi-pkg.yaml"), []byte(`package:
  name: auth-svc
  version: 0.3.0
dependencies:
  - shared-utils
phases:
  build:
    commands:
      - echo "building auth-svc..."
  test:
    commands:
      - echo "testing auth-svc... PASS"
`), 0644)
	os.WriteFile(filepath.Join(authDir, "auth.go"), []byte("package auth\n"), 0644)

	// api-gateway: depends on both shared-utils and auth-svc
	gwDir := filepath.Join(dir, "api-gateway")
	require.NoError(t, os.MkdirAll(gwDir, 0755))
	os.WriteFile(filepath.Join(gwDir, "takumi-pkg.yaml"), []byte(`package:
  name: api-gateway
  version: 0.1.0
dependencies:
  - shared-utils
  - auth-svc
phases:
  build:
    commands:
      - echo "building api-gateway..."
  test:
    commands:
      - echo "testing api-gateway... PASS"
`), 0644)
	os.WriteFile(filepath.Join(gwDir, "gateway.go"), []byte("package gateway\n"), 0644)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial platform setup")

	chdir(t, dir)

	// ── Step 2: Agent onboards — "what am I looking at?" ───────────────
	t.Run("onboard_status", func(t *testing.T) {
		text, isErr := call(t, handleStatus, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: platform")
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "shared-utils")
		assert.Contains(t, text, "auth-svc")
		assert.Contains(t, text, "api-gateway")
		assert.Contains(t, text, "Sources (1)")
		assert.Contains(t, text, "✓ shared-utils")
		assert.Contains(t, text, "AI Agent: claude")
	})

	// ── Step 3: Agent explores the dependency graph ────────────────────
	t.Run("dependency_graph", func(t *testing.T) {
		text, isErr := call(t, handleGraph, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 packages")
		assert.Contains(t, text, "Level 0 (no dependencies)")
		assert.Contains(t, text, "shared-utils")
		// auth-svc at level 1, api-gateway at level 2
		assert.Contains(t, text, "auth-svc → shared-utils")
		assert.Contains(t, text, "api-gateway")
	})

	// ── Step 4: Agent validates everything ──────────────────────────────
	t.Run("validate_all", func(t *testing.T) {
		text, isErr := call(t, handleValidate, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 5: Agent builds the entire workspace ──────────────────────
	t.Run("full_build", func(t *testing.T) {
		text, isErr := call(t, handleBuild, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
		assert.Contains(t, text, "shared-utils")
		assert.Contains(t, text, "auth-svc")
		assert.Contains(t, text, "api-gateway")
	})

	// ── Step 6: Agent tests everything ─────────────────────────────────
	t.Run("full_test", func(t *testing.T) {
		text, isErr := call(t, handleTest, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 3 passed")
	})

	// ── Step 7: Developer says "update the token validation in shared-utils"
	//    Agent modifies the library package
	os.WriteFile(filepath.Join(utilsDir, "utils.go"), []byte(`package utils

// ValidateToken checks JWT validity with the new signing algorithm.
func ValidateToken(token string) bool {
	return len(token) > 0
}
`), 0644)

	// ── Step 8: Agent checks the blast radius ──────────────────────────
	t.Run("affected_after_lib_change", func(t *testing.T) {
		text, isErr := call(t, handleAffected, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Directly affected")
		assert.Contains(t, text, "shared-utils")
		assert.Contains(t, text, "Transitively affected")
		// Both auth-svc and api-gateway depend on shared-utils
		assert.Contains(t, text, "auth-svc")
		assert.Contains(t, text, "api-gateway")
		assert.Contains(t, text, "Total affected: 3")
	})

	// ── Step 9: Agent builds only affected packages ────────────────────
	t.Run("affected_build", func(t *testing.T) {
		text, isErr := call(t, handleBuild, map[string]any{"affected": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed")
		// All 3 should rebuild (library + both consumers)
		assert.Contains(t, text, "shared-utils")
	})

	// ── Step 10: Agent tests only the specific package first ───────────
	t.Run("targeted_test", func(t *testing.T) {
		text, isErr := call(t, handleTest, map[string]any{"packages": "shared-utils"})
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 1 passed")
		assert.Contains(t, text, "shared-utils")
	})

	// ── Step 11: Agent does a full test to verify nothing broke ────────
	t.Run("full_test_after_change", func(t *testing.T) {
		text, isErr := call(t, handleTest, map[string]any{"no_cache": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 3 passed")
	})

	// ── Step 12: Cache works — unchanged packages are cached ───────────
	t.Run("build_with_cache", func(t *testing.T) {
		text, isErr := call(t, handleBuild, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "cached")
	})

	// ── Step 13: Final status shows healthy workspace ──────────────────
	t.Run("final_status", func(t *testing.T) {
		text, isErr := call(t, handleStatus, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: platform")
		assert.Contains(t, text, "Recent Builds")
	})
}

// ---------------------------------------------------------------------------
// Scenario 3: Vibe-coder — zero experience, just wants an app
//
// Simulates: someone who can't code at all found takumi on the internet,
// pastes their repo into Claude Code / Kiro, and says:
// "ok ai broski build me a portfolio app, also use this takumi thing idk"
//
// The agent has to do EVERYTHING: create the repo, init takumi, scaffold
// the project, set up packages, build, hit errors, iterate, and ship.
// This tests that the MCP tools work for a fully agent-driven workflow
// with zero human setup.
// ---------------------------------------------------------------------------

func TestE2E_VibeCoder(t *testing.T) {
	dir := t.TempDir()

	// ── Step 1: Agent starts from literally nothing ────────────────────
	//    User said "build me a portfolio app" — agent creates everything
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "vibes@example.com")
	gitRun(t, dir, "config", "user.name", "vibes")

	chdir(t, dir)

	// ── Step 2: Agent tries status before setup — gets error ───────────
	t.Run("no_workspace_yet", func(t *testing.T) {
		_, isErr := call(t, handleStatus, nil)
		assert.True(t, isErr, "should fail — no workspace exists yet")
	})

	// ── Step 3: Agent sets up takumi workspace from scratch ────────────
	//    This is what the agent would do after reading takumi docs
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: my-portfolio
  ai:
    agent: claude
`), 0644)

	// ── Step 4: Agent scaffolds a frontend package ─────────────────────
	frontendDir := filepath.Join(dir, "frontend")
	require.NoError(t, os.MkdirAll(frontendDir, 0755))
	os.WriteFile(filepath.Join(frontendDir, "takumi-pkg.yaml"), []byte(`package:
  name: frontend
  version: 0.1.0
phases:
  build:
    commands:
      - echo "bundling React app..."
      - echo "output -> build/index.html"
  test:
    commands:
      - echo "running vitest... 3 tests passed"
`), 0644)
	os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<html><body>portfolio</body></html>\n"), 0644)
	os.WriteFile(filepath.Join(frontendDir, "app.js"), []byte("console.log('portfolio');\n"), 0644)

	// ── Step 5: Agent scaffolds an API backend package ──────────────────
	backendDir := filepath.Join(dir, "backend")
	require.NoError(t, os.MkdirAll(backendDir, 0755))
	os.WriteFile(filepath.Join(backendDir, "takumi-pkg.yaml"), []byte(`package:
  name: backend
  version: 0.1.0
phases:
  build:
    commands:
      - echo "compiling Go API server..."
  test:
    commands:
      - echo "go test ./... ok"
`), 0644)
	os.WriteFile(filepath.Join(backendDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)

	// ── Step 6: Agent creates a shared types package ────────────────────
	typesDir := filepath.Join(dir, "types")
	require.NoError(t, os.MkdirAll(typesDir, 0755))
	os.WriteFile(filepath.Join(typesDir, "takumi-pkg.yaml"), []byte(`package:
  name: types
  version: 0.1.0
phases:
  build:
    commands:
      - echo "generating type definitions..."
`), 0644)
	os.WriteFile(filepath.Join(typesDir, "portfolio.go"), []byte("package types\n\ntype Project struct{ Name string }\n"), 0644)

	// Wire up dependencies: frontend and backend both use types
	os.WriteFile(filepath.Join(frontendDir, "takumi-pkg.yaml"), []byte(`package:
  name: frontend
  version: 0.1.0
dependencies:
  - types
phases:
  build:
    commands:
      - echo "bundling React app..."
  test:
    commands:
      - echo "running vitest... 3 tests passed"
`), 0644)
	os.WriteFile(filepath.Join(backendDir, "takumi-pkg.yaml"), []byte(`package:
  name: backend
  version: 0.1.0
dependencies:
  - types
phases:
  build:
    commands:
      - echo "compiling Go API server..."
  test:
    commands:
      - echo "go test ./... ok"
`), 0644)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial scaffold")

	// ── Step 7: Agent checks what it built ─────────────────────────────
	t.Run("status_after_scaffold", func(t *testing.T) {
		text, isErr := call(t, handleStatus, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: my-portfolio")
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "frontend")
		assert.Contains(t, text, "backend")
		assert.Contains(t, text, "types")
	})

	// ── Step 8: Agent validates before building ────────────────────────
	t.Run("validate_scaffold", func(t *testing.T) {
		text, isErr := call(t, handleValidate, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 9: Agent checks the dependency graph ──────────────────────
	t.Run("graph_shows_structure", func(t *testing.T) {
		text, isErr := call(t, handleGraph, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 packages")
		assert.Contains(t, text, "types")           // level 0
		assert.Contains(t, text, "frontend → types") // level 1
		assert.Contains(t, text, "backend → types")  // level 1
	})

	// ── Step 10: Agent builds everything ───────────────────────────────
	t.Run("first_build", func(t *testing.T) {
		text, isErr := call(t, handleBuild, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
	})

	// ── Step 11: Agent runs tests ──────────────────────────────────────
	t.Run("first_test", func(t *testing.T) {
		text, isErr := call(t, handleTest, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed")
		// types has no test phase — only frontend + backend run
		assert.Contains(t, text, "frontend")
		assert.Contains(t, text, "backend")
	})

	// ── Step 12: User says "add a contact form to the portfolio" ───────
	//    Agent modifies frontend, but introduces a build error
	os.WriteFile(filepath.Join(frontendDir, "contact.js"), []byte("// contact form component\n"), 0644)
	os.WriteFile(filepath.Join(frontendDir, "takumi-pkg.yaml"), []byte(`package:
  name: frontend
  version: 0.2.0
dependencies:
  - types
phases:
  build:
    commands:
      - echo "bundling React app..."
      - "echo ERROR: Cannot find module contact-form && exit 1"
  test:
    commands:
      - echo "running vitest... 3 tests passed"
`), 0644)

	// ── Step 13: Agent builds — fails! ─────────────────────────────────
	t.Run("build_fails_on_new_feature", func(t *testing.T) {
		text, isErr := call(t, handleBuild, map[string]any{
			"packages": "frontend",
			"no_cache": true,
		})
		assert.True(t, isErr)
		assert.Contains(t, text, "failed")
		assert.Contains(t, text, "frontend")
	})

	// ── Step 14: Agent diagnoses — diagnose prefers test logs over build,
	//    but either way it gives the agent useful context
	t.Run("diagnose_build_failure", func(t *testing.T) {
		text, isErr := call(t, handleDiagnose, map[string]any{"package": "frontend"})
		assert.False(t, isErr)
		assert.Contains(t, text, "Diagnosis for frontend")
		assert.Contains(t, text, "Log file:")
		assert.Contains(t, text, "Changed files")
		assert.Contains(t, text, "Dependencies: types")
	})

	// ── Step 15: Agent fixes the build ─────────────────────────────────
	os.WriteFile(filepath.Join(frontendDir, "takumi-pkg.yaml"), []byte(`package:
  name: frontend
  version: 0.2.0
dependencies:
  - types
phases:
  build:
    commands:
      - echo "bundling React app with contact form..."
      - echo "output -> build/index.html"
  test:
    commands:
      - echo "running vitest... 5 tests passed"
`), 0644)

	// ── Step 16: Agent rebuilds — passes ───────────────────────────────
	t.Run("build_after_fix", func(t *testing.T) {
		text, isErr := call(t, handleBuild, map[string]any{
			"packages": "frontend",
			"no_cache": true,
		})
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 1 passed")
	})

	// ── Step 17: Agent runs full test suite before shipping ────────────
	t.Run("full_test_before_ship", func(t *testing.T) {
		text, isErr := call(t, handleTest, map[string]any{"no_cache": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed")
		assert.Contains(t, text, "frontend")
		assert.Contains(t, text, "backend")
	})

	// ── Step 18: Full build for shipping — types + backend cached ──────
	t.Run("ship_build", func(t *testing.T) {
		text, isErr := call(t, handleBuild, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "cached")
	})

	// ── Step 19: Final status — the portfolio app is ready ─────────────
	t.Run("ready_to_ship", func(t *testing.T) {
		text, isErr := call(t, handleStatus, nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: my-portfolio")
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "Recent Builds")
	})

	// ── Step 20: Verify the full build history exists ───────────────────
	t.Run("build_history", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(dir, ".takumi", "metrics.json"))
		require.NoError(t, err)
		var metrics executor.MetricsFile
		require.NoError(t, json.Unmarshal(data, &metrics))
		assert.GreaterOrEqual(t, len(metrics.Runs), 5, "should have build+test history")
	})
}
