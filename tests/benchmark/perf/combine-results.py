#!/usr/bin/env python3
"""
Combine per-model benchmark results into a single multi-model results file.

Usage:
  python3 combine-results.py --input-dir bench-results/ --output results-combined.json
"""

import argparse
import json
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Combine multi-model benchmark results")
    parser.add_argument("--input-dir", required=True, help="Directory containing results-*.json files")
    parser.add_argument("--output", required=True, help="Path to write combined results")
    args = parser.parse_args()

    input_dir = Path(args.input_dir)
    if not input_dir.exists():
        print(f"Error: {input_dir} not found", file=sys.stderr)
        sys.exit(1)

    models = {}
    for f in sorted(input_dir.glob("results-*.json")):
        with open(f) as fp:
            data = json.load(fp)
        model = data.get("model", f.stem)
        models[model] = data

    if not models:
        # Fall back to a single results.json
        single = input_dir / "results.json"
        if single.exists():
            with open(single) as fp:
                data = json.load(fp)
            models[data.get("model", "unknown")] = data

    if not models:
        print("Error: no results files found", file=sys.stderr)
        sys.exit(1)

    combined = {"models": models}

    with open(args.output, "w") as fp:
        json.dump(combined, fp, indent=2)

    print(f"Combined {len(models)} model(s): {', '.join(models.keys())}")
    print(f"Output: {args.output}")


if __name__ == "__main__":
    main()
