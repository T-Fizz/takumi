# Commands Reference

## Building & Testing

### `takumi build [packages...]`

Build packages in dependency order. If no packages specified, builds all.

```bash
takumi build                     # Build everything
takumi build api web             # Build specific packages
takumi build --affected          # Only packages changed since last commit
takumi build --dry-run           # Show execution plan without running
takumi build --no-cache          # Force rebuild, ignore cache
```

**Flags:**
- `--affected` — only build packages with changed files + downstream dependents
- `--dry-run` — show what would run, with cache status per package
- `--no-cache` — skip cache lookup and force execution

### `takumi build clean`

Remove the `build/` directory and clear the build cache.

### `takumi test [packages...]`

Run test phase for packages. Same flags as `build`.

```bash
takumi test                      # Test everything
takumi test --affected           # Only changed packages
takumi test api --no-cache       # Force retest
```

### `takumi run <phase> [packages...]`

Run any named phase. Use this for custom phases like `lint`, `deploy`, `bundle`.

```bash
takumi run lint
takumi run deploy api
takumi run lint --affected --dry-run
```

## Workspace Info

### `takumi status`

Full workspace health dashboard: packages, environments, recent builds, AI agent.

### `takumi graph`

Print the dependency graph with parallel level annotations.

```
Level 0 (no deps)
  shared-lib

Level 1
  api ← shared-lib
  web ← shared-lib
```

### `takumi affected`

List packages affected by recent file changes, including downstream dependents.

```bash
takumi affected                  # Changes in working tree
takumi affected --since main     # Changes since branch point
takumi affected --since HEAD~3   # Last 3 commits
```

### `takumi validate`

Check all configuration files for errors:

1. Structural validation of `takumi.yaml`, all `takumi-pkg.yaml` files, and `takumi-versions.yaml`
2. Cross-validation: unresolved dependency references
3. Cycle detection in the dependency graph

## Source Tracking

### `takumi checkout <url>`

Clone a git repository and register it as a tracked source in `takumi.yaml`.

```bash
takumi checkout git@github.com:org/repo.git
takumi checkout git@github.com:org/repo.git --branch dev
takumi checkout git@github.com:org/repo.git --path ./libs/repo
```

### `takumi sync`

Pull updates for all tracked sources. Clones any that are missing.

### `takumi remove <package>`

Unregister a tracked source from `takumi.yaml` and clean up its runtime environment.

```bash
takumi remove shared-lib             # Unregister only
takumi remove shared-lib --delete    # Also delete from disk
```

## Runtime Environments

### `takumi env setup [packages...]`

Run `runtime.setup` commands for packages that define a `runtime` section. Creates isolated environments in `.takumi/envs/<package>/`.

```bash
takumi env setup                 # All packages with runtime
takumi env setup api             # Specific package
```

### `takumi env list`

Show environment status (ready / not set up) for all packages with runtime config.

### `takumi env clean [packages...]`

Remove environment directories.

## Version Sets

### `takumi version-set check`

Display pinned dependency versions from `takumi-versions.yaml`. Alias: `takumi vs check`.

## AI Skills

### `takumi ai context`

Regenerate `.takumi/TAKUMI.md` and the AI agent config file.

### `takumi ai diagnose <package>`

Render a diagnostic prompt for a failed package. Reads the most recent log from `.takumi/logs/`, collects git diff, dependency chain, and env status.

### `takumi ai review`

Render a code review prompt from the current git diff and affected packages.

### `takumi ai optimize`

Render a build optimization prompt from `.takumi/metrics.json` and the dependency graph.

### `takumi ai onboard`

Render a workspace briefing prompt with all configs and the dependency graph. Designed to bootstrap a new AI session.

### `takumi ai skill list`

List all available skills with source labels (built-in, workspace, package).

### `takumi ai skill show <name>`

Print a skill's prompt template and metadata.

### `takumi ai skill run <name>`

Render a skill with workspace context and print the result.

## Documentation

### `takumi docs generate`

Generate documentation from the current workspace state:

- `docs/user/commands.md` — CLI reference
- `docs/user/skills-reference.md` — AI skills
- `docs/user/config-reference.md` — config schemas
- `docs/user/packages.md` — package table

```bash
takumi docs generate             # Generate docs
takumi docs generate --ai        # Also run doc-writer skill
```

### `takumi docs hook install`

Install a git pre-commit hook that auto-regenerates docs.

### `takumi docs hook remove`

Remove the pre-commit hook.

## Benchmarking

### `takumi benchmark [scenarios...]`

Run performance benchmarks comparing agent work with and without Takumi. Measures token usage, tool calls, turns, errors, and wall-clock time.

```bash
takumi benchmark                         # Run all scenarios
takumi benchmark fix-build-error         # Run specific scenario
takumi benchmark --publish               # Publish results to GitHub Gist
takumi benchmark --model claude-sonnet-4-5-20241022
```

**Available scenarios:**

| Scenario | Description |
|----------|-------------|
| `fix-build-error` | Find and fix a type error in a Go HTTP handler |
| `scoped-rebuild` | After changing shared lib, build only affected packages |
| `understand-structure` | Explain dependency graph and build order of a 4-package monorepo |

**Flags:**
- `--publish` — generate a markdown report and publish to a GitHub Gist
- `--model <model>` — override the LLM model (default: `claude-haiku-4-5-20251001`)

Requires `ANTHROPIC_API_KEY` set in environment or `.env` file.

### `takumi benchmark iterate`

Run an iterative setup benchmark that tests how efficiently an agent can onboard to a freshly-cloned project. Results are appended to `history.json` for tracking improvements over time.

```bash
takumi benchmark iterate                 # Run and append to history
takumi benchmark iterate --note "improved README"
takumi benchmark iterate --publish       # Publish trend report to Gist
```

**Flags:**
- `--note <text>` — annotate this run (e.g. what changed since last run)
- `--publish` — publish trend report with full history to GitHub Gist
- `--model <model>` — override the LLM model

Each run creates a transcript log in `tests/benchmark/iterative/logs/` and appends metrics to `history.json`. The dashboard shows the current run and a trend comparison against the first run.

## MCP Server

### `takumi mcp serve`

Start a Model Context Protocol server over stdio. This allows AI agents (Claude Code, etc.) to operate the workspace directly via JSON-RPC.

```bash
takumi mcp serve
```

The server exposes 7 tools:

| Tool | Description |
|------|-------------|
| `takumi_status` | Workspace health dashboard |
| `takumi_build` | Build packages (supports `packages`, `affected`, `no_cache` params) |
| `takumi_test` | Run tests (same params as build) |
| `takumi_diagnose` | Read build/test failure logs for a package |
| `takumi_affected` | List packages affected by file changes |
| `takumi_validate` | Validate all config files |
| `takumi_graph` | Show dependency graph |

For Claude Code integration, add a `.mcp.json` file to your project root:

```json
{
  "mcpServers": {
    "takumi": {
      "command": "go",
      "args": ["run", "./cmd/takumi", "mcp", "serve"]
    }
  }
}
```

Or if Takumi is installed (recommended — avoids `go run` startup time on every invocation):

```json
{
  "mcpServers": {
    "takumi": {
      "command": "takumi",
      "args": ["mcp", "serve"]
    }
  }
}
```

The `go run` variant is suitable for development only; use the installed binary for regular use.

### `takumi mcp install`

Register Takumi as a global MCP server so its tools are available in every project — even before running `takumi init`.

```bash
takumi mcp install
```

Currently supports Claude Code. Writes to `~/.claude/claude_desktop_config.json`. If the current directory is not a Takumi workspace, `takumi_status` will guide the agent to run `takumi init`.

## Initialization

### `takumi init [name]`

Initialize a Takumi package in the current directory. If `name` is given, creates or enters that directory first.

```bash
takumi init                      # Init in cwd
takumi init service-a            # Init in ./service-a/
takumi init --agent claude       # Skip agent selection prompt
```

### `takumi init --root <name>`

Create a new project directory with workspace + package inside it.

```bash
takumi init --root my-project    # Creates my-project/ with full setup
```
