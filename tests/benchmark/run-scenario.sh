#!/bin/bash
# run-scenario.sh — promptfoo exec provider wrapper for benchmark scenarios
#
# Usage: run-scenario.sh <scenario-name>
# Environment: TAKUMI_BIN must point to the compiled binary.
#
# promptfoo appends extra arguments (prompt, provider JSON, test JSON) after
# our args. We strip them using SCENARIO_ARGC.

set -uo pipefail

TAKUMI_BIN="${TAKUMI_BIN:-$(dirname "$0")/../../build/takumi}"
TAKUMI_BIN="$(cd "$(dirname "$TAKUMI_BIN")" && pwd)/$(basename "$TAKUMI_BIN")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Strip promptfoo's extra args
ARGC="${SCENARIO_ARGC:-$(( $# - 3 ))}"
ARGS=("${@:1:$ARGC}")

SCENARIO="${ARGS[0]}"

export TAKUMI_BIN
exec bash "$SCRIPT_DIR/scenarios/${SCENARIO}.sh" 2>&1
