# AI Agent Setup

Takumi generates an operator prompt (`.takumi/TAKUMI.md`) that teaches AI agents how to operate your workspace — which commands to use, the recommended workflow, when raw tools are appropriate, and how to handle failures.

## Agent Configuration

During `takumi init`, Takumi asks which AI agent you use and creates the appropriate config file:

| Agent | Config File | Flag |
|-------|-------------|------|
| Claude Code | `CLAUDE.md` | `--agent claude` |
| Cursor | `.cursor/rules` | `--agent cursor` |
| GitHub Copilot | `.github/copilot-instructions.md` | `--agent copilot` |
| Windsurf | `.windsurfrules` | `--agent windsurf` |
| Cline | `.clinerules` | `--agent cline` |
| Kiro | `AGENTS.md` | `--agent kiro` |

Pass `--agent <name>` to skip the interactive prompt. Use `--agent none` to skip agent setup entirely (`.takumi/TAKUMI.md` is still created).

All agent config files contain the same include line:

```
Read .takumi/TAKUMI.md for Takumi build tool instructions.
```

If a config file already exists (e.g., you have custom rules in `CLAUDE.md`), Takumi appends the include line without overwriting your content.

## The Operator Prompt

`.takumi/TAKUMI.md` is a static markdown file generated during `takumi init`. It contains:

- **Command reference** — every Takumi command with a one-line description
- **Workflow** — numbered steps: status → affected → build → test → fix
- **When NOT to use raw commands** — guides agents to use `takumi build` instead of `go build`, `takumi lint` instead of `eslint`, etc.
- **When raw tools ARE appropriate** — REPLs, git operations, user-explicit requests, pre-init state
- **Config locations** — where to find `takumi.yaml`, `takumi-pkg.yaml`, etc.
- **Rules** — never build with raw commands, use `takumi checkout` not `git clone`, check `takumi affected` before building

The prompt is workspace-specific (includes the workspace name) but otherwise identical across all agent types.

## MCP Server Integration

For AI agents that support the Model Context Protocol (MCP), Takumi provides a built-in server that exposes workspace operations as tools — the most direct integration.

```bash
takumi mcp serve    # Start over stdio
```

Available tools: `takumi_status`, `takumi_build`, `takumi_test`, `takumi_affected`, `takumi_validate`, `takumi_graph`. Build and test output goes to `.takumi/logs/` files — tool results return summaries and file paths to keep token usage low.

### Per-Project Setup

Add `.mcp.json` to your project root:

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

### Global Setup

Register Takumi once for all Claude Code sessions:

```bash
takumi mcp install
```

This writes to `~/.claude/claude_desktop_config.json` so Takumi tools are available everywhere — even in projects that haven't run `takumi init` yet.

## LLM Code Review

`takumi review` runs a thorough code review of uncommitted changes using an LLM (Anthropic or OpenAI). Output is structured markdown saved to `.takumi/reviews/`.

```bash
takumi review                            # Uses default provider
takumi review --provider openai          # Use OpenAI
takumi review --model claude-sonnet-4-6  # Override model
```

Set `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` in your environment.
