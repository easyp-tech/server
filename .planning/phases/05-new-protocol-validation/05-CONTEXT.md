# Phase 5: New Protocol Validation - Context

**Gathered:** 2026-05-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Confirm buf v1.69.0+ commands work against the proxy, and discover/fix any required new RPC implementations. This phase writes new protocol integration tests, captures what RPCs the modern buf CLI actually calls via verbose logging, and fixes blockers discovered during testing. The existing smoke test already has a v1.69.0 subtest — this phase builds on that foundation to address the known content-type blocker and validate both `buf mod update` (NEW-01) and `buf dep update` (NEW-02).

</domain>

<decisions>
## Implementation Decisions

### Content-type fix strategy
- **D-01:** Investigate first — run the existing smoke test v1.69.0 subtest with verbose logging to capture the actual RPC request/response exchange. Determine root cause before applying a fix (the `rootHandler` returns `text/plain` but the connect-go RPC handlers should handle content types automatically — the issue may not be where we think it is).
- **D-02:** Capture diagnostics via debug logging — set proxy log level to `debug` in tests and include full server subprocess output in failure messages. No custom middleware needed.
- **D-03:** Fix immediately — investigate and fix in the same plan. Don't separate investigation and fix into different plans.

### New RPC discovery approach
- **D-04:** Empirical discovery — write tests, run them, capture server debug logs showing what RPCs the v1.69.0 CLI actually calls. Implement only what's needed based on actual behavior, not speculation.
- **D-05:** Unimplemented first — all unimplemented RPCs return `CodeUnimplemented`. If the CLI tolerates them (optional calls), no fix needed. Only implement RPCs that block the CLI from succeeding.

### buf dep update test approach
- **D-06:** Test the real `buf dep update` command — v1.69.0 has an actual `buf dep update` command (unlike v1.30.1). Test the real user workflow, not the Phase 4 workaround of two-step `buf mod update`.
- **D-07:** Add a `RunBufDepUpdate` helper to `e2e/testutil/` — follows the established pattern of splitting helpers by concern (parallel to existing `RunBufModUpdate`).

### Claude's Discretion
- Whether the content-type fix requires connect-go configuration changes, middleware, or handler changes — depends on investigation results
- Test file location and naming (follow existing `e2e/` convention)
- Whether to use the existing smoke test v1.69.0 subtest as a starting point or write a new dedicated test file
- Level of debug logging detail — enough to capture RPC calls and content types without flooding output
- Whether plan 05-01 and 05-02 should remain separate or merge (depends on complexity of discoveries)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing test infrastructure (build upon)
- `e2e/smoke_test.go` — Existing smoke test with v1.69.0 subtest that may already pass or reveal the content-type issue
- `e2e/old_proto_test.go` — Phase 4 test pattern for reference (two-step buf mod update)
- `e2e/testutil/server.go` — StartServer helper with ServerResult (port + output buffer)
- `e2e/testutil/bufbin.go` — GetBuf helper, BufV169 constant, RequireEnvToken
- `e2e/testutil/config.go` — TestConfig, DefaultTestConfig, generateConfigYAML

### Server and handler code (understand what the proxy serves)
- `internal/connect/api.go` — Connect RPC handler struct, rootHandler (returns text/plain), handler wiring
- `internal/connect/modulepins.go` — GetModulePins handler
- `internal/connect/blobs.go` — DownloadManifestAndBlobs handler
- `internal/connect/bynames.go` — Repository service handlers
- `cmd/easyp/main.go` — Server entry point, config loading, handler wiring, TLS setup
- `cmd/easyp/internal/config/config.go` — Config struct definitions

### Project decisions (carry forward)
- `.planning/PROJECT.md` — Key Decisions table (content-type mismatch, GetSDKInfo unknown, manifest_digest unknown)
- `.planning/phases/04-old-protocol-validation/04-CONTEXT.md` — Phase 4 decisions (exit code 0 validation, server output on failure)
- `.planning/phases/03-test-infrastructure/03-CONTEXT.md` — Phase 3 test infrastructure decisions

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `e2e/testutil` — Full helper package: StartServer (subprocess + TCP poll + output buffer), GetBuf (auto-download BufV169), RunBufModUpdate (workspace setup + buf execution), RequireEnvToken. Phase 5 imports and extends these.
- `e2e/smoke_test.go` — Already has a `buf_v1.69.0` subtest in the table-driven test. This is the starting point for investigation.
- `testdata/buf/` — Cached buf binaries including v1.69.0 (auto-downloaded by GetBuf).
- `connect-go v1.18.1` — Handles Connect, gRPC, and gRPC-Web protocols. The content-type behavior is managed by the library.

### Established Patterns
- Table-driven tests with `t.Parallel()` for concurrent execution.
- Subprocess server with TCP polling for readiness (30s timeout).
- Temp directory for each test's workspace (`t.TempDir()`).
- Config generated from TestConfig struct → YAML → subprocess flag.
- Server output captured in `bytes.Buffer`, included in failure messages.
- `stretchr/testify` for assertions (`require` for setup, `assert` for checks).
- Log level configurable in TestConfig (currently defaults to "info").

### Integration Points
- Phase 5 test → `e2e/testutil.StartServer` → proxy subprocess (with debug log level) → real GitHub API
- Phase 5 test → `e2e/testutil.GetBuf(BufV169)` → buf v1.69.0 binary
- Phase 5 test → `e2e/testutil.RunBufModUpdate` → validates NEW-01
- Phase 5 test → new `e2e/testutil.RunBufDepUpdate` → validates NEW-02
- Debug log level → reveals all RPC calls, content types, request paths

</code_context>

<specifics>
## Specific Ideas

- The smoke test already tests v1.69.0 — the content-type mismatch may cause the existing subtest to fail. If so, the investigation is straightforward: run the smoke test and read the debug output.
- The `rootHandler` at `/` explicitly returns `text/plain; charset=utf-8`. If modern buf CLI hits the root path for health checks or discovery, this could be the source of the "content-type mismatch" report.
- `buf dep update` in v1.69.0 likely calls the same GetModulePins + DownloadManifestAndBlobs RPCs as `buf mod update`, but may also call additional RPCs (e.g., GetSDKInfo, repository metadata). The empirical discovery approach will reveal this.
- The buf.yaml format for v1.69.0 uses `version: v2` by default, but the proxy's `RunBufModUpdate` helper generates `version: v1` buf.yaml. The v1.69.0 CLI should still support v1 format.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 5-New Protocol Validation*
*Context gathered: 2026-05-07*
