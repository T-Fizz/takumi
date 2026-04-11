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

**`promptfooconfig.yaml`** — 18 deterministic test cases across 10 providers. Uses `contains` and `regex` assertions to check that CLI output includes expected strings.

**`promptfooconfig.llm.yaml`** — 9 LLM-graded test cases. Uses Claude Haiku (`anthropic:messages:claude-haiku-4-5-20251001`) to evaluate whether rendered prompts are useful to an AI agent. Each test uses an `llm-rubric` assertion with a qualitative description of what good output looks like.

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
  - id: exec:./run-skill.sh takumi ai diagnose api
    label: diagnose

tests:
  - vars:
      provider_id: diagnose
    assert:
      - type: contains
        value: "Package: api"
      - type: contains
        value: "Root cause category"
```

### Troubleshooting

**"npx not found"** — Install Node.js 18+. Promptfoo runs via `npx --yes promptfoo@latest`.

**Extra arguments in output** — The exec provider appends 3 args. If `run-skill.sh` isn't stripping them, check the `TAKUMI_ARGC` environment variable.

**Template interpolation** — Promptfoo uses Nunjucks templates. If your assertion contains `{{...}}`, use a `regex` assertion with escaped braces (`\\{\\{...\\}\\}`) instead of `contains`.
