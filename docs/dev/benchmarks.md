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

| Model | Without Takumi | With Takumi | Saved | Turns | Tool Calls |
|-------|---------------|-------------|-------|-------|------------|
| **Haiku 4.5** | 128,999 | 24,429 | **81.1%** | 41 / 13 | 69 / 14 |
| **Sonnet 4.6** | 40,627 | 25,415 | **37.4%** | 22 / 13 | 46 / 21 |
| **Opus 4.6** | 45,646 | 30,981 | **32.1%** | 23 / 16 | 49 / 25 |

### Scenarios

#### Fix Build Error

> Find and fix a type error in a Go HTTP handler

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 14,153 | 14,171 | 11,892 | 12,024 | 12,627 | 13,681 |
| Turns | 8 | 7 | 7 | 6 | 7 | 7 |
| Tool calls | 12 | 7 | 12 | 7 | 13 | 7 |
| Time | 12.5s | 9.3s | 16.8s | 18.6s | 21.1s | 22.6s |
| **Saved** | **-0.1%** || **-1.1%** || **-8.3%** ||

#### Scoped Rebuild

> After changing shared lib, build only affected packages

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 97,085 | 6,638 | 20,603 | 5,026 | 22,502 | 6,934 |
| Turns | 25 | 4 | 10 | 3 | 10 | 4 |
| Tool calls | 37 | 4 | 20 | 4 | 21 | 5 |
| Time | 59.1s | 11.1s | 27.3s | 11.2s | 38.3s | 11.5s |
| **Saved** | **93.2%** || **75.6%** || **69.2%** ||

#### Understand Structure

> Explain dependency graph and build order of a 4-package monorepo

| Metric | Haiku 4.5 || Sonnet 4.6 || Opus 4.6 ||
|--------|------|------|------|------|------|------|
| | Without | With | Without | With | Without | With |
| Tokens | 17,761 | 3,620 | 8,132 | 8,365 | 10,517 | 10,366 |
| Turns | 8 | 2 | 5 | 4 | 6 | 5 |
| Tool calls | 20 | 3 | 14 | 10 | 15 | 13 |
| Time | 20.9s | 7.0s | 22.2s | 18.9s | 26.7s | 24.4s |
| **Saved** | **79.6%** || **-2.9%** || **1.4%** ||

---

