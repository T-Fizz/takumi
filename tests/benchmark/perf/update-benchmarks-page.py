#!/usr/bin/env python3
"""
Update docs/dev/benchmarks.md with results from the latest benchmark run.

Usage:
  python3 update-benchmarks-page.py --version v1.0.2
  python3 update-benchmarks-page.py --version v1.0.2 --results results.json --page ../../docs/dev/benchmarks.md
"""

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

SCENARIOS = {
    "fix-build-error": {
        "title": "Fix Build Error",
        "desc": "Find and fix a type error in a Go HTTP handler",
    },
    "scoped-rebuild": {
        "title": "Scoped Rebuild",
        "desc": "After changing shared lib, build only affected packages",
    },
    "understand-structure": {
        "title": "Understand Structure",
        "desc": "Explain dependency graph and build order of a 4-package monorepo",
    },
}

MARKER = "<!-- BENCHMARK_INSERT -->"


def fmt(n: int) -> str:
    return f"{n:,}"


def pct(old, new) -> str:
    if old == 0:
        return "—"
    return f"{(old - new) / old * 100:.1f}%"


def generate_section(version: str, results: dict) -> str:
    model = results.get("model", "unknown")
    date = datetime.now(timezone.utc).strftime("%Y-%m-%d")

    w = results["totals"]["without"]
    t = results["totals"]["with"]

    lines = [
        f"## {version}\n",
        f"> {date} | model: `{model}`\n",
        "### Overall\n",
        "| Metric | Without Takumi | With Takumi | Saved |",
        "|--------|---------------|-------------|-------|",
        f"| **Tokens** | {fmt(w['tokens'])} | {fmt(t['tokens'])} | **{pct(w['tokens'], t['tokens'])}** |",
        f"| Turns | {w['turns']} | {t['turns']} | {pct(w['turns'], t['turns'])} |",
        f"| Tool calls | {w['calls']} | {t['calls']} | {pct(w['calls'], t['calls'])} |",
        f"| Errors | {w['errors']} | {t['errors']} | {pct(w['errors'], t['errors'])} |",
        "",
        "### Scenarios\n",
    ]

    for sid, meta in SCENARIOS.items():
        if sid not in results["scenarios"]:
            continue

        wo = results["scenarios"][sid]["without_takumi"]
        wi = results["scenarios"][sid]["with_takumi"]

        w_tok = wo["input_tokens"] + wo["output_tokens"]
        t_tok = wi["input_tokens"] + wi["output_tokens"]

        lines.extend([
            f"#### {meta['title']}\n",
            f"> {meta['desc']}\n",
            "| Metric | Without | With Takumi | Saved |",
            "|--------|---------|-------------|-------|",
            f"| Tokens | {fmt(w_tok)} | {fmt(t_tok)} | {pct(w_tok, t_tok)} |",
            f"| Time | {wo['wall_time_s']:.1f}s | {wi['wall_time_s']:.1f}s | {pct(wo['wall_time_s'], wi['wall_time_s'])} |",
            f"| Turns | {wo['turns']} | {wi['turns']} | {pct(wo['turns'], wi['turns'])} |",
            f"| Tool calls | {wo['tool_calls']} | {wi['tool_calls']} | {pct(wo['tool_calls'], wi['tool_calls'])} |",
            f"| Completed | {'yes' if wo.get('task_completed') else 'no'} | {'yes' if wi.get('task_completed') else 'no'} | |",
            "",
        ])

    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Update benchmarks page")
    parser.add_argument("--version", required=True, help="Version tag (e.g. v1.0.2)")
    parser.add_argument("--results", default=None, help="Path to results.json")
    parser.add_argument("--page", default=None, help="Path to benchmarks.md")
    args = parser.parse_args()

    # Resolve paths relative to this script
    script_dir = Path(__file__).parent
    results_path = Path(args.results) if args.results else script_dir / "results.json"
    page_path = Path(args.page) if args.page else script_dir / ".." / ".." / ".." / "docs" / "dev" / "benchmarks.md"

    if not results_path.exists():
        print(f"Error: {results_path} not found", file=sys.stderr)
        sys.exit(1)

    if not page_path.exists():
        print(f"Error: {page_path} not found", file=sys.stderr)
        sys.exit(1)

    with open(results_path) as f:
        results = json.load(f)

    section = generate_section(args.version, results)
    page = page_path.read_text()

    if MARKER not in page:
        print(f"Error: marker {MARKER!r} not found in {page_path}", file=sys.stderr)
        sys.exit(1)

    # Check if this version already exists
    if f"## {args.version}" in page:
        print(f"Version {args.version} already in {page_path} — skipping")
        sys.exit(0)

    page = page.replace(MARKER, f"{MARKER}\n\n{section}\n---\n", 1)
    page_path.write_text(page)
    print(f"Added {args.version} results to {page_path}")


if __name__ == "__main__":
    main()
