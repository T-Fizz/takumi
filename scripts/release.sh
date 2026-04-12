#!/usr/bin/env bash
set -euo pipefail

# Takumi release script — called by `VERSION=x.y.z t run release`
#
# Steps:
#   1. Verify prerequisites (clean tree, VERSION set, tools available)
#   2. Update version in takumi-pkg.yaml
#   3. Build and run tests
#   4. Clean submodule state (test artifacts)
#   5. Commit, tag, push
#   6. Run goreleaser (builds binaries, creates GitHub Release)
#   7. Attach benchmark link to release notes if available

VERSION="${VERSION:?VERSION is required. Usage: VERSION=x.y.z t run release}"

# ── 1. Prerequisites ───────────────────────────────────────────────────

echo "==> Checking prerequisites..."

command -v gh >/dev/null 2>&1 || { echo "Error: gh CLI not found"; exit 1; }
command -v goreleaser >/dev/null 2>&1 || {
    GORELEASER="$(go env GOPATH)/bin/goreleaser"
    if [ ! -f "$GORELEASER" ]; then
        echo "Error: goreleaser not found. Run: go install github.com/goreleaser/goreleaser/v2@latest"
        exit 1
    fi
}
GORELEASER="${GORELEASER:-goreleaser}"

# ── 2. Update version ──────────────────────────────────────────────────

echo "==> Updating version to ${VERSION}..."
sed -i '' "s/^  version: .*/  version: ${VERSION}/" takumi-pkg.yaml

# ── 3. Build and test ──────────────────────────────────────────────────

echo "==> Building..."
go build -o build/takumi ./cmd/takumi
ln -sf takumi build/t

echo "==> Running tests..."
go test ./...

# ── 4. Clean submodules ────────────────────────────────────────────────

echo "==> Cleaning submodule state..."
git submodule foreach --recursive git checkout -- . 2>/dev/null || true
git submodule foreach --recursive git clean -fd 2>/dev/null || true

# ── 5. Commit, tag, push ──────────────────────────────────────────────

echo "==> Committing and tagging v${VERSION}..."
git add -A
if ! git diff --cached --quiet; then
    git commit -m "Release v${VERSION}"
fi

git tag "v${VERSION}"
git push origin main
git push origin "v${VERSION}"

# ── 6. Goreleaser ──────────────────────────────────────────────────────

echo "==> Running goreleaser..."
GITHUB_TOKEN="$(gh auth token)" "$GORELEASER" release --clean

# ── 7. Attach benchmark link ──────────────────────────────────────────

BENCH_GIST="tests/benchmark/perf/report.md"
ITERATE_HISTORY="tests/benchmark/iterative/history.json"

RELEASE_NOTES=""
if [ -f "$ITERATE_HISTORY" ]; then
    RUNS=$(python3 -c "import json; h=json.load(open('$ITERATE_HISTORY')); print(len(h))" 2>/dev/null || echo "0")
    if [ "$RUNS" != "0" ]; then
        LATEST=$(python3 -c "
import json
h = json.load(open('$ITERATE_HISTORY'))
r = h[-1]
print(f\"Tokens: {r['tokens']['total']:,d} | Turns: {r['turns']} | Calls: {r['tool_calls']} | Time: {r['wall_time_s']:.1f}s\")
" 2>/dev/null || echo "")
        if [ -n "$LATEST" ]; then
            RELEASE_NOTES="${RELEASE_NOTES}\n### Iterative Setup Benchmark\n\n${LATEST} (${RUNS} runs tracked)\n"
        fi
    fi
fi

PERF_RESULTS="tests/benchmark/perf/results.json"
if [ -f "$PERF_RESULTS" ]; then
    PERF_SUMMARY=$(python3 -c "
import json
r = json.load(open('$PERF_RESULTS'))
t = r.get('totals', {})
w = t.get('without', {})
wt = t.get('with', {})
if w.get('tokens') and wt.get('tokens'):
    pct = (w['tokens'] - wt['tokens']) / w['tokens'] * 100
    print(f\"Token reduction: {pct:.1f}% ({w['tokens']:,d} -> {wt['tokens']:,d})\")
" 2>/dev/null || echo "")
    if [ -n "$PERF_SUMMARY" ]; then
        RELEASE_NOTES="${RELEASE_NOTES}\n### Performance Benchmark\n\n${PERF_SUMMARY}\n"
    fi
fi

if [ -n "$RELEASE_NOTES" ]; then
    echo "==> Updating release notes with benchmark data..."
    EXISTING=$(gh release view "v${VERSION}" --json body -q .body 2>/dev/null || echo "")
    FULL_NOTES="${EXISTING}\n\n## Benchmarks\n${RELEASE_NOTES}"
    echo -e "$FULL_NOTES" | gh release edit "v${VERSION}" --notes-file -
fi

echo ""
echo "==> v${VERSION} released: https://github.com/T-Fizz/takumi/releases/tag/v${VERSION}"
