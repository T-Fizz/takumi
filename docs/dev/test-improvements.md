# Deferred Test Improvements

Tracking test coverage gaps that were intentionally deferred so they don't get forgotten. Items here were identified during a 2026-04-27 test-quality pass but skipped because they require non-trivial scaffolding (HTTP mocking, OS-level failure injection, etc.) and weren't blocking.

## Still deferred

### `src/cli/benchmark.go`, `src/cli/docs.go`, `src/cli/mcp.go`, `src/cli/review.go`
- **Why deferred:** All run external commands (Python scripts, `go doc`, MCP install, LLM calls). The agent HTTP-mocking pattern (package-level var swap) could be applied to each `exec.Command` call, but it's a separate refactor per command and the value is lower than core path coverage.
- **Estimated effort:** Medium per file. Best done as a unified "exec mocking" pass.
- **What it would unlock:** ~30 functions currently at 0%. Total cli coverage would jump from ~55% to ~85%.

### `src/cli/init.go` — interactive `huh` prompts
- **Why deferred:** `promptAgentSelection` is already mocked in tests via package-level var. Other interactive paths use `huh.Run()` directly. Refactoring isn't blocking anything.
- **Estimated effort:** Trivial.

### `cmd/takumi/main.go` — entrypoint
- **Why deferred:** Only calls `cli.Execute()`. Single line, hard to test in isolation, near-zero regression risk.
- **Estimated effort:** Skip permanently.

## Done in this pass (2026-04-27)

### Coverage uplift
- **agent: 36% → 97%** — full HTTP mocking via package-level `httpPostJSON` var swap. Both providers' `chat()` and the `Run()` orchestration loop now covered including: tool-call dispatch, completion-tool handling, unknown-tool fallback, tool error forwarding, MaxTurns enforcement, OnToolCall callback, transport-error wrapping, unsupported-provider fail-fast.
- **executor: 95.4% → 97.1%** — log-file creation failure path covered.
- **cache: edge cases added** — corrupt-entry recovery, write-creates-dir, clean-removes-dir, symlink-followed, dangling-symlink-tolerated.
- **graph (cli): cycle error path + --phases flag rendering covered.

### Pinpoint assertion strengthening
- Pre-phase failure skips Commands and Post (executor)
- Post-phase failure surfaces correct exit code (executor)
- Whitespace-only workspace name treated as empty (config)
- Whitespace-only source URL treated as empty (config)
- Removed low-value `_Cov` tests (`TestAgentByName_Cov`, `TestTakumiMDContent_Cov`)
- Strengthened `AgentByName`/`agentNames`/flag-registration tests to pinpoint exact values
- Replaced 22 `defer os.Chdir` patterns with `t.Cleanup`-based helper in mcp tests
- Strengthened `TestRunSync_HandlesBadURL` to verify failure count appears in output
- Fixed empty `tests/integration/promptfoo/fixtures/sample-ws/api/handler.go`
