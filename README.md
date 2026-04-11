# 匠 Takumi

An AI-aware, language-agnostic package builder.

Takumi runs user-defined shell commands, manages optional per-package runtime environments, builds a dependency DAG for parallel execution, and ships with an AI skills system that teaches AI assistants how to operate your workspace.

## Quick Start

```bash
# Install
go install github.com/tfitz/takumi@latest

# Create a new project
takumi init my-project
cd my-project

# Or initialize in an existing directory
cd existing-project
takumi init
```

Edit `takumi-pkg.yaml` with your build commands, then:

```bash
takumi build                  # Build in dependency order
takumi test                   # Run tests
takumi status                 # Workspace dashboard
```

Unchanged packages are automatically skipped via content-addressed caching.

## Features

- **User-defined phases** — `build`, `test`, `lint`, `deploy`, or any custom phase with shell commands
- **Dependency DAG** — packages declare dependencies; Takumi auto-resolves build order
- **Parallel execution** — independent packages run concurrently within each dependency level
- **Content-addressed caching** — SHA-256 of source files + config + dependency keys; incremental builds
- **Runtime isolation** — optional per-package environments (virtualenv, nvm, etc.) with `{{env_dir}}` substitution
- **AI skills** — prompt templates for Claude, Cursor, Copilot, Windsurf, and Cline
- **Source tracking** — clone and sync external git repositories into the workspace
- **Version pinning** — centralized dependency version sets with configurable strategies

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

## Building from Source

```bash
git clone https://github.com/tfitz/takumi.git
cd takumi
make build          # → ./build/takumi
make install        # → $GOPATH/bin/takumi
make test           # Unit tests
make test-all       # Unit + integration + LLM-graded tests
```

Requires Go 1.26+.

## License

[PolyForm Noncommercial 1.0.0](LICENSE) — free for personal, educational, research, and nonprofit use. Not for commercial purposes.
