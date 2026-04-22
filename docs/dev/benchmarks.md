# Performance Benchmarks

Automated results from `takumi benchmark` — comparing AI agent efficiency
with and without Takumi operator instructions across identical tasks.

**Models:** Haiku 4.5, Sonnet 4.6, Opus 4.6 | **Max turns:** 25 | Configurable via `BENCH_MODEL`

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

Each scenario runs in **three languages** (Go, Python, TypeScript) to validate
that Takumi is genuinely language-agnostic — 9 scenarios total per model.

**Fix Build Error** — a type error that prevents the project from building.
The agent must find the bug, fix it, and verify the build passes.

| Language | Bug |
|----------|-----|
| Go | `WriteHeader("200")` — string instead of int |
| Python | `"Server on port " + port` — str + int TypeError |
| TypeScript | `const port: number = "8080"` — type mismatch caught by tsc |

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
(one per model) each execute all 9 scenarios (3 tasks x 3 languages x 2 modes
= 18 agent conversations per model), upload per-model JSON artifacts, then a
publish job combines them and commits the results to this page.

Source: [`tests/benchmark/perf/`](https://github.com/T-Fizz/takumi/tree/main/tests/benchmark/perf)

---

<!-- BENCHMARK_INSERT -->

## v1.0.1

> 2026-04-22 | models: Haiku 4.5, Sonnet 4.6, Opus 4.6

### Token Savings by Model

| Model | Without Takumi | With Takumi | Saved | Turns | Tool Calls | Correctness |
|-------|---------------|-------------|-------|-------|------------|-------------|
| **Haiku 4.5** | 315,321 | 301,548 | **4.4%** | 117 / 99 | 199 / 148 | 96% / 100% |
| **Sonnet 4.6** | 132,515 | 281,952 | **-112.8%** | 65 / 89 | 149 / 160 | 100% / 100% |
| **Opus 4.6** | 138,596 | 235,431 | **-69.9%** | 65 / 79 | 153 / 160 | 100% / 97% |

### Scenarios

#### Fix Build Error

> Find and fix a type error

| Language | Haiku 4.5 ||| Sonnet 4.6 ||| Opus 4.6 |||
|----------|------|------|------|------|------|------|------|------|------|
| | Without | With | Saved | Without | With | Saved | Without | With | Saved |
| **Go** | 18,239 | 14,336 | **21.4%** | 11,983 | 12,016 | **-0.3%** | 12,606 | 14,364 | **-13.9%** |
| **Python** | 16,369 | 60,956 | **-272.4%** | 11,633 | 65,167 | **-460.2%** | 11,523 | 45,248 | **-292.7%** |
| **TypeScript** | 64,207 | 52,688 | **17.9%** | 13,157 | 51,348 | **-290.3%** | 20,567 | 38,270 | **-86.1%** |

**Correctness** (without / with):

| Language | Haiku 4.5 | Sonnet 4.6 | Opus 4.6 |
|----------|------|------|------|
| Go | 100% / 100% | 100% / 100% | 100% / 100% |
| Python | 100% / 100% | 100% / 100% | 100% / 100% |
| TypeScript | 100% / 100% | 100% / 100% | 100% / 100% |

#### Scoped Rebuild

> After changing shared lib, build only affected packages

| Language | Haiku 4.5 ||| Sonnet 4.6 ||| Opus 4.6 |||
|----------|------|------|------|------|------|------|------|------|------|
| | Without | With | Saved | Without | With | Saved | Without | With | Saved |
| **Go** | 84,263 | 6,561 | **92.2%** | 16,135 | 4,988 | **69.1%** | 21,085 | 7,457 | **64.6%** |
| **Python** | 34,162 | 75,394 | **-120.7%** | 15,101 | 62,769 | **-315.7%** | 21,720 | 51,436 | **-136.8%** |
| **TypeScript** | 56,062 | 56,809 | **-1.3%** | 33,109 | 47,229 | **-42.6%** | 30,420 | 45,063 | **-48.1%** |

**Correctness** (without / with):

| Language | Haiku 4.5 | Sonnet 4.6 | Opus 4.6 |
|----------|------|------|------|
| Go | 60% / 100% | 100% / 100% | 100% / 100% |
| Python | 100% / 100% | 100% / 100% | 100% / 70% |
| TypeScript | 100% / 100% | 100% / 100% | 100% / 100% |

#### Understand Structure

> Explain dependency graph and build order of a monorepo

| Language | Haiku 4.5 ||| Sonnet 4.6 ||| Opus 4.6 |||
|----------|------|------|------|------|------|------|------|------|------|
| | Without | With | Saved | Without | With | Saved | Without | With | Saved |
| **Go** | 22,995 | 20,207 | **12.1%** | 16,966 | 14,859 | **12.4%** | 6,343 | 11,055 | **-74.3%** |
| **Python** | 6,910 | 6,510 | **5.8%** | 6,509 | 14,994 | **-130.4%** | 6,406 | 11,116 | **-73.5%** |
| **TypeScript** | 12,114 | 8,087 | **33.2%** | 7,922 | 8,582 | **-8.3%** | 7,926 | 11,422 | **-44.1%** |

**Correctness** (without / with):

| Language | Haiku 4.5 | Sonnet 4.6 | Opus 4.6 |
|----------|------|------|------|
| Go | 100% / 100% | 100% / 100% | 100% / 100% |
| Python | 100% / 100% | 100% / 100% | 100% / 100% |
| TypeScript | 100% / 100% | 100% / 100% | 100% / 100% |

---

