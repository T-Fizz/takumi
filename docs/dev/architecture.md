# Architecture

## Overview

Takumi is a CLI tool written in Go. It uses Cobra for command routing, YAML for configuration, and a content-addressed cache for incremental builds. The binary is self-contained — built-in AI skill templates are embedded at compile time via `//go:embed`.

## Directory Layout

```
cmd/
  takumi/
    main.go                    # Entry point — calls cli.Execute()

src/
  cli/                         # Cobra commands (25+ files, one per command group)
  config/                      # YAML config parsing and validation
  workspace/                   # Workspace detection, package discovery
  graph/                       # Dependency DAG, topological sort
  cache/                       # Content-addressed build cache
  executor/                    # Phase execution, parallelism, logging
  mcp/                         # MCP server (Model Context Protocol)
  skills/                      # AI skill loading and rendering
    builtin/                   # Embedded YAML skill templates
  ui/                          # Terminal styling (lipgloss)
```

## Package Responsibilities

### `config`

Parses and validates the three YAML config files:

- `WorkspaceConfig` — `takumi.yaml` (workspace name, ignore list, sources, settings, AI config)
- `PackageConfig` — `takumi-pkg.yaml` (package name, version, dependencies, runtime, phases, AI metadata)
- `VersionSetConfig` — `takumi-versions.yaml` (version pinning with strategy)

Validation is centralized here. `takumi validate` calls the config validators, which return typed errors and warnings.

### `workspace`

Detects the workspace root by walking up the filesystem looking for a `.takumi/` marker directory. Once found, recursively scans for `takumi-pkg.yaml` files (respecting the ignore list) and returns a `workspace.Info` struct containing all discovered packages.

Key constants: `MarkerDir = ".takumi"`, `ConfigFile = "takumi.yaml"`, `PackageFile = "takumi-pkg.yaml"`.

### `graph`

Builds a directed acyclic graph from package dependency declarations. Uses Kahn's algorithm for topological sort, which naturally produces parallel levels — groups of packages with no interdependencies that can build concurrently.

Cycle detection is a side effect of Kahn's algorithm: if the sorted output has fewer nodes than the input, a cycle exists.

### `cache`

Content-addressed caching using SHA-256. A cache key is computed from:

1. Phase name
2. Package config file hash
3. All source file hashes (recursive, respects ignore list)
4. Dependency cache keys (sorted, cascading)

Cache entries are stored as JSON in `.takumi/cache/{pkg}.{phase}.json`. A cache hit means the computed key matches the stored key — the phase is skipped entirely.

### `executor`

Runs phase commands in the correct order. For each phase: `pre` → `commands` → `post`. Commands execute via `sh -c` in the package's working directory with merged environment variables (OS env + runtime env).

Parallel execution works level by level: all packages within a level are dispatched as goroutines, synchronized with `sync.WaitGroup`. Execution stops immediately on first error.

Output is tee'd to both `.takumi/logs/{pkg}.{phase}.log` and the terminal (with `[package-name]` prefix). Build metrics are appended to `.takumi/metrics.json` after each run.

### `skills`

Loads AI skill templates from three sources:

1. **Built-in** — Embedded via `//go:embed builtin/*.yaml`, always available
2. **Workspace** — YAML files in `.takumi/skills/`
3. **Package** — Defined in `takumi-pkg.yaml` under `ai.tasks`

Template rendering is simple string substitution: `{{key}}` placeholders are replaced with values from a `map[string]string`. No loops, conditionals, or filters.

### `mcp`

Model Context Protocol server that exposes Takumi operations as tools for AI agents. Uses [mcp-go](https://github.com/mark3labs/mcp-go) SDK with stdio transport. The server registers 7 tools (status, build, test, diagnose, affected, validate, graph) and serializes execution with `WorkerPoolSize(1)`.

Build/test output goes to log files — tool results return summaries and file paths to reduce token consumption. All handlers set `executor.Quiet = true` to prevent terminal output from corrupting the stdio JSON-RPC transport.

### `cli`

One file per command group (e.g., `build.go`, `ai.go`, `env.go`, `init.go`). Each file registers its commands in an `init()` function. Commands that need workspace context call `requireWorkspace()`, which detects and loads the workspace or exits with an error.

### `ui`

Terminal styling using Charmbracelet Lipgloss. Defines color constants and helper functions for consistent output formatting across commands.

## Data Flow: Build Command

```
takumi build --affected
    │
    ├─ workspace.Detect()          Find .takumi/ marker
    ├─ workspace.Load()            Parse configs, discover packages
    ├─ graph.Build()               Construct dependency DAG
    ├─ graph.Sort()                Topological sort → []Level
    ├─ affected.Compute()          Filter to changed packages + dependents
    │
    └─ for each Level:
        ├─ for each Package (parallel):
        │   ├─ cache.ComputeKey()  SHA-256 of sources + config + dep keys
        │   ├─ cache.Lookup()      Check .takumi/cache/
        │   │   ├─ HIT → skip
        │   │   └─ MISS → continue
        │   ├─ executor.Run()      pre → commands → post
        │   │   ├─ sh -c <cmd>     In package dir, with merged env
        │   │   └─ tee → log + terminal
        │   ├─ cache.Store()       Write entry on success
        │   └─ metrics.Record()    Append to metrics.json
        └─ WaitGroup.Wait()        Block until level completes
```

## Data Flow: AI Diagnose

```
takumi ai diagnose api
    │
    ├─ workspace.Load()
    ├─ Read .takumi/logs/api.build.log     Last error output
    ├─ git diff                             Changed files
    ├─ graph.TransitiveDependents("api")   Dependency chain
    ├─ env.Status("api")                   Environment health
    │
    ├─ skills.LoadBuiltins()               Find "diagnose" skill
    ├─ skills.Render(prompt, vars)         Substitute {{variables}}
    │
    └─ Print rendered prompt to stdout
```

## Data Flow: MCP Tool Call

```
Agent calls takumi_build(affected=true)
    │
    ├─ MCP server receives JSON-RPC request over stdio
    ├─ handleBuild() dispatched (WorkerPoolSize=1 serializes)
    │
    ├─ os.Getwd() → workspace.Load()
    ├─ graph.Build() → graph.Sort()
    ├─ gitChangedFiles() → mapFilesToPackages() → filter packages
    │
    ├─ executor.Run() with Quiet=true
    │   ├─ Terminal output → io.Discard (protect stdio transport)
    │   ├─ Log files → .takumi/logs/<pkg>.<phase>.log (still written)
    │   └─ Results collected
    │
    ├─ Format summary: "Build completed: 2 passed, 1 cached"
    ├─ Append per-package results + log file paths
    │
    └─ Return gomcp.NewToolResultText(summary)
        → serialized as JSON-RPC response over stdio
```

## Key Design Decisions

**No daemon, optional server.** Takumi is a stateless CLI. All state lives in the filesystem (`.takumi/` directory). The MCP server (`takumi mcp serve`) runs only when invoked by an AI agent and communicates over stdio — no background processes or ports.

**Shell commands, not plugins.** Phases are plain shell commands. Takumi doesn't need language-specific plugins — if it runs in your shell, it runs in Takumi.

**Fail fast.** Any non-zero exit code stops execution immediately. No partial builds, no "continue on error" mode.

**Cache cascading.** Dependency cache keys are included in the hash computation. If a dependency changes, all downstream packages automatically invalidate. No manual cache busting needed.

**Embedded skills.** Built-in skills are compiled into the binary via `//go:embed`. No external files to distribute or lose.
