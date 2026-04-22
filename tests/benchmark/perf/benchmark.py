#!/usr/bin/env python3
"""
Takumi Performance Benchmark — Agent Token & Turn Comparison

Runs identical tasks with and without Takumi operator instructions, measuring:
  - Token usage (input + output)
  - Tool call count and turn count
  - Error count (failed commands)
  - Wall-clock time
  - Task completion and verification

Each scenario sets up a real workspace, gives the agent a task, and lets it
work via tool calls. In "with Takumi" mode, the workspace has takumi.yaml and
the agent gets operator instructions. In "without" mode, raw project only.

Usage:
  ANTHROPIC_API_KEY=sk-... TAKUMI_BIN=../../build/takumi python3 benchmark.py
  ANTHROPIC_API_KEY=sk-... TAKUMI_BIN=../../build/takumi python3 benchmark.py fix-build-error
  BENCH_MODEL=claude-sonnet-4-5-20241022 python3 benchmark.py

Or from repo root:
  make benchmark-perf
"""

import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
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
MODEL = os.environ.get("BENCH_MODEL", "claude-sonnet-4-6")
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
    success: bool = False
    transcript: list = field(default_factory=list)

    @property
    def total_tokens(self):
        return self.input_tokens + self.output_tokens

# ---------------------------------------------------------------------------
# Tools — available to the agent in both modes
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
# Operator prompt (embedded from operator.yaml)
# ---------------------------------------------------------------------------

OPERATOR_PROMPT = """\
You are working in a Takumi workspace — an AI-aware, language-agnostic package builder.

## Commands (use these instead of raw shell commands)
- takumi status    — workspace dashboard (run first in a new session)
- takumi build     — build packages in dependency order (not go build, npm run build, etc.)
- takumi test      — test packages in dependency order (not pytest, go test, etc.)
- takumi PHASE     — any phase is a top-level command (deploy, lint, dev, etc.)
- takumi affected  — list packages affected by changes (scope before building)
- takumi graph     — dependency DAG with topological levels
- takumi validate  — check configs for errors and cycles
- takumi env setup — install deps & set up isolated runtime environments
- takumi review     — AI-powered code review of workspace changes

## Workflow
1. takumi status — understand the workspace
2. takumi affected — scope what changed
3. takumi build --affected — build only what changed
4. takumi test --affected — test only what changed
5. On failure: read .takumi/logs/ — fix — repeat from 3

## When NOT to use raw commands
- See go.mod / package.json? Use takumi build, not language tools directly
- Build failed? Read .takumi/logs/<pkg>.<phase>.log for details
- Need project structure? Use takumi graph, not grep for imports"""

# ---------------------------------------------------------------------------
# Scenario setup functions
# ---------------------------------------------------------------------------

def _git_init(workdir):
    subprocess.run(
        "git init -q && git add -A && git commit -m init -q",
        shell=True, cwd=workdir, capture_output=True, env=GIT_ENV,
    )

def _git_commit(workdir, msg="update"):
    subprocess.run(
        f"git add -A && git commit -m '{msg}' -q",
        shell=True, cwd=workdir, capture_output=True, env=GIT_ENV,
    )

def _takumi_init(workdir):
    subprocess.run(
        f"{TAKUMI_BIN} init --agent none",
        shell=True, cwd=workdir, capture_output=True,
    )


def setup_fix_build_error(workdir, with_takumi):
    """
    Scenario: Go monorepo with a type error in the api package.
    Agent must find and fix the bug, then verify the build passes.
    """
    # shared lib (compiles fine)
    os.makedirs(f"{workdir}/shared")
    Path(f"{workdir}/shared/go.mod").write_text("module example.com/shared\n\ngo 1.22\n")
    Path(f"{workdir}/shared/lib.go").write_text(
        'package shared\n\nfunc Greet(name string) string { return "Hello, " + name }\n'
    )

    # api service — broken: WriteHeader expects int, not string
    os.makedirs(f"{workdir}/api")
    Path(f"{workdir}/api/go.mod").write_text("module example.com/api\n\ngo 1.22\n")
    Path(f"{workdir}/api/main.go").write_text("""\
package main

import (
\t"fmt"
\t"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
\tw.WriteHeader("200")
\tfmt.Fprintf(w, "ok")
}

func main() {
\thttp.HandleFunc("/", handler)
\tfmt.Println("listening on :8080")
\thttp.ListenAndServe(":8080", nil)
}
""")

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - build/\n"
        )
        Path(f"{workdir}/shared/takumi-pkg.yaml").write_text("""\
package:
  name: shared
  version: 0.1.0
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
  version: 0.1.0
dependencies:
  - shared
phases:
  build:
    commands:
      - go build ./...
  test:
    commands:
      - go test ./...
""")
        os.makedirs(f"{workdir}/build", exist_ok=True)
        _git_commit(workdir, "add takumi")

    task = (
        "The build is broken in this project. Find the error, fix it, "
        "and verify the build passes. Call task_complete when done."
    )

    def verify(wd):
        r = subprocess.run(
            "go build ./...", shell=True, cwd=f"{wd}/api",
            capture_output=True, text=True,
        )
        return r.returncode == 0

    return {"task": task, "verify": verify}


def setup_scoped_rebuild(workdir, with_takumi):
    """
    Scenario: 3-package Go monorepo. shared lib was just modified.
    Agent must figure out what's affected and build only those packages.
    """
    for pkg in ("shared", "api", "web"):
        os.makedirs(f"{workdir}/{pkg}")
        Path(f"{workdir}/{pkg}/go.mod").write_text(
            f"module example.com/{pkg}\n\ngo 1.22\n"
        )

    Path(f"{workdir}/shared/lib.go").write_text(
        'package shared\n\nfunc Version() string { return "1.0" }\n'
    )
    Path(f"{workdir}/api/main.go").write_text(
        'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("api") }\n'
    )
    Path(f"{workdir}/web/main.go").write_text(
        'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("web") }\n'
    )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - build/\n"
        )
        for pkg, deps in [("shared", []), ("api", ["shared"]), ("web", ["shared"])]:
            dep_yaml = ""
            if deps:
                dep_yaml = "dependencies:\n" + "".join(f"  - {d}\n" for d in deps)
            Path(f"{workdir}/{pkg}/takumi-pkg.yaml").write_text(
                f"package:\n  name: {pkg}\n  version: 0.1.0\n{dep_yaml}"
                f"phases:\n  build:\n    commands:\n      - go build ./...\n"
                f"  test:\n    commands:\n      - go test ./...\n"
            )
        os.makedirs(f"{workdir}/build", exist_ok=True)
        _git_commit(workdir, "add takumi")

    # Now make a change to shared (after the committed baseline)
    Path(f"{workdir}/shared/lib.go").write_text(
        'package shared\n\nfunc Version() string { return "2.0" }\n'
    )

    task = (
        "I just changed the shared library. Figure out which packages are "
        "affected by this change and build only those — don't rebuild anything "
        "that hasn't changed. Call task_complete when done."
    )

    def verify(wd):
        # All packages should still compile
        for pkg in ("shared", "api", "web"):
            r = subprocess.run(
                "go build ./...", shell=True, cwd=f"{wd}/{pkg}",
                capture_output=True, text=True,
            )
            if r.returncode != 0:
                return False
        return True

    return {"task": task, "verify": verify}


def setup_understand_structure(workdir, with_takumi):
    """
    Scenario: 4-package monorepo with a diamond dependency.
    Agent must explain the dependency structure and build order.
    """
    for pkg in ("core", "auth", "api", "gateway"):
        os.makedirs(f"{workdir}/{pkg}")
        Path(f"{workdir}/{pkg}/go.mod").write_text(
            f"module example.com/{pkg}\n\ngo 1.22\n"
        )
        Path(f"{workdir}/{pkg}/main.go").write_text(
            f'package main\n\nimport "fmt"\n\nfunc main() {{ fmt.Println("{pkg}") }}\n'
        )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: platform\n"
        )
        # Diamond: core <- auth, core <- api, auth <- gateway, api <- gateway
        configs = {
            "core": [],
            "auth": ["core"],
            "api": ["core"],
            "gateway": ["auth", "api"],
        }
        for pkg, deps in configs.items():
            dep_yaml = ""
            if deps:
                dep_yaml = "dependencies:\n" + "".join(f"  - {d}\n" for d in deps)
            Path(f"{workdir}/{pkg}/takumi-pkg.yaml").write_text(
                f"package:\n  name: {pkg}\n  version: 0.1.0\n{dep_yaml}"
                f"phases:\n  build:\n    commands:\n      - go build ./...\n"
            )
        _git_commit(workdir, "add takumi")

    task = (
        "I'm new to this project. Explain the dependency structure: "
        "which packages depend on which, and what order should they be "
        "built in? Call task_complete when done."
    )

    def verify(_wd):
        return True  # Success if the agent completes and explains

    return {"task": task, "verify": verify}


# ---------------------------------------------------------------------------
# Scenario registry
# ---------------------------------------------------------------------------

SCENARIOS = {
    "fix-build-error": {
        "name": "Fix Build Error",
        "desc": "Find and fix a type error in a Go HTTP handler",
        "setup": setup_fix_build_error,
    },
    "scoped-rebuild": {
        "name": "Scoped Rebuild",
        "desc": "After changing shared lib, build only affected packages",
        "setup": setup_scoped_rebuild,
    },
    "understand-structure": {
        "name": "Understand Structure",
        "desc": "Explain dependency graph and build order of a 4-package monorepo",
        "setup": setup_understand_structure,
    },
}

# ---------------------------------------------------------------------------
# Agent runner
# ---------------------------------------------------------------------------

def run_agent(task, workdir, with_takumi, model):
    """Drive a multi-turn agent loop and collect metrics."""
    client = anthropic.Anthropic()
    metrics = Metrics()

    system = (
        "You are a software engineer working in a project directory. "
        "Use the provided tools to complete the task efficiently. "
        "Call task_complete when you are done."
    )
    if with_takumi:
        system += "\n\n" + OPERATOR_PROMPT

    # Build PATH — only include takumi bin dir in "with" mode
    base_path = os.environ.get("PATH", "")
    if with_takumi and TAKUMI_BIN:
        takumi_dir = os.path.dirname(os.path.abspath(TAKUMI_BIN))
        env_path = f"{takumi_dir}:{base_path}"
    else:
        env_path = base_path

    messages = [{"role": "user", "content": task}]
    start = time.time()

    for _ in range(MAX_TURNS):
        resp = client.messages.create(
            model=model,
            max_tokens=4096,
            system=system,
            tools=TOOLS,
            messages=messages,
        )

        metrics.input_tokens += resp.usage.input_tokens
        metrics.output_tokens += resp.usage.output_tokens
        metrics.turns += 1

        # Record assistant text
        for block in resp.content:
            if hasattr(block, "text"):
                metrics.transcript.append({"role": "assistant", "text": block.text})

        if resp.stop_reason == "end_turn":
            break

        # Process tool calls
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

            entry["output"] = output[:500]  # truncate for transcript
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
# Reporting
# ---------------------------------------------------------------------------

W = 72  # dashboard width


def _bar(value, max_value, width=20, fill="█", empty="░"):
    """Render a proportional bar."""
    if max_value <= 0:
        return empty * width
    n = int(round(value / max_value * width))
    return fill * n + empty * (width - n)


def _pct_str(without_val, with_val):
    """Return a savings percentage string like '+87.3%' or '-4.1%'."""
    if without_val == 0:
        return "  n/a"
    pct = (without_val - with_val) / without_val * 100
    return f"{pct:+.1f}%"


def _status(completed, verified):
    if completed and verified:
        return "PASS"
    if completed:
        return "DONE (unverified)"
    return "INCOMPLETE"


def write_transcript_log(metrics_dict, filepath):
    """Write a human-readable transcript log to a file."""
    transcript = metrics_dict.get("transcript", [])
    with open(filepath, "w") as f:
        f.write(f"# Tokens: {metrics_dict['input_tokens'] + metrics_dict['output_tokens']:,d} "
                f"(in: {metrics_dict['input_tokens']:,d}, out: {metrics_dict['output_tokens']:,d})\n")
        f.write(f"# Turns: {metrics_dict['turns']}, "
                f"Tool calls: {metrics_dict['tool_calls']}, "
                f"Errors: {metrics_dict['errors']}\n")
        f.write(f"# Time: {metrics_dict['wall_time_s']:.1f}s\n")
        f.write(f"# Completed: {metrics_dict['task_completed']}, "
                f"Verified: {metrics_dict['success']}\n")
        f.write("\n")

        step = 0
        for entry in transcript:
            if "tool" in entry:
                step += 1
                tool = entry["tool"]
                inp = entry.get("input", {})
                is_err = entry.get("is_error", False)
                output = entry.get("output", "")

                if tool == "run_command":
                    f.write(f"--- [{step}] run_command {'[ERROR]' if is_err else ''}\n")
                    f.write(f"$ {inp.get('command', '')}\n")
                    if output:
                        f.write(f"\n{output}\n")
                elif tool == "read_file":
                    f.write(f"--- [{step}] read_file {inp.get('path', '')} {'[ERROR]' if is_err else ''}\n")
                    if output:
                        f.write(f"\n{output}\n")
                elif tool == "write_file":
                    f.write(f"--- [{step}] write_file {inp.get('path', '')} {'[ERROR]' if is_err else ''}\n")
                    content = inp.get("content", "")
                    if content:
                        f.write(f"\n{content}\n")
                elif tool == "list_files":
                    f.write(f"--- [{step}] list_files {inp.get('path', '.')} {'[ERROR]' if is_err else ''}\n")
                    if output:
                        f.write(f"\n{output}\n")
                elif tool == "task_complete":
                    f.write(f"--- [{step}] task_complete\n")
                    summary = inp.get("summary", "")
                    if summary:
                        f.write(f"\n{summary}\n")
                else:
                    f.write(f"--- [{step}] {tool} {'[ERROR]' if is_err else ''}\n")
                    f.write(f"{json.dumps(inp, indent=2)}\n")

                f.write("\n")

            elif "text" in entry:
                text = entry["text"].strip()
                if text:
                    f.write(f"ASSISTANT: {text}\n\n")


def print_scenario_dashboard(scenario_name, desc, without, with_t):
    """Print a full dashboard for a single scenario."""
    def tok(m): return m["input_tokens"] + m["output_tokens"]

    w_tok = tok(without)
    t_tok = tok(with_t)
    max_tok = max(w_tok, t_tok, 1)

    w_time = without["wall_time_s"]
    t_time = with_t["wall_time_s"]
    max_time = max(w_time, t_time, 0.1)

    w_calls = without["tool_calls"]
    t_calls = with_t["tool_calls"]
    max_calls = max(w_calls, t_calls, 1)

    # Header
    print(f"  ┌{'─' * (W - 4)}┐")
    print(f"  │ {scenario_name:<{W - 6}} │")
    print(f"  │ {desc:<{W - 6}} │")
    print(f"  ├{'─' * (W - 4)}┤")

    # Token usage
    print(f"  │ {'TOKENS':<{W - 6}} │")
    print(f"  │   Without:    {w_tok:>8,d}  {_bar(w_tok, max_tok):<20s}       │")
    print(f"  │   With Takumi:{t_tok:>8,d}  {_bar(t_tok, max_tok):<20s}       │")
    print(f"  │   {'Savings:':<14s}{_pct_str(w_tok, t_tok):>7s}  ({w_tok - t_tok:+,d} tokens){' ' * max(0, 17 - len(f'{w_tok - t_tok:+,d}'))}│")
    print(f"  │   {'  Input:':<14s}{without['input_tokens']:>7,d} -> {with_t['input_tokens']:>7,d}{'':>{W - 39}}│")
    print(f"  │   {'  Output:':<14s}{without['output_tokens']:>7,d} -> {with_t['output_tokens']:>7,d}{'':>{W - 39}}│")
    print(f"  │{'':>{W - 4}}│")

    # Time
    print(f"  │ {'TIME':<{W - 6}} │")
    print(f"  │   Without:    {w_time:>7.1f}s  {_bar(w_time, max_time):<20s}       │")
    print(f"  │   With Takumi:{t_time:>7.1f}s  {_bar(t_time, max_time):<20s}       │")
    print(f"  │   {'Savings:':<14s}{_pct_str(w_time, t_time):>7s}  ({w_time - t_time:+.1f}s){' ' * max(0, 24 - len(f'{w_time - t_time:+.1f}'))}│")
    print(f"  │{'':>{W - 4}}│")

    # Efficiency
    print(f"  │ {'EFFICIENCY':<{W - 6}} │")
    print(f"  │   {'':14s} {'Without':>8s}  {'Takumi':>8s}  {'Saved':>7s}       │")
    print(f"  │   {'Turns':14s} {without['turns']:>8d}  {with_t['turns']:>8d}  {without['turns'] - with_t['turns']:>+7d}       │")
    print(f"  │   {'Tool calls':14s} {w_calls:>8d}  {t_calls:>8d}  {w_calls - t_calls:>+7d}       │")
    print(f"  │   {'Errors':14s} {without['errors']:>8d}  {with_t['errors']:>8d}  {without['errors'] - with_t['errors']:>+7d}       │")
    print(f"  │   {'Tok/call':14s} {w_tok // max(w_calls, 1):>8,d}  {t_tok // max(t_calls, 1):>8,d}{'':>16s}│")
    print(f"  │{'':>{W - 4}}│")

    # Status
    w_status = _status(without["task_completed"], without["success"])
    t_status = _status(with_t["task_completed"], with_t["success"])
    print(f"  │ {'STATUS':<{W - 6}} │")
    print(f"  │   Without:     {w_status:<{W - 21}}│")
    print(f"  │   With Takumi: {t_status:<{W - 21}}│")
    print(f"  └{'─' * (W - 4)}┘")
    print()


def print_tool_log(metrics_dict, label):
    """Print the tool calls from a run's transcript."""
    transcript = metrics_dict.get("transcript", [])
    tools = [e for e in transcript if "tool" in e]
    if not tools:
        return
    print(f"  {label} ({len(tools)} calls):")
    for i, t in enumerate(tools, 1):
        cmd = ""
        if t["tool"] == "run_command":
            raw = t["input"].get("command", "")
            # Truncate long commands (bash scripts)
            if len(raw) > 80:
                raw = raw[:77] + "..."
            cmd = f" $ {raw}"
        elif t["tool"] == "read_file":
            cmd = f' {t["input"].get("path", "")}'
        elif t["tool"] == "write_file":
            cmd = f' {t["input"].get("path", "")}'
        elif t["tool"] == "list_files":
            cmd = f' {t["input"].get("path", ".")}'
        err = " [ERROR]" if t.get("is_error") else ""
        print(f"    {i:>2d}. {t['tool']}{cmd}{err}")
    print()


def print_summary_dashboard(all_results, scenarios):
    """Print an overall summary dashboard across all scenarios."""
    print(f"  ┌{'─' * (W - 4)}┐")
    print(f"  │ {'OVERALL RESULTS':^{W - 6}} │")
    print(f"  ├{'─' * (W - 4)}┤")

    # Per-scenario summary row
    print(f"  │ {'Scenario':<24s} {'Tokens':>14s} {'Time':>10s} {'Calls':>8s} │")
    print(f"  │ {'':─<24s} {'':─>14s} {'':─>10s} {'':─>8s} │")

    tot_w_tok = tot_t_tok = 0
    tot_w_time = tot_t_time = 0.0
    tot_w_calls = tot_t_calls = 0
    tot_w_turns = tot_t_turns = 0
    tot_w_errors = tot_t_errors = 0
    n_scenarios = 0

    for sid in scenarios:
        r = all_results.get(sid, {})
        w = r.get("without_takumi", {})
        t = r.get("with_takumi", {})
        if "error" in w or "error" in t:
            name = SCENARIOS[sid]["name"]
            print(f"  │ {name:<24s} {'ERROR':>14s} {'':>10s} {'':>8s} │")
            continue

        n_scenarios += 1
        name = SCENARIOS[sid]["name"]
        w_tok = w["input_tokens"] + w["output_tokens"]
        t_tok = t["input_tokens"] + t["output_tokens"]

        tot_w_tok += w_tok
        tot_t_tok += t_tok
        tot_w_time += w["wall_time_s"]
        tot_t_time += t["wall_time_s"]
        tot_w_calls += w["tool_calls"]
        tot_t_calls += t["tool_calls"]
        tot_w_turns += w["turns"]
        tot_t_turns += t["turns"]
        tot_w_errors += w["errors"]
        tot_t_errors += t["errors"]

        tok_s = _pct_str(w_tok, t_tok)
        time_s = _pct_str(w["wall_time_s"], t["wall_time_s"])
        calls_s = f"{w['tool_calls']} -> {t['tool_calls']}"

        print(f"  │ {name:<24s} {tok_s:>14s} {time_s:>10s} {calls_s:>8s} │")

    print(f"  ├{'─' * (W - 4)}┤")

    # Totals
    if n_scenarios > 0 and tot_w_tok > 0:
        tok_saved = tot_w_tok - tot_t_tok
        tok_pct = tok_saved / tot_w_tok * 100

        time_saved = tot_w_time - tot_t_time
        time_pct = (time_saved / tot_w_time * 100) if tot_w_time > 0 else 0

        print(f"  │ {'TOTALS':^{W - 6}} │")
        print(f"  │{'':>{W - 4}}│")
        print(f"  │   Tokens:  {tot_w_tok:>8,d}  ->  {tot_t_tok:>8,d}  saved {tok_saved:>7,d} ({tok_pct:+.1f}%) │")
        print(f"  │   Time:    {tot_w_time:>7.1f}s  ->  {tot_t_time:>7.1f}s  saved {time_saved:>6.1f}s ({time_pct:+.1f}%) │")
        print(f"  │   Turns:   {tot_w_turns:>8d}  ->  {tot_t_turns:>8d}  saved {tot_w_turns - tot_t_turns:>7d}         │")
        print(f"  │   Calls:   {tot_w_calls:>8d}  ->  {tot_t_calls:>8d}  saved {tot_w_calls - tot_t_calls:>7d}         │")
        print(f"  │   Errors:  {tot_w_errors:>8d}  ->  {tot_t_errors:>8d}  saved {tot_w_errors - tot_t_errors:>7d}         │")
        print(f"  │{'':>{W - 4}}│")

        # Headline metrics
        print(f"  ├{'─' * (W - 4)}┤")
        tok_bar = _bar(tot_t_tok, tot_w_tok, width=30)
        print(f"  │ {'TOKEN REDUCTION':^{W - 6}} │")
        print(f"  │   {tok_bar}  {tok_pct:+.1f}%{' ' * max(0, W - 44)}│")
        print(f"  │   {tot_w_tok:,d} -> {tot_t_tok:,d}{' ' * max(0, W - 6 - len(f'{tot_w_tok:,d} -> {tot_t_tok:,d}') - 2)}│")
        print(f"  │{'':>{W - 4}}│")

        time_bar = _bar(tot_t_time, tot_w_time, width=30)
        print(f"  │ {'TIME REDUCTION':^{W - 6}} │")
        print(f"  │   {time_bar}  {time_pct:+.1f}%{' ' * max(0, W - 44)}│")
        print(f"  │   {tot_w_time:.1f}s -> {tot_t_time:.1f}s{' ' * max(0, W - 6 - len(f'{tot_w_time:.1f}s -> {tot_t_time:.1f}s') - 2)}│")

    print(f"  └{'─' * (W - 4)}┘")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    # Resolve TAKUMI_BIN to absolute path
    global TAKUMI_BIN
    if TAKUMI_BIN:
        TAKUMI_BIN = str(Path(TAKUMI_BIN).resolve())
    else:
        # Try default location
        default = Path(__file__).resolve().parent.parent.parent.parent / "build" / "takumi"
        if default.exists():
            TAKUMI_BIN = str(default)
        else:
            print("Error: TAKUMI_BIN not set and build/takumi not found", file=sys.stderr)
            sys.exit(1)

    if not Path(TAKUMI_BIN).exists():
        print(f"Error: TAKUMI_BIN not found: {TAKUMI_BIN}", file=sys.stderr)
        sys.exit(1)

    # Parse optional scenario filter
    selected = sys.argv[1:] or list(SCENARIOS.keys())
    for s in selected:
        if s not in SCENARIOS:
            print(f"Unknown scenario: {s}. Available: {', '.join(SCENARIOS.keys())}", file=sys.stderr)
            sys.exit(1)

    print()
    print(f"  ┌{'─' * (W - 4)}┐")
    print(f"  │ {'TAKUMI PERFORMANCE BENCHMARK':^{W - 6}} │")
    print(f"  ├{'─' * (W - 4)}┤")
    print(f"  │   Model:      {MODEL:<{W - 20}}│")
    print(f"  │   Max turns:  {MAX_TURNS:<{W - 20}}│")
    print(f"  │   Scenarios:  {len(selected):<{W - 20}}│")
    ts = time.strftime("%Y-%m-%d %H:%M:%S")
    print(f"  │   Timestamp:  {ts:<{W - 20}}│")
    print(f"  └{'─' * (W - 4)}┘")
    print()

    all_results = {}

    for scenario_id in selected:
        scenario = SCENARIOS[scenario_id]
        results = {}

        for mode_label, with_takumi in [("without_takumi", False), ("with_takumi", True)]:
            label = "WITH Takumi" if with_takumi else "WITHOUT Takumi"
            print(f"  Running {scenario['name']} ({label}) ...", end=" ", flush=True)

            workdir = tempfile.mkdtemp(prefix=f"takumi-perf-{scenario_id}-")
            try:
                config = scenario["setup"](workdir, with_takumi)
                metrics = run_agent(config["task"], workdir, with_takumi, MODEL)
                metrics.success = config["verify"](workdir)

                results[mode_label] = asdict(metrics)
                print(
                    f"{metrics.total_tokens:,} tok, "
                    f"{metrics.turns} turns, "
                    f"{metrics.tool_calls} calls, "
                    f"{metrics.wall_time_s:.1f}s"
                )
            except Exception as e:
                print(f"ERROR: {e}")
                results[mode_label] = {"error": str(e)}
            finally:
                shutil.rmtree(workdir, ignore_errors=True)

        all_results[scenario_id] = results
        print()

    # ── Write transcript logs ───────────────────────────────────────────
    logs_dir = Path(__file__).resolve().parent / "logs"
    logs_dir.mkdir(exist_ok=True)

    log_files = {}
    for scenario_id in selected:
        results = all_results[scenario_id]
        for mode_label in ("without_takumi", "with_takumi"):
            r = results.get(mode_label, {})
            if "error" in r:
                continue
            tag = "without" if "without" in mode_label else "with-takumi"
            log_path = logs_dir / f"{scenario_id}.{tag}.log"
            write_transcript_log(r, str(log_path))
            log_files.setdefault(scenario_id, {})[mode_label] = str(log_path)

    # ── Per-scenario dashboards ──────────────────────────────────────────
    for scenario_id in selected:
        scenario = SCENARIOS[scenario_id]
        results = all_results[scenario_id]

        if (
            "without_takumi" in results
            and "with_takumi" in results
            and "error" not in results["without_takumi"]
            and "error" not in results["with_takumi"]
        ):
            print_scenario_dashboard(
                scenario["name"], scenario["desc"],
                results["without_takumi"], results["with_takumi"],
            )

            # Show log file paths
            lf = log_files.get(scenario_id, {})
            if lf:
                print(f"  Logs:")
                for label, path in [("without", lf.get("without_takumi")),
                                     ("with-takumi", lf.get("with_takumi"))]:
                    if path:
                        print(f"    {label}: {path}")
                print(f"    diff: diff {lf.get('without_takumi', '')} {lf.get('with_takumi', '')}")
                print()

            print_tool_log(results["without_takumi"], "Without Takumi")
            print_tool_log(results["with_takumi"], "With Takumi")

    # ── Overall dashboard ────────────────────────────────────────────────
    print_summary_dashboard(all_results, selected)
    print()

    # Write JSON
    output_file = os.environ.get("BENCH_OUTPUT", str(
        Path(__file__).resolve().parent / "results.json"
    ))
    slim_results = {}
    for sid, results in all_results.items():
        slim_results[sid] = {}
        for mode, data in results.items():
            if isinstance(data, dict) and "transcript" in data:
                d = dict(data)
                d.pop("transcript")
                slim_results[sid][mode] = d
            else:
                slim_results[sid][mode] = data

    # Compute totals for JSON
    totals = {"without": {"tokens": 0, "turns": 0, "calls": 0, "errors": 0},
              "with":    {"tokens": 0, "turns": 0, "calls": 0, "errors": 0}}
    for results in all_results.values():
        for mode, key in [("without", "without_takumi"), ("with", "with_takumi")]:
            r = results.get(key, {})
            if "error" in r:
                continue
            totals[mode]["tokens"] += r.get("input_tokens", 0) + r.get("output_tokens", 0)
            totals[mode]["turns"] += r.get("turns", 0)
            totals[mode]["calls"] += r.get("tool_calls", 0)
            totals[mode]["errors"] += r.get("errors", 0)

    with open(output_file, "w") as f:
        json.dump(
            {
                "model": MODEL,
                "max_turns": MAX_TURNS,
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "scenarios": slim_results,
                "totals": totals,
            },
            f,
            indent=2,
        )
    print(f"  Results: {output_file}")
    print(f"  Logs:    {logs_dir}/")


if __name__ == "__main__":
    main()
