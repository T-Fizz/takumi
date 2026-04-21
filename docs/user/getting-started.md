# Getting Started

## Installation

```bash
go install github.com/tfitz/takumi@latest
```

Or build from source:

```bash
git clone https://github.com/T-Fizz/takumi.git
cd takumi
make build        # → ./build/takumi
make install      # → $GOPATH/bin/takumi
```

Requires Go 1.26+.

### Shell Alias

Add a short alias to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
alias t='takumi'
```

Then use `t` anywhere you'd use `takumi`:

```bash
t init my-project
t build
t status
```

## Create a New Project

```bash
takumi init --root my-project
cd my-project
```

This creates:

```
my-project/
├── .takumi/              # Workspace marker
│   └── TAKUMI.md         # AI workspace instructions
├── takumi.yaml           # Workspace config
└── takumi-pkg.yaml       # Root package config
```

During init, Takumi asks which AI agent you use (Claude, Cursor, Copilot, Windsurf, Cline, Kiro) and creates the appropriate config file. Pass `--agent claude` to skip the prompt.

## Edit Your Package Config

Open `takumi-pkg.yaml` and replace the placeholder commands:

```yaml
package:
  name: my-project
  version: 0.1.0
phases:
  build:
    commands:
      - go build ./...           # your real build command
  test:
    commands:
      - go test ./...            # your real test command
```

Phases can be anything — `build`, `test`, `lint`, `deploy`, `bundle`, whatever your project needs. Each phase supports `pre`, `commands`, and `post` steps.

## Build and Test

```bash
takumi build                     # Build in dependency order
takumi test                      # Run tests
takumi status                    # See workspace dashboard
```

## MCP Setup (Claude Code)

If you use Claude Code, Takumi can be operated directly by the AI agent via the MCP server. Add a `.mcp.json` file to your project root:

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

Once configured, Claude Code can call `takumi_build`, `takumi_test`, `takumi_affected`, and other tools directly. See [Commands Reference](commands.md) for the full tool list.

## Next Steps

- [Onboarding an Existing Project](onboarding-existing-project.md) — add Takumi to code you already have
- [Commands Reference](commands.md) — every command and flag
- [Configuration Reference](configuration.md) — all three config file formats
- [AI Agent Setup](ai-agent-setup.md) — supported agents and MCP integration
