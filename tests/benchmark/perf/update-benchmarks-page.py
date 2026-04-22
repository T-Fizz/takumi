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
    "fix-build-error-go": {"title": "Fix Build Error (Go)", "desc": "Find and fix a type error in a Go HTTP handler", "group": "fix-build-error", "lang": "Go"},
    "fix-build-error-python": {"title": "Fix Build Error (Python)", "desc": "Find and fix a TypeError in a Python project", "group": "fix-build-error", "lang": "Python"},
    "fix-build-error-ts": {"title": "Fix Build Error (TypeScript)", "desc": "Find and fix a type error caught by tsc", "group": "fix-build-error", "lang": "TypeScript"},
    "fix-build-error-rust": {"title": "Fix Build Error (Rust)", "desc": "Find and fix a type error in a Rust workspace", "group": "fix-build-error", "lang": "Rust"},
    "fix-build-error-java": {"title": "Fix Build Error (Java)", "desc": "Find and fix a type error in a Java project", "group": "fix-build-error", "lang": "Java"},
    "scoped-rebuild-go": {"title": "Scoped Rebuild (Go)", "desc": "After changing shared lib, build only affected Go packages", "group": "scoped-rebuild", "lang": "Go"},
    "scoped-rebuild-python": {"title": "Scoped Rebuild (Python)", "desc": "After changing shared lib, rebuild only affected Python packages", "group": "scoped-rebuild", "lang": "Python"},
    "scoped-rebuild-ts": {"title": "Scoped Rebuild (TypeScript)", "desc": "After changing shared lib, build only affected TS packages", "group": "scoped-rebuild", "lang": "TypeScript"},
    "scoped-rebuild-rust": {"title": "Scoped Rebuild (Rust)", "desc": "After changing shared crate, build only affected Rust crates", "group": "scoped-rebuild", "lang": "Rust"},
    "scoped-rebuild-java": {"title": "Scoped Rebuild (Java)", "desc": "After changing shared lib, build only affected Java packages", "group": "scoped-rebuild", "lang": "Java"},
    "understand-structure-go": {"title": "Understand Structure (Go)", "desc": "Explain dependency graph and build order of a Go monorepo", "group": "understand-structure", "lang": "Go"},
    "understand-structure-python": {"title": "Understand Structure (Python)", "desc": "Explain dependency graph and build order of a Python project", "group": "understand-structure", "lang": "Python"},
    "understand-structure-ts": {"title": "Understand Structure (TypeScript)", "desc": "Explain dependency graph and build order of a TS monorepo", "group": "understand-structure", "lang": "TypeScript"},
    "understand-structure-rust": {"title": "Understand Structure (Rust)", "desc": "Explain dependency graph and build order of a Rust workspace", "group": "understand-structure", "lang": "Rust"},
    "understand-structure-java": {"title": "Understand Structure (Java)", "desc": "Explain dependency graph and build order of a Java project", "group": "understand-structure", "lang": "Java"},
}

SCENARIO_GROUPS = {
    "fix-build-error": {"title": "Fix Build Error", "desc": "Find and fix a type error"},
    "scoped-rebuild": {"title": "Scoped Rebuild", "desc": "After changing shared lib, build only affected packages"},
    "understand-structure": {"title": "Understand Structure", "desc": "Explain dependency graph and build order of a monorepo"},
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
            f"| Correctness | {wo.get('correctness', 0) * 100:.0f}% | {wi.get('correctness', 0) * 100:.0f}% | |",
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
        "| Model | Without Takumi | With Takumi | Saved | Turns | Tool Calls | Correctness |",
        "|-------|---------------|-------------|-------|-------|------------|-------------|",
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

        # Compute average correctness score
        w_corr = w.get("correctness", 0.0)
        t_corr = t.get("correctness", 0.0)
        w_n = w.get("scenarios", 0) or 1
        t_n = t.get("scenarios", 0) or 1
        w_avg = w_corr / w_n * 100
        t_avg = t_corr / t_n * 100

        lines.append(
            f"| **{short_name(model)}**{suffix} "
            f"| {fmt(w.get('tokens', 0))} "
            f"| {fmt(t.get('tokens', 0))} "
            f"| **{pct(w.get('tokens', 0), t.get('tokens', 0))}** "
            f"| {w.get('turns', 0)} / {t.get('turns', 0)} "
            f"| {w.get('calls', 0)} / {t.get('calls', 0)} "
            f"| {w_avg:.0f}% / {t_avg:.0f}% |"
        )

    if partial_models:
        notes = "; ".join(f"{name} ran {n}/{all_scenario_count} scenarios" for name, n in partial_models)
        lines.append(f"\n*{notes} (cost control)*")

    lines.extend(["", "### Scenarios\n"])

    # Group scenarios by type, show one table per group with language rows
    for group_id, group_meta in SCENARIO_GROUPS.items():
        # Collect scenario IDs in this group
        group_sids = [sid for sid, meta in SCENARIOS.items() if meta.get("group") == group_id]
        if not group_sids:
            continue

        # Check if any model has data for any scenario in this group
        has_data = False
        for sid in group_sids:
            for model in models:
                data = models_data[model]
                if sid in data.get("scenarios", {}):
                    has_data = True
                    break

        if not has_data:
            continue

        lines.extend([
            f"#### {group_meta['title']}\n",
            f"> {group_meta['desc']}\n",
        ])

        # One summary table: Language × Model showing tokens saved + correctness
        header = "| Language |"
        sep = "|----------|"
        for model in models:
            name = short_name(model)
            header += f" {name} |||"
            sep += "------|------|------|"

        lines.append(header)
        lines.append(sep)

        # Subheader
        subheader = "| |"
        for _ in models:
            subheader += " Without | With | Saved |"
        lines.append(subheader)

        for sid in group_sids:
            meta = SCENARIOS[sid]
            lang = meta.get("lang", "?")
            row = f"| **{lang}** |"

            for model in models:
                data = models_data[model]
                scenario = data.get("scenarios", {}).get(sid, {})
                wo = scenario.get("without_takumi", {})
                wi = scenario.get("with_takumi", {})

                if not wo or (wo.get("error") and "input_tokens" not in wo):
                    row += " — | — | — |"
                    continue

                w_tok = wo.get("input_tokens", 0) + wo.get("output_tokens", 0)
                t_tok = wi.get("input_tokens", 0) + wi.get("output_tokens", 0)
                row += f" {fmt(w_tok)} | {fmt(t_tok)} | **{pct(w_tok, t_tok)}** |"

            lines.append(row)

        # Correctness sub-table
        lines.append("")
        lines.append(f"**Correctness** (without / with):\n")

        corr_header = "| Language |"
        corr_sep = "|----------|"
        for model in models:
            corr_header += f" {short_name(model)} |"
            corr_sep += "------|"
        lines.append(corr_header)
        lines.append(corr_sep)

        for sid in group_sids:
            meta = SCENARIOS[sid]
            lang = meta.get("lang", "?")
            row = f"| {lang} |"
            for model in models:
                data = models_data[model]
                scenario = data.get("scenarios", {}).get(sid, {})
                wo = scenario.get("without_takumi", {})
                wi = scenario.get("with_takumi", {})
                if not wo or (wo.get("error") and "input_tokens" not in wo):
                    row += " — |"
                    continue
                w_corr = wo.get("correctness", 0.0) * 100
                t_corr = wi.get("correctness", 0.0) * 100
                row += f" {w_corr:.0f}% / {t_corr:.0f}% |"
            lines.append(row)

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
