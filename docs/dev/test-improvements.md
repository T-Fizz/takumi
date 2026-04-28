# Deferred Test Improvements

Tracking test coverage gaps that were intentionally deferred so they don't get forgotten. Items here were identified during a 2026-04-27 test-quality pass but skipped because they require non-trivial scaffolding (HTTP mocking, OS-level failure injection, etc.) and weren't blocking.

## Deferred

### `src/agent` — HTTP mocking for `chat()`, `Run()`, `httpPostJSON`
- **Why deferred:** Requires an HTTP test server or interface seam in the provider so we can stub responses. Both Anthropic and OpenAI providers currently call `httpPostJSON` directly.
- **Estimated effort:** Medium. Introduce a `httpClient` interface field on each provider, default to a real client, swap in tests.
- **What it would unlock:** `chat()` and `Run()` move from 0% to 90%+. Tool-call loops, error retries, and message-format roundtrips all become testable.

### `src/cache` — symlink cycle handling in `hashDirectory()`
- **Why deferred:** `filepath.Walk` follows symlinks; a cycle (`src → ../`) would loop or fail. Setting up a symlink cycle in a test is fiddly on macOS/Linux differences.
- **Estimated effort:** Medium. Decide the desired behavior first (skip, error, or detect-cycle); then add a fixture under `testdata/`.

### `src/cache` — corrupted/partial-write recovery
- **Why deferred:** Need to simulate a torn write (process killed mid-`os.WriteFile`). Current `TestStore_CorruptData` only verifies `Lookup` returns nil; doesn't verify a subsequent `Write` recovers cleanly.
- **Estimated effort:** Trivial-medium. Truncate the cache file mid-bytes in a test and assert next `Write` succeeds.

### `src/executor` — log file creation failure
- **Why deferred:** Tests don't exercise the `os.Create` error path on `.takumi/logs/<pkg>.log`. Would need a read-only `.takumi/` setup.
- **Estimated effort:** Trivial. `os.Chmod(logsDir, 0500)` in test, verify error before phase runs.

### `src/cli/benchmark.go` — pure helpers (`fmtInt`, `fmtBool`, `findPython`, `loadDotEnv`)
- **Why deferred:** Low priority — these are tiny formatting helpers. Tests would inflate coverage without catching real regressions.
- **Estimated effort:** Trivial if ever wanted.

### `src/cli/mcp.go`, `src/cli/review.go`, `src/cli/docs.go`
- **Why deferred:** All run external commands or LLM calls; would need mocking infrastructure similar to `src/agent`.
- **Estimated effort:** Large. Best tackled together with the agent HTTP refactor.

## Done in this pass

- Pre-phase failure skips Commands and Post (executor)
- Post-phase failure surfaces correct exit code (executor)
- Whitespace-only workspace name treated as empty (config)
- Whitespace-only source URL treated as empty (config)
- Removed low-value `_Cov` tests (`TestAgentByName_Cov`, `TestTakumiMDContent_Cov`)
- Strengthened `AgentByName`/`agentNames`/flag-registration tests to pinpoint exact values
- Replaced 22 `defer os.Chdir` patterns with `t.Cleanup`-based helper in mcp tests
- Fixed empty `tests/integration/promptfoo/fixtures/sample-ws/api/handler.go`
