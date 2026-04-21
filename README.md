# 匠 Takumi

An AI-aware, language-agnostic workspace builder.

Takumi runs user-defined shell commands, manages optional per-package runtime environments, builds a dependency DAG for parallel execution, and generates an operator prompt (`.takumi/TAKUMI.md`) that teaches AI agents how to use your workspace correctly.

## Install

### Homebrew (macOS/Linux)

```bash
brew install T-Fizz/tap/takumi
```

### Shell installer (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/T-Fizz/takumi/main/install.sh | sh
```

Installs to `/usr/local/bin` (or `~/.local/bin` if not writable). Override with `TAKUMI_INSTALL_DIR`:

```bash
TAKUMI_INSTALL_DIR=~/.bin curl -fsSL https://raw.githubusercontent.com/T-Fizz/takumi/main/install.sh | sh
```

### Manual download

Grab a prebuilt binary from [Releases](https://github.com/T-Fizz/takumi/releases) and place it on your PATH.

Both Homebrew and the shell installer create a `t` shorthand symlink automatically.

### Uninstall

```bash
# Homebrew
brew uninstall takumi

# Shell installer
curl -fsSL https://raw.githubusercontent.com/T-Fizz/takumi/main/uninstall.sh | sh
```

## Quick Start

```bash
# Create a new project (picks your AI agent interactively)
takumi init --root my-project
cd my-project

# Or initialize in an existing directory
cd existing-project
takumi init
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
- **Multi-agent support** — operator prompt works with Claude, Cursor, Copilot, Windsurf, Cline, and Kiro
- **Source tracking** — clone and sync external git repositories into the workspace
- **Version pinning** — centralized dependency version sets with configurable strategies
- **LLM code review** — `takumi review` runs code review via Anthropic or OpenAI, outputs structured markdown

## The Operator Prompt

During `takumi init`, Takumi generates `.takumi/TAKUMI.md` — a workspace-specific instruction set that teaches AI agents:

- Which commands to use (`takumi build`, not `go build`)
- The recommended workflow (status → affected → build → test)
- When raw tools are appropriate (REPLs, git, no workspace yet)
- How to handle failures (read logs → fix → rebuild)
- Environment management (edit manifest → `takumi env setup`)

This is a static file, not a running service. Any AI agent that reads it gets the same guidance.

## AI Agent Setup

Pass `--agent <name>` during init (or pick interactively). Takumi creates the agent's config file with a pointer to `.takumi/TAKUMI.md`:

| Agent | Config File | Flag |
|-------|-------------|------|
| Claude Code | `CLAUDE.md` | `--agent claude` |
| Cursor | `.cursor/rules` | `--agent cursor` |
| GitHub Copilot | `.github/copilot-instructions.md` | `--agent copilot` |
| Windsurf | `.windsurfrules` | `--agent windsurf` |
| Cline | `.clinerules` | `--agent cline` |
| Kiro | `AGENTS.md` | `--agent kiro` |

All agents get the same operator prompt content. If a config file already exists, Takumi appends the include line without overwriting your existing rules.

## MCP Server

For agents that support the Model Context Protocol (MCP), Takumi exposes workspace operations as tools — no copy-paste needed.

```bash
# Register globally for all Claude Code sessions
takumi mcp install

# Or add .mcp.json to your project root for per-project setup
```

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

Available tools: `takumi_status`, `takumi_build`, `takumi_test`, `takumi_affected`, `takumi_validate`, `takumi_graph`. Build/test output goes to log files — tool results return summaries and paths to keep token usage low.

## Workflow

```bash
t status                      # 1. Understand workspace state
t affected --since main       # 2. Scope what changed
t build --affected            # 3. Build only what changed
t test --affected             # 4. Test only what changed
# On failure → read .takumi/logs/ → fix → repeat from 3
```

## Multi-Package Example

```yaml
# lib/takumi-pkg.yaml
package:
  name: lib
  version: 1.0.0
phases:
  build:
    commands:
      - go build ./...

# api/takumi-pkg.yaml
package:
  name: api
  version: 2.0.0
dependencies:
  - lib
phases:
  build:
    commands:
      - go build -o ../../build/api .
  deploy:
    commands:
      - fly deploy
```

```bash
t graph                       # See dependency order
t build                       # lib builds first, then api
t deploy                      # Any phase is a top-level command
t env setup                   # Set up runtime environments
```

## Documentation

### User Guides

- [Getting Started](docs/user/getting-started.md) — installation, new project setup
- [Onboarding an Existing Project](docs/user/onboarding-existing-project.md) — add Takumi to existing code
- [Commands Reference](docs/user/commands.md) — every command and flag
- [Configuration Reference](docs/user/configuration.md) — all three config file formats

### Developer Docs

- [Architecture](docs/dev/architecture.md) — package structure, data flow, design decisions
- [Package Reference](docs/dev/packages.md) — Go package API reference
- [Testing Guide](docs/dev/testing.md) — unit tests, integration tests, benchmarks
- [Contributing](docs/dev/contributing.md) — build, test, code style

## Building from Source

```bash
git clone https://github.com/T-Fizz/takumi.git
cd takumi
make build          # → ./build/takumi
make install        # → $GOPATH/bin/takumi
make test           # Unit tests
make test-all       # Unit + integration tests
```

Requires Go 1.22+.

## License

[GNU Affero General Public License v3.0 (AGPLv3)](LICENSE) — free to use, modify, and distribute. Derivative works and network-accessible services must release source under the same license.
