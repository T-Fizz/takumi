#!/usr/bin/env python3
"""
Update docs/dev/benchmarks.md with results from the latest benchmark run.

Supports both single-model results.json and multi-model results-combined.json.

Usage:
  python3 update-benchmarks-page.py --version v1.0.2
  python3 update-benchmarks-page.py --version v1.0.2 --results results-combined.json
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

MODEL_NAMES = {
    "claude-haiku-4-5-20251001": "Haiku 4.5",
    "claude-sonnet-4-6": "Sonnet 4.6",
    "claude-opus-4-6": "Opus 4.6",
}

MARKER = "<!-- BENCHMARK_INSERT -->"


def fmt(n):
    return f"{n:,}"


def pct(old, new):
    if old == 0:
        return "—"
    return f"{(old - new) / old * 100:.1f}%"


def short_name(model):
    return MODEL_NAMES.get(model, model)


def is_multi_model(data):
    return "models" in data


def generate_section_single(version, results):
    """Generate markdown for a single-model result (backward compat)."""
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
        if sid not in results.get("scenarios", {}):
            continue

        scenario = results["scenarios"][sid]
        wo = scenario.get("without_takumi", {})
        wi = scenario.get("with_takumi", {})
        if wo.get("error") and "input_tokens" not in wo:
            continue
        if wi.get("error") and "input_tokens" not in wi:
            continue

        w_tok = wo.get("input_tokens", 0) + wo.get("output_tokens", 0)
        t_tok = wi.get("input_tokens", 0) + wi.get("output_tokens", 0)

        lines.extend([
            f"#### {meta['title']}\n",
            f"> {meta['desc']}\n",
            "| Metric | Without | With Takumi | Saved |",
            "|--------|---------|-------------|-------|",
            f"| Tokens | {fmt(w_tok)} | {fmt(t_tok)} | {pct(w_tok, t_tok)} |",
            f"| Time | {wo.get('wall_time_s', 0):.1f}s | {wi.get('wall_time_s', 0):.1f}s | {pct(wo.get('wall_time_s', 0), wi.get('wall_time_s', 0))} |",
            f"| Turns | {wo.get('turns', 0)} | {wi.get('turns', 0)} | {pct(wo.get('turns', 0), wi.get('turns', 0))} |",
            f"| Tool calls | {wo.get('tool_calls', 0)} | {wi.get('tool_calls', 0)} | {pct(wo.get('tool_calls', 0), wi.get('tool_calls', 0))} |",
            f"| Completed | {'yes' if wo.get('task_completed') else 'no'} | {'yes' if wi.get('task_completed') else 'no'} | |",
            "",
        ])

    return "\n".join(lines)


def generate_section_multi(version, combined):
    """Generate markdown for multi-model results."""
    date = datetime.now(timezone.utc).strftime("%Y-%m-%d")
    models_data = combined["models"]

    # Sort models: haiku, sonnet, opus
    model_order = ["claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-6"]
    models = [m for m in model_order if m in models_data]
    # Add any models not in our known order
    for m in models_data:
        if m not in models:
            models.append(m)

    model_list = ", ".join(short_name(m) for m in models)

    lines = [
        f"## {version}\n",
        f"> {date} | models: {model_list}\n",
        "### Token Savings by Model\n",
        "| Model | Without Takumi | With Takumi | Saved | Turns | Tool Calls |",
        "|-------|---------------|-------------|-------|-------|------------|",
    ]

    # Track which models ran partial scenarios
    partial_models = []
    all_scenario_count = len(SCENARIOS)

    for model in models:
        data = models_data[model]
        w = data.get("totals", {}).get("without", {})
        t = data.get("totals", {}).get("with", {})
        if not w or not t:
            continue

        # Count how many scenarios this model ran
        scenario_count = sum(
            1 for s in data.get("scenarios", {}).values()
            if "error" not in s.get("without_takumi", {})
        )
        suffix = ""
        if scenario_count < all_scenario_count:
            partial_models.append((short_name(model), scenario_count))
            suffix = "*"

        lines.append(
            f"| **{short_name(model)}**{suffix} "
            f"| {fmt(w.get('tokens', 0))} "
            f"| {fmt(t.get('tokens', 0))} "
            f"| **{pct(w.get('tokens', 0), t.get('tokens', 0))}** "
            f"| {w.get('turns', 0)} / {t.get('turns', 0)} "
            f"| {w.get('calls', 0)} / {t.get('calls', 0)} |"
        )

    if partial_models:
        notes = "; ".join(f"{name} ran {n}/{all_scenario_count} scenarios" for name, n in partial_models)
        lines.append(f"\n*{notes} (cost control)*")

    lines.extend(["", "### Scenarios\n"])

    # Per-scenario comparison across models
    for sid, meta in SCENARIOS.items():
        # Find which models have data for this scenario
        avail = []
        for model in models:
            data = models_data[model]
            if sid not in data.get("scenarios", {}):
                continue
            scenario = data["scenarios"][sid]
            wo = scenario.get("without_takumi", {})
            wi = scenario.get("with_takumi", {})
            if wo.get("error") and "input_tokens" not in wo:
                continue
            if wi.get("error") and "input_tokens" not in wi:
                continue
            avail.append(model)

        if not avail:
            continue

        lines.extend([
            f"#### {meta['title']}\n",
            f"> {meta['desc']}\n",
        ])

        # Build table with model columns
        header = "| Metric |"
        sep = "|--------|"
        for model in avail:
            name = short_name(model)
            header += f" {name} ||"
            sep += "------|------|"

        lines.append(header)
        lines.append(sep)

        # Subheader row
        subheader = "| |"
        for _ in avail:
            subheader += " Without | With |"
        lines.append(subheader)

        # Data rows
        for metric, key_w, key_t, is_time in [
            ("Tokens", lambda wo, wi: wo.get("input_tokens", 0) + wo.get("output_tokens", 0),
                        lambda wo, wi: wi.get("input_tokens", 0) + wi.get("output_tokens", 0), False),
            ("Turns", lambda wo, wi: wo.get("turns", 0), lambda wo, wi: wi.get("turns", 0), False),
            ("Tool calls", lambda wo, wi: wo.get("tool_calls", 0), lambda wo, wi: wi.get("tool_calls", 0), False),
            ("Time", lambda wo, wi: wo.get("wall_time_s", 0), lambda wo, wi: wi.get("wall_time_s", 0), True),
        ]:
            row = f"| {metric} |"
            for model in avail:
                data = models_data[model]
                wo = data["scenarios"][sid].get("without_takumi", {})
                wi = data["scenarios"][sid].get("with_takumi", {})
                wv = key_w(wo, wi)
                tv = key_t(wo, wi)
                if is_time:
                    row += f" {wv:.1f}s | {tv:.1f}s |"
                elif isinstance(wv, int):
                    row += f" {fmt(wv)} | {fmt(tv)} |"
                else:
                    row += f" {wv} | {tv} |"
            lines.append(row)

        # Saved row
        saved_row = "| **Saved** |"
        for model in avail:
            data = models_data[model]
            wo = data["scenarios"][sid].get("without_takumi", {})
            wi = data["scenarios"][sid].get("with_takumi", {})
            w_tok = wo.get("input_tokens", 0) + wo.get("output_tokens", 0)
            t_tok = wi.get("input_tokens", 0) + wi.get("output_tokens", 0)
            saved_row += f" **{pct(w_tok, t_tok)}** ||"
        lines.append(saved_row)

        lines.append("")

    return "\n".join(lines)


def generate_section(version, data):
    if is_multi_model(data):
        return generate_section_multi(version, data)
    return generate_section_single(version, data)


def main():
    parser = argparse.ArgumentParser(description="Update benchmarks page")
    parser.add_argument("--version", required=True, help="Version tag (e.g. v1.0.2)")
    parser.add_argument("--results", default=None, help="Path to results.json or results-combined.json")
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
        data = json.load(f)

    page = page_path.read_text()

    if MARKER not in page:
        print(f"Error: marker {MARKER!r} not found in {page_path}", file=sys.stderr)
        sys.exit(1)

    # Check if this version already exists
    if f"## {args.version}" in page:
        print(f"Version {args.version} already in {page_path} — skipping")
        sys.exit(0)

    section = generate_section(args.version, data)
    page = page.replace(MARKER, f"{MARKER}\n\n{section}\n---\n", 1)
    page_path.write_text(page)
    print(f"Added {args.version} results to {page_path}")


if __name__ == "__main__":
    main()
