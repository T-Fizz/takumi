# Testing Guide

## Unit Tests

Each package in `src/` has `_test.go` files alongside the source. Run all unit tests:

```bash
make test
```

Or run tests for a specific package:

```bash
go test ./src/config/...
go test ./src/graph/...
go test ./src/cache/...
```

Generate a coverage report:

```bash
make cover
```

## Integration Tests

Integration tests live in `tests/integration/` and use [promptfoo](https://www.promptfoo.dev/) to evaluate Takumi's CLI output against assertions.

### Fixture Workspace

Tests run against a self-contained fixture workspace at `tests/integration/fixtures/sample-ws/`. This fixture includes:

- `takumi.yaml` — workspace config with sources, version-set, AI agent
- `api/takumi-pkg.yaml` — package with dependencies, runtime, pre/post steps
- `shared-lib/takumi-pkg.yaml` — leaf dependency
- `.takumi/logs/api.build.log` — sample build failure log
- `.takumi/metrics.json` — sample build telemetry
- `.takumi/TAKUMI.md` — AI workspace instructions
- `takumi-versions.yaml` — version set config
- `.git/` — initialized git repo with a committed file and an uncommitted change

### Running

**Deterministic tests** (no API key needed):

```bash
make integration-test
```

**LLM-graded tests** (requires `ANTHROPIC_API_KEY`):

```bash
export ANTHROPIC_API_KEY=sk-ant-...
make integration-test-llm
```

**All tests** (unit + integration + LLM):

```bash
make test-all
```

### Test Structure

There are two promptfoo config files:

**`promptfooconfig.yaml`** — 33 deterministic test cases across 5 providers (status, graph, validate, dry-run, operator). Uses `contains` and `regex` assertions to check CLI output and operator prompt content.

**`promptfooconfig.llm.yaml`** — 18 LLM-graded test cases. Uses Claude Haiku (`anthropic:messages:claude-haiku-4-5-20251001`) to evaluate prompt quality, CLI output parseability, and whether the operator prompt steers agents to correct Takumi workflows. Each test uses an `llm-rubric` assertion with a scenario-based rubric.

### Exec Provider

Promptfoo's `exec:` provider runs `tests/integration/run-skill.sh`, which:

1. Strips the 3 extra arguments promptfoo appends (prompt text, provider JSON, test JSON)
2. Changes to the fixture directory
3. Runs the Takumi binary with the remaining arguments
4. Captures stdout + stderr

Environment variables:
- `TAKUMI_BIN` — path to the Takumi binary (set by Makefile)
- `FIXTURE_DIR` — path to the fixture workspace (set in promptfoo config)

### Adding a Test

1. Add a provider in the `providers` section if the command isn't already covered
2. Add a test under `tests` with `vars.provider_id` matching your provider
3. Use `contains` for exact substring matching, `regex` for patterns
4. For LLM-graded tests, write a rubric that describes what a useful prompt looks like — don't test exact wording

Example:

```yaml
providers:
  - id: exec:bash run-skill.sh status
    label: status

tests:
  - description: "status: shows workspace name"
    vars:
      test_name: status-name
    providers: [status]
    assert:
      - type: contains
        value: "sample-platform"
```

## E2E Simulation Tests

End-to-end tests in `src/mcp/e2e_test.go` simulate full agent workflows by calling MCP tool handlers in sequence against real (temporary) workspaces. These test the complete cycle from workspace setup through build/test/diagnose/fix iterations.

### Scenarios

**TestE2E_LocalProject** (18 steps) — A developer with an existing local project: init → status → validate → graph → build → test → cache verification → feature change → affected analysis → targeted build → test failure → diagnose → fix → green build → metrics check → final status.

**TestE2E_GitHubClone** (13 steps) — A developer cloning a multi-package project from GitHub: 3-package workspace with sources → onboard → graph → validate → full build → full test → modify lib → blast radius analysis → affected build → targeted test → full test → cache → final status.

**TestE2E_VibeCoder** (20 steps) — A first-time user with no coding experience: empty dir → error handling → scaffold workspace + 3 packages with dependencies → full workflow through build/test/failure/diagnose/fix cycle → ship build → build history verification.

### Running

E2E tests run as part of the standard test suite:

```bash
go test ./src/mcp/ -run TestE2E -v
```

Each scenario creates a temporary directory, builds a real workspace with config files, and calls tool handlers directly (no network). Tests verify tool output text, error conditions, and state changes across the full workflow.

### Troubleshooting

**"npx not found"** — Install Node.js 18+. Promptfoo runs via `npx --yes promptfoo@latest`.

**Extra arguments in output** — The exec provider appends 3 args. If `run-skill.sh` isn't stripping them, check the `TAKUMI_ARGC` environment variable.

**Template interpolation** — Promptfoo uses Nunjucks templates. If your assertion contains `{{...}}`, use a `regex` assertion with escaped braces (`\\{\\{...\\}\\}`) instead of `contains`.
