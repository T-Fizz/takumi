#!/bin/bash
# run-skill.sh — promptfoo exec provider wrapper
# Invokes a Takumi AI skill inside the fixture workspace and captures output.
#
# Usage: run-skill.sh <takumi-args...>
# Environment: TAKUMI_BIN must point to the compiled binary.
#              FIXTURE_DIR must point to the fixture workspace.
#
# promptfoo appends extra arguments (prompt, provider JSON, test JSON) after
# our args. We use TAKUMI_ARGC to know how many args are ours and ignore the rest.

set -euo pipefail

TAKUMI_BIN="${TAKUMI_BIN:-$(dirname "$0")/../../build/takumi}"
FIXTURE_DIR="${FIXTURE_DIR:-$(dirname "$0")/fixtures/sample-ws}"

# Resolve to absolute paths
TAKUMI_BIN="$(cd "$(dirname "$TAKUMI_BIN")" && pwd)/$(basename "$TAKUMI_BIN")"
FIXTURE_DIR="$(cd "$FIXTURE_DIR" && pwd)"

# TAKUMI_ARGC tells us how many args are ours (set in provider config).
# Default: take all args except the last 3 (promptfoo appends prompt, provider, test).
ARGC="${TAKUMI_ARGC:-$(( $# - 3 ))}"

# Extract only our args
TAKUMI_ARGS=("${@:1:$ARGC}")

# Run the skill from within the fixture workspace
cd "$FIXTURE_DIR"
exec "$TAKUMI_BIN" "${TAKUMI_ARGS[@]}" 2>&1
