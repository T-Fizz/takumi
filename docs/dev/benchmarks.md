# Performance Benchmarks

Automated results from `takumi benchmark` — comparing AI agent efficiency
with and without Takumi operator instructions across identical tasks.

**Models:** Haiku 4.5, Sonnet 4.6, Opus 4.6 | **Max turns:** 25 | Configurable via `BENCH_MODEL`

See also: [`takumi benchmark`](../user/commands/takumi_benchmark.md) | [`takumi benchmark iterate`](../user/commands/takumi_benchmark_iterate.md)

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

