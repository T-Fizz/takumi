#!/bin/bash
# agent-setup.sh — Benchmark: Verify all supported agents get correct config
#
# Tests that `takumi init --agent <name>` creates the right config file for
# every supported agent, with the correct include line pointing to TAKUMI.md.
# Also tests idempotency (re-running doesn't duplicate the include line).
#
# This validates that any AI agent can be onboarded with a single command.

set -uo pipefail

TAKUMI_BIN="${TAKUMI_BIN:?TAKUMI_BIN must be set}"
TAKUMI_BIN="$(cd "$(dirname "$TAKUMI_BIN")" && pwd)/$(basename "$TAKUMI_BIN")"
WORKDIR=$(mktemp -d /tmp/takumi-bench-setup-XXXXXX)
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

INCLUDE_LINE="Read .takumi/TAKUMI.md for Takumi build tool instructions."

echo "=== SCENARIO: Agent Setup (all supported agents) ==="
echo "Working directory: $WORKDIR"

# ─── Step 1: Claude ──────────────────────────────────────────────────────────
step 1 "Init with --agent claude → creates CLAUDE.md"
cd "$WORKDIR"
PROJ="$WORKDIR/proj-claude"
"$TAKUMI_BIN" init --root proj-claude --agent claude >/dev/null 2>&1
if [[ -f "$PROJ/CLAUDE.md" ]] && grep -q "$INCLUDE_LINE" "$PROJ/CLAUDE.md"; then
    echo "  CLAUDE.md exists with include line"
    pass
else
    echo "  Expected: $PROJ/CLAUDE.md with include line"
    fail "CLAUDE.md missing or wrong content"
fi

# ─── Step 2: Cursor ──────────────────────────────────────────────────────────
step 2 "Init with --agent cursor → creates .cursor/rules"
PROJ="$WORKDIR/proj-cursor"
"$TAKUMI_BIN" init --root proj-cursor --agent cursor >/dev/null 2>&1
if [[ -f "$PROJ/.cursor/rules" ]] && grep -q "$INCLUDE_LINE" "$PROJ/.cursor/rules"; then
    echo "  .cursor/rules exists with include line"
    pass
else
    echo "  Expected: $PROJ/.cursor/rules with include line"
    fail ".cursor/rules missing or wrong content"
fi

# ─── Step 3: GitHub Copilot ──────────────────────────────────────────────────
step 3 "Init with --agent copilot → creates .github/copilot-instructions.md"
PROJ="$WORKDIR/proj-copilot"
"$TAKUMI_BIN" init --root proj-copilot --agent copilot >/dev/null 2>&1
if [[ -f "$PROJ/.github/copilot-instructions.md" ]] && grep -q "$INCLUDE_LINE" "$PROJ/.github/copilot-instructions.md"; then
    echo "  .github/copilot-instructions.md exists with include line"
    pass
else
    echo "  Expected: $PROJ/.github/copilot-instructions.md with include line"
    fail ".github/copilot-instructions.md missing or wrong content"
fi

# ─── Step 4: Windsurf ────────────────────────────────────────────────────────
step 4 "Init with --agent windsurf → creates .windsurfrules"
PROJ="$WORKDIR/proj-windsurf"
"$TAKUMI_BIN" init --root proj-windsurf --agent windsurf >/dev/null 2>&1
if [[ -f "$PROJ/.windsurfrules" ]] && grep -q "$INCLUDE_LINE" "$PROJ/.windsurfrules"; then
    echo "  .windsurfrules exists with include line"
    pass
else
    echo "  Expected: $PROJ/.windsurfrules with include line"
    fail ".windsurfrules missing or wrong content"
fi

# ─── Step 5: Cline ───────────────────────────────────────────────────────────
step 5 "Init with --agent cline → creates .clinerules"
PROJ="$WORKDIR/proj-cline"
"$TAKUMI_BIN" init --root proj-cline --agent cline >/dev/null 2>&1
if [[ -f "$PROJ/.clinerules" ]] && grep -q "$INCLUDE_LINE" "$PROJ/.clinerules"; then
    echo "  .clinerules exists with include line"
    pass
else
    echo "  Expected: $PROJ/.clinerules with include line"
    fail ".clinerules missing or wrong content"
fi

# ─── Step 6: Kiro ────────────────────────────────────────────────────────────
step 6 "Init with --agent kiro → creates AGENTS.md"
PROJ="$WORKDIR/proj-kiro"
"$TAKUMI_BIN" init --root proj-kiro --agent kiro >/dev/null 2>&1
if [[ -f "$PROJ/AGENTS.md" ]] && grep -q "$INCLUDE_LINE" "$PROJ/AGENTS.md"; then
    echo "  AGENTS.md exists with include line"
    pass
else
    echo "  Expected: $PROJ/AGENTS.md with include line"
    fail "AGENTS.md missing or wrong content"
fi

# ─── Step 7: None ────────────────────────────────────────────────────────────
step 7 "Init with --agent none → no agent config file created"
PROJ="$WORKDIR/proj-none"
"$TAKUMI_BIN" init --root proj-none --agent none >/dev/null 2>&1
# Should have .takumi/TAKUMI.md but NO agent config file
FOUND_AGENT_FILE=false
for f in CLAUDE.md .cursor/rules .github/copilot-instructions.md .windsurfrules .clinerules AGENTS.md; do
    if [[ -f "$PROJ/$f" ]]; then
        FOUND_AGENT_FILE=true
        echo "  UNEXPECTED: $f exists"
    fi
done
if [[ "$FOUND_AGENT_FILE" == "false" && -f "$PROJ/.takumi/TAKUMI.md" ]]; then
    echo "  No agent config file, but TAKUMI.md exists (correct)"
    pass
else
    fail "agent file found or TAKUMI.md missing"
fi

# ─── Step 8: TAKUMI.md structure is the same regardless of agent choice ──────
step 8 "TAKUMI.md has same structure regardless of agent (only workspace name differs)"
# Strip the workspace name line (line 1 contains workspace name) and compare structure
REF=$(tail -n +2 "$WORKDIR/proj-claude/.takumi/TAKUMI.md")
ALL_MATCH=true
for proj in proj-cursor proj-copilot proj-windsurf proj-cline proj-kiro proj-none; do
    CONTENT=$(tail -n +2 "$WORKDIR/$proj/.takumi/TAKUMI.md" 2>/dev/null || echo "MISSING")
    if [[ "$CONTENT" != "$REF" ]]; then
        echo "  MISMATCH: $proj has different TAKUMI.md structure"
        ALL_MATCH=false
    fi
done
if [[ "$ALL_MATCH" == "true" ]]; then
    echo "  All 7 workspaces have identical TAKUMI.md content (modulo workspace name)"
    pass
else
    fail "TAKUMI.md structure differs across agents"
fi

# ─── Step 9: Idempotency — re-running doesn't duplicate include ──────────────
step 9 "Idempotency: setupAgentConfig doesn't duplicate include line"
PROJ="$WORKDIR/proj-claude"
cd "$PROJ"
# Run init again in the same workspace (simulates re-running setup)
"$TAKUMI_BIN" init --agent claude >/dev/null 2>&1 || true
COUNT=$(grep -c "$INCLUDE_LINE" "$PROJ/CLAUDE.md" 2>/dev/null || echo 0)
if [[ "$COUNT" == "1" ]]; then
    echo "  Include line appears exactly once after re-init"
    pass
else
    echo "  Include line appears $COUNT times (expected 1)"
    fail "include line duplicated"
fi

# ─── Step 10: Agent config file with existing content gets appended ──────────
step 10 "Existing agent config file gets include line appended (not overwritten)"
PROJ="$WORKDIR/proj-existing"
mkdir -p "$PROJ"
cd "$PROJ"
# Create a pre-existing CLAUDE.md with custom content
cat > CLAUDE.md << 'EOF'
# My Custom Rules

Always use TypeScript strict mode.
EOF
# Now init with claude agent (creates .takumi/ and appends to CLAUDE.md)
"$TAKUMI_BIN" init --agent claude >/dev/null 2>&1 || true
if grep -q "TypeScript strict mode" "$PROJ/CLAUDE.md" && grep -q "$INCLUDE_LINE" "$PROJ/CLAUDE.md"; then
    echo "  Original content preserved, include line appended"
    pass
else
    echo "  Content:"
    cat "$PROJ/CLAUDE.md" 2>/dev/null
    fail "original content lost or include line missing"
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
