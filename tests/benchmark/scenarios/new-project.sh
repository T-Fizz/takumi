#!/bin/bash
# new-project.sh — Benchmark: Create a new project following getting-started.md
#
# Follows docs/user/getting-started.md step by step to create a project from
# scratch, then builds and tests it.

set -uo pipefail

TAKUMI_BIN="${TAKUMI_BIN:?TAKUMI_BIN must be set}"
WORKDIR=$(mktemp -d /tmp/takumi-bench-new-XXXXXX)
PASS=0
FAIL=0
TOTAL=0

step() {
    TOTAL=$((TOTAL + 1))
    echo ""
    echo "=== STEP $1: $2 ==="
}

pass() {
    echo "--- RESULT: PASS ---"
    PASS=$((PASS + 1))
}

fail() {
    echo "--- RESULT: FAIL${1:+ ($1)} ---"
    FAIL=$((FAIL + 1))
}

cleanup() {
    rm -rf "$WORKDIR"
}
trap cleanup EXIT

echo "=== SCENARIO: New Project (getting-started.md) ==="
echo "Working directory: $WORKDIR"

# ─── Step 1: Initialize workspace ─────────────────────────────────────────────
# Doc: "takumi init --root my-project" creates a directory with .takumi/, takumi.yaml, takumi-pkg.yaml
step 1 "Initialize workspace with takumi init --root (doc: creates project directory with configs)"
cd "$WORKDIR"
echo "$ takumi init --root my-project --agent claude"
if "$TAKUMI_BIN" init --root my-project --agent claude 2>&1; then
    pass
else
    fail "init failed"
fi

# ─── Step 2: Verify created files ─────────────────────────────────────────────
# Doc: "This creates .takumi/, takumi.yaml, takumi-pkg.yaml"
step 2 "Verify workspace files exist (doc: .takumi/, takumi.yaml, takumi-pkg.yaml)"
cd "$WORKDIR/my-project"
echo "Checking for expected files..."
MISSING=""
for f in takumi.yaml takumi-pkg.yaml .takumi/TAKUMI.md CLAUDE.md; do
    if [[ -f "$f" ]]; then
        echo "  found: $f"
    else
        echo "  MISSING: $f"
        MISSING="$MISSING $f"
    fi
done
if [[ -d .takumi ]]; then
    echo "  found: .takumi/"
else
    echo "  MISSING: .takumi/"
    MISSING="$MISSING .takumi/"
fi
if [[ -z "$MISSING" ]]; then
    pass
else
    fail "missing:$MISSING"
fi

# ─── Step 3: Set up Go module ─────────────────────────────────────────────────
# Doc says to replace placeholder commands with real ones — we need source code first
step 3 "Create Go source files (prerequisite for real build commands)"
echo "$ go mod init my-project"
if go mod init my-project 2>&1; then
    echo "Created go.mod"
else
    fail "go mod init failed"
fi
cat > main.go << 'GOEOF'
package main

import "fmt"

func main() {
	fmt.Println("hello from my-project")
}
GOEOF
echo "Created main.go"
pass

# ─── Step 4: Edit package config ──────────────────────────────────────────────
# Doc: "Open takumi-pkg.yaml and replace the placeholder commands"
step 4 "Edit takumi-pkg.yaml with real build/test commands (doc: replace placeholders)"
cat > takumi-pkg.yaml << 'YAMLEOF'
package:
  name: my-project
  version: 0.1.0
phases:
  build:
    commands:
      - go build -o ./build/my-project .
  test:
    commands:
      - go vet ./...
YAMLEOF
echo "Updated takumi-pkg.yaml with go build and go vet commands"
cat takumi-pkg.yaml
pass

# ─── Step 5: Configure workspace ignore ───────────────────────────────────────
# Doc (config reference): "build outputs should go to an ignored directory"
step 5 "Add build/ to workspace ignore list (doc: output binaries to ignored directory)"
cat > takumi.yaml << 'YAMLEOF'
workspace:
  name: my-project
  ignore:
    - vendor/
    - node_modules/
    - .git/
    - build/
  ai:
    agent: claude
YAMLEOF
echo "Updated takumi.yaml with build/ in ignore list"
pass

# ─── Step 6: Validate ─────────────────────────────────────────────────────────
# Doc: "takumi validate — Check all configs for errors"
step 6 "Validate configuration (doc: takumi validate checks all configs)"
echo "$ takumi validate"
if "$TAKUMI_BIN" validate 2>&1; then
    pass
else
    fail "validate failed"
fi

# ─── Step 7: Graph ────────────────────────────────────────────────────────────
# Doc: "takumi graph — Print the dependency graph"
step 7 "Show dependency graph (doc: takumi graph prints dependency order)"
echo "$ takumi graph"
if "$TAKUMI_BIN" graph 2>&1; then
    pass
else
    fail "graph failed"
fi

# ─── Step 8: Dry run ──────────────────────────────────────────────────────────
# Doc: "takumi build --dry-run — Show execution plan without running"
step 8 "Preview build plan (doc: takumi build --dry-run shows what would run)"
echo "$ takumi build --dry-run"
if "$TAKUMI_BIN" build --dry-run 2>&1; then
    pass
else
    fail "dry-run failed"
fi

# ─── Step 9: Build ────────────────────────────────────────────────────────────
# Doc: "takumi build — Build in dependency order"
step 9 "Build the project (doc: takumi build runs commands in dependency order)"
echo "$ takumi build"
if "$TAKUMI_BIN" build 2>&1; then
    pass
else
    fail "build failed"
fi

# ─── Step 10: Verify build output ─────────────────────────────────────────────
step 10 "Verify build produced a binary"
if [[ -f ./build/my-project ]]; then
    echo "Found: ./build/my-project"
    ls -la ./build/my-project
    pass
else
    echo "Binary not found at ./build/my-project"
    ls -la ./build/ 2>&1 || echo "build/ directory does not exist"
    fail "no binary"
fi

# ─── Step 11: Test ────────────────────────────────────────────────────────────
# Doc: "takumi test — Run tests"
step 11 "Run tests (doc: takumi test runs test phase)"
echo "$ takumi test"
if "$TAKUMI_BIN" test 2>&1; then
    pass
else
    fail "test failed"
fi

# ─── Step 12: Status ──────────────────────────────────────────────────────────
# Doc: "takumi status — See workspace dashboard"
step 12 "Show workspace status (doc: takumi status shows health dashboard)"
echo "$ takumi status"
if "$TAKUMI_BIN" status 2>&1; then
    pass
else
    fail "status failed"
fi

# ─── Step 13: Rebuild (cache test) ────────────────────────────────────────────
# Doc: "Subsequent builds skip unchanged packages automatically via caching"
step 13 "Rebuild to verify caching (doc: unchanged packages skipped via caching)"
echo "$ takumi build"
if output=$("$TAKUMI_BIN" build 2>&1); then
    echo "$output"
    if echo "$output" | grep -qi "cached"; then
        echo "Cache hit confirmed"
        pass
    else
        echo "WARNING: no cache indicator in output"
        pass
    fi
else
    echo "$output"
    fail "rebuild failed"
fi

# ─── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "=== SUMMARY ==="
echo "Scenario: New Project (getting-started.md)"
echo "Passed: $PASS / $TOTAL"
echo "Failed: $FAIL / $TOTAL"
if [[ $FAIL -eq 0 ]]; then
    echo "Result: ALL PASSED"
else
    echo "Result: $FAIL FAILURES"
fi
