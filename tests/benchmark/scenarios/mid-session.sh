#!/bin/bash
# mid-session.sh — Benchmark: Mid-session agent workflows
#
# Tests steady-state scenarios that happen AFTER initial setup:
#   - Modify code → affected → scoped build/test
#   - Build failure → read logs → fix → rebuild
#   - Add a new package to existing workspace
#   - Run custom phase (lint/deploy)
#
# Unlike new-project.sh and agent-discovery.sh which test cold-start,
# this tests the edit→build→test loop that agents do 90% of the time.

set -uo pipefail

TAKUMI_BIN="${TAKUMI_BIN:?TAKUMI_BIN must be set}"
TAKUMI_BIN="$(cd "$(dirname "$TAKUMI_BIN")" && pwd)/$(basename "$TAKUMI_BIN")"
WORKDIR=$(mktemp -d /tmp/takumi-bench-mid-XXXXXX)
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

echo "=== SCENARIO: Mid-Session Workflows (steady-state agent operations) ==="
echo "Working directory: $WORKDIR"

# ─── Setup: Create a working multi-package workspace ─────────────────────────
echo ""
echo "--- SETUP: Creating multi-package workspace ---"
cd "$WORKDIR"
git init . >/dev/null 2>&1
git config user.email "dev@example.com"
git config user.name "dev"

"$TAKUMI_BIN" init --agent claude >/dev/null 2>&1

# Create lib package (shared library)
mkdir -p lib
cat > lib/takumi-pkg.yaml << 'EOF'
package:
  name: lib
  version: 1.0.0
phases:
  build:
    commands:
      - echo "compiling lib"
  test:
    commands:
      - echo "testing lib"
  lint:
    commands:
      - echo "linting lib"
EOF
cat > lib/lib.go << 'EOF'
package lib

func Hello() string { return "hello" }
EOF

# Create api package (depends on lib)
mkdir -p api
cat > api/takumi-pkg.yaml << 'EOF'
package:
  name: api
  version: 2.0.0
dependencies:
  - lib
phases:
  build:
    commands:
      - echo "compiling api"
  test:
    commands:
      - echo "testing api"
  lint:
    commands:
      - echo "linting api"
  deploy:
    commands:
      - echo "deploying api to staging"
EOF
cat > api/main.go << 'EOF'
package main

import "fmt"

func main() { fmt.Println("api server") }
EOF

# Create web package (depends on api)
mkdir -p web
cat > web/takumi-pkg.yaml << 'EOF'
package:
  name: web
  version: 1.5.0
dependencies:
  - api
phases:
  build:
    commands:
      - echo "bundling web"
  test:
    commands:
      - echo "testing web"
EOF
cat > web/index.js << 'EOF'
console.log("web frontend");
EOF

# Remove root takumi-pkg.yaml that init creates
rm -f takumi-pkg.yaml

# Update workspace config
cat > takumi.yaml << 'EOF'
workspace:
  name: mid-session-app
  ai:
    agent: claude
EOF

git add -A && git commit -m "initial workspace" -q 2>&1
echo "  Workspace: 3 packages (lib → api → web)"
echo "  Setup complete."

# ═══════════════════════════════════════════════════════════════════════════════
# PHASE 1: Edit → Affected → Scoped Build/Test
# ═══════════════════════════════════════════════════════════════════════════════

step 1 "Agent modifies lib source code"
echo "package lib" > lib/lib.go
echo "" >> lib/lib.go
echo 'func Hello() string { return "hello world" }' >> lib/lib.go
echo "  Modified: lib/lib.go (changed return value)"
pass

step 2 "Agent checks affected packages (operator workflow step 2)"
echo "$ takumi affected"
OUTPUT=$("$TAKUMI_BIN" affected 2>&1)
echo "$OUTPUT"
# Modifying lib should affect lib, api (depends on lib), and web (depends on api)
if echo "$OUTPUT" | grep -q "lib" && echo "$OUTPUT" | grep -q "api" && echo "$OUTPUT" | grep -q "web"; then
    echo "  Cascade detected: lib → api → web (all 3 affected)"
    pass
else
    fail "affected didn't show full cascade"
fi

step 3 "Agent runs scoped build (operator workflow step 3)"
echo "$ takumi build --affected"
OUTPUT=$("$TAKUMI_BIN" build --affected 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "3 passed"; then
    echo "  All 3 affected packages rebuilt in dependency order"
    pass
else
    fail "scoped build didn't pass all affected"
fi

step 4 "Agent runs scoped test (operator workflow step 4)"
echo "$ takumi test --affected"
OUTPUT=$("$TAKUMI_BIN" test --affected 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "3 passed"; then
    echo "  All 3 affected packages tested"
    pass
else
    fail "scoped test didn't pass all affected"
fi

step 5 "Agent commits and rebuilds — caching kicks in"
git add -A && git commit -m "update lib" -q 2>&1
echo "$ takumi build"
OUTPUT=$("$TAKUMI_BIN" build 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "cached"; then
    echo "  After commit, build is cached (no-op)"
    pass
else
    fail "caching not working after commit"
fi

# ═══════════════════════════════════════════════════════════════════════════════
# PHASE 2: Build Failure → Recovery
# ═══════════════════════════════════════════════════════════════════════════════

step 6 "Agent introduces a build error"
cat > api/takumi-pkg.yaml << 'EOF'
package:
  name: api
  version: 2.0.0
dependencies:
  - lib
phases:
  build:
    commands:
      - exit 1
  test:
    commands:
      - echo "testing api"
  lint:
    commands:
      - echo "linting api"
  deploy:
    commands:
      - echo "deploying api to staging"
EOF
echo "  Broke api build phase (exit 1)"
pass

step 7 "Build fails — agent sees failure output"
echo "$ takumi build"
OUTPUT=$("$TAKUMI_BIN" build 2>&1) || true
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "fail"; then
    echo "  Build reports failure (agent can see which package failed)"
    pass
else
    fail "build didn't report failure"
fi

step 8 "Agent checks logs for failure details (operator workflow step 5)"
echo "$ ls .takumi/logs/"
if [[ -d .takumi/logs ]]; then
    LOGS=$(ls .takumi/logs/ 2>/dev/null | head -5)
    if [[ -n "$LOGS" ]]; then
        echo "  Logs found:"
        echo "$LOGS" | sed 's/^/    /'
        echo "  Agent reads logs to understand failure"
        pass
    else
        echo "  Log directory exists but empty (failure info was in stdout)"
        pass
    fi
else
    echo "  No .takumi/logs/ directory"
    fail "logs directory missing"
fi

step 9 "Agent fixes the build error and rebuilds"
cat > api/takumi-pkg.yaml << 'EOF'
package:
  name: api
  version: 2.0.0
dependencies:
  - lib
phases:
  build:
    commands:
      - echo "compiling api (fixed)"
  test:
    commands:
      - echo "testing api"
  lint:
    commands:
      - echo "linting api"
  deploy:
    commands:
      - echo "deploying api to staging"
EOF
echo "$ takumi build"
OUTPUT=$("$TAKUMI_BIN" build 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "passed"; then
    echo "  Build succeeds after fix"
    pass
else
    fail "rebuild after fix failed"
fi

# ═══════════════════════════════════════════════════════════════════════════════
# PHASE 3: Add New Package
# ═══════════════════════════════════════════════════════════════════════════════

step 10 "Agent adds a new 'worker' package to the workspace"
mkdir -p worker
cat > worker/takumi-pkg.yaml << 'EOF'
package:
  name: worker
  version: 0.1.0
dependencies:
  - lib
phases:
  build:
    commands:
      - echo "compiling worker"
  test:
    commands:
      - echo "testing worker"
EOF
cat > worker/worker.go << 'EOF'
package worker

func Process() {}
EOF
echo "  Created: worker/ with takumi-pkg.yaml (depends on lib)"
pass

step 11 "Agent validates new config (operator: run validate after editing configs)"
echo "$ takumi validate"
OUTPUT=$("$TAKUMI_BIN" validate 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "valid" && echo "$OUTPUT" | grep -q "no cycles"; then
    echo "  Config valid, no cycles introduced"
    pass
else
    fail "validation failed after adding package"
fi

step 12 "Agent checks graph — new package appears"
echo "$ takumi graph"
OUTPUT=$("$TAKUMI_BIN" graph 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "worker" && echo "$OUTPUT" | grep -q "4 packages"; then
    echo "  Graph shows worker in dependency tree"
    pass
else
    fail "worker not in graph"
fi

step 13 "Agent builds the new package"
echo "$ takumi build"
OUTPUT=$("$TAKUMI_BIN" build 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "passed"; then
    echo "  New package builds alongside existing ones"
    pass
else
    fail "build with new package failed"
fi

# ═══════════════════════════════════════════════════════════════════════════════
# PHASE 4: Custom Phases (lint, deploy)
# ═══════════════════════════════════════════════════════════════════════════════

step 14 "Agent runs lint phase (operator: takumi lint, not eslint/ruff)"
echo "$ takumi lint"
OUTPUT=$("$TAKUMI_BIN" lint 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "passed"; then
    echo "  Lint phase ran on packages that define it"
    pass
else
    fail "lint phase failed"
fi

step 15 "Agent runs deploy phase (operator: takumi deploy, not fly/vercel)"
echo "$ takumi deploy"
OUTPUT=$("$TAKUMI_BIN" deploy 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "passed" || echo "$OUTPUT" | grep -q "deploying"; then
    echo "  Deploy phase ran on api (only package with deploy)"
    pass
else
    fail "deploy phase failed"
fi

# ═══════════════════════════════════════════════════════════════════════════════
# PHASE 5: Status Check (agent resuming session)
# ═══════════════════════════════════════════════════════════════════════════════

step 16 "Agent checks status (operator: ALWAYS run first in new session)"
echo "$ takumi status"
OUTPUT=$("$TAKUMI_BIN" status 2>&1)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "lib" && echo "$OUTPUT" | grep -q "api" && echo "$OUTPUT" | grep -q "web" && echo "$OUTPUT" | grep -q "worker"; then
    echo "  Status shows all 4 packages"
    pass
else
    fail "status doesn't show all packages"
fi

# ─── Summary ─────────────────────────────────────────────────────────────────
echo ""
echo "=== SUMMARY ==="
echo "Passed: $PASS / $TOTAL"
echo "Failed: $FAIL / $TOTAL"
echo ""
echo "Phases:"
echo "  1. Edit→Affected→Scoped Build/Test: steps 1-5"
echo "  2. Build Failure→Recovery: steps 6-9"
echo "  3. Add New Package: steps 10-13"
echo "  4. Custom Phases (lint, deploy): steps 14-15"
echo "  5. Status Check (session resume): step 16"
if [[ $FAIL -eq 0 ]]; then
    echo ""
    echo "Result: ALL PASSED"
else
    echo ""
    echo "Result: $FAIL FAILURES"
fi
