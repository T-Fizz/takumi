#!/bin/bash
# run-takumi-md.sh — promptfoo exec provider that outputs .takumi/TAKUMI.md
# Used to test operator prompt content without needing the removed `takumi ai` commands.
#
# Usage: run-takumi-md.sh [ignored promptfoo args...]

set -euo pipefail

FIXTURE_DIR="${FIXTURE_DIR:-$(dirname "$0")/fixtures/sample-ws}"
FIXTURE_DIR="$(cd "$FIXTURE_DIR" && pwd)"

cat "$FIXTURE_DIR/.takumi/TAKUMI.md"
