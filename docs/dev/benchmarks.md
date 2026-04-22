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
| **Haiku 4.5** | 137,911 | 31,723 | **77.0%** | 50 / 15 | 72 / 28 | 87% / 67% |
| **Sonnet 4.6** | 50,188 | 25,287 | **49.6%** | 24 / 13 | 49 / 21 | 100% / 100% |
| **Opus 4.6** | 35,989 | 33,557 | **6.8%** | 20 / 17 | 43 / 26 | 100% / 100% |

### Scenarios

#### Fix Build Error

> Find and fix a type error in a Go HTTP handler

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 13,869 | 12,879 | 11,997 | 11,983 | 12,600 | 13,676 |
| Turns | 8 | 6 | 7 | 6 | 7 | 7 |
| Tool calls | 12 | 7 | 12 | 7 | 13 | 7 |
| Time | 9.8s | 10.4s | 17.7s | 15.0s | 23.3s | 25.1s |
| Correctness | 100% | 100% | 100% | 100% | 100% | 100% |
| **Saved** | **7.1%** || **0.1%** || **-8.5%** ||

#### Scoped Rebuild

> After changing shared lib, build only affected packages

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 72,203 | 5,106 | 30,041 | 4,992 | 15,363 | 9,260 |
| Turns | 25 | 3 | 12 | 3 | 8 | 5 |
| Tool calls | 30 | 4 | 23 | 4 | 16 | 6 |
| Time | 31.5s | 7.8s | 35.8s | 9.7s | 30.3s | 19.9s |
| Correctness | 60% | 100% | 100% | 100% | 100% | 100% |
| **Saved** | **92.9%** || **83.4%** || **39.7%** ||

#### Understand Structure

> Explain dependency graph and build order of a 4-package monorepo

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 51,839 | 13,738 | 8,150 | 8,312 | 8,026 | 10,621 |
| Turns | 17 | 6 | 5 | 4 | 5 | 5 |
| Tool calls | 30 | 17 | 14 | 10 | 14 | 13 |
| Time | 60.7s | 11.1s | 22.4s | 17.5s | 22.5s | 25.3s |
| Correctness | 100% | 0% | 100% | 100% | 100% | 100% |
| **Saved** | **73.5%** || **-2.0%** || **-32.3%** ||

---


