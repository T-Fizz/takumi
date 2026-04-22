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

**Fix Build Error** — a Go HTTP handler calls `WriteHeader("200")` (string instead of int). The agent must find and fix it.

Correctness checks (4 weighted equally):
1. `go build ./...` passes after the fix
2. The bug was fixed in the correct file (`api/main.go`)
3. The fix is correct (`WriteHeader(200)` or `WriteHeader(http.StatusOK)`)
4. Other packages still compile (no collateral damage)

**Scoped Rebuild** — a 3-package Go monorepo where `shared/` was just modified. The agent must identify affected downstream packages and build only those.

Correctness checks:
1. All packages still compile
2. Agent identified that `api` and `web` depend on `shared`
3. Agent actually scoped its build (used `takumi affected`, `--affected`, or per-package builds instead of a blanket `go build` at root)

**Understand Structure** — a 4-package monorepo with a diamond dependency (core -> auth, core -> api, auth+api -> gateway). The agent must explain the structure and build order.

Correctness checks (5 weighted equally):
1. Identifies `core` as the base package (no dependencies)
2. Identifies `auth` depends on `core`
3. Identifies `api` depends on `core`
4. Identifies `gateway` depends on both `auth` and `api`
5. Correct build order (core first, gateway last)

### How results are produced

Benchmarks run in GitHub Actions CI on every version tag. Three parallel jobs
(one per model) each execute all scenarios, upload per-model JSON artifacts,
then a publish job combines them and commits the results to this page.

Source: [`tests/benchmark/perf/`](https://github.com/T-Fizz/takumi/tree/main/tests/benchmark/perf)

---

<!-- BENCHMARK_INSERT -->

## v1.0.1

> 2026-04-22 | models: Haiku 4.5, Sonnet 4.6, Opus 4.6

### Token Savings by Model

| Model | Without Takumi | With Takumi | Saved | Turns | Tool Calls | Correctness |
|-------|---------------|-------------|-------|-------|------------|-------------|
| **Haiku 4.5** | 130,473 | 38,243 | **70.7%** | 46 / 19 | 78 / 20 | 87% / 100% |
| **Sonnet 4.6** | 35,881 | 25,416 | **29.2%** | 20 / 13 | 44 / 21 | 100% / 100% |
| **Opus 4.6** | 39,847 | 33,297 | **16.4%** | 21 / 17 | 48 / 26 | 100% / 100% |

### Scenarios

#### Fix Build Error

> Find and fix a type error in a Go HTTP handler

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 13,891 | 14,140 | 11,954 | 12,122 | 12,621 | 13,672 |
| Turns | 8 | 7 | 7 | 6 | 7 | 7 |
| Tool calls | 12 | 7 | 12 | 7 | 13 | 7 |
| Time | 12.0s | 12.1s | 19.1s | 17.4s | 22.2s | 22.5s |
| Correctness | 100% | 100% | 100% | 100% | 100% | 100% |
| **Saved** | **-1.8%** || **-1.4%** || **-8.3%** ||

#### Scoped Rebuild

> After changing shared lib, build only affected packages

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 76,620 | 6,551 | 15,663 | 5,004 | 19,188 | 9,238 |
| Turns | 25 | 4 | 8 | 3 | 9 | 5 |
| Tool calls | 31 | 4 | 18 | 4 | 21 | 6 |
| Time | 34.3s | 12.3s | 25.0s | 9.5s | 34.4s | 14.8s |
| Correctness | 60% | 100% | 100% | 100% | 100% | 100% |
| **Saved** | **91.5%** || **68.1%** || **51.9%** ||

#### Understand Structure

> Explain dependency graph and build order of a 4-package monorepo

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 39,962 | 17,552 | 8,264 | 8,290 | 8,038 | 10,387 |
| Turns | 13 | 8 | 5 | 4 | 5 | 5 |
| Tool calls | 35 | 9 | 14 | 10 | 14 | 13 |
| Time | 43.1s | 22.2s | 22.9s | 17.0s | 26.3s | 21.5s |
| Correctness | 100% | 100% | 100% | 100% | 100% | 100% |
| **Saved** | **56.1%** || **-0.3%** || **-29.2%** ||

---

