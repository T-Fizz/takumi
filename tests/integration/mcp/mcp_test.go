// Package mcp_test contains end-to-end integration tests for Takumi's MCP server.
// Each scenario simulates a full agent workflow: setup → build → test → break →
// fix → ship. Tests go through the real MCP server dispatch path
// (NewServer + HandleMessage) rather than calling handlers directly.
package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	takumimcp "github.com/tfitz/takumi/src/mcp"
)

// ---------------------------------------------------------------------------
// MCP call helpers — goes through the full JSON-RPC dispatch
// ---------------------------------------------------------------------------

// call invokes a tool through HandleMessage and returns the text result.
func call(t *testing.T, toolName string, args map[string]any) (string, bool) {
	t.Helper()
	s := takumimcp.NewServer()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	raw, err := json.Marshal(req)
	require.NoError(t, err)

	resp := s.HandleMessage(context.Background(), raw)

	// Parse the JSON-RPC response
	respBytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var rpcResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(respBytes, &rpcResp))
	if rpcResp.Error != nil {
		t.Fatalf("JSON-RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	require.NotEmpty(t, rpcResp.Result.Content, "expected at least one content block")

	return rpcResp.Result.Content[0].Text, rpcResp.Result.IsError
}

// ---------------------------------------------------------------------------
// Transcript logger — writes human-readable session logs
// ---------------------------------------------------------------------------

type transcript struct {
	f       *os.File
	step    int
	rootDir string // workspace root for path sanitization
}

func newTranscript(t *testing.T, name string) *transcript {
	t.Helper()
	logDir := filepath.Join("..", "..", "..", "testdata")
	os.MkdirAll(logDir, 0755)
	path := filepath.Join(logDir, name+".log")
	f, err := os.Create(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		f.Close()
		abs, _ := filepath.Abs(path)
		t.Logf("Transcript written to: %s", abs)
	})
	tr := &transcript{f: f}
	tr.writef("# %s", name)
	tr.writef("# simulated terminal session")
	tr.writef("")
	return tr
}

// setRoot sets the workspace root for path sanitization in output.
func (tr *transcript) setRoot(dir string) {
	tr.rootDir = dir
}

func (tr *transcript) writef(format string, args ...any) {
	fmt.Fprintf(tr.f, format+"\n", args...)
}

func (tr *transcript) stepHeader(desc string) {
	tr.step++
	tr.writef("# ── %d. %s", tr.step, desc)
	tr.writef("")
}

func (tr *transcript) action(desc string) {
	tr.writef("# %s", desc)
	tr.writef("")
}

func (tr *transcript) toolCall(toolName string, args map[string]any, output string, isError bool) {
	cmd := toolToCmd(toolName, args)
	tr.writef("$ %s", cmd)

	// Sanitize temp dir paths so logs are readable
	clean := output
	if tr.rootDir != "" {
		// macOS /private/var symlink resolves differently — handle both
		clean = strings.ReplaceAll(clean, "/private"+tr.rootDir, ".")
		clean = strings.ReplaceAll(clean, tr.rootDir, ".")
	}

	for _, line := range strings.Split(strings.TrimRight(clean, "\n"), "\n") {
		tr.writef("%s", line)
	}
	tr.writef("")
}

// toolToCmd converts an MCP tool call to a CLI-style command string.
func toolToCmd(toolName string, args map[string]any) string {
	// takumi_status -> takumi status
	sub := strings.TrimPrefix(toolName, "takumi_")
	parts := []string{"t", sub}

	// "package" is a positional arg for some tools
	if pkg, ok := args["package"]; ok {
		parts = append(parts, fmt.Sprintf("%v", pkg))
	}

	// Boolean flags
	for _, flag := range []string{"affected", "no_cache"} {
		if v, ok := args[flag]; ok && v == true {
			parts = append(parts, "--"+strings.ReplaceAll(flag, "_", "-"))
		}
	}

	// String flags
	for _, flag := range []string{"packages", "phase"} {
		if v, ok := args[flag]; ok {
			parts = append(parts, "--"+flag, fmt.Sprintf("%v", v))
		}
	}

	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Git + filesystem helpers
// ---------------------------------------------------------------------------

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

// tcall is a convenience that calls a tool and logs it to the transcript.
func tcall(t *testing.T, tr *transcript, toolName string, args map[string]any) (string, bool) {
	t.Helper()
	text, isErr := call(t, toolName, args)
	tr.toolCall(toolName, args, text, isErr)
	return text, isErr
}

// ---------------------------------------------------------------------------
// Scenario 1: Local project — new developer, fresh repo
// ---------------------------------------------------------------------------

func TestE2E_LocalProject(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "local-project")

	// ── Step 1: Developer has a local Go project with git ──────────────
	tr.stepHeader("Developer has a local Go project with git")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	svcDir := filepath.Join(dir, "user-svc")
	require.NoError(t, os.MkdirAll(svcDir, 0755))
	os.WriteFile(filepath.Join(svcDir, "main.go"), []byte(`package main

import "fmt"

func main() { fmt.Println("user-svc running") }
`), 0644)
	os.WriteFile(filepath.Join(svcDir, "main_test.go"), []byte(`package main

import "testing"

func TestMain_Runs(t *testing.T) {
	// placeholder
}
`), 0644)
	tr.action("Local Go project with git, one service package: user-svc/")

	// ── Step 2: Agent sets up takumi workspace ─────────────────────────
	tr.stepHeader("Agent sets up takumi workspace")
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
	tr.action("Created takumi.yaml, user-svc/takumi-pkg.yaml, committed")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 3: Agent asks "what does this workspace look like?" ────────
	tr.stepHeader("Agent asks: what does this workspace look like?")
	t.Run("status_after_init", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: my-app")
		assert.Contains(t, text, "user-svc v0.1.0")
		assert.Contains(t, text, "Packages (1)")
		assert.Contains(t, text, "AI Agent: claude")
	})

	// ── Step 4: Agent validates the setup ──────────────────────────────
	tr.stepHeader("Agent validates the setup")
	t.Run("validate_clean", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 5: Agent checks the dependency graph ──────────────────────
	tr.stepHeader("Agent checks the dependency graph")
	t.Run("graph_single_pkg", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "1 packages, 1 levels")
		assert.Contains(t, text, "user-svc")
	})

	// ── Step 6: Agent builds the project ───────────────────────────────
	tr.stepHeader("Agent builds the project")
	t.Run("first_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 1 passed")
		assert.Contains(t, text, "user-svc")
		assert.FileExists(t, filepath.Join(dir, ".takumi", "logs", "user-svc.build.log"))
	})

	// ── Step 7: Agent runs tests ───────────────────────────────────────
	tr.stepHeader("Agent runs tests")
	t.Run("first_test", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_test", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 1 passed")
	})

	// ── Step 8: Second build hits cache ────────────────────────────────
	tr.stepHeader("Second build — should hit cache")
	t.Run("cached_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "1 cached")
	})

	// ── Step 9: Developer says "add a /health endpoint" ────────────────
	tr.stepHeader("Developer: 'add a /health endpoint' — agent modifies source")
	os.WriteFile(filepath.Join(svcDir, "main.go"), []byte(`package main

import "fmt"

func main() { fmt.Println("user-svc running") }

func healthCheck() string { return "ok" }
`), 0644)
	tr.action("Added healthCheck() to user-svc/main.go")

	// ── Step 10: Agent checks what's affected ──────────────────────────
	tr.stepHeader("Agent checks what's affected by the change")
	t.Run("affected_after_change", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_affected", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Directly affected")
		assert.Contains(t, text, "user-svc")
	})

	// ── Step 11: Agent builds only affected packages ───────────────────
	tr.stepHeader("Agent builds only affected packages")
	t.Run("affected_build", func(t *testing.T) {
		args := map[string]any{"affected": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed")
		assert.Contains(t, text, "user-svc")
	})

	// ── Step 12: Agent runs tests — this time they fail ────────────────
	tr.stepHeader("Test failure — new health endpoint test fails")
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
	tr.action("Simulated test failure in user-svc test phase")

	t.Run("test_failure", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_test", args)
		assert.True(t, isErr, "test failure should be a tool error")
		assert.Contains(t, text, "failed")
		assert.Contains(t, text, "user-svc")
		assert.Contains(t, text, "Failed package logs")
	})

	// ── Step 13: Agent fixes the test ──────────────────────────────────
	tr.stepHeader("Agent fixes the test — bumps version, fixes test command")
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
	tr.action("Fixed test command, bumped version to 0.2.0")

	// ── Step 15: Agent re-runs tests — now they pass ───────────────────
	tr.stepHeader("Agent re-runs tests after fix")
	t.Run("test_after_fix", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_test", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 1 passed")
	})

	// ── Step 16: Final build before shipping ───────────────────────────
	tr.stepHeader("Final build before shipping")
	t.Run("final_build", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 1 passed")
	})

	// ── Step 17: Verify metrics were recorded ──────────────────────────
	tr.stepHeader("Verify build metrics history")
	t.Run("metrics_recorded", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(dir, ".takumi", "metrics.json"))
		require.NoError(t, err)
		var metrics struct{ Runs []any }
		require.NoError(t, json.Unmarshal(data, &metrics))
		tr.action(fmt.Sprintf("Metrics file has %d recorded runs", len(metrics.Runs)))
		assert.GreaterOrEqual(t, len(metrics.Runs), 4)
	})

	// ── Step 18: Final status ──────────────────────────────────────────
	tr.stepHeader("Final status — the full picture")
	t.Run("final_status", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Recent Builds")
	})
}

// ---------------------------------------------------------------------------
// Scenario 2: GitHub clone — multi-repo workspace with dependencies
// ---------------------------------------------------------------------------

func TestE2E_GitHubClone(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "github-clone")

	// ── Step 1: Simulate a cloned workspace with sources ───────────────
	tr.stepHeader("Clone multi-package workspace from GitHub")
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
	tr.action("Created 3-package workspace: shared-utils, auth-svc, api-gateway")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 2: Agent onboards ─────────────────────────────────────────
	tr.stepHeader("Agent onboards — what am I looking at?")
	t.Run("onboard_status", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: platform")
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "shared-utils")
		assert.Contains(t, text, "auth-svc")
		assert.Contains(t, text, "api-gateway")
	})

	// ── Step 3: Agent explores the dependency graph ────────────────────
	tr.stepHeader("Agent explores the dependency graph")
	t.Run("dependency_graph", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 packages")
		assert.Contains(t, text, "Level 0 (no dependencies)")
		assert.Contains(t, text, "shared-utils")
		assert.Contains(t, text, "auth-svc")
		assert.Contains(t, text, "api-gateway")
	})

	// ── Step 4: Agent validates everything ──────────────────────────────
	tr.stepHeader("Agent validates all configs")
	t.Run("validate_all", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 5: Agent builds the entire workspace ──────────────────────
	tr.stepHeader("Agent builds the entire workspace")
	t.Run("full_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
	})

	// ── Step 6: Agent tests everything ─────────────────────────────────
	tr.stepHeader("Agent tests everything")
	t.Run("full_test", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_test", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 3 passed")
	})

	// ── Step 7: Developer modifies the library ─────────────────────────
	tr.stepHeader("Developer: 'update token validation in shared-utils'")
	os.WriteFile(filepath.Join(utilsDir, "utils.go"), []byte(`package utils

func ValidateToken(token string) bool {
	return len(token) > 0
}
`), 0644)
	tr.action("Modified shared-utils/utils.go")

	// ── Step 8: Agent checks the blast radius ──────────────────────────
	tr.stepHeader("Agent checks the blast radius")
	t.Run("affected_after_lib_change", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_affected", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Directly affected")
		assert.Contains(t, text, "shared-utils")
		assert.Contains(t, text, "Transitively affected")
		assert.Contains(t, text, "auth-svc")
		assert.Contains(t, text, "api-gateway")
		assert.Contains(t, text, "Total affected: 3")
	})

	// ── Step 9: Agent builds only affected packages ────────────────────
	tr.stepHeader("Agent builds only affected packages")
	t.Run("affected_build", func(t *testing.T) {
		args := map[string]any{"affected": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed")
		assert.Contains(t, text, "shared-utils")
	})

	// ── Step 10: Targeted test on the changed package ──────────────────
	tr.stepHeader("Agent tests only the changed package first")
	t.Run("targeted_test", func(t *testing.T) {
		args := map[string]any{"packages": "shared-utils"}
		text, isErr := tcall(t, tr, "takumi_test", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 1 passed")
		assert.Contains(t, text, "shared-utils")
	})

	// ── Step 11: Full test to verify nothing broke ─────────────────────
	tr.stepHeader("Full test to verify nothing broke")
	t.Run("full_test_after_change", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_test", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 3 passed")
	})

	// ── Step 12: Verify caching works ──────────────────────────────────
	tr.stepHeader("Verify caching — unchanged packages should be cached")
	t.Run("build_with_cache", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "cached")
	})

	// ── Step 13: Final status ──────────────────────────────────────────
	tr.stepHeader("Final status — healthy workspace")
	t.Run("final_status", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: platform")
		assert.Contains(t, text, "Recent Builds")
	})
}

// ---------------------------------------------------------------------------
// Scenario 3: Vibe-coder — zero experience, just wants an app
// ---------------------------------------------------------------------------

func TestE2E_VibeCoder(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "vibe-coder")

	// ── Step 1: Agent starts from nothing ──────────────────────────────
	tr.stepHeader("User: 'ok ai broski build me a portfolio app, also use this takumi thing idk'")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "vibes@example.com")
	gitRun(t, dir, "config", "user.name", "vibes")
	tr.action("Agent creates empty git repo")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 2: Agent tries status before setup — gets discovery message ─
	tr.stepHeader("Agent tries takumi status before setup — gets discovery guidance")
	t.Run("no_workspace_yet", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr, "status returns discovery message, not an error")
		assert.Contains(t, text, "not a Takumi workspace")
		assert.Contains(t, text, "takumi init")
	})

	// ── Step 3: Agent sets up workspace from scratch ───────────────────
	tr.stepHeader("Agent reads takumi docs and sets up workspace from scratch")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: my-portfolio
  ai:
    agent: claude
`), 0644)
	tr.action("Created .takumi/ directory and takumi.yaml")

	// ── Step 4: Agent scaffolds frontend ───────────────────────────────
	tr.stepHeader("Agent scaffolds frontend package")
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
	tr.action("Created frontend/ with takumi-pkg.yaml, index.html, app.js")

	// ── Step 5: Agent scaffolds backend ────────────────────────────────
	tr.stepHeader("Agent scaffolds backend package")
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
	tr.action("Created backend/ with takumi-pkg.yaml, main.go")

	// ── Step 6: Agent creates shared types + wires deps ────────────────
	tr.stepHeader("Agent creates shared types package and wires dependencies")
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

	// Wire up dependencies
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
	tr.action("Created types/ with takumi-pkg.yaml, wired frontend/backend deps, committed")

	// ── Step 7: Agent checks what it built ─────────────────────────────
	tr.stepHeader("Agent checks what it built")
	t.Run("status_after_scaffold", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: my-portfolio")
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "frontend")
		assert.Contains(t, text, "backend")
		assert.Contains(t, text, "types")
	})

	// ── Step 8: Agent validates ────────────────────────────────────────
	tr.stepHeader("Agent validates before building")
	t.Run("validate_scaffold", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 9: Agent checks the graph ─────────────────────────────────
	tr.stepHeader("Agent checks the dependency graph")
	t.Run("graph_shows_structure", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 packages")
		assert.Contains(t, text, "types")
	})

	// ── Step 10: Agent builds everything ───────────────────────────────
	tr.stepHeader("Agent builds everything")
	t.Run("first_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
	})

	// ── Step 11: Agent runs tests ──────────────────────────────────────
	tr.stepHeader("Agent runs tests")
	t.Run("first_test", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_test", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed")
		assert.Contains(t, text, "frontend")
		assert.Contains(t, text, "backend")
	})

	// ── Step 12: User requests a contact form — agent introduces error ─
	tr.stepHeader("User: 'add a contact form' -- agent introduces build error")
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
	tr.action("Added contact.js, updated frontend build -- has missing module error")

	// ── Step 13: Agent builds frontend — fails ─────────────────────────
	tr.stepHeader("Agent builds frontend -- fails!")
	t.Run("build_fails_on_new_feature", func(t *testing.T) {
		args := map[string]any{"packages": "frontend", "no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.True(t, isErr)
		assert.Contains(t, text, "failed")
		assert.Contains(t, text, "frontend")
	})

	// ── Step 14: Agent fixes the build ─────────────────────────────────
	tr.stepHeader("Agent fixes the build error")
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
	tr.action("Fixed frontend build commands, removed missing module reference")

	// ── Step 16: Agent rebuilds — passes ───────────────────────────────
	tr.stepHeader("Agent rebuilds frontend -- passes")
	t.Run("build_after_fix", func(t *testing.T) {
		args := map[string]any{"packages": "frontend", "no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 1 passed")
	})

	// ── Step 17: Full test suite before shipping ───────────────────────
	tr.stepHeader("Full test suite before shipping")
	t.Run("full_test_before_ship", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_test", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed")
		assert.Contains(t, text, "frontend")
		assert.Contains(t, text, "backend")
	})

	// ── Step 18: Ship build — cached where possible ────────────────────
	tr.stepHeader("Ship build -- types + backend should be cached")
	t.Run("ship_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "cached")
	})

	// ── Step 19: Final status ──────────────────────────────────────────
	tr.stepHeader("Final status -- portfolio app is ready to ship")
	t.Run("ready_to_ship", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: my-portfolio")
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "Recent Builds")
	})

	// ── Step 20: Verify build history ──────────────────────────────────
	tr.stepHeader("Verify full build history")
	t.Run("build_history", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(dir, ".takumi", "metrics.json"))
		require.NoError(t, err)
		var metrics struct{ Runs []any }
		require.NoError(t, json.Unmarshal(data, &metrics))
		tr.action(fmt.Sprintf("Metrics file has %d recorded runs", len(metrics.Runs)))
		assert.GreaterOrEqual(t, len(metrics.Runs), 5)
	})
}

// Ensure gomcp is used (tool definitions reference it in the server).
var _ = gomcp.TextContent{}
