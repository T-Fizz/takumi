# Performance Benchmarks

Automated results from `takumi benchmark` — comparing AI agent efficiency
with and without Takumi operator instructions across identical tasks.

**Models:** Haiku 4.5, Sonnet 4.6, Opus 4.6 | **Languages:** Go, Python, TypeScript, Rust, Java | **Max turns:** 25

See also: [`takumi benchmark`](../user/commands/takumi_benchmark.md) | [`takumi benchmark iterate`](../user/commands/takumi_benchmark_iterate.md)

---

## Methodology

Each scenario runs an AI agent loop twice on an identical task: once in a plain
workspace (**without Takumi**) and once in a workspace configured with
`takumi.yaml`, `takumi-pkg.yaml`, and operator instructions (**with Takumi**).

### What varies between runs

| | Without Takumi | With Takumi |
|---|---|---|
| Source code | identical | identical |
| Task prompt | identical | identical |
| Available tools | `run_command`, `read_file`, `write_file`, `list_files`, `task_complete` | same |
| Workspace config | none | `takumi.yaml` + per-package `takumi-pkg.yaml` |
| Operator instructions | none | ~400-token prompt teaching Takumi workflow |
| PATH | system only | system + `takumi` binary |

### What we measure

- **Tokens** — total input + output tokens consumed across the full conversation
- **Turns** — number of API round-trips (max 25)
- **Tool calls** — number of tool invocations
- **Time** — wall-clock seconds for the full agent loop
- **Correctness** — scenario-specific verification (0–100%), detailed below

### Scenarios

Each scenario runs in **five languages** (Go, Python, TypeScript, Rust, Java)
to validate that Takumi is genuinely language-agnostic — 15 scenarios total per
model.

**Fix Build Error** — a type error that prevents the project from building.
The agent must find the bug, fix it, and verify the build passes.

| Language | Bug |
|----------|-----|
| Go | `WriteHeader("200")` — string instead of int |
| Python | `"Server on port " + port` — str + int TypeError |
| TypeScript | `const port: number = "8080"` — type mismatch caught by tsc |
| Rust | `let port: u16 = "8080"` — mismatched types |
| Java | `int port = "8080"` — incompatible types |

Correctness checks:
1. Build/run passes after the fix
2. The bug was fixed in the correct file
3. The fix is correct (proper type conversion or assignment)
4. Other packages still compile (no collateral damage)

**Scoped Rebuild** — a 3-package monorepo where `shared/` was just modified.
The agent must identify affected downstream packages and build only those.

Correctness checks:
1. All packages still compile/run
2. Agent identified that `api` and `web` depend on `shared`
3. Agent actually scoped its work (used `takumi affected`, `--affected`, or
   per-package builds instead of rebuilding everything)

**Understand Structure** — a 4-package monorepo with a diamond dependency
(core -> auth, core -> api, auth+api -> gateway). The agent must explain the
structure and build order.

Correctness checks (5 weighted equally):
1. Identifies `core` as the base package (no dependencies)
2. Identifies `auth` depends on `core`
3. Identifies `api` depends on `core`
4. Identifies `gateway` depends on both `auth` and `api`
5. Correct build order (core first, gateway last)

### How results are produced

Benchmarks run in GitHub Actions CI on every version tag. Three parallel jobs
(one per model) each execute all 15 scenarios (3 tasks x 5 languages x 2 modes
= 30 agent conversations per model), upload per-model JSON artifacts, then a
publish job combines them and commits the results to this page.

Source: [`tests/benchmark/perf/`](https://github.com/T-Fizz/takumi/tree/main/tests/benchmark/perf)

---

<!-- BENCHMARK_INSERT -->

## v1.0.2

> 2026-04-22 | models: Haiku 4.5, Sonnet 4.6, Opus 4.6

### Token Savings by Model

| Model | Without Takumi | With Takumi | Saved | Turns | Tool Calls | Correctness |
|-------|---------------|-------------|-------|-------|------------|-------------|
| **Haiku 4.5*** | 0 | 0 | **—** | 0 / 0 | 0 / 0 | 0% / 0% |
| **Sonnet 4.6*** | 0 | 0 | **—** | 0 / 0 | 0 / 0 | 0% / 0% |
| **Opus 4.6*** | 0 | 0 | **—** | 0 / 0 | 0 / 0 | 0% / 0% |

*Haiku 4.5 ran 0/15 scenarios; Sonnet 4.6 ran 0/15 scenarios; Opus 4.6 ran 0/15 scenarios (cost control)*

### Scenarios

#### Fix Build Error

> Find and fix a type error

| Language | Haiku 4.5 ||| Sonnet 4.6 ||| Opus 4.6 |||
|----------|------|------|------|------|------|------|------|------|------|
| | Without | With | Saved | Without | With | Saved | Without | With | Saved |
| **Go** | — | — | — | — | — | — | — | — | — |
| **Python** | — | — | — | — | — | — | — | — | — |
| **TypeScript** | — | — | — | — | — | — | — | — | — |
| **Rust** | — | — | — | — | — | — | — | — | — |
| **Java** | — | — | — | — | — | — | — | — | — |

**Correctness** (without / with):

| Language | Haiku 4.5 | Sonnet 4.6 | Opus 4.6 |
|----------|------|------|------|
| Go | — | — | — |
| Python | — | — | — |
| TypeScript | — | — | — |
| Rust | — | — | — |
| Java | — | — | — |

#### Scoped Rebuild

> After changing shared lib, build only affected packages

| Language | Haiku 4.5 ||| Sonnet 4.6 ||| Opus 4.6 |||
|----------|------|------|------|------|------|------|------|------|------|
| | Without | With | Saved | Without | With | Saved | Without | With | Saved |
| **Go** | — | — | — | — | — | — | — | — | — |
| **Python** | — | — | — | — | — | — | — | — | — |
| **TypeScript** | — | — | — | — | — | — | — | — | — |
| **Rust** | — | — | — | — | — | — | — | — | — |
| **Java** | — | — | — | — | — | — | — | — | — |

**Correctness** (without / with):

| Language | Haiku 4.5 | Sonnet 4.6 | Opus 4.6 |
|----------|------|------|------|
| Go | — | — | — |
| Python | — | — | — |
| TypeScript | — | — | — |
| Rust | — | — | — |
| Java | — | — | — |

#### Understand Structure

> Explain dependency graph and build order of a monorepo

| Language | Haiku 4.5 ||| Sonnet 4.6 ||| Opus 4.6 |||
|----------|------|------|------|------|------|------|------|------|------|
| | Without | With | Saved | Without | With | Saved | Without | With | Saved |
| **Go** | — | — | — | — | — | — | — | — | — |
| **Python** | — | — | — | — | — | — | — | — | — |
| **TypeScript** | — | — | — | — | — | — | — | — | — |
| **Rust** | — | — | — | — | — | — | — | — | — |
| **Java** | — | — | — | — | — | — | — | — | — |

**Correctness** (without / with):

| Language | Haiku 4.5 | Sonnet 4.6 | Opus 4.6 |
|----------|------|------|------|
| Go | — | — | — |
| Python | — | — | — |
| TypeScript | — | — | — |
| Rust | — | — | — |
| Java | — | — | — |

---


