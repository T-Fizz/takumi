# 匠 Takumi

An AI-aware, language-agnostic package builder.

Takumi runs user-defined shell commands, manages optional per-package runtime environments, builds a dependency DAG for parallel execution, and ships with an AI skills system that teaches AI assistants how to operate your workspace.

## Quick Start

```bash
# Install
go install github.com/tfitz/takumi@latest

# Create a new project
takumi init --root my-project
cd my-project

# Or initialize in an existing directory
cd existing-project
takumi init
```

Add a short alias to your shell profile (`~/.bashrc`, `~/.zshrc`):

```bash
alias t='takumi'
```

Edit `takumi-pkg.yaml` with your build commands, then:

```bash
t build                       # Build in dependency order
t test                        # Run tests
t status                      # Workspace dashboard
```

Unchanged packages are automatically skipped via content-addressed caching.

## Features

- **User-defined phases** — `build`, `test`, `lint`, `deploy`, or any custom phase with shell commands
- **Dependency DAG** — packages declare dependencies; Takumi auto-resolves build order
- **Parallel execution** — independent packages run concurrently within each dependency level
- **Content-addressed caching** — SHA-256 of source files + config + dependency keys; incremental builds
- **Runtime isolation** — optional per-package environments (virtualenv, nvm, etc.) with `{{env_dir}}` substitution
- **MCP server** — Model Context Protocol server for direct AI agent integration (`takumi mcp serve`)
- **AI skills** — prompt templates for Claude, Cursor, Copilot, Windsurf, and Cline
- **Source tracking** — clone and sync external git repositories into the workspace
- **Version pinning** — centralized dependency version sets with configurable strategies
- **LLM code review** — `takumi review` runs a thorough code review via any supported LLM, outputs structured markdown
- **Performance benchmarks** — measure agent token/turn efficiency with and without Takumi, track improvements over iterations

## Concepts

**Workspace** — A directory containing `.takumi/` and a `takumi.yaml`. Holds one or more packages.

**Package** — A directory with a `takumi-pkg.yaml`. Declares dependencies and defines build phases.

**Phase** — A named set of commands (`pre` → `commands` → `post`). Any name works.

**Skill** — A prompt template that collects workspace context and generates structured output for AI assistants.

## Documentation

### User Guides

- [Getting Started](docs/user/getting-started.md) — installation, new project setup
- [Onboarding an Existing Project](docs/user/onboarding-existing-project.md) — step-by-step guide for existing code
- [Commands Reference](docs/user/commands.md) — every command and flag
- [Configuration Reference](docs/user/configuration.md) — all three config file formats
- [AI Skills](docs/user/ai-skills.md) — built-in skills, custom skills, workflow

### Developer Docs

- [Architecture](docs/dev/architecture.md) — package structure, data flow, design decisions
- [Package Reference](docs/dev/packages.md) — Go package API reference
- [Testing Guide](docs/dev/testing.md) — unit tests, integration tests, promptfoo setup
- [Contributing](docs/dev/contributing.md) — build, test, code style

## AI Agent Integration

During `takumi init`, select your AI agent. Takumi creates the appropriate config file (`CLAUDE.md`, `.cursor/rules`, etc.) pointing to `.takumi/TAKUMI.md` — a workspace-aware instruction set.

| Agent | Config File |
|-------|-------------|
| Claude | `CLAUDE.md` |
| Cursor | `.cursor/rules` |
| Copilot | `.github/copilot-instructions.md` |
| Windsurf | `.windsurfrules` |
| Cline | `.clinerules` |

Six built-in skills: **operator**, **diagnose**, **review**, **optimize**, **onboard**, **doc-writer**. See [AI Skills](docs/user/ai-skills.md) for details.

### MCP Server

Takumi includes a Model Context Protocol (MCP) server that lets AI agents operate your workspace directly — no copy-paste needed.

```bash
takumi mcp install  # Register globally for all Claude Code sessions
# or add .mcp.json to your project root for per-project setup
```

The server exposes 7 tools: `takumi_status`, `takumi_build`, `takumi_test`, `takumi_diagnose`, `takumi_affected`, `takumi_validate`, `takumi_graph`. See [Commands Reference](docs/user/commands.md) for details.

## Building from Source

```bash
git clone https://github.com/T-Fizz/takumi.git
cd takumi
make build          # → ./build/takumi
make install        # → $GOPATH/bin/takumi
make test           # Unit tests
make test-all       # Unit + integration + LLM-graded tests
```

Requires Go 1.26+.

## License

[GNU Affero General Public License v3.0 (AGPLv3)](LICENSE) — free to use, modify, and distribute. Derivative works and network-accessible services must release source under the same license.
