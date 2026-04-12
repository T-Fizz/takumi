#!/usr/bin/env python3
"""
Takumi Iterative Setup Benchmark

Tests how efficiently an AI agent can set up a freshly-cloned project when
Takumi is configured.  Run after improving README, operator skill, or
TAKUMI.md and compare metrics across iterations to track improvement.

Each run appends to history.json for trend tracking.

Usage:
  ANTHROPIC_API_KEY=sk-... TAKUMI_BIN=build/takumi python3 benchmark.py
  ANTHROPIC_API_KEY=sk-... python3 benchmark.py --note "improved README"

Or from repo root:
  make benchmark-iterate
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import uuid
from dataclasses import dataclass, field, asdict
from pathlib import Path

try:
    import anthropic
except ImportError:
    print("Error: anthropic package required. Run: pip install anthropic", file=sys.stderr)
    sys.exit(1)

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

MAX_TURNS = 25
MODEL = os.environ.get("BENCH_MODEL", "claude-haiku-4-5-20251001")
TAKUMI_BIN = os.environ.get("TAKUMI_BIN", "")

GIT_ENV = {
    **os.environ,
    "GIT_AUTHOR_NAME": "bench",
    "GIT_AUTHOR_EMAIL": "bench@test.com",
    "GIT_COMMITTER_NAME": "bench",
    "GIT_COMMITTER_EMAIL": "bench@test.com",
}

# ---------------------------------------------------------------------------
# Metrics
# ---------------------------------------------------------------------------

@dataclass
class Metrics:
    input_tokens: int = 0
    output_tokens: int = 0
    tool_calls: int = 0
    turns: int = 0
    errors: int = 0
    wall_time_s: float = 0.0
    task_completed: bool = False
    transcript: list = field(default_factory=list)

    @property
    def total_tokens(self):
        return self.input_tokens + self.output_tokens

# ---------------------------------------------------------------------------
# Tools — same set as the perf benchmark
# ---------------------------------------------------------------------------

TOOLS = [
    {
        "name": "run_command",
        "description": "Run a shell command in the project directory. Returns stdout+stderr.",
        "input_schema": {
            "type": "object",
            "properties": {
                "command": {"type": "string", "description": "Shell command to execute"}
            },
            "required": ["command"],
        },
    },
    {
        "name": "read_file",
        "description": "Read the contents of a file.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "Path relative to project root"}
            },
            "required": ["path"],
        },
    },
    {
        "name": "write_file",
        "description": "Write content to a file (creates parent directories if needed).",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "Path relative to project root"},
                "content": {"type": "string", "description": "File content"},
            },
            "required": ["path", "content"],
        },
    },
    {
        "name": "list_files",
        "description": "List files and directories at a path.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Directory path relative to project root (default: .)",
                }
            },
        },
    },
    {
        "name": "task_complete",
        "description": "Signal that you have finished the task.",
        "input_schema": {
            "type": "object",
            "properties": {
                "summary": {"type": "string", "description": "What you did"}
            },
            "required": ["summary"],
        },
    },
]


def execute_tool(name, inputs, workdir, env_path):
    """Execute a tool call. Returns (output_text, is_error)."""
    try:
        if name == "run_command":
            env = {**os.environ, "PATH": env_path}
            r = subprocess.run(
                inputs["command"],
                shell=True,
                cwd=workdir,
                capture_output=True,
                text=True,
                timeout=30,
                env=env,
            )
            out = (r.stdout + "\n" + r.stderr).strip()
            if r.returncode != 0:
                out += f"\n(exit code {r.returncode})"
            return out[:4000] or "(no output)", r.returncode != 0

        if name == "read_file":
            p = os.path.join(workdir, inputs["path"])
            return open(p).read()[:8000], False

        if name == "write_file":
            p = os.path.join(workdir, inputs["path"])
            os.makedirs(os.path.dirname(p) or ".", exist_ok=True)
            with open(p, "w") as f:
                f.write(inputs["content"])
            return f"Wrote {inputs['path']}", False

        if name == "list_files":
            p = os.path.join(workdir, inputs.get("path", "."))
            entries = sorted(os.listdir(p))
            return "\n".join(entries) or "(empty)", False

        if name == "task_complete":
            return "Task complete.", False

    except Exception as e:
        return f"Error: {e}", True

    return f"Unknown tool: {name}", True


# ---------------------------------------------------------------------------
# Agent runner
# ---------------------------------------------------------------------------

def run_agent(task, workdir, system_prompt, model, env_path):
    """Drive a multi-turn agent loop and collect metrics."""
    client = anthropic.Anthropic()
    metrics = Metrics()

    messages = [{"role": "user", "content": task}]
    start = time.time()

    for _ in range(MAX_TURNS):
        resp = client.messages.create(
            model=model,
            max_tokens=4096,
            system=system_prompt,
            tools=TOOLS,
            messages=messages,
        )

        metrics.input_tokens += resp.usage.input_tokens
        metrics.output_tokens += resp.usage.output_tokens
        metrics.turns += 1

        for block in resp.content:
            if hasattr(block, "text"):
                metrics.transcript.append({"role": "assistant", "text": block.text})

        if resp.stop_reason == "end_turn":
            break

        tool_results = []
        done = False

        for block in resp.content:
            if block.type != "tool_use":
                continue

            metrics.tool_calls += 1
            entry = {"tool": block.name, "input": block.input}

            if block.name == "task_complete":
                done = True
                entry["output"] = "(completed)"
                metrics.transcript.append(entry)
                tool_results.append({
                    "type": "tool_result",
                    "tool_use_id": block.id,
                    "content": "Task complete.",
                })
                continue

            output, is_error = execute_tool(block.name, block.input, workdir, env_path)
            if is_error:
                metrics.errors += 1

            entry["output"] = output[:500]
            entry["is_error"] = is_error
            metrics.transcript.append(entry)

            tool_results.append({
                "type": "tool_result",
                "tool_use_id": block.id,
                "content": output,
                **({"is_error": True} if is_error else {}),
            })

        messages.append({"role": "assistant", "content": resp.content})
        messages.append({"role": "user", "content": tool_results})

        if done:
            metrics.task_completed = True
            break

    metrics.wall_time_s = time.time() - start
    return metrics


# ---------------------------------------------------------------------------
# Project setup — realistic multi-package Go monorepo
# ---------------------------------------------------------------------------

def setup_project(workdir):
    """
    Create a 3-package Go monorepo with Takumi config and README.
    Returns the content of the generated TAKUMI.md.
    """

    # ── README ──────────────────────────────────────────────────────────
    Path(f"{workdir}/README.md").write_text("""\
# Platform Service

A Go monorepo with three packages: core library, API server, and background worker.

## Prerequisites

- Go 1.22+
- [Takumi](https://github.com/tfitz/takumi) for build orchestration

## Setup

This project uses Takumi. To get started:

```bash
takumi status       # see workspace overview
takumi build        # build all packages in dependency order
takumi test         # run all tests
```

## Packages

| Package | Description | Dependencies |
|---------|-------------|--------------|
| core    | Shared utilities and types | none |
| api     | HTTP API server | core |
| worker  | Background job processor | core |

## Development

After making changes, use `takumi affected` to see what needs rebuilding,
then `takumi build --affected` to rebuild only those packages.
""")

    # ── core package ────────────────────────────────────────────────────
    os.makedirs(f"{workdir}/core")
    Path(f"{workdir}/core/go.mod").write_text(
        "module example.com/platform/core\n\ngo 1.22\n"
    )
    Path(f"{workdir}/core/core.go").write_text("""\
package core

import "fmt"

// Version returns the platform version.
func Version() string { return "1.0.0" }

// FormatMessage creates a standard log-style message.
func FormatMessage(service, msg string) string {
\treturn fmt.Sprintf("[%s] %s (v%s)", service, msg, Version())
}
""")
    Path(f"{workdir}/core/core_test.go").write_text("""\
package core

import "testing"

func TestVersion(t *testing.T) {
\tif v := Version(); v != "1.0.0" {
\t\tt.Errorf("expected 1.0.0, got %s", v)
\t}
}

func TestFormatMessage(t *testing.T) {
\tgot := FormatMessage("api", "started")
\twant := "[api] started (v1.0.0)"
\tif got != want {
\t\tt.Errorf("got %q, want %q", got, want)
\t}
}
""")

    # ── api package ─────────────────────────────────────────────────────
    os.makedirs(f"{workdir}/api")
    Path(f"{workdir}/api/go.mod").write_text(
        "module example.com/platform/api\n\ngo 1.22\n"
    )
    Path(f"{workdir}/api/main.go").write_text("""\
package main

import (
\t"fmt"
\t"net/http"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
\tw.WriteHeader(http.StatusOK)
\tfmt.Fprintf(w, "ok")
}

func main() {
\thttp.HandleFunc("/health", healthHandler)
\tfmt.Println("api listening on :8080")
}
""")
    Path(f"{workdir}/api/main_test.go").write_text("""\
package main

import (
\t"net/http"
\t"net/http/httptest"
\t"testing"
)

func TestHealthHandler(t *testing.T) {
\treq := httptest.NewRequest("GET", "/health", nil)
\tw := httptest.NewRecorder()
\thealthHandler(w, req)
\tif w.Code != http.StatusOK {
\t\tt.Errorf("expected 200, got %d", w.Code)
\t}
}
""")

    # ── worker package ──────────────────────────────────────────────────
    os.makedirs(f"{workdir}/worker")
    Path(f"{workdir}/worker/go.mod").write_text(
        "module example.com/platform/worker\n\ngo 1.22\n"
    )
    Path(f"{workdir}/worker/main.go").write_text("""\
package main

import "fmt"

func process(job string) string {
\treturn fmt.Sprintf("processed: %s", job)
}

func main() {
\tfmt.Println("worker started")
\tfmt.Println(process("init"))
}
""")
    Path(f"{workdir}/worker/main_test.go").write_text("""\
package main

import "testing"

func TestProcess(t *testing.T) {
\tresult := process("test-job")
\tif result != "processed: test-job" {
\t\tt.Errorf("unexpected: %s", result)
\t}
}
""")

    # ── git init ────────────────────────────────────────────────────────
    subprocess.run(
        "git init -q && git add -A && git commit -m 'Initial commit' -q",
        shell=True, cwd=workdir, capture_output=True, env=GIT_ENV,
    )

    # ── takumi init (generates real TAKUMI.md) ──────────────────────────
    subprocess.run(
        f"{TAKUMI_BIN} init --agent none",
        shell=True, cwd=workdir, capture_output=True,
    )

    # Remove root package config — we provide per-package configs
    root_pkg = Path(f"{workdir}/takumi-pkg.yaml")
    if root_pkg.exists():
        root_pkg.unlink()

    # ── package configs ─────────────────────────────────────────────────
    Path(f"{workdir}/core/takumi-pkg.yaml").write_text("""\
package:
  name: core
  version: 1.0.0
ai:
  description: Shared utilities and types used by api and worker
phases:
  build:
    commands:
      - go build ./...
  test:
    commands:
      - go test ./...
""")

    Path(f"{workdir}/api/takumi-pkg.yaml").write_text("""\
package:
  name: api
  version: 1.0.0
dependencies:
  - core
ai:
  description: HTTP API server with health endpoint
phases:
  build:
    commands:
      - go build ./...
  test:
    commands:
      - go test ./...
""")

    Path(f"{workdir}/worker/takumi-pkg.yaml").write_text("""\
package:
  name: worker
  version: 1.0.0
dependencies:
  - core
ai:
  description: Background job processor
phases:
  build:
    commands:
      - go build ./...
  test:
    commands:
      - go test ./...
""")

    # Commit takumi config
    subprocess.run(
        "git add -A && git commit -m 'Add takumi config' -q",
        shell=True, cwd=workdir, capture_output=True, env=GIT_ENV,
    )

    # Read the generated TAKUMI.md
    takumi_md_path = Path(f"{workdir}/.takumi/TAKUMI.md")
    takumi_md = takumi_md_path.read_text() if takumi_md_path.exists() else ""
    return takumi_md


# ---------------------------------------------------------------------------
# Verification — scan transcript for successful build/test
# ---------------------------------------------------------------------------

def verify_transcript(metrics):
    """Check whether the agent ran build and test commands successfully."""
    build_ok = False
    test_ok = False

    for entry in metrics.transcript:
        if "tool" not in entry or entry["tool"] != "run_command":
            continue
        cmd = entry.get("input", {}).get("command", "")
        is_error = entry.get("is_error", False)

        if is_error:
            continue
        if "takumi build" in cmd or ("go build" in cmd and "go.mod" not in cmd):
            build_ok = True
        if "takumi test" in cmd or ("go test" in cmd and "go.mod" not in cmd):
            test_ok = True

    return {"build": build_ok, "test": test_ok}


# ---------------------------------------------------------------------------
# History
# ---------------------------------------------------------------------------

def history_path():
    return Path(__file__).resolve().parent / "history.json"


def load_history():
    hp = history_path()
    if hp.exists():
        return json.loads(hp.read_text())
    return []


def save_history(history):
    hp = history_path()
    hp.write_text(json.dumps(history, indent=2) + "\n")


# ---------------------------------------------------------------------------
# Transcript log (same format as perf benchmark)
# ---------------------------------------------------------------------------

def write_transcript_log(metrics, run_id):
    logs_dir = Path(__file__).resolve().parent / "logs"
    logs_dir.mkdir(exist_ok=True)

    log_path = logs_dir / f"{run_id}.log"
    m = asdict(metrics)

    with open(log_path, "w") as f:
        f.write(f"# Tokens: {m['input_tokens'] + m['output_tokens']:,d} "
                f"(in: {m['input_tokens']:,d}, out: {m['output_tokens']:,d})\n")
        f.write(f"# Turns: {m['turns']}, "
                f"Tool calls: {m['tool_calls']}, "
                f"Errors: {m['errors']}\n")
        f.write(f"# Time: {m['wall_time_s']:.1f}s\n")
        f.write(f"# Completed: {m['task_completed']}\n\n")

        step = 0
        for entry in m.get("transcript", []):
            if "tool" in entry:
                step += 1
                tool = entry["tool"]
                inp = entry.get("input", {})
                is_err = entry.get("is_error", False)
                output = entry.get("output", "")
                err_tag = " [ERROR]" if is_err else ""

                if tool == "run_command":
                    f.write(f"--- [{step}] run_command{err_tag}\n")
                    f.write(f"$ {inp.get('command', '')}\n\n{output}\n\n")
                elif tool == "read_file":
                    f.write(f"--- [{step}] read_file {inp.get('path', '')}{err_tag}\n\n{output}\n\n")
                elif tool == "write_file":
                    f.write(f"--- [{step}] write_file {inp.get('path', '')}{err_tag}\n\n{inp.get('content', '')}\n\n")
                elif tool == "list_files":
                    f.write(f"--- [{step}] list_files {inp.get('path', '.')}{err_tag}\n\n{output}\n\n")
                elif tool == "task_complete":
                    f.write(f"--- [{step}] task_complete\n\n{inp.get('summary', '')}\n\n")
                else:
                    f.write(f"--- [{step}] {tool}{err_tag}\n{json.dumps(inp, indent=2)}\n\n")

            elif "text" in entry:
                text = entry["text"].strip()
                if text:
                    f.write(f"ASSISTANT: {text}\n\n")

    return str(log_path)


# ---------------------------------------------------------------------------
# Dashboard
# ---------------------------------------------------------------------------

W = 72


def _status_str(completed, verification):
    if not completed:
        return "INCOMPLETE"
    b = "build" if verification.get("build") else "no-build"
    t = "test" if verification.get("test") else "no-test"
    if verification.get("build") and verification.get("test"):
        return "PASS"
    return f"PARTIAL ({b}, {t})"


def _pct_change(old, new):
    if old == 0:
        return "n/a"
    pct = (new - old) / old * 100
    return f"{pct:+.1f}%"


def print_dashboard(current, history):
    """Print dashboard with current run metrics + historical trend."""
    run_num = len(history)
    ts = current.get("timestamp", "")

    print()
    print(f"  \u250c{'\u2500' * (W - 4)}\u2510")
    print(f"  \u2502 {'TAKUMI ITERATIVE SETUP BENCHMARK':^{W - 6}} \u2502")
    print(f"  \u251c{'\u2500' * (W - 4)}\u2524")
    print(f"  \u2502   Model:      {MODEL:<{W - 20}}\u2502")
    print(f"  \u2502   Run:        #{run_num:<{W - 21}}\u2502")
    print(f"  \u2502   Timestamp:  {ts:<{W - 20}}\u2502")
    note = current.get("note", "")
    if note:
        # Truncate note to fit
        max_note = W - 20
        display_note = note[:max_note]
        print(f"  \u2502   Note:       {display_note:<{W - 20}}\u2502")
    print(f"  \u251c{'\u2500' * (W - 4)}\u2524")

    # Current run
    tok = current["tokens"]["total"]
    tok_in = current["tokens"]["input"]
    tok_out = current["tokens"]["output"]
    turns = current["turns"]
    calls = current["tool_calls"]
    errors = current["errors"]
    wall = current["wall_time_s"]
    status = _status_str(current["task_completed"], current["verification"])

    print(f"  \u2502 {'CURRENT RUN':<{W - 6}} \u2502")
    print(f"  \u2502   Tokens:      {tok:>7,d}  (in: {tok_in:,d}, out: {tok_out:,d}){'':<{max(0, W - 51 - len(f'{tok_in:,d}') - len(f'{tok_out:,d}'))}}\u2502")
    print(f"  \u2502   Turns:       {turns:>7d}{'':<{W - 24}}\u2502")
    print(f"  \u2502   Tool calls:  {calls:>7d}{'':<{W - 24}}\u2502")
    print(f"  \u2502   Errors:      {errors:>7d}{'':<{W - 24}}\u2502")
    print(f"  \u2502   Time:        {wall:>6.1f}s{'':<{W - 24}}\u2502")
    print(f"  \u2502   Status:      {status:<{W - 20}}\u2502")

    # History table
    if len(history) > 1:
        print(f"  \u251c{'\u2500' * (W - 4)}\u2524")
        print(f"  \u2502 {'HISTORY (last 10 runs)':<{W - 6}} \u2502")
        hdr = f"{'Run':>5s}  {'Date':<10s}  {'Tokens':>7s}  {'Turns':>5s}  {'Calls':>5s}  {'Err':>3s}  {'Time':>6s}  {'Status':<6s}"
        print(f"  \u2502   {hdr}{'':<{max(0, W - 6 - len(hdr) - 2)}}\u2502")

        show = history[-10:]
        offset = max(0, len(history) - 10)
        for i, h in enumerate(show):
            n = offset + i + 1
            arrow = "\u2192" if n == run_num else " "
            date = h.get("timestamp", "")[:10]
            ht = h["tokens"]["total"]
            s = "PASS" if h["verification"].get("build") and h["verification"].get("test") else "PART"
            row = f"{arrow}#{n:<3d}  {date:<10s}  {ht:>7,d}  {h['turns']:>5d}  {h['tool_calls']:>5d}  {h['errors']:>3d}  {h['wall_time_s']:>5.1f}s  {s:<6s}"
            print(f"  \u2502   {row}{'':<{max(0, W - 6 - len(row) - 2)}}\u2502")

        # Trend: first vs latest
        if len(history) >= 2:
            first = history[0]
            latest = history[-1]
            print(f"  \u251c{'\u2500' * (W - 4)}\u2524")
            print(f"  \u2502 {'TREND (run #1 \u2192 latest)':<{W - 6}} \u2502")

            ft = first["tokens"]["total"]
            lt = latest["tokens"]["total"]
            tok_chg = _pct_change(ft, lt)
            turn_chg = _pct_change(first["turns"], latest["turns"])
            call_chg = _pct_change(first["tool_calls"], latest["tool_calls"])
            time_chg = _pct_change(first["wall_time_s"], latest["wall_time_s"])

            print(f"  \u2502   Tokens:  {ft:>7,d} \u2192 {lt:>7,d}  ({tok_chg}){'':<{max(0, W - 44 - len(tok_chg))}}\u2502")
            print(f"  \u2502   Turns:   {first['turns']:>7d} \u2192 {latest['turns']:>7d}  ({turn_chg}){'':<{max(0, W - 44 - len(turn_chg))}}\u2502")
            print(f"  \u2502   Calls:   {first['tool_calls']:>7d} \u2192 {latest['tool_calls']:>7d}  ({call_chg}){'':<{max(0, W - 44 - len(call_chg))}}\u2502")
            print(f"  \u2502   Time:    {first['wall_time_s']:>6.1f}s \u2192 {latest['wall_time_s']:>6.1f}s  ({time_chg}){'':<{max(0, W - 45 - len(time_chg))}}\u2502")

    print(f"  \u2514{'\u2500' * (W - 4)}\u2518")
    print()


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Takumi iterative setup benchmark")
    parser.add_argument("--note", default="", help="Annotation for this run")
    args = parser.parse_args()

    # Resolve TAKUMI_BIN
    global TAKUMI_BIN
    if TAKUMI_BIN:
        TAKUMI_BIN = str(Path(TAKUMI_BIN).resolve())
    else:
        default = Path(__file__).resolve().parent.parent.parent.parent / "build" / "takumi"
        if default.exists():
            TAKUMI_BIN = str(default)
        else:
            print("Error: TAKUMI_BIN not set and build/takumi not found", file=sys.stderr)
            sys.exit(1)

    if not Path(TAKUMI_BIN).exists():
        print(f"Error: TAKUMI_BIN not found: {TAKUMI_BIN}", file=sys.stderr)
        sys.exit(1)

    # Build env PATH with takumi
    takumi_dir = os.path.dirname(TAKUMI_BIN)
    env_path = f"{takumi_dir}:{os.environ.get('PATH', '')}"

    # Get takumi version for metadata
    try:
        r = subprocess.run(
            f"{TAKUMI_BIN} version", shell=True,
            capture_output=True, text=True,
        )
        takumi_version = r.stdout.strip() or "unknown"
    except Exception:
        takumi_version = "unknown"

    print()
    print(f"  Setting up project ...", end=" ", flush=True)

    workdir = tempfile.mkdtemp(prefix="takumi-iterate-")
    try:
        takumi_md = setup_project(workdir)
        print("done")

        # System prompt: simulates auto-loaded TAKUMI.md (as Claude Code does)
        system = (
            "You are a software engineer who just cloned a new project. "
            "The project uses Takumi for build orchestration. "
            "Use the provided tools to complete the task. "
            "Call task_complete with a summary when done.\n\n"
            + takumi_md
        )

        task = (
            "I just cloned this project and need to verify it works. "
            "Understand the project structure, build all packages, run all tests, "
            "and tell me if the project is healthy. Call task_complete when done."
        )

        print(f"  Running agent ({MODEL}) ...", end=" ", flush=True)
        metrics = run_agent(task, workdir, system, MODEL, env_path)

        print(
            f"{metrics.total_tokens:,} tok, "
            f"{metrics.turns} turns, "
            f"{metrics.tool_calls} calls, "
            f"{metrics.wall_time_s:.1f}s"
        )

        # Verify
        verification = verify_transcript(metrics)

        # Build run record
        run_id = str(uuid.uuid4())[:8]
        ts = time.strftime("%Y-%m-%d %H:%M:%S")

        record = {
            "run_id": run_id,
            "timestamp": ts,
            "model": MODEL,
            "takumi_version": takumi_version,
            "note": args.note,
            "tokens": {
                "input": metrics.input_tokens,
                "output": metrics.output_tokens,
                "total": metrics.total_tokens,
            },
            "turns": metrics.turns,
            "tool_calls": metrics.tool_calls,
            "errors": metrics.errors,
            "wall_time_s": round(metrics.wall_time_s, 1),
            "task_completed": metrics.task_completed,
            "verification": verification,
        }

        # Write transcript log
        log_path = write_transcript_log(metrics, run_id)

        # Append to history
        hist = load_history()
        hist.append(record)
        save_history(hist)

        # Write latest results for Go CLI
        results_path = Path(__file__).resolve().parent / "results.json"
        results_path.write_text(json.dumps({
            "current": record,
            "history": hist,
        }, indent=2) + "\n")

        # Dashboard
        print_dashboard(record, hist)

        print(f"  Log:     {log_path}")
        print(f"  Results: {results_path}")
        print(f"  History: {history_path()} ({len(hist)} runs)")
        print()

    finally:
        shutil.rmtree(workdir, ignore_errors=True)


if __name__ == "__main__":
    main()
