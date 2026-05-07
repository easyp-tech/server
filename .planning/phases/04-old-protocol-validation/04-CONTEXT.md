# Phase 4: Old Protocol Validation - Context

**Gathered:** 2026-05-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Confirm buf v1.30.1 still works against the updated proxy using real binaries and real GitHub API. OLD-01 (buf mod update) is already verified by the existing smoke test — this phase focuses on OLD-02 (buf dep update) and formalizing OLD-01 as verified.

</domain>

<decisions>
## Implementation Decisions

### OLD-01 validation
- **D-01:** OLD-01 is verified by the existing smoke test (`e2e/smoke_test.go` `TestSmokeBufModUpdate` with `buf_v1.30.1` subtest). No additional test needed — the smoke test confirms `buf mod update` exit code 0 and `buf.lock` creation with v1.30.1.

### OLD-02 test structure
- **D-02:** Two-step test — first run `buf mod update` to create `buf.lock`, then run `buf dep update` on the same workspace. Both commands use v1.30.1 binary against the real TLS proxy.
- **D-03:** Validation is exit code 0 only — keep tests simple. No buf.lock content inspection.

### Failure diagnostics
- **D-04:** On test failure, surface the server subprocess output (proxy logs) in the test failure message. The `StartServer` helper already captures stdout/stderr in a buffer — tests should include this in `t.Fatalf` messages.

### Claude's Discretion
- Test file location and naming (follow existing `e2e/` convention)
- Whether to add a `RunBufDepUpdate` helper to testutil or implement inline
- Whether OLD-02 test should share the test case structure from the smoke test or be standalone

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing test infrastructure (build upon)
- `e2e/smoke_test.go` — Existing smoke test that already validates OLD-01
- `e2e/testutil/server.go` — StartServer, RunBufModUpdate helpers
- `e2e/testutil/bufbin.go` — GetBuf binary management, RequireEnvToken, version constants
- `e2e/testutil/config.go` — TestConfig, DefaultTestConfig, generateConfigYAML

### Server and handler code (understand what the proxy serves)
- `cmd/easyp/main.go` — Server entry point, config loading, handler wiring
- `cmd/easyp/internal/config/config.go` — Config struct definitions
- `internal/connect/api.go` — Connect RPC handler struct
- `internal/connect/modulepins.go` — GetModulePins handler
- `internal/connect/blobs.go` — DownloadManifestAndBlobs handler
- `internal/connect/bynames.go` — Repository service handlers

### Project decisions (carry forward)
- `.planning/PROJECT.md` — Key Decisions table
- `.planning/phases/03-test-infrastructure/03-CONTEXT.md` — Test infrastructure decisions

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `e2e/testutil` — Full helper package: StartServer (subprocess + TCP poll), GetBuf (auto-download + cache), RunBufModUpdate (workspace setup + buf execution), RequireEnvToken (env var check). Phase 4 imports and uses these directly.
- `e2e/smoke_test.go` — Working test pattern: table-driven with `t.Parallel()`, uses all testutil helpers. OLD-02 test should follow this pattern.
- `testdata/buf/` — Cached buf binaries (v1.30.1 and v1.69.0) auto-downloaded by GetBuf.

### Established Patterns
- Table-driven tests with `t.Parallel()` for concurrent execution.
- Subprocess server with TCP polling for readiness (30s timeout).
- Temp directory for each test's workspace (`t.TempDir()`).
- Config generated from TestConfig struct → YAML → subprocess flag.
- `stretchr/testify` for assertions (`require` for setup, `assert` for checks).

### Integration Points
- Phase 4 test → `e2e/testutil.StartServer` → proxy subprocess → real GitHub API
- Phase 4 test → `e2e/testutil.GetBuf(BufV130)` → buf v1.30.1 binary
- Phase 4 test → `e2e/testutil.RunBufModUpdate` → creates buf.yaml + runs buf mod update
- Phase 4 test → new `RunBufDepUpdate` (or inline) → runs buf dep update on same workspace

</code_context>

<specifics>
## Specific Ideas

- OLD-02 test should reuse the same TestConfig and GitHub target repo (googleapis/googleapis with google/type/ path) as the smoke test.
- buf v1.30.1 binary path is managed by testutil — auto-downloads if not cached.
- The test runs against real GitHub API — requires EASYP_GITHUB_TOKEN env var.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 4-Old Protocol Validation*
*Context gathered: 2026-05-07*
