#!/bin/bash
# agent-discovery.sh — Benchmark: Agent discovers Takumi in existing project
#
# Simulates a real scenario: a developer has an existing polyglot project
# (Go backend + JS frontend) with NO Takumi setup. They install Takumi,
# register the MCP server, and an agent discovers it via takumi_status,
# then sets up the workspace and uses Takumi for all build/test operations.
#
# This tests the cold-start discovery flow that every new user hits.

set -uo pipefail

TAKUMI_BIN="${TAKUMI_BIN:?TAKUMI_BIN must be set}"
TAKUMI_BIN="$(cd "$(dirname "$TAKUMI_BIN")" && pwd)/$(basename "$TAKUMI_BIN")"
WORKDIR=$(mktemp -d /tmp/takumi-bench-discovery-XXXXXX)
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

echo "=== SCENARIO: Agent Discovery (existing project, no Takumi setup) ==="
echo "Working directory: $WORKDIR"

# ─── Step 1: Create existing project WITHOUT Takumi ──────────────────────────
step 1 "Set up existing polyglot project (Go backend + JS frontend, no Takumi)"
cd "$WORKDIR"
mkdir -p backend frontend
git init . >/dev/null 2>&1
git config user.email "dev@example.com"
git config user.name "dev"

# Go backend
cat > backend/main.go << 'EOF'
package main

import "fmt"

func main() { fmt.Println("backend server") }
EOF
cat > backend/main_test.go << 'EOF'
package main

import "testing"

func TestServer(t *testing.T) {}
EOF
cat > backend/go.mod << 'EOF'
module example.com/backend

go 1.22
EOF

# JS frontend
cat > frontend/index.js << 'EOF'
console.log("frontend app");
EOF
cat > frontend/package.json << 'EOF'
{"name": "frontend", "version": "0.1.0", "scripts": {"build": "echo built", "test": "echo tested"}}
EOF

git add -A && git commit -m "initial project" -q 2>&1

if [[ -f backend/main.go && -f frontend/index.js ]]; then
    echo "  Go backend: backend/main.go, backend/go.mod"
    echo "  JS frontend: frontend/index.js, frontend/package.json"
    echo "  No takumi.yaml, no .takumi/ — project has no Takumi awareness"
    pass
else
    fail "project setup failed"
fi

# ─── Step 2: Agent calls takumi status — gets discovery message ──────────────
step 2 "Agent calls takumi status in bare project — gets discovery guidance"
echo "$ takumi status (outside workspace)"
OUTPUT=$("$TAKUMI_BIN" status 2>&1) || true
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "takumi init"; then
    echo "  Agent sees: Takumi exists but workspace not set up"
    echo "  Agent learns: needs to run takumi init"
    pass
else
    fail "no discovery message — agent has no guidance"
fi

# ─── Step 3: Agent runs takumi init ──────────────────────────────────────────
step 3 "Agent runs takumi init to set up workspace"
echo "$ takumi init --agent claude"
if "$TAKUMI_BIN" init --agent claude 2>&1; then
    pass
else
    fail "init failed"
fi

# ─── Step 4: Verify workspace created ────────────────────────────────────────
step 4 "Verify workspace files exist after init"
MISSING=""
for f in takumi.yaml takumi-pkg.yaml .takumi/TAKUMI.md CLAUDE.md; do
    if [[ -f "$f" ]]; then
        echo "  found: $f"
    else
        echo "  MISSING: $f"
        MISSING="$MISSING $f"
    fi
done
if [[ -z "$MISSING" ]]; then
    pass
else
    fail "missing:$MISSING"
fi

# ─── Step 5: Agent reads TAKUMI.md — finds complete instructions ─────────────
step 5 "Agent reads TAKUMI.md — should contain full command reference"
echo "$ cat .takumi/TAKUMI.md (checking for key instructions)"
CONTENT=$(cat .takumi/TAKUMI.md)
FOUND_ALL=true
for keyword in "takumi build" "takumi test" "takumi run" "takumi affected" "takumi graph" "takumi validate" "takumi review" "takumi env setup" "takumi init"; do
    if echo "$CONTENT" | grep -q "$keyword"; then
        echo "  found: $keyword"
    else
        echo "  MISSING: $keyword"
        FOUND_ALL=false
    fi
done
if [[ "$FOUND_ALL" == "true" ]]; then
    pass
else
    fail "TAKUMI.md missing key instructions"
fi

# ─── Step 6: Agent sets up per-package configs ───────────────────────────────
step 6 "Agent creates takumi-pkg.yaml for each package"

# Remove the root-level one that init created and make per-package configs
rm -f takumi-pkg.yaml

cat > backend/takumi-pkg.yaml << 'EOF'
package:
  name: backend
  version: 0.1.0
phases:
  build:
    commands:
      - go build -o ../../build/backend ./...
  test:
    commands:
      - go test ./...
ai:
  description: "Go backend API server"
EOF
mkdir -p build

cat > frontend/takumi-pkg.yaml << 'EOF'
package:
  name: frontend
  version: 0.1.0
dependencies:
  - backend
phases:
  build:
    commands:
      - echo "npm run build"
  test:
    commands:
      - echo "npm test"
ai:
  description: "JavaScript frontend app — depends on backend API"
EOF

if [[ -f backend/takumi-pkg.yaml && -f frontend/takumi-pkg.yaml ]]; then
    echo "  created: backend/takumi-pkg.yaml"
    echo "  created: frontend/takumi-pkg.yaml (depends on backend)"
    pass
else
    fail "failed to create package configs"
fi

# Add build/ to workspace ignore so build output doesn't invalidate cache
cat > takumi.yaml << 'EOF'
workspace:
  name: fullstack-app
  ignore:
    - build/
  ai:
    agent: claude
EOF

# Commit configs so caching has a clean baseline
git add -A && git commit -m "add takumi configs" -q 2>&1

# ─── Step 7: Agent validates config ──────────────────────────────────────────
step 7 "Agent validates workspace config (following operator instructions)"
echo "$ takumi validate"
if "$TAKUMI_BIN" validate 2>&1; then
    pass
else
    fail "validation failed"
fi

# ─── Step 8: Agent checks dependency graph ───────────────────────────────────
step 8 "Agent checks dependency graph"
echo "$ takumi graph"
OUTPUT=$("$TAKUMI_BIN" graph 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "2 packages"; then
    echo "  Agent sees: backend (level 0) → frontend (level 1)"
    pass
else
    fail "graph doesn't show 2 packages"
fi

# ─── Step 9: Agent checks status — now sees workspace ───────────────────────
step 9 "Agent checks status — workspace is fully set up"
echo "$ takumi status"
OUTPUT=$("$TAKUMI_BIN" status 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "backend" && echo "$OUTPUT" | grep -q "frontend"; then
    pass
else
    fail "status doesn't show both packages"
fi

# ─── Step 10: Agent builds ───────────────────────────────────────────────────
step 10 "Agent builds with takumi (not go build / npm run build)"
echo "$ takumi build"
OUTPUT=$("$TAKUMI_BIN" build 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "2 passed"; then
    echo "  Both packages built in dependency order"
    pass
else
    fail "build didn't pass both packages"
fi

# ─── Step 11: Agent tests ────────────────────────────────────────────────────
step 11 "Agent tests with takumi (not go test / npm test)"
echo "$ takumi test"
OUTPUT=$("$TAKUMI_BIN" test 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "2 passed"; then
    echo "  Both packages tested in dependency order"
    pass
else
    fail "test didn't pass both packages"
fi

# ─── Step 12: Agent rebuilds — caching works ─────────────────────────────────
step 12 "Rebuild — caching should work without any extra setup"
echo "$ takumi build (second run)"
OUTPUT=$("$TAKUMI_BIN" build 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "cached"; then
    echo "  Caching works — no unnecessary rebuilds"
    pass
else
    fail "caching not working"
fi

# ─── Step 13: Agent modifies source, checks affected ─────────────────────────
step 13 "Modify backend, check affected packages"
echo "package main" > backend/main.go
echo "func main() {}" >> backend/main.go
# Don't commit — affected detects uncommitted changes vs HEAD

echo "$ takumi affected"
OUTPUT=$("$TAKUMI_BIN" affected 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "backend" && echo "$OUTPUT" | grep -q "frontend"; then
    echo "  Agent sees: modifying backend affects frontend too (cascade)"
    pass
else
    fail "affected doesn't show cascade"
fi

# ─── Step 14: Scoped build ───────────────────────────────────────────────────
step 14 "Build only affected packages (following operator workflow)"
echo "$ takumi build --affected"
OUTPUT=$("$TAKUMI_BIN" build --affected 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "passed"; then
    pass
else
    fail "affected build failed"
fi

# ─── Summary ─────────────────────────────────────────────────────────────────
echo ""
echo "=== SUMMARY ==="
echo "Passed: $PASS / $TOTAL"
echo "Failed: $FAIL / $TOTAL"
if [[ $FAIL -eq 0 ]]; then
    echo "Result: ALL PASSED"
else
    echo "Result: $FAIL FAILURES"
fi
