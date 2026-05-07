# Phase 3: Test Infrastructure - Context

**Gathered:** 2026-05-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Build reusable test helpers for starting a TLS proxy server, managing pinned buf binaries, and making authenticated GitHub API calls. These helpers are the foundation for Phases 4 and 5 (protocol validation tests). The phase refactors the minimal E2E smoke test infrastructure from Phase 2 into a proper, reusable test helper package.

</domain>

<decisions>
## Implementation Decisions

### Server lifecycle
- **D-01:** Subprocess approach — start the proxy as a subprocess (via `go run` or pre-built binary). Tests the real compiled binary end-to-end, matching what users experience. Do not switch to in-process httptest.
- **D-02:** TCP poll for readiness — keep the existing approach of polling the TCP port until the server accepts connections. Do not add readiness signals to production code.
- **D-03:** Config struct → YAML — helper accepts a Go config struct and generates the YAML config file into `t.TempDir()`. This is what the existing smoke test does inline; formalize it into a reusable function.

### Buf binary management
- **D-04:** Auto-download from GitHub releases — the helper downloads pinned buf versions from `github.com/bufbuild/buf/releases` if not found locally. Makes the test suite self-contained; no manual binary setup required.
- **D-05:** Cache in project `testdata/` — downloaded binaries are stored in `testdata/buf/v{version}/buf` (or platform-specific subdirectory). Tests check cache first, download only if missing. Add `testdata/buf/` to `.gitignore`.

### Helper packaging
- **D-06:** Helpers live in `e2e/testutil/` — separate package scoped to integration/e2e tests. Importable by Phases 4 and 5 test files.
- **D-07:** Split by concern — three files: `server.go` (startServer, port allocation, config generation), `bufbin.go` (binary download, cache management, version assertion), `config.go` (test config struct and YAML generation). Each file has a single responsibility.

### Claude's Discretion
- Exact config struct field names and types — follow existing `cmd/easyp/internal/config/config.go` patterns.
- Whether to use `go run` or pre-build the binary — optimize for test speed as long as it's a subprocess.
- Platform detection logic for buf binary download (darwin/linux, amd64/arm64).
- Whether to verify checksums on downloaded binaries.
- How to handle the existing `e2e/smoke_test.go` — refactor to use new helpers or leave as-is.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing test infrastructure (refactor into helpers)
- `e2e/smoke_test.go` — Current E2E smoke test with inline startServer(), runBufModUpdate(), findProjectRoot() helpers. Phase 3 extracts these into e2e/testutil/.
- `testdata/cert.pem` — Test TLS certificate (existing fixture)
- `testdata/key.pem` — Test TLS private key (existing fixture)

### Server wiring (understand for subprocess startup)
- `cmd/easyp/main.go` — Server entry point, config loading, handler wiring, TLS setup
- `cmd/easyp/internal/config/config.go` — Config struct definitions (TLS, proxy, cache, repos)
- `internal/https/https.go` — TLS server with ListenAndServeTLS

### Handler layer (understand what the proxy serves)
- `internal/connect/api.go` — Connect RPC handler struct, New() constructor
- `internal/connect/modulepins.go` — GetModulePins handler
- `internal/connect/blobs.go` — DownloadManifestAndBlobs handler
- `internal/connect/bynames.go` — Repository service handlers

### Project decisions (carry forward)
- `.planning/PROJECT.md` — Key Decisions table (TLS cert location, real buf binaries, GitHub token)
- `.planning/phases/02-handler-adaptation/02-CONTEXT.md` — D-04: "Phase 3 will formalize test infrastructure"

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `e2e/smoke_test.go` — Contains working implementations of startServer(), runBufModUpdate(), findProjectRoot(). These are the starting point for the reusable helpers.
- `~/local-tls/server/` — Self-signed TLS certs (server-cert.pem, server-key.pem) for local testing. Already used by smoke test.
- `testdata/` — Existing test fixture directory. Will host downloaded buf binaries at `testdata/buf/`.
- `stretchr/testify` — Already in go.mod, allowed by depguard for test files. Use `require` for setup assertions, `assert` for test assertions.

### Established Patterns
- Config via YAML file with env var expansion (`cmd/easyp/internal/config/read.go` uses `os.ExpandEnv`).
- Port allocation via `net.Listen("tcp", "127.0.0.1:0")` — zero-port lets OS assign a free port.
- Parallel test execution with `t.Parallel()` — each test gets its own port via the allocation pattern.
- Go standard `testing` package — no custom test runner.

### Integration Points
- `e2e/testutil/server.go` → `go run ./cmd/easyp -cfg <generated-yaml>` — server startup subprocess
- `e2e/testutil/bufbin.go` → `github.com/bufbuild/buf/releases` — binary download source
- `e2e/testutil/config.go` → `cmd/easyp/internal/config/config.go` — config struct mirrors production config
- Phase 4/5 tests → `e2e/testutil.*` — helpers consumed by future validation phases

</code_context>

<specifics>
## Specific Ideas

- Buf binary paths: v1.30.1 at `~/go/bin/buf`, v1.69.0 at `/usr/local/bin/buf` — these already exist on this machine. Auto-download is for portability (CI, other machines).
- Test target repo: `googleapis/googleapis` with path `google/type/` — used by existing smoke test, carry forward.
- TLS cert paths: `~/local-tls/server/server-cert.pem` and `~/local-tls/server/server-key.pem`.
- GitHub token env var: `EASYP_GITHUB_TOKEN` — used by existing smoke test, carry forward.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 3-Test Infrastructure*
*Context gathered: 2026-05-07*
