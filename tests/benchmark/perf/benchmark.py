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

TODO: Future improvements
  - fix-build-error Python/Go token regression: operator prompt overhead
    outweighs call savings for simple bugs. Consider trimming prompt for
    fix-build-error scenarios or making the prompt adaptive.
  - Haiku variance on fix-build-error: agent sometimes tries invalid
    commands (e.g. `takumi runtime`) or re-reads files after fix. May
    stabilize with Sonnet/Opus. Run multi-model CI to confirm.
  - scoped-rebuild TS: 13 calls vs 4 for other languages. Agent reads
    and rewrites tsconfig.json files. May need TS-specific build config
    in the scenario or better composite project references.
  - Average across N runs per scenario to reduce variance noise.
  - Add `deploy` and `lint` phase scenarios.
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
    correctness: float = 0.0
    correctness_checks: dict = field(default_factory=dict)
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
- takumi status    — workspace dashboard: packages, deps, phases, envs (run first)
- takumi build     — build packages in dependency order (not go build, npm run build, etc.)
- takumi test      — test packages in dependency order (not pytest, go test, etc.)
- takumi PHASE     — any phase is a top-level command (deploy, lint, dev, etc.)
- takumi affected  — list packages affected by changes (scope before building)
- takumi graph     — full dependency DAG (authoritative — no need to also read configs or source imports)
- takumi graph --phases — same, plus shows build/test commands for each package
- takumi validate  — check configs for errors and cycles
- takumi env setup — install deps & set up isolated runtime environments

## Workflow
1. takumi status — understand the workspace
2. takumi build (or takumi build --affected) — build first, let errors guide you
3. On failure: error output + .takumi/logs/<pkg>.<phase>.log tell you exactly where to look
4. Fix the code, then rebuild

For change-scoped work:
1. takumi affected — what packages changed
2. takumi build --affected — build only those
3. takumi test --affected — test only those

## Key principles
- **Build first, explore later.** Run takumi build before reading source files. Errors pinpoint the problem — you don't need to explore the whole project first.
- **Trust takumi output.** takumi status, graph, and affected give you complete, accurate info about packages, dependencies, and what changed. If these commands answered your question, you're done — don't also read config files or source imports to double-check.
- **Use takumi commands, not raw builds.** go build, npm run build, javac, cargo build — replace all of these with takumi build.
- **Read error paths relative to the package.** Build errors are prefixed with `[pkg]` and show paths relative to that package directory. To read a failing file, prepend the package name: `[api] src/Main.java` means `api/src/Main.java`.
- **A passing rebuild is sufficient.** After fixing a build error, run takumi build. If it passes, you're done — no need to re-read the file you just wrote or re-check status.

## Config files
- `takumi.yaml` — workspace root (one per workspace)
- `takumi-pkg.yaml` — per-package config (NOT `takumi.yaml` in subdirectories)
- You usually don't need to read these — takumi commands already use them."""

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

def _takumi_env_setup(workdir):
    subprocess.run(
        f"{TAKUMI_BIN} env setup",
        shell=True, cwd=workdir, capture_output=True,
    )


def setup_fix_build_error_go(workdir, with_takumi):
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

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. Build passes (0.4)
        r = subprocess.run(
            "go build ./...", shell=True, cwd=f"{wd}/api",
            capture_output=True, text=True,
        )
        checks["build_passes"] = r.returncode == 0
        if checks["build_passes"]:
            score += 0.4

        # 2. Fix is in the right file (0.2)
        try:
            source = Path(f"{wd}/api/main.go").read_text()
            checks["correct_file"] = 'WriteHeader("200")' not in source
        except Exception:
            checks["correct_file"] = False
        if checks["correct_file"]:
            score += 0.2

        # 3. Fix is correct — WriteHeader takes an int (0.2)
        checks["correct_fix"] = False
        if checks["correct_file"]:
            # Accept WriteHeader(200) or WriteHeader(http.StatusOK)
            if "WriteHeader(200)" in source or "WriteHeader(http.StatusOK)" in source:
                checks["correct_fix"] = True
                score += 0.2

        # 4. Shared package still compiles (0.2)
        r2 = subprocess.run(
            "go build ./...", shell=True, cwd=f"{wd}/shared",
            capture_output=True, text=True,
        )
        checks["no_collateral"] = r2.returncode == 0
        if checks["no_collateral"]:
            score += 0.2

        return {"passed": checks["build_passes"] and checks["correct_fix"], "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_scoped_rebuild_go(workdir, with_takumi):
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

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. All packages still compile (0.3)
        all_compile = True
        for pkg in ("shared", "api", "web"):
            r = subprocess.run(
                "go build ./...", shell=True, cwd=f"{wd}/{pkg}",
                capture_output=True, text=True,
            )
            if r.returncode != 0:
                all_compile = False
        checks["all_compile"] = all_compile
        if all_compile:
            score += 0.3

        # 2. Agent identified affected packages (0.3)
        # Look through transcript for evidence the agent recognized dependencies
        transcript_text = " ".join(
            e.get("output", "") + " " + e.get("text", "") + " " +
            e.get("input", {}).get("summary", "") + " " +
            e.get("input", {}).get("command", "")
            for e in metrics.transcript
        ).lower()
        # Agent should mention that api and web depend on/are affected by shared
        dep_words = ("affected", "depend", "←", "<-", "downstream", "consumer")
        found_api_affected = ("api" in transcript_text and
                              any(w in transcript_text for w in dep_words))
        found_web_affected = ("web" in transcript_text and
                              any(w in transcript_text for w in dep_words))
        checks["identified_affected"] = found_api_affected and found_web_affected
        if checks["identified_affected"]:
            score += 0.3

        # 3. Agent actually scoped (didn't just build everything blindly) (0.4)
        # Check tool calls: did the agent use targeted builds or affected?
        build_commands = []
        for e in metrics.transcript:
            if e.get("tool") == "run_command":
                cmd = e.get("input", {}).get("command", "")
                if "build" in cmd:
                    build_commands.append(cmd)
        # Signs of scoping: used takumi affected, takumi build --affected,
        # or built specific packages rather than a blanket "go build" at root
        used_affected = any("affected" in cmd for cmd in build_commands)
        used_targeted = any(
            pkg in cmd for cmd in build_commands
            for pkg in ("shared", "api", "web", "./shared", "./api", "./web")
        )
        blanket_root_build = any(
            cmd.strip() in ("go build ./...", "go build .")
            and "cd" not in cmd and "shared" not in cmd and "api" not in cmd and "web" not in cmd
            for cmd in build_commands
        )
        checks["scoped_build"] = used_affected or (used_targeted and not blanket_root_build)
        if checks["scoped_build"]:
            score += 0.4

        passed = all_compile and checks["identified_affected"]
        return {"passed": passed, "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_understand_structure_go(workdir, with_takumi):
    """
    Scenario: 4-package Go monorepo with a diamond dependency.
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
        "I'm new to this Go project. Explain the dependency structure: "
        "which packages depend on which, and what order should they be "
        "built in? Call task_complete when done."
    )

    def verify(_wd, metrics):
        return _verify_understand_structure(metrics)

    return {"task": task, "verify": verify}


# ---------------------------------------------------------------------------
# Python scenario setup functions
# ---------------------------------------------------------------------------

def setup_fix_build_error_python(workdir, with_takumi):
    """
    Scenario: Python project with a TypeError in the api package.
    Agent must find and fix the bug, then verify it runs.
    """
    os.makedirs(f"{workdir}/shared")
    Path(f"{workdir}/shared/__init__.py").write_text("")
    Path(f"{workdir}/shared/lib.py").write_text(
        'def greet(name):\n    return "Hello, " + name\n'
    )

    os.makedirs(f"{workdir}/api")
    Path(f"{workdir}/api/__init__.py").write_text("")
    # Bug: string + int → TypeError
    Path(f"{workdir}/api/main.py").write_text("""\
import sys
sys.path.insert(0, ".")
from shared.lib import greet

def run():
    msg = greet("World")
    port = 8080
    print("Server on port " + port)
    return msg

if __name__ == "__main__":
    run()
""")

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - __pycache__/\n"
        )
        Path(f"{workdir}/shared/takumi-pkg.yaml").write_text("""\
package:
  name: shared
  version: 0.1.0
runtime:
  setup:
    - python3 -m venv {{env_dir}}
  env:
    PATH: "{{env_dir}}/bin:$PATH"
    VIRTUAL_ENV: "{{env_dir}}"
    PYTHONPATH: ".."
phases:
  build:
    commands:
      - python -m py_compile lib.py
  test:
    commands:
      - python -c "from shared.lib import greet; assert greet('X') == 'Hello, X'"
""")
        Path(f"{workdir}/api/takumi-pkg.yaml").write_text("""\
package:
  name: api
  version: 0.1.0
dependencies:
  - shared
runtime:
  setup:
    - python3 -m venv {{env_dir}}
  env:
    PATH: "{{env_dir}}/bin:$PATH"
    VIRTUAL_ENV: "{{env_dir}}"
    PYTHONPATH: ".."
phases:
  build:
    commands:
      - python main.py
  test:
    commands:
      - python main.py
""")
        os.makedirs(f"{workdir}/__pycache__", exist_ok=True)
        _git_commit(workdir, "add takumi")
        _takumi_env_setup(workdir)

    task = (
        "The build is broken in this Python project. Running the api package "
        "crashes with an error. Find the bug, fix it, and verify it runs. "
        "Call task_complete when done."
    )

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. api/main.py runs without error (0.4)
        r = subprocess.run(
            "python3 api/main.py", shell=True, cwd=wd,
            capture_output=True, text=True,
        )
        checks["runs_ok"] = r.returncode == 0
        if checks["runs_ok"]:
            score += 0.4

        # 2. Fix is in the right file (0.2)
        try:
            source = Path(f"{wd}/api/main.py").read_text()
            checks["correct_file"] = '"Server on port " + port' not in source
        except Exception:
            checks["correct_file"] = False
        if checks["correct_file"]:
            score += 0.2

        # 3. Fix is correct — str(port), f-string, or str concat (0.2)
        checks["correct_fix"] = False
        if checks["correct_file"]:
            if any(p in source for p in (
                "str(port)", 'f"Server', "f'Server",
                '"Server on port " + str(', "Server on port {",
            )):
                checks["correct_fix"] = True
                score += 0.2

        # 4. shared still works (0.2)
        r2 = subprocess.run(
            'python3 -c "from shared.lib import greet; assert greet(\'X\') == \'Hello, X\'"',
            shell=True, cwd=wd, capture_output=True, text=True,
        )
        checks["no_collateral"] = r2.returncode == 0
        if checks["no_collateral"]:
            score += 0.2

        return {"passed": checks["runs_ok"] and checks.get("correct_fix", False), "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_scoped_rebuild_python(workdir, with_takumi):
    """
    Scenario: 3-package Python project. shared lib was just modified.
    Agent must figure out what's affected and rebuild only those.
    """
    for pkg in ("shared", "api", "web"):
        os.makedirs(f"{workdir}/{pkg}")
        Path(f"{workdir}/{pkg}/__init__.py").write_text("")

    Path(f"{workdir}/shared/lib.py").write_text(
        'VERSION = "1.0"\n\ndef get_version():\n    return VERSION\n'
    )
    Path(f"{workdir}/api/main.py").write_text(
        'import sys\nsys.path.insert(0, ".")\n'
        'from shared.lib import get_version\n\n'
        'def run():\n    print(f"api {get_version()}")\n\n'
        'if __name__ == "__main__":\n    run()\n'
    )
    Path(f"{workdir}/web/main.py").write_text(
        'import sys\nsys.path.insert(0, ".")\n'
        'from shared.lib import get_version\n\n'
        'def run():\n    print(f"web {get_version()}")\n\n'
        'if __name__ == "__main__":\n    run()\n'
    )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - __pycache__/\n"
        )
        _RUNTIME_PY = (
            "runtime:\n"
            "  setup:\n"
            "    - python3 -m venv {{env_dir}}\n"
            "  env:\n"
            '    PATH: "{{env_dir}}/bin:$PATH"\n'
            '    VIRTUAL_ENV: "{{env_dir}}"\n'
            '    PYTHONPATH: ".."\n'
        )
        for pkg, deps in [("shared", []), ("api", ["shared"]), ("web", ["shared"])]:
            dep_yaml = ""
            if deps:
                dep_yaml = "dependencies:\n" + "".join(f"  - {d}\n" for d in deps)
            if deps:
                Path(f"{workdir}/{pkg}/takumi-pkg.yaml").write_text(
                    f"package:\n  name: {pkg}\n  version: 0.1.0\n{dep_yaml}"
                    f"{_RUNTIME_PY}"
                    f"phases:\n  build:\n    commands:\n      - python main.py\n"
                    f"  test:\n    commands:\n      - python main.py\n"
                )
            else:
                Path(f"{workdir}/{pkg}/takumi-pkg.yaml").write_text(
                    f"package:\n  name: {pkg}\n  version: 0.1.0\n"
                    f"{_RUNTIME_PY}"
                    f"phases:\n  build:\n    commands:\n      - python -m py_compile lib.py\n"
                    f"  test:\n    commands:\n"
                    f'      - python -c "from {pkg}.lib import get_version; assert get_version()"\n'
                )
        _git_commit(workdir, "add takumi")
        _takumi_env_setup(workdir)

    # Make a change to shared after baseline
    Path(f"{workdir}/shared/lib.py").write_text(
        'VERSION = "2.0"\n\ndef get_version():\n    return VERSION\n'
    )

    task = (
        "I just changed the shared library in this Python project. Figure out "
        "which packages are affected by this change and build/test only those — "
        "don't rebuild anything that hasn't changed. Call task_complete when done."
    )

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. All packages still work (0.3)
        all_ok = True
        for pkg in ("api", "web"):
            r = subprocess.run(
                f"python3 {pkg}/main.py", shell=True, cwd=wd,
                capture_output=True, text=True,
            )
            if r.returncode != 0:
                all_ok = False
        checks["all_run"] = all_ok
        if all_ok:
            score += 0.3

        # 2. Agent identified affected packages (0.3)
        transcript_text = " ".join(
            e.get("output", "") + " " + e.get("text", "") + " " +
            e.get("input", {}).get("summary", "") + " " +
            e.get("input", {}).get("command", "")
            for e in metrics.transcript
        ).lower()
        dep_words = ("affected", "depend", "import", "←", "<-", "downstream", "consumer")
        found_api = "api" in transcript_text and any(w in transcript_text for w in dep_words)
        found_web = "web" in transcript_text and any(w in transcript_text for w in dep_words)
        checks["identified_affected"] = found_api and found_web
        if checks["identified_affected"]:
            score += 0.3

        # 3. Agent scoped its work (0.4)
        run_commands = []
        for e in metrics.transcript:
            if e.get("tool") == "run_command":
                run_commands.append(e.get("input", {}).get("command", ""))
        used_affected = any("affected" in cmd for cmd in run_commands)
        used_targeted = any(
            pkg in cmd for cmd in run_commands
            for pkg in ("shared", "api", "web")
            if ("python" in cmd or "takumi" in cmd or "build" in cmd or "test" in cmd)
        )
        checks["scoped_build"] = used_affected or used_targeted
        if checks["scoped_build"]:
            score += 0.4

        passed = all_ok and checks["identified_affected"]
        return {"passed": passed, "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_understand_structure_python(workdir, with_takumi):
    """
    Scenario: 4-package Python project with a diamond dependency.
    Agent must explain the dependency structure and build order.
    """
    for pkg in ("core", "auth", "api", "gateway"):
        os.makedirs(f"{workdir}/{pkg}")
        Path(f"{workdir}/{pkg}/__init__.py").write_text("")
        Path(f"{workdir}/{pkg}/main.py").write_text(
            f'def run():\n    print("{pkg}")\n\nif __name__ == "__main__":\n    run()\n'
        )

    # Add imports to show dependency structure
    Path(f"{workdir}/auth/main.py").write_text(
        'import sys\nsys.path.insert(0, ".")\nfrom core.main import run as core_run\n\n'
        'def run():\n    core_run()\n    print("auth")\n\nif __name__ == "__main__":\n    run()\n'
    )
    Path(f"{workdir}/api/main.py").write_text(
        'import sys\nsys.path.insert(0, ".")\nfrom core.main import run as core_run\n\n'
        'def run():\n    core_run()\n    print("api")\n\nif __name__ == "__main__":\n    run()\n'
    )
    Path(f"{workdir}/gateway/main.py").write_text(
        'import sys\nsys.path.insert(0, ".")\n'
        'from auth.main import run as auth_run\nfrom api.main import run as api_run\n\n'
        'def run():\n    auth_run()\n    api_run()\n    print("gateway")\n\n'
        'if __name__ == "__main__":\n    run()\n'
    )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: platform\n"
        )
        _RUNTIME_PY = (
            "runtime:\n"
            "  setup:\n"
            "    - python3 -m venv {{env_dir}}\n"
            "  env:\n"
            '    PATH: "{{env_dir}}/bin:$PATH"\n'
            '    VIRTUAL_ENV: "{{env_dir}}"\n'
            '    PYTHONPATH: ".."\n'
        )
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
                f"{_RUNTIME_PY}"
                f"phases:\n  build:\n    commands:\n      - python main.py\n"
            )
        _git_commit(workdir, "add takumi")
        _takumi_env_setup(workdir)

    task = (
        "I'm new to this Python project. Explain the dependency structure: "
        "which packages depend on which, and what order should they be "
        "built in? Call task_complete when done."
    )

    # Reuse the same verify logic as Go — it's text analysis
    def verify(_wd, metrics):
        return _verify_understand_structure(metrics)

    return {"task": task, "verify": verify}


# ---------------------------------------------------------------------------
# TypeScript scenario setup functions
# ---------------------------------------------------------------------------

def setup_fix_build_error_ts(workdir, with_takumi):
    """
    Scenario: TypeScript project with a type error in the api package.
    Agent must find and fix the bug, then verify tsc passes.
    """
    os.makedirs(f"{workdir}/shared")
    Path(f"{workdir}/shared/index.ts").write_text(
        'export function greet(name: string): string {\n  return "Hello, " + name;\n}\n'
    )
    Path(f"{workdir}/shared/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist","declaration":true},'
        '"include":["*.ts"]}\n'
    )
    Path(f"{workdir}/shared/package.json").write_text(
        '{"name":"shared","version":"0.1.0","main":"dist/index.js"}\n'
    )

    os.makedirs(f"{workdir}/api")
    # Bug: assigning string to number type
    Path(f"{workdir}/api/index.ts").write_text("""\
import { greet } from "../shared";

const port: number = "8080";

console.log(`Server on port ${port}`);
console.log(greet("World"));
""")
    Path(f"{workdir}/api/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist"},'
        '"include":["*.ts"],'
        '"references":[{"path":"../shared"}]}\n'
    )
    Path(f"{workdir}/api/package.json").write_text(
        '{"name":"api","version":"0.1.0","dependencies":{"shared":"*"}}\n'
    )

    # Root tsconfig for project references
    Path(f"{workdir}/tsconfig.json").write_text(
        '{"files":[],"references":[{"path":"shared"},{"path":"api"}]}\n'
    )
    Path(f"{workdir}/package.json").write_text(
        '{"private":true,"devDependencies":{"typescript":"^5.0.0"}}\n'
    )

    # Install typescript locally so tsc is available
    subprocess.run(
        "npm install --silent 2>/dev/null",
        shell=True, cwd=workdir, capture_output=True,
    )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - node_modules/\n    - dist/\n"
        )
        Path(f"{workdir}/shared/takumi-pkg.yaml").write_text("""\
package:
  name: shared
  version: 0.1.0
phases:
  build:
    commands:
      - npx tsc -p tsconfig.json
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
      - npx tsc -p tsconfig.json
""")
        _git_commit(workdir, "add takumi")

    task = (
        "The build is broken in this TypeScript project. Running the TypeScript "
        "compiler gives a type error. Find the bug, fix it, and verify the build "
        "passes. Call task_complete when done."
    )

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. tsc passes (0.4)
        r = subprocess.run(
            "npx tsc -p api/tsconfig.json --noEmit",
            shell=True, cwd=wd, capture_output=True, text=True,
        )
        checks["build_passes"] = r.returncode == 0
        if checks["build_passes"]:
            score += 0.4

        # 2. Fix is in the right file (0.2)
        try:
            source = Path(f"{wd}/api/index.ts").read_text()
            checks["correct_file"] = 'const port: number = "8080"' not in source
        except Exception:
            checks["correct_file"] = False
        if checks["correct_file"]:
            score += 0.2

        # 3. Fix is correct — port should be a number or type should match (0.2)
        checks["correct_fix"] = False
        if checks["correct_file"]:
            if any(p in source for p in (
                "port: number = 8080", "port = 8080",
                'port: string = "8080"', "parseInt(",
                "Number(",
            )):
                checks["correct_fix"] = True
                score += 0.2

        # 4. shared still compiles (0.2)
        r2 = subprocess.run(
            "npx tsc -p shared/tsconfig.json --noEmit",
            shell=True, cwd=wd, capture_output=True, text=True,
        )
        checks["no_collateral"] = r2.returncode == 0
        if checks["no_collateral"]:
            score += 0.2

        return {"passed": checks["build_passes"] and checks.get("correct_fix", False), "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_scoped_rebuild_ts(workdir, with_takumi):
    """
    Scenario: 3-package TypeScript monorepo. shared was just modified.
    Agent must figure out what's affected and build only those.
    """
    for pkg in ("shared", "api", "web"):
        os.makedirs(f"{workdir}/{pkg}")
        Path(f"{workdir}/{pkg}/package.json").write_text(
            f'{{"name":"{pkg}","version":"0.1.0"}}\n'
        )

    Path(f"{workdir}/shared/index.ts").write_text(
        'export const VERSION = "1.0";\n'
    )
    Path(f"{workdir}/shared/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist","declaration":true},'
        '"include":["*.ts"]}\n'
    )

    Path(f"{workdir}/api/index.ts").write_text(
        'import { VERSION } from "../shared";\nconsole.log(`api ${VERSION}`);\n'
    )
    Path(f"{workdir}/api/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist"},'
        '"include":["*.ts"],"references":[{"path":"../shared"}]}\n'
    )

    Path(f"{workdir}/web/index.ts").write_text(
        'import { VERSION } from "../shared";\nconsole.log(`web ${VERSION}`);\n'
    )
    Path(f"{workdir}/web/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist"},'
        '"include":["*.ts"],"references":[{"path":"../shared"}]}\n'
    )

    Path(f"{workdir}/tsconfig.json").write_text(
        '{"files":[],"references":[{"path":"shared"},{"path":"api"},{"path":"web"}]}\n'
    )
    Path(f"{workdir}/package.json").write_text(
        '{"private":true,"devDependencies":{"typescript":"^5.0.0"}}\n'
    )

    subprocess.run(
        "npm install --silent 2>/dev/null",
        shell=True, cwd=workdir, capture_output=True,
    )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - node_modules/\n    - dist/\n"
        )
        for pkg, deps in [("shared", []), ("api", ["shared"]), ("web", ["shared"])]:
            dep_yaml = ""
            if deps:
                dep_yaml = "dependencies:\n" + "".join(f"  - {d}\n" for d in deps)
            Path(f"{workdir}/{pkg}/takumi-pkg.yaml").write_text(
                f"package:\n  name: {pkg}\n  version: 0.1.0\n{dep_yaml}"
                f"phases:\n  build:\n    commands:\n      - npx tsc -p tsconfig.json\n"
            )
        _git_commit(workdir, "add takumi")

    # Make a change to shared after baseline
    Path(f"{workdir}/shared/index.ts").write_text(
        'export const VERSION = "2.0";\n'
    )

    task = (
        "I just changed the shared library in this TypeScript monorepo. Figure "
        "out which packages are affected by this change and build only those — "
        "don't rebuild anything that hasn't changed. Call task_complete when done."
    )

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. All packages compile (0.3)
        all_ok = True
        for pkg in ("shared", "api", "web"):
            r = subprocess.run(
                f"npx tsc -p {pkg}/tsconfig.json --noEmit",
                shell=True, cwd=wd, capture_output=True, text=True,
            )
            if r.returncode != 0:
                all_ok = False
        checks["all_compile"] = all_ok
        if all_ok:
            score += 0.3

        # 2. Agent identified affected packages (0.3)
        transcript_text = " ".join(
            e.get("output", "") + " " + e.get("text", "") + " " +
            e.get("input", {}).get("summary", "") + " " +
            e.get("input", {}).get("command", "")
            for e in metrics.transcript
        ).lower()
        dep_words = ("affected", "depend", "import", "reference", "←", "<-", "downstream")
        found_api = "api" in transcript_text and any(w in transcript_text for w in dep_words)
        found_web = "web" in transcript_text and any(w in transcript_text for w in dep_words)
        checks["identified_affected"] = found_api and found_web
        if checks["identified_affected"]:
            score += 0.3

        # 3. Agent scoped its work (0.4)
        run_commands = [
            e.get("input", {}).get("command", "")
            for e in metrics.transcript if e.get("tool") == "run_command"
        ]
        used_affected = any("affected" in cmd for cmd in run_commands)
        used_targeted = any(
            pkg in cmd for cmd in run_commands
            for pkg in ("shared", "api", "web")
            if ("tsc" in cmd or "takumi" in cmd or "build" in cmd)
        )
        checks["scoped_build"] = used_affected or used_targeted
        if checks["scoped_build"]:
            score += 0.4

        passed = all_ok and checks["identified_affected"]
        return {"passed": passed, "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_understand_structure_ts(workdir, with_takumi):
    """
    Scenario: 4-package TypeScript monorepo with a diamond dependency.
    Agent must explain the dependency structure and build order.
    """
    for pkg in ("core", "auth", "api", "gateway"):
        os.makedirs(f"{workdir}/{pkg}")
        Path(f"{workdir}/{pkg}/index.ts").write_text(
            f'export const name = "{pkg}";\nconsole.log(name);\n'
        )
        Path(f"{workdir}/{pkg}/package.json").write_text(
            f'{{"name":"{pkg}","version":"0.1.0"}}\n'
        )

    # Add imports showing dependency structure
    Path(f"{workdir}/auth/index.ts").write_text(
        'import { name as coreName } from "../core";\n'
        'export const name = "auth";\nconsole.log(coreName, name);\n'
    )
    Path(f"{workdir}/api/index.ts").write_text(
        'import { name as coreName } from "../core";\n'
        'export const name = "api";\nconsole.log(coreName, name);\n'
    )
    Path(f"{workdir}/gateway/index.ts").write_text(
        'import { name as authName } from "../auth";\n'
        'import { name as apiName } from "../api";\n'
        'export const name = "gateway";\nconsole.log(authName, apiName, name);\n'
    )

    # tsconfig references mirror the dependency graph
    Path(f"{workdir}/core/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist","declaration":true},"include":["*.ts"]}\n'
    )
    Path(f"{workdir}/auth/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist","declaration":true},'
        '"include":["*.ts"],"references":[{"path":"../core"}]}\n'
    )
    Path(f"{workdir}/api/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist","declaration":true},'
        '"include":["*.ts"],"references":[{"path":"../core"}]}\n'
    )
    Path(f"{workdir}/gateway/tsconfig.json").write_text(
        '{"compilerOptions":{"strict":true,"outDir":"dist"},'
        '"include":["*.ts"],"references":[{"path":"../auth"},{"path":"../api"}]}\n'
    )
    Path(f"{workdir}/tsconfig.json").write_text(
        '{"files":[],"references":[{"path":"core"},{"path":"auth"},{"path":"api"},{"path":"gateway"}]}\n'
    )
    Path(f"{workdir}/package.json").write_text(
        '{"private":true,"devDependencies":{"typescript":"^5.0.0"}}\n'
    )

    subprocess.run(
        "npm install --silent 2>/dev/null",
        shell=True, cwd=workdir, capture_output=True,
    )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: platform\n  ignore:\n    - node_modules/\n"
        )
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
                f"phases:\n  build:\n    commands:\n      - npx tsc -p tsconfig.json\n"
            )
        _git_commit(workdir, "add takumi")

    task = (
        "I'm new to this TypeScript project. Explain the dependency structure: "
        "which packages depend on which, and what order should they be "
        "built in? Call task_complete when done."
    )

    def verify(_wd, metrics):
        return _verify_understand_structure(metrics)

    return {"task": task, "verify": verify}


# ---------------------------------------------------------------------------
# Rust scenario setup functions
# ---------------------------------------------------------------------------

def setup_fix_build_error_rust(workdir, with_takumi):
    """
    Scenario: Rust workspace with a type error in the api crate.
    Agent must find and fix the bug, then verify cargo build passes.
    """
    # Workspace Cargo.toml
    Path(f"{workdir}/Cargo.toml").write_text(
        '[workspace]\nmembers = ["shared", "api"]\nresolver = "2"\n'
    )

    # shared crate (compiles fine)
    os.makedirs(f"{workdir}/shared/src")
    Path(f"{workdir}/shared/Cargo.toml").write_text(
        '[package]\nname = "shared"\nversion = "0.1.0"\nedition = "2021"\n'
    )
    Path(f"{workdir}/shared/src/lib.rs").write_text(
        'pub fn greet(name: &str) -> String {\n'
        '    format!("Hello, {}", name)\n'
        '}\n'
    )

    # api crate — broken: assigning &str to u16
    os.makedirs(f"{workdir}/api/src")
    Path(f"{workdir}/api/Cargo.toml").write_text(
        '[package]\nname = "api"\nversion = "0.1.0"\nedition = "2021"\n\n'
        '[dependencies]\nshared = { path = "../shared" }\n'
    )
    Path(f"{workdir}/api/src/main.rs").write_text("""\
use shared::greet;

fn main() {
    let port: u16 = "8080";
    println!("Server on port {}", port);
    println!("{}", greet("World"));
}
""")

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - target/\n"
        )
        Path(f"{workdir}/shared/takumi-pkg.yaml").write_text("""\
package:
  name: shared
  version: 0.1.0
phases:
  build:
    commands:
      - cargo build -p shared
  test:
    commands:
      - cargo test -p shared
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
      - cargo build -p api
  test:
    commands:
      - cargo test -p api
""")
        os.makedirs(f"{workdir}/target", exist_ok=True)
        _git_commit(workdir, "add takumi")

    task = (
        "The build is broken in this Rust project. Running cargo build gives "
        "a type error. Find the bug, fix it, and verify the build passes. "
        "Call task_complete when done."
    )

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. cargo build passes (0.4)
        r = subprocess.run(
            "cargo build", shell=True, cwd=wd,
            capture_output=True, text=True,
        )
        checks["build_passes"] = r.returncode == 0
        if checks["build_passes"]:
            score += 0.4

        # 2. Fix is in the right file (0.2)
        try:
            source = Path(f"{wd}/api/src/main.rs").read_text()
            checks["correct_file"] = 'let port: u16 = "8080";' not in source
        except Exception:
            checks["correct_file"] = False
        if checks["correct_file"]:
            score += 0.2

        # 3. Fix is correct (0.2)
        checks["correct_fix"] = False
        if checks["correct_file"]:
            if any(p in source for p in (
                "port: u16 = 8080", "port = 8080",
                "port: u16 = 8080_u16", 'parse::<u16>',
                "port: u16 = 8_080", ".parse()",
            )):
                checks["correct_fix"] = True
                score += 0.2

        # 4. shared crate still compiles (0.2)
        r2 = subprocess.run(
            "cargo build -p shared", shell=True, cwd=wd,
            capture_output=True, text=True,
        )
        checks["no_collateral"] = r2.returncode == 0
        if checks["no_collateral"]:
            score += 0.2

        return {"passed": checks["build_passes"] and checks.get("correct_fix", False), "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_scoped_rebuild_rust(workdir, with_takumi):
    """
    Scenario: 3-crate Rust workspace. shared was just modified.
    Agent must figure out what's affected and build only those.
    """
    Path(f"{workdir}/Cargo.toml").write_text(
        '[workspace]\nmembers = ["shared", "api", "web"]\nresolver = "2"\n'
    )

    for crate in ("shared", "api", "web"):
        os.makedirs(f"{workdir}/{crate}/src")

    Path(f"{workdir}/shared/Cargo.toml").write_text(
        '[package]\nname = "shared"\nversion = "0.1.0"\nedition = "2021"\n'
    )
    Path(f"{workdir}/shared/src/lib.rs").write_text(
        'pub fn version() -> &\'static str { "1.0" }\n'
    )

    Path(f"{workdir}/api/Cargo.toml").write_text(
        '[package]\nname = "api"\nversion = "0.1.0"\nedition = "2021"\n\n'
        '[dependencies]\nshared = { path = "../shared" }\n'
    )
    Path(f"{workdir}/api/src/main.rs").write_text(
        'fn main() { println!("api {}", shared::version()); }\n'
    )

    Path(f"{workdir}/web/Cargo.toml").write_text(
        '[package]\nname = "web"\nversion = "0.1.0"\nedition = "2021"\n\n'
        '[dependencies]\nshared = { path = "../shared" }\n'
    )
    Path(f"{workdir}/web/src/main.rs").write_text(
        'fn main() { println!("web {}", shared::version()); }\n'
    )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - target/\n"
        )
        for crate, deps in [("shared", []), ("api", ["shared"]), ("web", ["shared"])]:
            dep_yaml = ""
            if deps:
                dep_yaml = "dependencies:\n" + "".join(f"  - {d}\n" for d in deps)
            Path(f"{workdir}/{crate}/takumi-pkg.yaml").write_text(
                f"package:\n  name: {crate}\n  version: 0.1.0\n{dep_yaml}"
                f"phases:\n  build:\n    commands:\n      - cargo build -p {crate}\n"
                f"  test:\n    commands:\n      - cargo test -p {crate}\n"
            )
        os.makedirs(f"{workdir}/target", exist_ok=True)
        _git_commit(workdir, "add takumi")

    # Make a change to shared after baseline
    Path(f"{workdir}/shared/src/lib.rs").write_text(
        'pub fn version() -> &\'static str { "2.0" }\n'
    )

    task = (
        "I just changed the shared library in this Rust workspace. Figure out "
        "which crates are affected by this change and build only those — "
        "don't rebuild anything that hasn't changed. Call task_complete when done."
    )

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. All crates compile (0.3)
        r = subprocess.run(
            "cargo build", shell=True, cwd=wd,
            capture_output=True, text=True,
        )
        checks["all_compile"] = r.returncode == 0
        if checks["all_compile"]:
            score += 0.3

        # 2. Agent identified affected crates (0.3)
        transcript_text = " ".join(
            e.get("output", "") + " " + e.get("text", "") + " " +
            e.get("input", {}).get("summary", "") + " " +
            e.get("input", {}).get("command", "")
            for e in metrics.transcript
        ).lower()
        dep_words = ("affected", "depend", "←", "<-", "downstream", "consumer")
        found_api = "api" in transcript_text and any(w in transcript_text for w in dep_words)
        found_web = "web" in transcript_text and any(w in transcript_text for w in dep_words)
        checks["identified_affected"] = found_api and found_web
        if checks["identified_affected"]:
            score += 0.3

        # 3. Agent scoped its work (0.4)
        run_commands = [
            e.get("input", {}).get("command", "")
            for e in metrics.transcript if e.get("tool") == "run_command"
        ]
        used_affected = any("affected" in cmd for cmd in run_commands)
        used_targeted = any(
            crate in cmd for cmd in run_commands
            for crate in ("shared", "api", "web")
            if ("cargo" in cmd or "takumi" in cmd or "build" in cmd)
        )
        checks["scoped_build"] = used_affected or used_targeted
        if checks["scoped_build"]:
            score += 0.4

        passed = checks["all_compile"] and checks["identified_affected"]
        return {"passed": passed, "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_understand_structure_rust(workdir, with_takumi):
    """
    Scenario: 4-crate Rust workspace with a diamond dependency.
    Agent must explain the dependency structure and build order.
    """
    Path(f"{workdir}/Cargo.toml").write_text(
        '[workspace]\nmembers = ["core", "auth", "api", "gateway"]\nresolver = "2"\n'
    )

    for crate in ("core", "auth", "api", "gateway"):
        os.makedirs(f"{workdir}/{crate}/src")

    Path(f"{workdir}/core/Cargo.toml").write_text(
        '[package]\nname = "core"\nversion = "0.1.0"\nedition = "2021"\n'
    )
    Path(f"{workdir}/core/src/lib.rs").write_text(
        'pub fn init() { println!("core"); }\n'
    )

    Path(f"{workdir}/auth/Cargo.toml").write_text(
        '[package]\nname = "auth"\nversion = "0.1.0"\nedition = "2021"\n\n'
        '[dependencies]\ncore = { path = "../core" }\n'
    )
    Path(f"{workdir}/auth/src/lib.rs").write_text(
        'pub fn init() { core::init(); println!("auth"); }\n'
    )

    Path(f"{workdir}/api/Cargo.toml").write_text(
        '[package]\nname = "api"\nversion = "0.1.0"\nedition = "2021"\n\n'
        '[dependencies]\ncore = { path = "../core" }\n'
    )
    Path(f"{workdir}/api/src/lib.rs").write_text(
        'pub fn init() { core::init(); println!("api"); }\n'
    )

    Path(f"{workdir}/gateway/Cargo.toml").write_text(
        '[package]\nname = "gateway"\nversion = "0.1.0"\nedition = "2021"\n\n'
        '[dependencies]\nauth = { path = "../auth" }\napi = { path = "../api" }\n'
    )
    Path(f"{workdir}/gateway/src/main.rs").write_text(
        'fn main() { auth::init(); api::init(); println!("gateway"); }\n'
    )

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: platform\n  ignore:\n    - target/\n"
        )
        configs = {
            "core": [],
            "auth": ["core"],
            "api": ["core"],
            "gateway": ["auth", "api"],
        }
        for crate, deps in configs.items():
            dep_yaml = ""
            if deps:
                dep_yaml = "dependencies:\n" + "".join(f"  - {d}\n" for d in deps)
            Path(f"{workdir}/{crate}/takumi-pkg.yaml").write_text(
                f"package:\n  name: {crate}\n  version: 0.1.0\n{dep_yaml}"
                f"phases:\n  build:\n    commands:\n      - cargo build -p {crate}\n"
            )
        _git_commit(workdir, "add takumi")

    task = (
        "I'm new to this Rust workspace. Explain the dependency structure: "
        "which crates depend on which, and what order should they be "
        "built in? Call task_complete when done."
    )

    def verify(_wd, metrics):
        return _verify_understand_structure(metrics)

    return {"task": task, "verify": verify}


# ---------------------------------------------------------------------------
# Java scenario setup functions
# ---------------------------------------------------------------------------

def setup_fix_build_error_java(workdir, with_takumi):
    """
    Scenario: Java project with a type error in the api package.
    Agent must find and fix the bug, then verify javac passes.
    """
    os.makedirs(f"{workdir}/shared/src")
    Path(f"{workdir}/shared/src/Lib.java").write_text("""\
public class Lib {
    public static String greet(String name) {
        return "Hello, " + name;
    }
}
""")

    os.makedirs(f"{workdir}/api/src")
    # Bug: assigning String to int
    Path(f"{workdir}/api/src/Main.java").write_text("""\
public class Main {
    public static void main(String[] args) {
        int port = "8080";
        System.out.println("Server on port " + port);
        System.out.println(Lib.greet("World"));
    }
}
""")

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - '*.class'\n"
        )
        Path(f"{workdir}/shared/takumi-pkg.yaml").write_text("""\
package:
  name: shared
  version: 0.1.0
phases:
  build:
    commands:
      - javac src/Lib.java
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
      - javac -cp ../shared/src src/Main.java
""")
        _git_commit(workdir, "add takumi")

    task = (
        "The build is broken in this Java project. Compiling gives a type "
        "error. Find the bug, fix it, and verify it compiles. "
        "Call task_complete when done."
    )

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. javac passes (0.4)
        r = subprocess.run(
            "javac -cp ../shared/src src/Main.java",
            shell=True, cwd=f"{wd}/api",
            capture_output=True, text=True,
        )
        checks["build_passes"] = r.returncode == 0
        if checks["build_passes"]:
            score += 0.4

        # 2. Fix is in the right file (0.2)
        try:
            source = Path(f"{wd}/api/src/Main.java").read_text()
            checks["correct_file"] = 'int port = "8080"' not in source
        except Exception:
            checks["correct_file"] = False
        if checks["correct_file"]:
            score += 0.2

        # 3. Fix is correct (0.2)
        checks["correct_fix"] = False
        if checks["correct_file"]:
            if any(p in source for p in (
                "int port = 8080",
                'String port = "8080"',
                "Integer.parseInt(",
                "int port = Integer",
            )):
                checks["correct_fix"] = True
                score += 0.2

        # 4. shared still compiles (0.2)
        r2 = subprocess.run(
            "javac src/Lib.java", shell=True, cwd=f"{wd}/shared",
            capture_output=True, text=True,
        )
        checks["no_collateral"] = r2.returncode == 0
        if checks["no_collateral"]:
            score += 0.2

        return {"passed": checks["build_passes"] and checks.get("correct_fix", False), "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_scoped_rebuild_java(workdir, with_takumi):
    """
    Scenario: 3-package Java project. shared was just modified.
    Agent must figure out what's affected and build only those.
    """
    for pkg in ("shared", "api", "web"):
        os.makedirs(f"{workdir}/{pkg}/src")

    Path(f"{workdir}/shared/src/Lib.java").write_text("""\
public class Lib {
    public static String version() { return "1.0"; }
}
""")
    Path(f"{workdir}/api/src/Main.java").write_text("""\
public class Main {
    public static void main(String[] args) {
        System.out.println("api " + Lib.version());
    }
}
""")
    Path(f"{workdir}/web/src/Main.java").write_text("""\
public class Main {
    public static void main(String[] args) {
        System.out.println("web " + Lib.version());
    }
}
""")

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: myapp\n  ignore:\n    - '*.class'\n"
        )
        for pkg, deps in [("shared", []), ("api", ["shared"]), ("web", ["shared"])]:
            dep_yaml = ""
            if deps:
                dep_yaml = "dependencies:\n" + "".join(f"  - {d}\n" for d in deps)
            if deps:
                Path(f"{workdir}/{pkg}/takumi-pkg.yaml").write_text(
                    f"package:\n  name: {pkg}\n  version: 0.1.0\n{dep_yaml}"
                    f"phases:\n  build:\n    commands:\n"
                    f"      - javac -cp ../shared/src src/Main.java\n"
                )
            else:
                Path(f"{workdir}/{pkg}/takumi-pkg.yaml").write_text(
                    f"package:\n  name: {pkg}\n  version: 0.1.0\n"
                    f"phases:\n  build:\n    commands:\n"
                    f"      - javac src/Lib.java\n"
                )
        _git_commit(workdir, "add takumi")

    # Make a change to shared after baseline
    Path(f"{workdir}/shared/src/Lib.java").write_text("""\
public class Lib {
    public static String version() { return "2.0"; }
}
""")

    task = (
        "I just changed the shared library in this Java project. Figure out "
        "which packages are affected by this change and build only those — "
        "don't rebuild anything that hasn't changed. Call task_complete when done."
    )

    def verify(wd, metrics):
        checks = {}
        score = 0.0

        # 1. All packages compile (0.3)
        all_ok = True
        for pkg in ("api", "web"):
            r = subprocess.run(
                f"javac -cp ../shared/src src/Main.java",
                shell=True, cwd=f"{wd}/{pkg}",
                capture_output=True, text=True,
            )
            if r.returncode != 0:
                all_ok = False
        checks["all_compile"] = all_ok
        if all_ok:
            score += 0.3

        # 2. Agent identified affected packages (0.3)
        transcript_text = " ".join(
            e.get("output", "") + " " + e.get("text", "") + " " +
            e.get("input", {}).get("summary", "") + " " +
            e.get("input", {}).get("command", "")
            for e in metrics.transcript
        ).lower()
        dep_words = ("affected", "depend", "import", "←", "<-", "downstream", "consumer", "classpath")
        found_api = "api" in transcript_text and any(w in transcript_text for w in dep_words)
        found_web = "web" in transcript_text and any(w in transcript_text for w in dep_words)
        checks["identified_affected"] = found_api and found_web
        if checks["identified_affected"]:
            score += 0.3

        # 3. Agent scoped its work (0.4)
        run_commands = [
            e.get("input", {}).get("command", "")
            for e in metrics.transcript if e.get("tool") == "run_command"
        ]
        used_affected = any("affected" in cmd for cmd in run_commands)
        used_targeted = any(
            pkg in cmd for cmd in run_commands
            for pkg in ("shared", "api", "web")
            if ("javac" in cmd or "takumi" in cmd or "build" in cmd)
        )
        checks["scoped_build"] = used_affected or used_targeted
        if checks["scoped_build"]:
            score += 0.4

        passed = all_ok and checks["identified_affected"]
        return {"passed": passed, "score": score, "checks": checks}

    return {"task": task, "verify": verify}


def setup_understand_structure_java(workdir, with_takumi):
    """
    Scenario: 4-package Java project with a diamond dependency.
    Agent must explain the dependency structure and build order.
    """
    for pkg in ("core", "auth", "api", "gateway"):
        os.makedirs(f"{workdir}/{pkg}/src")

    Path(f"{workdir}/core/src/Core.java").write_text("""\
public class Core {
    public static void init() { System.out.println("core"); }
}
""")
    Path(f"{workdir}/auth/src/Auth.java").write_text("""\
public class Auth {
    public static void init() { Core.init(); System.out.println("auth"); }
}
""")
    Path(f"{workdir}/api/src/Api.java").write_text("""\
public class Api {
    public static void init() { Core.init(); System.out.println("api"); }
}
""")
    Path(f"{workdir}/gateway/src/Gateway.java").write_text("""\
public class Gateway {
    public static void main(String[] args) {
        Auth.init();
        Api.init();
        System.out.println("gateway");
    }
}
""")

    _git_init(workdir)

    if with_takumi:
        _takumi_init(workdir)
        os.remove(f"{workdir}/takumi-pkg.yaml")
        Path(f"{workdir}/takumi.yaml").write_text(
            "workspace:\n  name: platform\n  ignore:\n    - '*.class'\n"
        )
        configs = {
            "core": ([], "javac src/Core.java"),
            "auth": (["core"], "javac -cp ../core/src src/Auth.java"),
            "api": (["core"], "javac -cp ../core/src src/Api.java"),
            "gateway": (["auth", "api"], "javac -cp ../core/src:../auth/src:../api/src src/Gateway.java"),
        }
        for pkg, (deps, build_cmd) in configs.items():
            dep_yaml = ""
            if deps:
                dep_yaml = "dependencies:\n" + "".join(f"  - {d}\n" for d in deps)
            Path(f"{workdir}/{pkg}/takumi-pkg.yaml").write_text(
                f"package:\n  name: {pkg}\n  version: 0.1.0\n{dep_yaml}"
                f"phases:\n  build:\n    commands:\n      - {build_cmd}\n"
            )
        _git_commit(workdir, "add takumi")

    task = (
        "I'm new to this Java project. Explain the dependency structure: "
        "which packages depend on which, and what order should they be "
        "built in? Call task_complete when done."
    )

    def verify(_wd, metrics):
        return _verify_understand_structure(metrics)

    return {"task": task, "verify": verify}


# ---------------------------------------------------------------------------
# Shared verify helpers
# ---------------------------------------------------------------------------

def _verify_understand_structure(metrics):
    """Common correctness check for understand-structure across languages."""
    checks = {}
    score = 0.0

    text = " ".join(
        e.get("text", "") + " " +
        e.get("output", "") + " " +
        e.get("input", {}).get("summary", "") + " " +
        e.get("input", {}).get("command", "")
        for e in metrics.transcript
    ).lower()

    checks["core_is_base"] = (
        "core" in text and
        any(w in text for w in (
            "no depend", "no dep", "no deps", "base", "root",
            "foundation", "leaf", "level 0", "independent",
        ))
    )
    if checks["core_is_base"]:
        score += 0.2

    dep_patterns_auth = (
        "auth depends on core", "auth -> core", "auth → core",
        "auth: core", "auth relies on core", "auth imports core",
        "auth ← core", "auth <- core",
    )
    checks["auth_depends_core"] = (
        "auth" in text and "core" in text and
        any(p in text for p in dep_patterns_auth)
    )
    if not checks["auth_depends_core"]:
        checks["auth_depends_core"] = (
            "auth" in text and "core" in text and
            any(w in text for w in ("depend", "←", "<-", "level", "import"))
        )
    if checks["auth_depends_core"]:
        score += 0.2

    dep_patterns_api = (
        "api depends on core", "api -> core", "api → core",
        "api: core", "api relies on core", "api imports core",
        "api ← core", "api <- core",
    )
    checks["api_depends_core"] = (
        "api" in text and "core" in text and
        any(p in text for p in dep_patterns_api)
    )
    if not checks["api_depends_core"]:
        checks["api_depends_core"] = (
            "api" in text and "core" in text and
            any(w in text for w in ("depend", "←", "<-", "level", "import"))
        )
    if checks["api_depends_core"]:
        score += 0.2

    checks["gateway_depends_both"] = (
        "gateway" in text and "auth" in text and "api" in text and
        any(w in text for w in (
            "depend", "gateway ->", "gateway →", "diamond",
            "gateway ←", "gateway <-", "level 2", "import",
        ))
    )
    if checks["gateway_depends_both"]:
        score += 0.2

    checks["correct_build_order"] = False
    core_pos = text.find("core")
    gateway_pos = text.rfind("gateway")
    if core_pos >= 0 and gateway_pos >= 0 and core_pos < gateway_pos:
        if any(w in text for w in (
            "order", "first", "last", "level", "before", "then",
            "level 0", "level 1", "level 2",
        )):
            checks["correct_build_order"] = True
    if checks["correct_build_order"]:
        score += 0.2

    n_correct = sum(1 for v in checks.values() if v)
    passed = n_correct >= 3
    return {"passed": passed, "score": score, "checks": checks}


# ---------------------------------------------------------------------------
# Scenario registry
# ---------------------------------------------------------------------------

SCENARIOS = {
    # Go
    "fix-build-error-go": {
        "name": "Fix Build Error (Go)",
        "desc": "Find and fix a type error in a Go HTTP handler",
        "setup": setup_fix_build_error_go,
        "group": "fix-build-error",
        "lang": "Go",
    },
    "scoped-rebuild-go": {
        "name": "Scoped Rebuild (Go)",
        "desc": "After changing shared lib, build only affected Go packages",
        "setup": setup_scoped_rebuild_go,
        "group": "scoped-rebuild",
        "lang": "Go",
    },
    "understand-structure-go": {
        "name": "Understand Structure (Go)",
        "desc": "Explain dependency graph and build order of a Go monorepo",
        "setup": setup_understand_structure_go,
        "group": "understand-structure",
        "lang": "Go",
    },
    # Python
    "fix-build-error-python": {
        "name": "Fix Build Error (Python)",
        "desc": "Find and fix a TypeError in a Python project",
        "setup": setup_fix_build_error_python,
        "group": "fix-build-error",
        "lang": "Python",
    },
    "scoped-rebuild-python": {
        "name": "Scoped Rebuild (Python)",
        "desc": "After changing shared lib, rebuild only affected Python packages",
        "setup": setup_scoped_rebuild_python,
        "group": "scoped-rebuild",
        "lang": "Python",
    },
    "understand-structure-python": {
        "name": "Understand Structure (Python)",
        "desc": "Explain dependency graph and build order of a Python project",
        "setup": setup_understand_structure_python,
        "group": "understand-structure",
        "lang": "Python",
    },
    # TypeScript
    "fix-build-error-ts": {
        "name": "Fix Build Error (TypeScript)",
        "desc": "Find and fix a type error caught by tsc",
        "setup": setup_fix_build_error_ts,
        "group": "fix-build-error",
        "lang": "TypeScript",
    },
    "scoped-rebuild-ts": {
        "name": "Scoped Rebuild (TypeScript)",
        "desc": "After changing shared lib, build only affected TS packages",
        "setup": setup_scoped_rebuild_ts,
        "group": "scoped-rebuild",
        "lang": "TypeScript",
    },
    "understand-structure-ts": {
        "name": "Understand Structure (TypeScript)",
        "desc": "Explain dependency graph and build order of a TS monorepo",
        "setup": setup_understand_structure_ts,
        "group": "understand-structure",
        "lang": "TypeScript",
    },
    # Rust
    "fix-build-error-rust": {
        "name": "Fix Build Error (Rust)",
        "desc": "Find and fix a type error in a Rust workspace",
        "setup": setup_fix_build_error_rust,
        "group": "fix-build-error",
        "lang": "Rust",
    },
    "scoped-rebuild-rust": {
        "name": "Scoped Rebuild (Rust)",
        "desc": "After changing shared crate, build only affected Rust crates",
        "setup": setup_scoped_rebuild_rust,
        "group": "scoped-rebuild",
        "lang": "Rust",
    },
    "understand-structure-rust": {
        "name": "Understand Structure (Rust)",
        "desc": "Explain dependency graph and build order of a Rust workspace",
        "setup": setup_understand_structure_rust,
        "group": "understand-structure",
        "lang": "Rust",
    },
    # Java
    "fix-build-error-java": {
        "name": "Fix Build Error (Java)",
        "desc": "Find and fix a type error in a Java project",
        "setup": setup_fix_build_error_java,
        "group": "fix-build-error",
        "lang": "Java",
    },
    "scoped-rebuild-java": {
        "name": "Scoped Rebuild (Java)",
        "desc": "After changing shared lib, build only affected Java packages",
        "setup": setup_scoped_rebuild_java,
        "group": "scoped-rebuild",
        "lang": "Java",
    },
    "understand-structure-java": {
        "name": "Understand Structure (Java)",
        "desc": "Explain dependency graph and build order of a Java project",
        "setup": setup_understand_structure_java,
        "group": "understand-structure",
        "lang": "Java",
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
        if tool_results:
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
        correctness = metrics_dict.get("correctness", 0.0)
        checks = metrics_dict.get("correctness_checks", {})
        f.write(f"# Correctness: {correctness:.0%}\n")
        if checks:
            for k, v in checks.items():
                f.write(f"#   {k}: {'PASS' if v else 'FAIL'}\n")
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

    # Status & Correctness
    w_status = _status(without["task_completed"], without["success"])
    t_status = _status(with_t["task_completed"], with_t["success"])
    w_score = without.get("correctness", 0.0)
    t_score = with_t.get("correctness", 0.0)
    print(f"  │ {'STATUS & CORRECTNESS':<{W - 6}} │")
    print(f"  │   {'':14s} {'Without':>8s}  {'Takumi':>8s}               │")
    print(f"  │   {'Status':14s} {w_status:>8s}  {t_status:>8s}               │")
    print(f"  │   {'Correctness':14s} {w_score:>7.0%}  {t_score:>8.0%}               │")

    # Show individual checks
    w_checks = without.get("correctness_checks", {})
    t_checks = with_t.get("correctness_checks", {})
    all_check_keys = list(dict.fromkeys(list(w_checks.keys()) + list(t_checks.keys())))
    for key in all_check_keys:
        w_val = "✓" if w_checks.get(key) else "✗"
        t_val = "✓" if t_checks.get(key) else "✗"
        label = key.replace("_", " ")
        print(f"  │     {label:12s} {w_val:>8s}  {t_val:>8s}               │")

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
    print(f"  │ {'Scenario':<20s} {'Tokens':>11s} {'Time':>8s} {'Calls':>7s} {'Score':>9s} │")
    print(f"  │ {'':─<20s} {'':─>11s} {'':─>8s} {'':─>7s} {'':─>9s} │")

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
            print(f"  │ {name:<20s} {'ERROR':>11s} {'':>8s} {'':>7s} {'':>9s} │")
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
        calls_s = f"{w['tool_calls']}->{t['tool_calls']}"
        w_score = w.get("correctness", 0.0)
        t_score = t.get("correctness", 0.0)
        score_s = f"{w_score:.0%}/{t_score:.0%}"

        print(f"  │ {name:<20s} {tok_s:>11s} {time_s:>8s} {calls_s:>7s} {score_s:>9s} │")

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
                verdict = config["verify"](workdir, metrics)
                metrics.success = verdict["passed"]
                metrics.correctness = verdict["score"]
                metrics.correctness_checks = verdict["checks"]

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
    totals = {"without": {"tokens": 0, "turns": 0, "calls": 0, "errors": 0, "correctness": 0.0, "scenarios": 0},
              "with":    {"tokens": 0, "turns": 0, "calls": 0, "errors": 0, "correctness": 0.0, "scenarios": 0}}
    for results in all_results.values():
        for mode, key in [("without", "without_takumi"), ("with", "with_takumi")]:
            r = results.get(key, {})
            if "error" in r:
                continue
            totals[mode]["tokens"] += r.get("input_tokens", 0) + r.get("output_tokens", 0)
            totals[mode]["turns"] += r.get("turns", 0)
            totals[mode]["calls"] += r.get("tool_calls", 0)
            totals[mode]["errors"] += r.get("errors", 0)
            totals[mode]["correctness"] += r.get("correctness", 0.0)
            totals[mode]["scenarios"] += 1

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
