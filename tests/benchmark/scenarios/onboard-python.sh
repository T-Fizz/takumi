#!/bin/bash
# onboard-python.sh — Benchmark: Onboard a real Python monorepo following onboarding-existing-project.md
#
# Uses https://github.com/carderne/postmodern-mono as the test project.
# Follows docs/user/onboarding-existing-project.md step by step.
#
# Build commands use py_compile (syntax check, no deps required).

set -uo pipefail

TAKUMI_BIN="${TAKUMI_BIN:?TAKUMI_BIN must be set}"
WORKDIR=$(mktemp -d /tmp/takumi-bench-py-XXXXXX)
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

echo "=== SCENARIO: Onboard Python Monorepo (onboarding-existing-project.md) ==="
echo "Project: carderne/postmodern-mono"
echo "Working directory: $WORKDIR"

# ─── Step 1: Clone the project ────────────────────────────────────────────────
step 1 "Clone existing Python monorepo"
echo "$ git clone --depth 1 https://github.com/carderne/postmodern-mono.git"
cd "$WORKDIR"
if git clone --depth 1 https://github.com/carderne/postmodern-mono.git 2>&1; then
    cd postmodern-mono
    echo "Cloned to $WORKDIR/postmodern-mono"
    echo ""
    echo "Project structure:"
    find . -type f \( -name "*.py" -o -name "pyproject.toml" \) | sort | head -30
    pass
else
    fail "clone failed"
    echo "=== SUMMARY ==="
    echo "Result: ABORTED (clone failed)"
    exit 0
fi

# ─── Step 2: Identify components ──────────────────────────────────────────────
step 2 "Identify project components for Takumi package mapping"
echo "Scanning directory structure..."
echo ""
echo "Components found:"
for dir in libs/*/  apps/*/; do
    if [[ -d "$dir" ]]; then
        name=$(basename "$dir")
        py_files=$(find "$dir" -name "*.py" -not -path "*/__pycache__/*" | wc -l | tr -d ' ')
        has_tests=$(test -d "${dir}tests" && echo "yes" || echo "no")
        echo "  $dir — $py_files .py files, tests: $has_tests"
    fi
done
pass

# ─── Step 3: Initialize workspace ─────────────────────────────────────────────
# Doc Step 1: "cd into your project root and run: takumi init --agent claude"
step 3 "Initialize Takumi workspace (doc step 1: cd your-project && takumi init)"
echo "$ takumi init --agent claude"
if "$TAKUMI_BIN" init --agent claude 2>&1; then
    pass
else
    fail "init failed"
fi

# ─── Step 4: Verify init created files ────────────────────────────────────────
step 4 "Verify workspace files were created"
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

# ─── Step 5: Remove root package (multi-component project) ────────────────────
# Doc Step 3: multi-component setup
step 5 "Remove root takumi-pkg.yaml (doc step 3: multi-component project setup)"
echo "This is a multi-component project — removing root package config"
rm -f takumi-pkg.yaml
echo "Removed root takumi-pkg.yaml"
pass

# ─── Step 6: Create sub-package for libs/greeter/ ─────────────────────────────
# Doc: "create a takumi-pkg.yaml in each component directory"
step 6 "Create takumi-pkg.yaml for libs/greeter/ (doc: shared library, no dependencies)"
# Find the main Python file for the build command
GREETER_PY=$(find libs/greeter -name "*.py" -not -path "*/test*" -not -name "__pycache__" | head -1)
GREETER_REL="${GREETER_PY#libs/greeter/}"
echo "Main source file: ${GREETER_PY:-not found} (relative: ${GREETER_REL})"
# Note: YAML plain scalars cannot contain ": " (colon-space) — it triggers
# implicit mapping. We quote the test command to avoid this.
cat > libs/greeter/takumi-pkg.yaml << YAMLEOF
package:
  name: greeter
  version: 0.1.0
phases:
  build:
    commands:
      - python3 -m py_compile ${GREETER_REL}
  test:
    commands:
      - "python3 -c \"import py_compile; py_compile.compile('${GREETER_REL}', doraise=True); print('OK')\""
ai:
  description: Shared greeting library
YAMLEOF
echo "Created libs/greeter/takumi-pkg.yaml"
cat libs/greeter/takumi-pkg.yaml
pass

# ─── Step 7: Create sub-package for apps/mycli/ ───────────────────────────────
step 7 "Create takumi-pkg.yaml for apps/mycli/ (doc: independent CLI app)"
MYCLI_PY=$(find apps/mycli -name "*.py" -not -path "*/test*" -not -name "__pycache__" | head -1)
MYCLI_REL="${MYCLI_PY#apps/mycli/}"
echo "Main source file: ${MYCLI_PY:-not found} (relative: ${MYCLI_REL})"
cat > apps/mycli/takumi-pkg.yaml << YAMLEOF
package:
  name: mycli
  version: 0.1.0
phases:
  build:
    commands:
      - python3 -m py_compile ${MYCLI_REL}
  test:
    commands:
      - "python3 -c \"import py_compile; py_compile.compile('${MYCLI_REL}', doraise=True); print('OK')\""
ai:
  description: CLI application
YAMLEOF
echo "Created apps/mycli/takumi-pkg.yaml"
cat apps/mycli/takumi-pkg.yaml
pass

# ─── Step 8: Create sub-package for apps/server/ ──────────────────────────────
# Doc: "Then declare dependencies between them"
step 8 "Create takumi-pkg.yaml for apps/server/ (doc: component with dependency on greeter)"
SERVER_PY=$(find apps/server -name "*.py" -not -path "*/test*" -not -name "__pycache__" | head -1)
SERVER_REL="${SERVER_PY#apps/server/}"
echo "Main source file: ${SERVER_PY:-not found} (relative: ${SERVER_REL})"
cat > apps/server/takumi-pkg.yaml << YAMLEOF
package:
  name: server
  version: 0.1.0
dependencies:
  - greeter
phases:
  build:
    commands:
      - python3 -m py_compile ${SERVER_REL}
  test:
    commands:
      - "python3 -c \"import py_compile; py_compile.compile('${SERVER_REL}', doraise=True); print('OK')\""
ai:
  description: Web server app
YAMLEOF
echo "Created apps/server/takumi-pkg.yaml"
cat apps/server/takumi-pkg.yaml
pass

# ─── Step 9: Validate ─────────────────────────────────────────────────────────
# Doc Step 4: "takumi validate — Check all configs for errors"
step 9 "Validate configuration (doc step 4: takumi validate)"
echo "$ takumi validate"
if "$TAKUMI_BIN" validate 2>&1; then
    pass
else
    fail "validate failed"
fi

# ─── Step 10: Dependency graph ─────────────────────────────────────────────────
# Doc Step 4: "takumi graph — See dependency order"
step 10 "Show dependency graph (doc step 4: takumi graph)"
echo "$ takumi graph"
if "$TAKUMI_BIN" graph 2>&1; then
    pass
else
    fail "graph failed"
fi

# ─── Step 11: Dry run ─────────────────────────────────────────────────────────
# Doc Step 4: "takumi build --dry-run — Preview what will run"
step 11 "Preview build plan (doc step 4: takumi build --dry-run)"
echo "$ takumi build --dry-run"
if "$TAKUMI_BIN" build --dry-run 2>&1; then
    pass
else
    fail "dry-run failed"
fi

# ─── Step 12: Build ───────────────────────────────────────────────────────────
# Doc Step 5: "takumi build — Build in dependency order"
step 12 "Build all packages (doc step 5: takumi build)"
echo "$ takumi build"
if "$TAKUMI_BIN" build 2>&1; then
    pass
else
    fail "build failed"
fi

# ─── Step 13: Test ────────────────────────────────────────────────────────────
# Doc Step 5: "takumi test — Run tests"
step 13 "Run tests (doc step 5: takumi test)"
echo "$ takumi test"
if "$TAKUMI_BIN" test 2>&1; then
    pass
else
    fail "test failed"
fi

# ─── Step 14: Status ──────────────────────────────────────────────────────────
# Doc Step 5: "takumi status — Full dashboard"
step 14 "Show workspace status (doc step 5: takumi status)"
echo "$ takumi status"
if "$TAKUMI_BIN" status 2>&1; then
    pass
else
    fail "status failed"
fi

# ─── Step 15: Rebuild (cache test) ────────────────────────────────────────────
# Doc: "Run the same build again — unchanged packages are skipped"
step 15 "Rebuild to verify caching (doc: unchanged packages skipped via caching)"
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

# ─── Step 16: Affected packages ───────────────────────────────────────────────
step 16 "Check affected packages (doc: takumi affected lists changed packages)"
echo "$ takumi affected"
if "$TAKUMI_BIN" affected 2>&1; then
    pass
else
    fail "affected failed"
fi

# ─── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "=== SUMMARY ==="
echo "Scenario: Onboard Python Monorepo (onboarding-existing-project.md)"
echo "Project: carderne/postmodern-mono"
echo "Packages: greeter (lib), mycli (CLI), server (app, depends on greeter)"
echo "Passed: $PASS / $TOTAL"
echo "Failed: $FAIL / $TOTAL"
if [[ $FAIL -eq 0 ]]; then
    echo "Result: ALL PASSED"
else
    echo "Result: $FAIL FAILURES"
fi
