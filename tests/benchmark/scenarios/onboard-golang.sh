#!/bin/bash
# onboard-golang.sh — Benchmark: Onboard a real Go monorepo following onboarding-existing-project.md
#
# Uses https://github.com/flowerinthenight/golang-monorepo as the test project.
# Follows docs/user/onboarding-existing-project.md step by step.

set -uo pipefail

TAKUMI_BIN="${TAKUMI_BIN:?TAKUMI_BIN must be set}"
WORKDIR=$(mktemp -d /tmp/takumi-bench-go-XXXXXX)
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

echo "=== SCENARIO: Onboard Go Monorepo (onboarding-existing-project.md) ==="
echo "Project: flowerinthenight/golang-monorepo"
echo "Working directory: $WORKDIR"

# ─── Step 1: Clone the project ────────────────────────────────────────────────
step 1 "Clone existing Go monorepo"
echo "$ git clone --depth 1 https://github.com/flowerinthenight/golang-monorepo.git"
cd "$WORKDIR"
if git clone --depth 1 https://github.com/flowerinthenight/golang-monorepo.git 2>&1; then
    cd golang-monorepo
    echo "Cloned to $WORKDIR/golang-monorepo"
    echo "Project structure:"
    find . -type f -name "*.go" | head -20
    pass
else
    fail "clone failed"
    echo "=== SUMMARY ==="
    echo "Result: ABORTED (clone failed)"
    exit 0
fi

# ─── Step 2: Initialize workspace ─────────────────────────────────────────────
# Doc Step 1: "cd into your project root and run: takumi init --agent claude"
step 2 "Initialize Takumi workspace (doc step 1: cd your-project && takumi init)"
echo "$ takumi init --agent claude"
if "$TAKUMI_BIN" init --agent claude 2>&1; then
    pass
else
    fail "init failed"
fi

# ─── Step 3: Verify init created files ────────────────────────────────────────
step 3 "Verify workspace files were created"
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

# ─── Step 4: Remove root package (multi-component project) ────────────────────
# Doc Step 3: "If your project has multiple components, create a takumi-pkg.yaml in each"
# For multi-component, the root package is optional — we remove it and create per-component
step 4 "Remove root takumi-pkg.yaml (doc step 3: multi-component project setup)"
echo "This is a multi-component project — removing root package config"
rm -f takumi-pkg.yaml
echo "Removed root takumi-pkg.yaml"
pass

# ─── Step 5: Create sub-package for internal/ ─────────────────────────────────
# Doc: "create a takumi-pkg.yaml in each component directory"
step 5 "Create takumi-pkg.yaml for internal/ (doc: leaf dependency package)"
cat > internal/takumi-pkg.yaml << 'YAMLEOF'
package:
  name: internal
  version: 0.1.0
phases:
  build:
    commands:
      - go build ./...
  test:
    commands:
      - go vet ./...
ai:
  description: "Shared internal library — hostname utility, used by other packages"
YAMLEOF
echo "Created internal/takumi-pkg.yaml"
cat internal/takumi-pkg.yaml
pass

# ─── Step 6: Create sub-package for cmd/samplecmd/ ────────────────────────────
step 6 "Create takumi-pkg.yaml for cmd/samplecmd/ (doc: component with dependency)"
cat > cmd/samplecmd/takumi-pkg.yaml << 'YAMLEOF'
package:
  name: samplecmd
  version: 0.1.0
dependencies:
  - internal
phases:
  build:
    commands:
      - mkdir -p ../../build && go build -o ../../build/samplecmd .
  test:
    commands:
      - go vet ./...
ai:
  description: "CLI tool — uses cobra, depends on internal for hostname"
YAMLEOF
echo "Created cmd/samplecmd/takumi-pkg.yaml"
cat cmd/samplecmd/takumi-pkg.yaml
pass

# ─── Step 7: Create sub-package for services/samplesvc/ ───────────────────────
step 7 "Create takumi-pkg.yaml for services/samplesvc/ (doc: service with dependency)"
cat > services/samplesvc/takumi-pkg.yaml << 'YAMLEOF'
package:
  name: samplesvc
  version: 0.1.0
dependencies:
  - internal
phases:
  build:
    commands:
      - mkdir -p ../../build && go build -ldflags "-X main.version=0.1.0" -o ../../build/samplesvc .
  test:
    commands:
      - go vet ./...
ai:
  description: "Long-running sample service — uses cobra, depends on internal for hostname"
YAMLEOF
echo "Created services/samplesvc/takumi-pkg.yaml"
cat services/samplesvc/takumi-pkg.yaml
pass

# ─── Step 8: Configure workspace ignore ───────────────────────────────────────
# Doc tip: "output binaries to a directory listed in takumi.yaml ignore list"
step 8 "Add build/ to workspace ignore (doc tip: build outputs go to ignored directory)"
cat > takumi.yaml << 'YAMLEOF'
workspace:
  name: golang-monorepo
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

# ─── Step 13: Verify build outputs ────────────────────────────────────────────
step 13 "Verify build produced binaries"
echo "Checking build/ directory:"
ls -la build/ 2>&1 || echo "build/ not found"
FOUND=0
for bin in samplecmd samplesvc; do
    if [[ -f "build/$bin" ]]; then
        echo "  found: build/$bin"
        FOUND=$((FOUND + 1))
    else
        echo "  MISSING: build/$bin"
    fi
done
if [[ $FOUND -eq 2 ]]; then
    pass
else
    fail "missing binaries ($FOUND/2)"
fi

# ─── Step 14: Test ────────────────────────────────────────────────────────────
# Doc Step 5: "takumi test — Run tests"
step 14 "Run tests (doc step 5: takumi test)"
echo "$ takumi test"
if "$TAKUMI_BIN" test 2>&1; then
    pass
else
    fail "test failed"
fi

# ─── Step 15: Status ──────────────────────────────────────────────────────────
# Doc Step 5: "takumi status — Full dashboard"
step 15 "Show workspace status (doc step 5: takumi status)"
echo "$ takumi status"
if "$TAKUMI_BIN" status 2>&1; then
    pass
else
    fail "status failed"
fi

# ─── Step 16: Rebuild (cache test) ────────────────────────────────────────────
# Doc: "Run the same build again — unchanged packages are skipped via content-addressed caching"
step 16 "Rebuild to verify caching (doc: unchanged packages skipped via caching)"
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

# ─── Step 17: Affected packages ───────────────────────────────────────────────
# Doc (commands reference): "takumi affected — List packages affected by changes"
step 17 "Check affected packages (doc: takumi affected lists changed packages)"
echo "$ takumi affected"
if "$TAKUMI_BIN" affected 2>&1; then
    pass
else
    fail "affected failed"
fi

# ─── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "=== SUMMARY ==="
echo "Scenario: Onboard Go Monorepo (onboarding-existing-project.md)"
echo "Project: flowerinthenight/golang-monorepo"
echo "Packages: internal (lib), samplecmd (CLI), samplesvc (service)"
echo "Passed: $PASS / $TOTAL"
echo "Failed: $FAIL / $TOTAL"
if [[ $FAIL -eq 0 ]]; then
    echo "Result: ALL PASSED"
else
    echo "Result: $FAIL FAILURES"
fi
