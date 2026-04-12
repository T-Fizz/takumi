# Contributing

## Requirements

- Go 1.26+
- Node.js 18+ (for integration tests only)

## Build

```bash
make build          # → ./build/takumi
make install        # → $GOPATH/bin/takumi
```

## Test

```bash
make test                  # Unit tests
make integration-test      # Deterministic integration tests
make integration-test-llm  # LLM-graded tests (needs ANTHROPIC_API_KEY)
make test-all              # Everything
make cover                 # Coverage report
make clean                 # Remove build artifacts
```

## Project Layout

```
cmd/takumi/main.go           Entry point
src/cli/                     Cobra commands (one file per command group)
src/config/                  YAML config parsing + validation
src/workspace/               Workspace detection + package discovery
src/graph/                   Dependency DAG + topological sort
src/cache/                   Content-addressed build cache
src/executor/                Phase execution + parallelism
src/mcp/                     MCP server (Model Context Protocol)
src/skills/                  AI skill templates
src/skills/builtin/          Embedded YAML skills
src/ui/                      Terminal styling
tests/integration/           Promptfoo integration tests
docs/user/                   User-facing documentation
docs/dev/                    Developer documentation
```

## Adding a CLI Command

1. Create a new file in `src/cli/` (e.g., `mycommand.go`)
2. Define the Cobra command and register it in an `init()` function
3. Call `requireWorkspace()` if the command needs workspace context
4. Add tests alongside (`mycommand_test.go`)

## Adding a Built-in Skill

1. Create a YAML file in `src/skills/builtin/`
2. Follow the schema: `skill.name`, `skill.description`, `skill.prompt`, optional `auto_context` and `max_tokens`
3. Use `{{variable}}` placeholders for context substitution
4. Add a command or integration point in `src/cli/ai.go`
5. Add integration tests in `tests/integration/`

## Adding an MCP Tool

1. Define the tool in `src/mcp/tools.go` using the `gomcp.NewTool()` builder:

```go
var myTool = gomcp.NewTool("takumi_mytool",
    gomcp.WithDescription("What this tool does."),
    gomcp.WithString("param", gomcp.Required(), gomcp.Description("...")),
    gomcp.WithBoolean("flag", gomcp.Description("...")),
)
```

2. Write the handler function following the [handler pattern](packages.md#handler-pattern):

```go
func handleMyTool(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
    // Load workspace, parse params, do work, return result
}
```

3. Register it in `registerTools()` at the top of `tools.go`:

```go
s.AddTool(myTool, handleMyTool)
```

4. Add tests in `src/mcp/tools_test.go` — cover success, error, and edge cases
5. Update `CODEBASE.md` and `docs/user/commands.md` with the new tool

Key rules:
- Use `gomcp.NewToolResultError()` for failures the agent should see, not Go `error`
- Set `executor.RunOptions.Quiet = true` on any executor calls
- Return file paths for large output, not inline content
- Use the `gomcp` alias for `github.com/mark3labs/mcp-go/mcp` (avoids collision with `package mcp`)

## Code Style

- Follow standard Go conventions (`go fmt`, `go vet`)
- No external test frameworks — use the standard `testing` package
- Keep commands thin — business logic belongs in `src/` packages, not `src/cli/`
- Prefer returning errors over `log.Fatal` in library code

## License

[GNU Affero General Public License v3.0 (AGPLv3)](../../LICENSE) — contributions are accepted under the same license.
