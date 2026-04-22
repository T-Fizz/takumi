# Performance Benchmarks

Automated results from `takumi benchmark` — comparing AI agent efficiency
with and without Takumi operator instructions across identical tasks.

**Model:** `claude-haiku-4-5-20251001` | **Max turns:** 25

---

<!-- BENCHMARK_INSERT -->

## v1.0.1

> 2026-04-21

### Overall

| Metric | Without Takumi | With Takumi | Saved |
|--------|---------------|-------------|-------|
| **Tokens** | 142,587 | 35,991 | **74.8%** |
| Turns | 51 | 17 | 66.7% |
| Tool calls | 76 | 28 | 63.2% |
| Errors | 10 | 5 | 50.0% |

### Scenarios

#### Fix Build Error

> Find and fix a type error in a Go HTTP handler

| Metric | Without | With Takumi | Saved |
|--------|---------|-------------|-------|
| Tokens | 16,075 | 14,321 | 10.9% |
| Time | 12.6s | 10.7s | 15.1% |
| Turns | 9 | 7 | 22.2% |
| Tool calls | 13 | 7 | 46.2% |
| Completed | yes | yes | |

#### Scoped Rebuild

> After changing shared lib, build only affected packages

| Metric | Without | With Takumi | Saved |
|--------|---------|-------------|-------|
| Tokens | 71,972 | 5,167 | 92.8% |
| Time | 38.2s | 5.4s | 85.8% |
| Turns | 25 | 3 | 88.0% |
| Tool calls | 31 | 4 | 87.1% |
| Completed | no | yes | |

#### Understand Structure

> Explain dependency graph and build order of a 4-package monorepo

| Metric | Without | With Takumi | Saved |
|--------|---------|-------------|-------|
| Tokens | 54,540 | 16,503 | 69.7% |
| Time | 62.6s | 16.7s | 73.3% |
| Turns | 17 | 7 | 58.8% |
| Tool calls | 32 | 17 | 46.9% |
| Completed | yes | yes | |
