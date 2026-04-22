package mcp_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Scenario 4: Polyglot workspace — Python + JS + Go in one DAG
//
// Takumi's differentiator over language-specific tools. Verifies that
// runtime envs, different build commands, and the DAG all work together
// across language boundaries.
// ---------------------------------------------------------------------------

func TestE2E_Polyglot(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "polyglot")

	tr.stepHeader("Set up polyglot workspace: Go CLI + Python API + JS frontend")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: polyglot-app
  settings:
    parallel: true
  ai:
    agent: claude
`), 0644)

	// Go CLI — leaf dependency, no runtime needed
	cliDir := filepath.Join(dir, "cli")
	require.NoError(t, os.MkdirAll(cliDir, 0755))
	os.WriteFile(filepath.Join(cliDir, "takumi-pkg.yaml"), []byte(`package:
  name: cli
  version: 1.0.0
phases:
  build:
    commands:
      - echo "go build -o cli ./..."
  test:
    commands:
      - echo "go test ./... ok"
`), 0644)
	os.WriteFile(filepath.Join(cliDir, "main.go"), []byte("package main\n"), 0644)

	// Python API — depends on CLI (uses it as subprocess), has runtime
	apiDir := filepath.Join(dir, "api")
	require.NoError(t, os.MkdirAll(apiDir, 0755))
	os.WriteFile(filepath.Join(apiDir, "takumi-pkg.yaml"), []byte(`package:
  name: api
  version: 0.3.0
dependencies:
  - cli
runtime:
  setup:
    - echo "python3 -m venv {{env_dir}}"
    - echo "pip install -r requirements.txt"
  env:
    PYTHONPATH: ./src
    API_CLI_PATH: ../../cli/cli
phases:
  build:
    commands:
      - echo "python -m compileall src/"
  test:
    commands:
      - echo "pytest tests/ ... 12 passed"
  lint:
    commands:
      - echo "ruff check src/ ... ok"
`), 0644)
	os.WriteFile(filepath.Join(apiDir, "app.py"), []byte("# fastapi app\n"), 0644)

	// JS frontend — depends on API (calls it), has runtime
	webDir := filepath.Join(dir, "web")
	require.NoError(t, os.MkdirAll(webDir, 0755))
	os.WriteFile(filepath.Join(webDir, "takumi-pkg.yaml"), []byte(`package:
  name: web
  version: 0.1.0
dependencies:
  - api
runtime:
  setup:
    - echo "npm install"
  env:
    NODE_ENV: development
    API_URL: http://localhost:8000
phases:
  build:
    commands:
      - echo "vite build ... done"
  test:
    commands:
      - echo "vitest run ... 8 tests passed"
`), 0644)
	os.WriteFile(filepath.Join(webDir, "index.tsx"), []byte("// react app\n"), 0644)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "polyglot workspace setup")
	tr.action("3 packages: cli (Go), api (Python, depends on cli), web (JS, depends on api)")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 2: Status shows all packages with their language context ──
	tr.stepHeader("Agent inspects the workspace")
	t.Run("status_polyglot", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: polyglot-app")
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "cli v1.0.0")
		assert.Contains(t, text, "api v0.3.0")
		assert.Contains(t, text, "web v0.1.0")
		// Runtime packages should show runtime info
		assert.Contains(t, text, "runtime")
	})

	// ── Step 3: Graph shows 3-level DAG across languages ───────────────
	tr.stepHeader("Agent checks the cross-language dependency graph")
	t.Run("graph_polyglot", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 packages")
		assert.Contains(t, text, "cli")        // level 0
		assert.Contains(t, text, "api")        // level 1
		assert.Contains(t, text, "web")        // level 2
	})

	// ── Step 4: Validate — mixed language configs all valid ────────────
	tr.stepHeader("Validate all configs")
	t.Run("validate_polyglot", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 5: Build — should respect dependency order ────────────────
	tr.stepHeader("Build all — respects Go -> Python -> JS dependency order")
	t.Run("build_all", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
		assert.Contains(t, text, "cli")
		assert.Contains(t, text, "api")
		assert.Contains(t, text, "web")
	})

	// ── Step 6: Test — all languages test successfully ──────────────────
	tr.stepHeader("Test all languages")
	t.Run("test_all", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_test", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 3 passed")
	})

	// ── Step 7: Modify the Go CLI — should cascade to Python + JS ──────
	tr.stepHeader("Modify Go CLI — should cascade through Python API to JS frontend")
	os.WriteFile(filepath.Join(cliDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	tr.action("Modified cli/main.go")

	t.Run("affected_cascades_across_languages", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_affected", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Directly affected")
		assert.Contains(t, text, "cli")
		assert.Contains(t, text, "Transitively affected")
		assert.Contains(t, text, "api")
		assert.Contains(t, text, "web")
		assert.Contains(t, text, "Total affected: 3")
	})

	// ── Step 8: Rebuild all after cascade — verify cache state ─────────
	tr.stepHeader("Rebuild all — cli changed so full cascade, verify DAG ordering holds")
	t.Run("rebuild_after_cascade", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", map[string]any{"no_cache": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
	})

	// ── Step 10: Final status — all green across languages ─────────────
	tr.stepHeader("Final status — polyglot workspace all green")
	t.Run("final_status", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: polyglot-app")
		assert.Contains(t, text, "Recent Builds")
	})
}

// ---------------------------------------------------------------------------
// Scenario 5: Cascading failure — root cause in leaf dependency
//
// Change a leaf dependency in a way that breaks a downstream package.
// Agent builds, sees failure in the downstream, traces it back to the
// root cause in the leaf. Tests whether affected gives the
// agent enough signal to find the actual problem, not just the symptom.
// ---------------------------------------------------------------------------

func TestE2E_CascadingFailure(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "cascading-failure")

	tr.stepHeader("Set up 3-package chain: core -> service -> gateway")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: cascade-test
`), 0644)

	// core — leaf dependency
	coreDir := filepath.Join(dir, "core")
	require.NoError(t, os.MkdirAll(coreDir, 0755))
	os.WriteFile(filepath.Join(coreDir, "takumi-pkg.yaml"), []byte(`package:
  name: core
  version: 1.0.0
phases:
  build:
    commands:
      - echo "building core... ok"
  test:
    commands:
      - echo "testing core... PASS"
`), 0644)
	os.WriteFile(filepath.Join(coreDir, "lib.go"), []byte("package core\n"), 0644)

	// service — depends on core
	svcDir := filepath.Join(dir, "service")
	require.NoError(t, os.MkdirAll(svcDir, 0755))
	os.WriteFile(filepath.Join(svcDir, "takumi-pkg.yaml"), []byte(`package:
  name: service
  version: 0.5.0
dependencies:
  - core
phases:
  build:
    commands:
      - echo "building service... ok"
  test:
    commands:
      - echo "testing service... PASS"
`), 0644)
	os.WriteFile(filepath.Join(svcDir, "svc.go"), []byte("package service\n"), 0644)

	// gateway — depends on service (transitive dep on core)
	gwDir := filepath.Join(dir, "gateway")
	require.NoError(t, os.MkdirAll(gwDir, 0755))
	os.WriteFile(filepath.Join(gwDir, "takumi-pkg.yaml"), []byte(`package:
  name: gateway
  version: 0.1.0
dependencies:
  - service
phases:
  build:
    commands:
      - echo "building gateway... ok"
  test:
    commands:
      - echo "testing gateway... PASS"
`), 0644)
	os.WriteFile(filepath.Join(gwDir, "gw.go"), []byte("package gateway\n"), 0644)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial setup")
	tr.action("Chain: core -> service -> gateway")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 2: Everything builds and tests clean ──────────────────────
	tr.stepHeader("Initial build + test — everything green")
	t.Run("initial_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
	})
	t.Run("initial_test", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_test", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 3 passed")
	})

	// ── Step 3: Change in core breaks service's build ──────────────────
	// Core itself builds fine, but service breaks due to the API change
	tr.stepHeader("Breaking change in core — core builds fine, service breaks")
	os.WriteFile(filepath.Join(coreDir, "lib.go"), []byte("package core\n\n// API changed\nfunc NewAPI() {}\n"), 0644)
	os.WriteFile(filepath.Join(svcDir, "takumi-pkg.yaml"), []byte(`package:
  name: service
  version: 0.5.0
dependencies:
  - core
phases:
  build:
    commands:
      - echo "building service..."
      - "echo ERROR: core.OldAPI undefined && exit 1"
  test:
    commands:
      - echo "testing service... PASS"
`), 0644)
	tr.action("Modified core/lib.go; service now fails because it calls removed core.OldAPI")

	// ── Step 4: Agent checks affected — all 3 packages ─────────────────
	tr.stepHeader("Agent checks affected packages")
	t.Run("affected_shows_chain", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_affected", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "core")
		assert.Contains(t, text, "service")
		assert.Contains(t, text, "gateway")
	})

	// ── Step 5: Build fails at service level, not core ─────────────────
	tr.stepHeader("Build — core passes, service fails, gateway never runs")
	t.Run("build_cascading_failure", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.True(t, isErr)
		assert.Contains(t, text, "failed")
		assert.Contains(t, text, "service")
		// Core should have passed
		assert.Contains(t, text, "core")
	})

	// ── Step 6: Agent fixes service to use new API ─────────────────────
	tr.stepHeader("Agent fixes service to use core's new API")
	os.WriteFile(filepath.Join(svcDir, "takumi-pkg.yaml"), []byte(`package:
  name: service
  version: 0.5.1
dependencies:
  - core
phases:
  build:
    commands:
      - echo "building service with new core API... ok"
  test:
    commands:
      - echo "testing service... PASS"
`), 0644)
	tr.action("Updated service to use core.NewAPI()")

	// ── Step 9: Check affected after fix — blast radius still understood ──
	tr.stepHeader("Agent checks affected after fix — verifies blast radius")
	t.Run("affected_after_fix", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_affected", nil)
		assert.False(t, isErr)
		// core and service were both modified, gateway is transitive
		assert.Contains(t, text, "core")
		assert.Contains(t, text, "service")
		assert.Contains(t, text, "gateway")
	})

	// ── Step 10: Full rebuild — everything green ───────────────────────
	tr.stepHeader("Full rebuild — everything green")
	t.Run("build_after_fix", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
	})

	// ── Step 11: Tests pass end-to-end ─────────────────────────────────
	tr.stepHeader("Full test — chain is healthy again")
	t.Run("test_after_fix", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_test", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 3 passed")
	})

	// ── Step 12: Verify gateway independently ─────────────────────────
	// Gateway was skipped during the failed build (correct). Agent verifies
	// it still works on its own now that the chain is fixed.
	tr.stepHeader("Verify gateway independently — it was skipped during failure")
	t.Run("gateway_builds_independently", func(t *testing.T) {
		args := map[string]any{"packages": "gateway", "no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 1 passed")
		assert.Contains(t, text, "gateway")
	})
	t.Run("gateway_tests_independently", func(t *testing.T) {
		args := map[string]any{"packages": "gateway", "no_cache": true}
		text, isErr := tcall(t, tr, "takumi_test", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 1 passed")
		assert.Contains(t, text, "gateway")
	})
}

// ---------------------------------------------------------------------------
// Scenario 6: Cache invalidation chain — selective invalidation
//
// Modify a leaf package, verify all downstream packages invalidate
// but unrelated packages stay cached.
// ---------------------------------------------------------------------------

func TestE2E_CacheInvalidation(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "cache-invalidation")

	tr.stepHeader("Set up diamond dependency graph: A <- B, A <- C, B <- D, C <- D, plus E (isolated)")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: cache-test
  settings:
    parallel: true
`), 0644)

	// A — leaf, depended on by B and C
	for _, pkg := range []struct {
		name string
		deps string
	}{
		{"pkg-a", ""},
		{"pkg-b", "  - pkg-a"},
		{"pkg-c", "  - pkg-a"},
		{"pkg-d", "  - pkg-b\n  - pkg-c"},
		{"pkg-e", ""}, // isolated — no deps on A
	} {
		pkgDir := filepath.Join(dir, pkg.name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		depSection := ""
		if pkg.deps != "" {
			depSection = fmt.Sprintf("dependencies:\n%s\n", pkg.deps)
		}
		os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(fmt.Sprintf(`package:
  name: %s
  version: 1.0.0
%sphases:
  build:
    commands:
      - echo "building %s..."
  test:
    commands:
      - echo "testing %s... PASS"
`, pkg.name, depSection, pkg.name, pkg.name)), 0644)
		os.WriteFile(filepath.Join(pkgDir, "src.go"), []byte(fmt.Sprintf("package %s\n", pkg.name)), 0644)
	}

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "diamond graph setup")
	tr.action("Diamond: pkg-a <- pkg-b, pkg-a <- pkg-c, pkg-b+pkg-c <- pkg-d, pkg-e (isolated)")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 2: Verify graph structure ─────────────────────────────────
	tr.stepHeader("Verify diamond graph structure")
	t.Run("graph_diamond", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "5 packages")
		assert.Contains(t, text, "pkg-a")
		assert.Contains(t, text, "pkg-e")
	})

	// ── Step 3: Initial build — all 5 built ────────────────────────────
	tr.stepHeader("Initial build — all 5 packages")
	t.Run("initial_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 5 passed")
	})

	// ── Step 4: Immediate rebuild — all 5 cached ───────────────────────
	tr.stepHeader("Immediate rebuild — all 5 should be cached")
	t.Run("all_cached", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "5 cached")
	})

	// ── Step 5: Modify pkg-a (the leaf) ────────────────────────────────
	tr.stepHeader("Modify pkg-a — downstream B, C, D should invalidate; E should stay cached")
	os.WriteFile(filepath.Join(dir, "pkg-a", "src.go"), []byte("package pkg_a\n\nfunc Changed() {}\n"), 0644)
	tr.action("Modified pkg-a/src.go")

	// ── Step 6: Build again — A, B, C, D rebuild; E stays cached ───────
	tr.stepHeader("Build after leaf change — selective invalidation")
	t.Run("selective_invalidation", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		// 4 packages rebuilt (A, B, C, D), 1 cached (E)
		assert.Contains(t, text, "4 passed")
		assert.Contains(t, text, "1 cached")
		// E should be the cached one
		assert.Contains(t, text, "pkg-e")
		assert.Contains(t, text, "cached")
	})

	// ── Step 7: Modify only pkg-e — everything else stays cached ───────
	tr.stepHeader("Modify pkg-e (isolated) — only E should rebuild")
	os.WriteFile(filepath.Join(dir, "pkg-e", "src.go"), []byte("package pkg_e\n\nfunc Independent() {}\n"), 0644)
	tr.action("Modified pkg-e/src.go")

	t.Run("isolated_change", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		// 1 rebuilt (E), 4 cached (A, B, C, D)
		assert.Contains(t, text, "1 passed")
		assert.Contains(t, text, "4 cached")
	})

	// ── Step 8: Config-only change — modify pkg-c's YAML, don't touch source ──
	tr.stepHeader("Config-only change — modify pkg-c's takumi-pkg.yaml without touching source")
	os.WriteFile(filepath.Join(dir, "pkg-c", "takumi-pkg.yaml"), []byte(`package:
  name: pkg-c
  version: 1.1.0
dependencies:
  - pkg-a
phases:
  build:
    commands:
      - echo "building pkg-c with new config..."
  test:
    commands:
      - echo "testing pkg-c... PASS"
`), 0644)
	tr.action("Changed pkg-c version 1.0.0 -> 1.1.0 and build command; source file untouched")

	t.Run("config_change_invalidates", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		// pkg-c's config changed → pkg-c rebuilds, pkg-d depends on pkg-c → rebuilds
		// pkg-a, pkg-b, pkg-e should stay cached
		assert.Contains(t, text, "2 passed")
		assert.Contains(t, text, "3 cached")
	})

	// ── Step 9: No changes — everything cached again ───────────────────
	tr.stepHeader("No changes — all cached")
	t.Run("all_cached_again", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "5 cached")
	})
}

// ---------------------------------------------------------------------------
// Scenario 7: Recovery from bad state
//
// Corrupted cache, missing .takumi/ directory, malformed takumi-pkg.yaml,
// circular dependency introduced mid-session. Tests that the agent gets
// useful errors and can recover without human intervention.
// ---------------------------------------------------------------------------

func TestE2E_Recovery(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "recovery")

	tr.stepHeader("Set up a healthy 2-package workspace")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "cache"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: recovery-test
`), 0644)

	for _, name := range []string{"alpha", "beta"} {
		pkgDir := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		deps := ""
		if name == "beta" {
			deps = "dependencies:\n  - alpha\n"
		}
		os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(fmt.Sprintf(`package:
  name: %s
  version: 1.0.0
%sphases:
  build:
    commands:
      - echo "building %s..."
  test:
    commands:
      - echo "testing %s... PASS"
`, name, deps, name, name)), 0644)
		os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte(fmt.Sprintf("package %s\n", name)), 0644)
	}

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial setup")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 2: Build successfully first ───────────────────────────────
	tr.stepHeader("Initial build — establish baseline")
	t.Run("initial_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 2 passed")
	})

	// ── Step 3: Corrupt the cache — write garbage to cache file ────────
	tr.stepHeader("Corrupt cache — write garbage to alpha.build.json")
	cacheFile := filepath.Join(dir, ".takumi", "cache", "alpha.build.json")
	os.WriteFile(cacheFile, []byte("NOT VALID JSON{{{"), 0644)
	tr.action("Wrote garbage to .takumi/cache/alpha.build.json")

	t.Run("build_with_corrupt_cache", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		// Should rebuild alpha (cache miss due to corruption) but not crash
		assert.Contains(t, text, "Build completed")
		assert.Contains(t, text, "alpha")
	})

	// ── Step 4: Malformed takumi-pkg.yaml ──────────────────────────────
	tr.stepHeader("Introduce malformed takumi-pkg.yaml")
	os.WriteFile(filepath.Join(dir, "alpha", "takumi-pkg.yaml"), []byte("this is: [not: valid: yaml: {{{\n"), 0644)
	tr.action("Wrote invalid YAML to alpha/takumi-pkg.yaml")

	t.Run("validate_catches_parse_error", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.True(t, isErr)
		// Should report the parse error, not crash
		assert.Contains(t, text, "alpha")
	})

	// ── Step 5: Fix the YAML, then introduce a circular dependency ─────
	tr.stepHeader("Fix YAML, then introduce circular dependency")
	os.WriteFile(filepath.Join(dir, "alpha", "takumi-pkg.yaml"), []byte(`package:
  name: alpha
  version: 1.0.0
dependencies:
  - beta
phases:
  build:
    commands:
      - echo "building alpha..."
  test:
    commands:
      - echo "testing alpha... PASS"
`), 0644)
	tr.action("alpha now depends on beta, beta already depends on alpha -> cycle")

	t.Run("validate_detects_cycle", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.True(t, isErr)
		assert.Contains(t, text, "cycle")
	})

	t.Run("build_rejects_cycle", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.True(t, isErr)
		assert.Contains(t, text, "cycle")
	})

	// ── Step 6: Agent fixes the cycle ──────────────────────────────────
	tr.stepHeader("Agent fixes the cycle — removes circular dependency")
	os.WriteFile(filepath.Join(dir, "alpha", "takumi-pkg.yaml"), []byte(`package:
  name: alpha
  version: 1.0.0
phases:
  build:
    commands:
      - echo "building alpha..."
  test:
    commands:
      - echo "testing alpha... PASS"
`), 0644)
	tr.action("Removed alpha's dependency on beta")

	t.Run("validate_after_fix", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 7: Build recovers ─────────────────────────────────────────
	tr.stepHeader("Build recovers after all fixes")
	t.Run("build_recovers", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 2 passed")
	})

	// ── Step 8: Delete entire .takumi/cache/ — cold-start recovery ────
	tr.stepHeader("Delete entire .takumi/cache/ — cold-start recovery")
	os.RemoveAll(filepath.Join(dir, ".takumi", "cache"))
	tr.action("Removed .takumi/cache/ directory entirely")

	t.Run("build_cold_start", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		// All packages rebuild from scratch — no cache at all
		assert.Contains(t, text, "Build completed: 2 passed")
	})

	// ── Step 9: Verify cache was recreated ─────────────────────────────
	tr.stepHeader("Verify cache was recreated — rebuild should be fully cached")
	t.Run("cache_recreated", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "2 cached")
	})

	// ── Step 10: Delete .takumi/logs/ — verify rebuild recreates them ──
	tr.stepHeader("Delete .takumi/logs/ — rebuild should recreate")
	os.RemoveAll(filepath.Join(dir, ".takumi", "logs"))
	tr.action("Removed .takumi/logs/ directory entirely")

	// ── Step 11: Rebuild creates the logs again ────────────────────────
	tr.stepHeader("Rebuild — logs directory recreated")
	t.Run("build_recreates_logs", func(t *testing.T) {
		args := map[string]any{"no_cache": true}
		text, isErr := tcall(t, tr, "takumi_build", args)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 2 passed")
		assert.FileExists(t, filepath.Join(dir, ".takumi", "logs", "alpha.build.log"))
	})

	// ── Step 12: Delete .takumi/ marker entirely — status should error ─
	tr.stepHeader("Delete .takumi/ marker directory — workspace becomes uninitialized")
	os.RemoveAll(filepath.Join(dir, ".takumi"))
	tr.action("Removed .takumi/ marker directory entirely")

	t.Run("status_no_marker", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr, "status returns discovery message, not an error")
		// Should guide agent to re-init, not crash
		assert.Contains(t, text, "not a Takumi workspace")
		assert.Contains(t, text, "takumi init")
	})

	// ── Step 13: Recreate marker — workspace recovers ──────────────────
	tr.stepHeader("Recreate .takumi/ marker — workspace recovers")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	tr.action("Recreated .takumi/ and .takumi/logs/")

	t.Run("status_after_marker_recreated", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: recovery-test")
		assert.Contains(t, text, "Packages (2)")
	})

	t.Run("build_after_marker_recreated", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 2 passed")
	})
}

// ---------------------------------------------------------------------------
// Scenario 8: Multi-session — second agent picks up from first
//
// Agent onboards into an existing workspace that another agent already
// built. Verifies that cache from the previous session is usable.
// ---------------------------------------------------------------------------

func TestE2E_MultiSession(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "multi-session")

	// ── Session 1: First agent sets up and builds ──────────────────────
	tr.stepHeader("Session 1: First agent sets up and builds the workspace")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "agent1@example.com")
	gitRun(t, dir, "config", "user.name", "agent1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: shared-project
  ai:
    agent: claude
`), 0644)

	for _, name := range []string{"auth", "payments", "notifications"} {
		pkgDir := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		deps := ""
		if name == "payments" || name == "notifications" {
			deps = "dependencies:\n  - auth\n"
		}
		os.WriteFile(filepath.Join(pkgDir, "takumi-pkg.yaml"), []byte(fmt.Sprintf(`package:
  name: %s
  version: 1.0.0
%sphases:
  build:
    commands:
      - echo "building %s..."
  test:
    commands:
      - echo "testing %s... PASS"
`, name, deps, name, name)), 0644)
		os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte(fmt.Sprintf("package %s\n", name)), 0644)
	}

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial setup by agent 1")
	tr.action("Agent 1 created workspace with auth, payments, notifications")

	chdir(t, dir)
	tr.setRoot(dir)

	t.Run("session1_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 3 passed")
	})

	t.Run("session1_test", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_test", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 3 passed")
	})
	tr.action("Session 1 complete — workspace built and tested, cache populated")

	// ── Session 2: Second agent arrives ────────────────────────────────
	tr.stepHeader("Session 2: New agent arrives — onboards into existing workspace")

	t.Run("session2_status", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: shared-project")
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "auth")
		assert.Contains(t, text, "payments")
		assert.Contains(t, text, "notifications")
		assert.Contains(t, text, "Recent Builds")
	})

	tr.stepHeader("Session 2: Agent explores the graph")
	t.Run("session2_graph", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 packages")
		assert.Contains(t, text, "auth")
	})

	// ── Session 2: Cache from session 1 is usable ──────────────────────
	tr.stepHeader("Session 2: Build — cache from session 1 should still be valid")
	t.Run("session2_cache_reuse", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		// All 3 should be cached — nothing changed since session 1
		assert.Contains(t, text, "3 cached")
	})

	// ── Session 2: Agent makes a change ────────────────────────────────
	tr.stepHeader("Session 2: Agent modifies auth — should cascade to payments + notifications")
	os.WriteFile(filepath.Join(dir, "auth", "main.go"), []byte("package auth\n\nfunc Login() {}\n"), 0644)
	tr.action("Modified auth/main.go — both payments and notifications depend on auth")

	t.Run("session2_selective_rebuild", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		// auth changed → auth rebuilds
		// payments depends on auth → cache key changes → payments rebuilds
		// notifications depends on auth → cache key changes → notifications rebuilds
		// All 3 should rebuild, proving cross-session DAG invalidation
		assert.Contains(t, text, "3 passed")
	})

	// ── Session 2: Final status ────────────────────────────────────────
	tr.stepHeader("Session 2: Final status — both sessions' history visible")
	t.Run("session2_final", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Recent Builds")
	})
}

// ---------------------------------------------------------------------------
// Scenario 9: Adding a new package mid-session
//
// Agent starts with 2 packages, creates a third that depends on one,
// wires it into the DAG, builds. Tests the full lifecycle of package
// creation without takumi init.
// ---------------------------------------------------------------------------

func TestE2E_AddPackageMidSession(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "add-package")

	tr.stepHeader("Start with 2-package workspace")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(`workspace:
  name: growing-project
`), 0644)

	libDir := filepath.Join(dir, "lib")
	require.NoError(t, os.MkdirAll(libDir, 0755))
	os.WriteFile(filepath.Join(libDir, "takumi-pkg.yaml"), []byte(`package:
  name: lib
  version: 1.0.0
phases:
  build:
    commands:
      - echo "building lib..."
  test:
    commands:
      - echo "testing lib... PASS"
`), 0644)
	os.WriteFile(filepath.Join(libDir, "lib.go"), []byte("package lib\n"), 0644)

	appDir := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0755))
	os.WriteFile(filepath.Join(appDir, "takumi-pkg.yaml"), []byte(`package:
  name: app
  version: 0.1.0
dependencies:
  - lib
phases:
  build:
    commands:
      - echo "building app..."
  test:
    commands:
      - echo "testing app... PASS"
`), 0644)
	os.WriteFile(filepath.Join(appDir, "main.go"), []byte("package main\n"), 0644)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial 2-package workspace")
	tr.action("Workspace: lib -> app")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 2: Verify starting state ──────────────────────────────────
	tr.stepHeader("Verify starting state — 2 packages")
	t.Run("initial_status", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Packages (2)")
		assert.Contains(t, text, "lib")
		assert.Contains(t, text, "app")
	})

	t.Run("initial_graph", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "2 packages")
	})

	// ── Step 3: Build the initial workspace ────────────────────────────
	tr.stepHeader("Build initial workspace")
	t.Run("initial_build", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 2 passed")
	})

	// ── Step 4: Agent creates a new package with a broken build ────────
	tr.stepHeader("Agent creates a new 'worker' package — build will fail on first attempt")
	workerDir := filepath.Join(dir, "worker")
	require.NoError(t, os.MkdirAll(workerDir, 0755))
	os.WriteFile(filepath.Join(workerDir, "takumi-pkg.yaml"), []byte(`package:
  name: worker
  version: 0.1.0
dependencies:
  - lib
phases:
  build:
    commands:
      - "echo ERROR: missing import && exit 1"
  test:
    commands:
      - echo "testing worker... PASS"
`), 0644)
	os.WriteFile(filepath.Join(workerDir, "worker.go"), []byte("package worker\n"), 0644)
	tr.action("Created worker/ with a broken build command (exit 1)")

	// ── Step 5: Build fails on new package, existing packages stay cached
	tr.stepHeader("Build — worker fails, lib and app should stay cached")
	t.Run("worker_build_fails", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.True(t, isErr)
		assert.Contains(t, text, "failed")
		assert.Contains(t, text, "worker")
		// lib should be cached from step 3
		assert.Contains(t, text, "cached")
	})

	os.WriteFile(filepath.Join(workerDir, "takumi-pkg.yaml"), []byte(`package:
  name: worker
  version: 0.1.0
dependencies:
  - lib
phases:
  build:
    commands:
      - echo "building worker..."
  test:
    commands:
      - echo "testing worker... PASS"
`), 0644)
	tr.action("Fixed worker build command")

	t.Run("worker_build_after_fix", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed")
		assert.Contains(t, text, "worker")
		// lib and app should still be cached — they were never invalidated
		assert.Contains(t, text, "cached")
	})

	// ── Step 7: Status sees the new package ────────────────────────────
	tr.stepHeader("Status picks up the new package without re-init")
	t.Run("status_sees_new_pkg", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Packages (3)")
		assert.Contains(t, text, "worker")
	})

	// ── Step 8: Graph includes the new package ────────────────────────
	tr.stepHeader("Graph includes worker in the DAG")
	t.Run("graph_with_new_pkg", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 packages")
		assert.Contains(t, text, "worker")
	})

	// ── Step 9: Validate — new package passes ─────────────────────────
	tr.stepHeader("Validate — new package config is valid")
	t.Run("validate_with_new_pkg", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 10: Build — all cached (worker already fixed in step 6) ──
	tr.stepHeader("Build — all cached after fix")
	t.Run("build_with_new_pkg", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 cached")
	})

	// ── Step 11: Test the new package ──────────────────────────────────
	tr.stepHeader("Test — includes worker")
	t.Run("test_with_new_pkg", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_test", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed")
		assert.Contains(t, text, "worker")
	})

	// ── Step 12: Modify lib — cascades to both app and worker ──────────
	tr.stepHeader("Modify lib — should invalidate both app and worker")
	os.WriteFile(filepath.Join(libDir, "lib.go"), []byte("package lib\n\nfunc Shared() {}\n"), 0644)
	tr.action("Modified lib/lib.go")

	t.Run("cascade_to_both_consumers", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		// All 3 should rebuild — lib changed, app and worker depend on it
		assert.Contains(t, text, "3 passed")
	})

	// ── Step 13: Everything cached now ─────────────────────────────────
	tr.stepHeader("Everything cached after rebuild")
	t.Run("all_cached", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "3 cached")
	})
}

// ---------------------------------------------------------------------------
// Scenario 10: Fullstack workspace — agent discovers custom phases
//
// Models a real situation: an agent is dropped into a fullstack project
// (backend + frontend) that has custom phases (deploy, lint, dev) beyond
// the standard build/test. The agent must discover the workspace structure,
// available phases, and AI context through Takumi's MCP tools.
// ---------------------------------------------------------------------------

func TestE2E_FullstackDiscovery(t *testing.T) {
	dir := t.TempDir()
	tr := newTranscript(t, "fullstack-discovery")

	// ── Step 1: Set up fullstack workspace ─────────────────────────
	tr.stepHeader("Set up fullstack workspace: backend (Python/Fly) + frontend (JS/Vercel)")
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "dev@example.com")
	gitRun(t, dir, "config", "user.name", "dev")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".takumi", "logs"), 0755))

	wsYAML := "workspace:\n  name: fullstack-app\n  settings:\n    parallel: true\n  ai:\n    agent: claude\n"
	os.WriteFile(filepath.Join(dir, "takumi.yaml"), []byte(wsYAML), 0644)

	// Backend — Python API with deploy, lint, and dev phases
	backendDir := filepath.Join(dir, "backend")
	require.NoError(t, os.MkdirAll(backendDir, 0755))
	backendPkg := `package:
  name: backend
  version: 1.2.0
runtime:
  setup:
    - echo "python3 -m venv {{env_dir}}"
    - echo "pip install -r requirements.txt"
  env:
    PYTHONPATH: ./src
    DATABASE_URL: postgres://localhost/dev
phases:
  build:
    commands:
      - echo "python -m compileall src/"
  test:
    commands:
      - echo "pytest tests/ ... 8 passed"
  lint:
    commands:
      - echo "ruff check src/ ... ok"
  deploy:
    commands:
      - echo "fly deploy --app backend-prod"
  dev:
    commands:
      - echo "uvicorn main:app --reload"
ai:
  description: "Python FastAPI backend — deployed to Fly.io"
  notes:
    - "Deploy requires FLY_API_TOKEN in environment"
    - "Integration tests need DATABASE_URL pointed at Supabase"
    - "Run lint before deploy to catch formatting issues"
`
	os.WriteFile(filepath.Join(backendDir, "takumi-pkg.yaml"), []byte(backendPkg), 0644)
	os.WriteFile(filepath.Join(backendDir, "main.py"), []byte("# fastapi app\n"), 0644)

	// Frontend — JS app with deploy and dev phases, depends on backend
	frontendDir := filepath.Join(dir, "frontend")
	require.NoError(t, os.MkdirAll(frontendDir, 0755))
	frontendPkg := `package:
  name: frontend
  version: 0.8.0
dependencies:
  - backend
runtime:
  setup:
    - echo "npm install"
  env:
    NODE_ENV: development
    API_URL: http://localhost:8000
phases:
  build:
    commands:
      - echo "vite build ... done"
  test:
    commands:
      - echo "vitest run ... 14 passed"
  lint:
    commands:
      - echo "eslint src/ ... ok"
  deploy:
    commands:
      - echo "vercel deploy --prod"
  dev:
    commands:
      - echo "vite dev --port 3000"
ai:
  description: "React frontend — deployed to Vercel, talks to backend API"
  notes:
    - "API_URL must match backend deploy target"
    - "Run build before deploy to verify production bundle"
`
	os.WriteFile(filepath.Join(frontendDir, "takumi-pkg.yaml"), []byte(frontendPkg), 0644)
	os.WriteFile(filepath.Join(frontendDir, "index.js"), []byte("// react app\n"), 0644)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial fullstack workspace")
	tr.action("Workspace: backend -> frontend, both have build/test/lint/deploy/dev phases")

	chdir(t, dir)
	tr.setRoot(dir)

	// ── Step 2: Agent lands in workspace, checks status ────────────
	tr.stepHeader("Agent discovers the workspace via status")
	t.Run("status_shows_packages", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Workspace: fullstack-app")
		assert.Contains(t, text, "Packages (2)")
		assert.Contains(t, text, "backend")
		assert.Contains(t, text, "frontend")
		assert.Contains(t, text, "AI Agent: claude")
	})

	// ── Step 3: Agent checks dependency graph ──────────────────────
	tr.stepHeader("Agent checks dependency graph — frontend depends on backend")
	t.Run("graph_shows_dependency", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_graph", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "2 packages")
		assert.Contains(t, text, "backend")
		assert.Contains(t, text, "frontend")
	})

	// ── Step 4: Agent validates config ─────────────────────────────
	tr.stepHeader("Agent validates — all configs pass")
	t.Run("validate_clean", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_validate", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "All configurations valid")
	})

	// ── Step 5: Status reports phase count (5 per package) ─────────
	tr.stepHeader("Status reports custom phases — agent can see deploy/lint/dev exist")
	t.Run("status_shows_phase_count", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		// Each package has 5 phases (build, test, lint, deploy, dev)
		assert.Contains(t, text, "5 phases")
		// Both have runtimes
		assert.Contains(t, text, "runtime")
	})

	// ── Step 6: Agent builds both in parallel ──────────────────────
	tr.stepHeader("Agent builds — backend first (leaf), then frontend")
	t.Run("build_both", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 2 passed")
	})

	// ── Step 7: Agent runs tests ───────────────────────────────────
	tr.stepHeader("Agent runs tests — pytest + vitest in dependency order")
	t.Run("test_both", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_test", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Test completed: 2 passed")
	})

	// ── Step 8: Rebuild is cached ���─────────────────────────────────
	tr.stepHeader("Rebuild — everything cached, no wasted cycles")
	t.Run("build_cached", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "2 cached")
	})

	// ── Step 9: Agent modifies backend, checks affected ���───────────
	tr.stepHeader("Modify backend — frontend should be affected too (downstream dep)")
	os.WriteFile(filepath.Join(backendDir, "main.py"), []byte("# fastapi app v2\nimport auth\n"), 0644)
	tr.action("Modified backend/main.py")

	t.Run("affected_cascades", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_affected", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "backend")
		assert.Contains(t, text, "frontend")
		assert.Contains(t, text, "Total affected: 2")
	})

	// ── Step 10: Targeted build of only affected packages ──────────
	tr.stepHeader("Build only affected packages")
	t.Run("build_affected", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", map[string]any{"affected": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "2 passed")
	})

	// ── Step 11: Break the frontend build ──────────────────────────
	tr.stepHeader("Frontend build breaks — agent needs to investigate")
	frontendBroken := `package:
  name: frontend
  version: 0.8.0
dependencies:
  - backend
runtime:
  setup:
    - echo "npm install"
  env:
    NODE_ENV: development
    API_URL: http://localhost:8000
phases:
  build:
    commands:
      - "echo ERROR: Cannot find module react && exit 1"
  test:
    commands:
      - echo "vitest run ... 14 passed"
  lint:
    commands:
      - echo "eslint src/ ... ok"
  deploy:
    commands:
      - echo "vercel deploy --prod"
  dev:
    commands:
      - echo "vite dev --port 3000"
ai:
  description: "React frontend — deployed to Vercel, talks to backend API"
  notes:
    - "API_URL must match backend deploy target"
    - "Run build before deploy to verify production bundle"
`
	os.WriteFile(filepath.Join(frontendDir, "takumi-pkg.yaml"), []byte(frontendBroken), 0644)
	tr.action("Broke frontend build — missing react module")

	t.Run("frontend_build_fails", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.True(t, isErr)
		assert.Contains(t, text, "failed")
		assert.Contains(t, text, "frontend")
		// Backend should be cached — it didn't change
		assert.Contains(t, text, "cached")
	})


	// ── Step 13: Agent fixes the build ─────────────────────────────
	tr.stepHeader("Agent fixes the build and verifies")
	os.WriteFile(filepath.Join(frontendDir, "takumi-pkg.yaml"), []byte(frontendPkg), 0644)
	tr.action("Fixed frontend build command")

	t.Run("frontend_builds_after_fix", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed")
		assert.Contains(t, text, "frontend")
	})

	// ── Step 14: Build just one package by name ────────────────────
	tr.stepHeader("Targeted build — just backend")
	t.Run("build_single_package", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_build", map[string]any{"packages": "backend", "no_cache": true})
		assert.False(t, isErr)
		assert.Contains(t, text, "Build completed: 1 passed")
		assert.Contains(t, text, "backend")
	})

	// ── Step 15: Final status — agent sees full picture ────────────
	tr.stepHeader("Final status — agent has full workspace context")
	t.Run("final_status", func(t *testing.T) {
		text, isErr := tcall(t, tr, "takumi_status", nil)
		assert.False(t, isErr)
		assert.Contains(t, text, "fullstack-app")
		assert.Contains(t, text, "backend v1.2.0")
		assert.Contains(t, text, "frontend v0.8.0")
		assert.Contains(t, text, "Recent Builds")
	})
}
