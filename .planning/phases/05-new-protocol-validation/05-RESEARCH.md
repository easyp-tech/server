# Phase 5: New Protocol Validation - Research

**Researched:** 2026-05-07
**Domain:** buf CLI v1.69.0 protocol compatibility with connect-go v1.18.1 proxy
**Confidence:** HIGH

## Summary

Phase 5 validates that the EasyP proxy correctly serves buf v1.69.0+ CLI clients. The core hypothesis is that the existing connect-go handlers already work for modern clients, with the primary unknown being whether the `manifest_digest` field or any additional RPCs (notably `GetSDKInfo`) are required. The empirical discovery approach (run tests with debug logging, observe actual RPC calls) is the correct strategy because the buf CLI's RPC behavior is not documented publicly and can only be determined by observation.

The routing architecture investigation confirms that the `rootHandler` returning `text/plain` does NOT intercept RPC calls -- Go's `ServeMux` routes specific prefix patterns before the catch-all `/`. This means the "content-type mismatch" blocker from STATE.md is likely a non-issue for actual RPC traffic. The real validation will come from running the existing smoke test with debug logging and observing whether v1.69.0 succeeds or reveals new requirements.

The proxy currently implements 4 of 33 total RPCs across three services (ResolveService: 1/10, DownloadService: 1/2, RepositoryService: 2/21). All unimplemented RPCs return `connect.CodeUnimplemented`. The empirical test approach will reveal which of the 29 unimplemented RPCs (if any) the v1.69.0 CLI actually calls and requires.

**Primary recommendation:** Run the existing smoke test v1.69.0 subtest with debug log level first, then build from there. The test infrastructure is mature and extensible.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Investigate first -- run the existing smoke test v1.69.0 subtest with verbose logging to capture the actual RPC request/response exchange. Determine root cause before applying a fix.
- **D-02:** Capture diagnostics via debug logging -- set proxy log level to `debug` in tests and include full server subprocess output in failure messages. No custom middleware needed.
- **D-03:** Fix immediately -- investigate and fix in the same plan. Don't separate investigation and fix into different plans.
- **D-04:** Empirical discovery -- write tests, run them, capture server debug logs showing what RPCs the v1.69.0 CLI actually calls. Implement only what's needed based on actual behavior, not speculation.
- **D-05:** Unimplemented first -- all unimplemented RPCs return `CodeUnimplemented`. If the CLI tolerates them (optional calls), no fix needed. Only implement RPCs that block the CLI from succeeding.
- **D-06:** Test the real `buf dep update` command -- v1.69.0 has an actual `buf dep update` command (unlike v1.30.1). Test the real user workflow, not the Phase 4 workaround of two-step `buf mod update`.
- **D-07:** Add a `RunBufDepUpdate` helper to `e2e/testutil/` -- follows the established pattern of splitting helpers by concern (parallel to existing `RunBufModUpdate`).

### Claude's Discretion
- Whether the content-type fix requires connect-go configuration changes, middleware, or handler changes -- depends on investigation results
- Test file location and naming (follow existing `e2e/` convention)
- Whether to use the existing smoke test v1.69.0 subtest as a starting point or write a new dedicated test file
- Level of debug logging detail -- enough to capture RPC calls and content types without flooding output
- Whether plan 05-01 and 05-02 should remain separate or merge (depends on complexity of discoveries)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| NEW-01 | `buf mod update` succeeds against proxy using buf v1.69.0+ binary with real GitHub provider | Existing smoke test already tests this. Debug logging in `loggingMiddleware` (main.go:149-205) captures request method, path, headers, status, duration at debug level. `TestConfig.LogLevel` field (config.go:32) is settable to `"debug"`. |
| NEW-02 | `buf dep update` succeeds against proxy using buf v1.69.0+ binary with real GitHub provider | Requires new `RunBufDepUpdate` helper (D-07). The `buf dep update` command in v1.69.0 likely calls the same RPCs as `buf mod update` (GetModulePins + DownloadManifestAndBlobs) but may also call additional RPCs -- empirical discovery required. |
| HAND-03 | `manifest_digest` field populated on `ModulePin` responses if modern buf CLI requires it | `resolveModulePin` in modulepins.go does NOT populate `manifest_digest`. Empirical test will reveal whether v1.69.0 requires it (i.e., fails without it). |
| HAND-04 | `GetSDKInfo` RPC returns appropriate response or `CodeUnimplemented` based on modern buf CLI behavior | Currently returns `CodeUnimplemented` via `UnimplementedResolveServiceHandler`. Empirical test will reveal whether v1.69.0 calls it and whether it tolerates `CodeUnimplemented`. |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| buf CLI command execution (test) | Test/E2E layer | -- | Tests invoke buf binary as subprocess |
| RPC request routing | HTTP Server (ServeMux) | -- | Go ServeMux routes path prefixes to generated connect handlers |
| Protocol negotiation (Connect/gRPC/gRPC-Web) | connect-go library | -- | connect-go v1.18.1 handles content-type negotiation transparently |
| Content-type response headers | connect-go library | -- | Generated handlers set correct content-type; rootHandler only serves catch-all `/` |
| ModulePin resolution (GetModulePins) | API handler (modulepins.go) | GitHub provider | Handler calls provider.GetMeta, constructs ModulePin response |
| Manifest/Blob download (DownloadManifestAndBlobs) | API handler (blobs.go) | GitHub provider | Handler calls provider.GetFiles, builds manifest with SHAKE256 digests |
| Repository metadata (GetRepositoryByFullName) | API handler (bynames.go) | GitHub provider | Handler calls provider.GetMeta, constructs Repository response |
| Debug logging of RPC calls | Server middleware (main.go) | -- | loggingMiddleware logs method, path, headers, status at debug level |
| Unimplemented RPC responses | Generated handlers | -- | UnimplementedHandler embeddings return CodeUnimplemented for all non-overridden RPCs |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| connectrpc.com/connect | v1.18.1 | RPC framework handling Connect, gRPC, gRPC-Web protocols | Project's RPC layer since Phase 1; handles content-type negotiation transparently [VERIFIED: go.mod] |
| stretchr/testify | v1.8.4 | Test assertions (`require` for setup, `assert` for checks) | Established pattern in all existing E2E tests [VERIFIED: go.mod] |
| google.golang.org/protobuf | v1.34.2 | Protobuf runtime for generated message types | Required by generated proto code [VERIFIED: go.mod] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| golang.org/x/exp/slog | v0.0.0-20231006 | Structured logging | Already used for debug logging in server; configurable via TestConfig.LogLevel [VERIFIED: go.mod] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| N/A | N/A | No new dependencies needed for Phase 5 |

**Installation:**
```bash
# No new packages needed -- all dependencies already in go.mod
go mod tidy
```

**Version verification:** All versions verified from go.mod in this session.

## Architecture Patterns

### System Architecture Diagram

```
[E2E Test]
    |
    | exec: buf {mod|dep} update
    v
[buf v1.69.0 CLI]
    |
    | HTTPS (Connect protocol)
    | Content-Type: application/proto  (or application/grpc)
    v
[Go ServeMux]
    |
    |-- /buf.alpha.registry.v1alpha1.ResolveService/* --> [ResolveServiceHandler]
    |       |-- GetModulePins          (IMPLEMENTED)
    |       |-- GetSDKInfo             (UNIMPLEMENTED -> CodeUnimplemented)
    |       |-- GetGoVersion           (UNIMPLEMENTED -> CodeUnimplemented)
    |       +-- ... 7 more             (UNIMPLEMENTED -> CodeUnimplemented)
    |
    |-- /buf.alpha.registry.v1alpha1.DownloadService/* --> [DownloadServiceHandler]
    |       |-- Download               (UNIMPLEMENTED -> CodeUnimplemented)
    |       +-- DownloadManifestAndBlobs (IMPLEMENTED)
    |
    |-- /buf.alpha.registry.v1alpha1.RepositoryService/* --> [RepositoryServiceHandler]
    |       |-- GetRepositoryByFullName        (IMPLEMENTED)
    |       |-- GetRepositoriesByFullName      (IMPLEMENTED)
    |       +-- ... 19 more                    (UNIMPLEMENTED -> CodeUnimplemented)
    |
    +-- / --> [rootHandler] (text/plain health check, NOT hit by RPC calls)
                |
                v
          [GitHub API] (real calls with EASYP_GITHUB_TOKEN)
```

### Recommended Project Structure
```
e2e/
├── smoke_test.go         # Existing: table-driven buf mod update for v1.30.1 + v1.69.0
├── old_proto_test.go     # Phase 4: two-step buf mod update (v1.30.1 only)
├── new_proto_test.go     # Phase 5: NEW -- dedicated new protocol validation tests
└── testutil/
    ├── server.go         # StartServer, RunBufModUpdate
    ├── bufbin.go         # GetBuf, BufV130, BufV169, RequireEnvToken
    └── config.go         # TestConfig, DefaultTestConfig, generateConfigYAML

internal/connect/
├── api.go               # ServeMux setup, rootHandler
├── modulepins.go        # GetModulePins handler
├── blobs.go             # DownloadManifestAndBlobs handler
└── bynames.go           # Repository service handlers
```

### Pattern 1: Table-Driven E2E Test with Debug Logging
**What:** Start server subprocess with debug log level, run buf command, capture full server output for diagnostics.
**When to use:** Every Phase 5 test that needs to observe RPC behavior.
**Example:**
```go
// Source: established pattern from e2e/smoke_test.go, e2e/testutil/config.go
cfg := testutil.DefaultTestConfig()
cfg.GithubToken = token
cfg.LogLevel = "debug"  // KEY: enables RPC-level logging in loggingMiddleware

srv := testutil.StartServer(t, cfg)
// srv.Output contains all debug logs including RPC calls, content types, headers
```

### Pattern 2: Empirical RPC Discovery via Server Output
**What:** Run buf command against debug-level server, examine server output to identify which RPCs were called.
**When to use:** Determining which unimplemented RPCs need implementation.
**Example:**
```go
// After running buf command:
// srv.Output contains JSON-formatted debug logs:
// {"level":"debug","msg":"request details","path":"/buf.alpha.registry.v1alpha1.ResolveService/GetModulePins",...}
// {"level":"debug","msg":"request details","path":"/buf.alpha.registry.v1alpha1.ResolveService/GetSDKInfo",...}
// Check srv.Output.String() in failure messages to see all RPC calls
```

### Pattern 3: RunBufDepUpdate Helper (NEW)
**What:** Parallel to RunBufModUpdate, creates workspace and runs `buf dep update` instead of `buf mod update`.
**When to use:** Testing NEW-02 requirement.
**Example:**
```go
// Source: pattern established by RunBufModUpdate in e2e/testutil/server.go
func RunBufDepUpdate(t *testing.T, bufBinary string, port int) (int, string) {
    t.Helper()
    tmpDir := t.TempDir()
    // Write buf.yaml with dependency referencing proxy
    // Run: buf dep update (not buf mod update)
    // Return exit code and stderr
}
```

### Anti-Patterns to Avoid
- **Don't test with `buf mod update` when the requirement says `buf dep update`:** v1.69.0 has both commands and they may call different RPCs. Test the real user workflow (D-06).
- **Don't implement RPCs speculatively:** Only implement what empirical testing reveals as blocking. 29 of 33 RPCs are unimplemented and the CLI may not need most of them (D-04, D-05).
- **Don't assume rootHandler intercepts RPC calls:** Go ServeMux routes specific path prefixes before the catch-all `/`. The `rootHandler` at `/` only handles requests that don't match any registered handler prefix.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Content-type negotiation | Custom content-type middleware or header manipulation | connect-go v1.18.1 built-in protocol handlers | connect-go automatically handles Connect, gRPC, and gRPC-Web content-types based on request headers [VERIFIED: connect-go docs, generated handler code] |
| RPC routing | Manual path-to-handler mapping | Generated `New*Handler` functions with ServeMux | Each generated handler returns a path prefix and an http.Handler that dispatches by `r.URL.Path` [VERIFIED: resolve.connect.go:373] |
| Unimplemented RPC responses | Custom error handlers for each unimplemented RPC | `Unimplemented*Handler` embedded structs | All unimplemented methods already return `connect.CodeUnimplemented` via the embedded struct pattern [VERIFIED: api.go:20-22, resolve.connect.go:405-441] |
| Structured debug logging | Custom request logging middleware | Existing `loggingMiddleware` in main.go | Already logs method, path, headers, status, duration at debug level [VERIFIED: main.go:149-205] |

**Key insight:** Phase 5 is primarily a testing and observation phase, not a building phase. The infrastructure is mature; the work is empirical discovery of what v1.69.0 actually calls.

## Common Pitfalls

### Pitfall 1: Misattributing content-type mismatch to rootHandler
**What goes wrong:** Assuming the `rootHandler` returning `text/plain` causes content-type issues for RPC calls.
**Why it happens:** The `rootHandler` is registered at `/` and returns `text/plain; charset=utf-8`, which looks like it could intercept all requests.
**How to avoid:** Go ServeMux routes specific path prefix patterns (`/buf.alpha.registry.v1alpha1.ResolveService/`) before the catch-all `/`. The rootHandler NEVER intercepts RPC calls. This is confirmed by the generated handler code returning path prefix `/buf.alpha.registry.v1alpha1.ResolveService/` [VERIFIED: resolve.connect.go:373].
**Warning signs:** If you see `text/plain` in debug logs, check the request path -- it will be `/`, not an RPC path.

### Pitfall 2: buf.yaml version mismatch
**What goes wrong:** Creating `buf.yaml` with `version: v2` format when v1 is expected, or vice versa.
**Why it happens:** v1.69.0 defaults to v2 buf.yaml but the proxy test helpers use v1 format.
**How to avoid:** The existing `RunBufModUpdate` helper generates `version: v1` buf.yaml and this works for v1.69.0. Keep using v1 format for consistency with existing tests. If v1.69.0 requires v2, the test will fail and reveal it [ASSUMED].
**Warning signs:** buf CLI exits with "unknown field" or "invalid config" errors.

### Pitfall 3: Assuming manifest_digest is required without testing
**What goes wrong:** Implementing manifest_digest population before confirming it's needed.
**Why it happens:** The `ModulePin` proto has a `manifest_digest` field and modern buf may expect it populated.
**How to avoid:** Run the test first. If it fails with an error about missing/invalid manifest_digest, then implement it. If it succeeds without it, no change needed (D-05: unimplemented first).
**Warning signs:** Test passes without manifest_digest -- no action needed.

### Pitfall 4: TLS certificate trust issues in test
**What goes wrong:** buf v1.69.0 CLI may reject self-signed TLS certificates differently than v1.30.1.
**Why it happens:** Different buf versions may have different TLS validation behavior.
**How to avoid:** The existing test infrastructure uses TLS with certificates from `~/local-tls/server/`. The v1.69.0 subtest in smoke_test.go already uses this setup. If TLS fails, the debug server output will show connection errors.
**Warning signs:** "certificate verify failed" or "tls handshake failure" in buf stderr or server output.

### Pitfall 5: buf dep update requires different buf.yaml format
**What goes wrong:** `buf dep update` in v1.69.0 may require `buf.yaml` v2 format with `deps` in a different location.
**Why it happens:** `buf dep update` is the v1.69.0 command replacing `buf mod update` and may have different config expectations [ASSUMED].
**How to avoid:** Start with v1 format (same as RunBufModUpdate uses). If it fails, examine stderr for format hints. The empirical approach (D-04) handles this.
**Warning signs:** "unknown command" or config parsing errors in buf stderr.

## Code Examples

### Setting Debug Log Level in Test
```go
// Source: e2e/testutil/config.go:32, e2e/smoke_test.go
cfg := testutil.DefaultTestConfig()
cfg.GithubToken = token
cfg.LogLevel = "debug"  // Enables full RPC logging via loggingMiddleware

srv := testutil.StartServer(t, cfg)
// After running buf command:
// srv.Output.String() contains JSON debug logs with:
//   "request details" -- method, path, headers (masked for auth)
//   "request completed" -- status, size, duration
```

### Examining Server Output for RPC Discovery
```go
// Source: pattern from e2e/old_proto_test.go:68
exitCode, stderr := testutil.RunBufModUpdate(t, bufPath, srv.Port)
if exitCode != 0 {
    t.Fatalf("buf mod update failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
        exitCode, srv.Output.String(), stderr)
}
// On success, srv.Output.String() still contains debug logs for analysis
// Parse for RPC paths: grep for "buf.alpha.registry.v1alpha1" in output
```

### Extend TestConfig for Debug Logging (already supported)
```go
// Source: e2e/testutil/config.go:38-49
// TestConfig.LogLevel is already a field. DefaultTestConfig() sets it to "info".
// Simply override:
cfg.LogLevel = "debug"
// generateConfigYAML (config.go:54-96) writes it as: log: { level: "debug" }
// main.go:131-146 newLogger parses "debug" -> slog.LevelDebug
```

### RunBufDepUpdate Helper Pattern (to implement)
```go
// Source: pattern from e2e/testutil/server.go:91-131 (RunBufModUpdate)
func RunBufDepUpdate(t *testing.T, bufBinary string, port int) (int, string) {
    t.Helper()
    tmpDir := t.TempDir()
    bufYAML := fmt.Sprintf(`version: v1
deps:
  - 127.0.0.1:%d/googleapis/googleapis
`, port)
    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600))

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx, bufBinary, "dep", "update")  // "dep update" not "mod update"
    cmd.Dir = tmpDir
    cmd.Env = os.Environ()

    var stderr bytes.Buffer
    cmd.Stderr = &stderr

    exitErr := cmd.Run()
    // ... same exit code handling as RunBufModUpdate ...
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `buf mod update` (v1.30.1) | `buf dep update` (v1.69.0+) | buf v1.32.0+ | Phase 5 tests the new command (NEW-02) |
| Hand-written proto handlers | Generated handlers from v1.69.0 protos | Phase 1 | All 33 RPCs have generated stubs; 4 implemented |
| Custom RPC routing | ServeMux + generated path prefix handlers | Phase 1 | Routing is automatic; rootHandler only for catch-all `/` |

**Deprecated/outdated:**
- `buf mod update` command: deprecated in buf v1.32.0+, replaced by `buf dep update`. Still works in v1.69.0 for backward compatibility [ASSUMED].

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `buf dep update` in v1.69.0 calls the same RPCs (GetModulePins, DownloadManifestAndBlobs) as `buf mod update` but may call additional RPCs | Phase Requirements, Pitfall 5 | If it calls completely different RPCs, more implementation work needed |
| A2 | `buf mod update` still works in v1.69.0 for backward compatibility | State of the Art | If removed, NEW-01 must use `buf dep update` instead |
| A3 | v1 format `buf.yaml` is supported by v1.69.0 CLI | Pitfall 2, Code Examples | If not supported, need to generate v2 format buf.yaml |
| A4 | The content-type mismatch is a non-issue for RPC traffic (routing analysis confirms rootHandler does not intercept) | Summary, Pitfall 1 | If there is a genuine content-type issue in connect-go responses, need connect-go configuration changes |

## Open Questions

1. **Does v1.69.0 `buf dep update` call GetSDKInfo or other unimplemented RPCs?**
   - What we know: The proxy returns `CodeUnimplemented` for all unimplemented RPCs. The CLI may tolerate this for optional RPCs.
   - What's unclear: Which RPCs v1.69.0 actually calls during `buf dep update`.
   - Recommendation: Run test with debug logging and observe. This is exactly what D-04 prescribes.

2. **Is `manifest_digest` required on ModulePin responses?**
   - What we know: The field exists in the proto and is currently not populated. Modern buf may or may not require it.
   - What's unclear: Whether v1.69.0 fails without it.
   - Recommendation: Run test first. Only populate if test fails with a related error.

3. **Does `buf dep update` require v2 format buf.yaml?**
   - What we know: v1.69.0 defaults to v2 format. The existing test helpers generate v1 format.
   - What's unclear: Whether `buf dep update` works with v1 format.
   - Recommendation: Try v1 first, examine stderr if it fails.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go runtime | Server compilation | Yes | go1.26.1 | -- |
| buf v1.69.0 binary | NEW-01, NEW-02 tests | Yes (auto-download) | v1.69.0 | GetBuf downloads from GitHub Releases |
| TLS certificates | Server HTTPS | Yes | ~/local-tls/server/ | -- |
| EASYP_GITHUB_TOKEN | GitHub API access | Configurable | -- | RequireEnvToken skips test if absent |
| GitHub API (network) | Real dependency resolution | Yes | -- | Test skips if no network |

**Missing dependencies with no fallback:**
- None -- all dependencies are either installed or auto-provisioned by test helpers.

**Missing dependencies with fallback:**
- None.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + stretchr/testify v1.8.4 |
| Config file | none -- tests use TestConfig struct |
| Quick run command | `go test ./e2e/ -run TestSmokeBufModUpdate/buf_v1.69.0 -v -timeout 120s` |
| Full suite command | `go test ./e2e/ -v -timeout 300s` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| NEW-01 | buf mod update succeeds with v1.69.0 | integration | `go test ./e2e/ -run TestSmokeBufModUpdate/buf_v1.69.0 -v -timeout 120s` | Yes (smoke_test.go) |
| NEW-02 | buf dep update succeeds with v1.69.0 | integration | `go test ./e2e/ -run TestNewProtoBufDepUpdate -v -timeout 120s` | Wave 0 (new file) |
| HAND-03 | manifest_digest populated if required | integration | (discovered via NEW-01/NEW-02 test failure) | N/A |
| HAND-04 | GetSDKInfo returns appropriate response | integration | (discovered via debug logging in NEW-01/NEW-02) | N/A |

### Sampling Rate
- **Per task commit:** `go test ./e2e/ -run <TestName> -v -timeout 120s`
- **Per wave merge:** `go test ./e2e/ -v -timeout 300s`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `e2e/new_proto_test.go` -- covers NEW-02 (TestNewProtoBufDepUpdate)
- [ ] `e2e/testutil/server.go` -- add RunBufDepUpdate helper (D-07)
- [ ] Consider: extend existing smoke test or create dedicated new_proto_test.go (Claude's discretion)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | -- |
| V3 Session Management | no | -- |
| V4 Access Control | no | -- |
| V5 Input Validation | yes | connect-go generated handlers validate protobuf wire format |
| V6 Cryptography | yes | TLS already configured; test uses existing cert infrastructure |

### Known Threat Patterns for Go E2E Testing

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Token exposure in test output | Information disclosure | loggingMiddleware masks Authorization headers (main.go:219-233); config written with mode 0600 (config.go:91) |
| Unencrypted test communication | Tampering | Tests use TLS (certs from ~/local-tls/) |

## Sources

### Primary (HIGH confidence)
- Source code analysis: `internal/connect/api.go`, `internal/connect/modulepins.go`, `internal/connect/blobs.go`, `internal/connect/bynames.go`, `cmd/easyp/main.go` -- routing, handler implementation, logging middleware
- Generated handler analysis: `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/resolve.connect.go`, `download.connect.go`, `repository.connect.go` -- RPC inventory, path prefixes, unimplemented handler behavior
- Test infrastructure: `e2e/testutil/server.go`, `e2e/testutil/config.go`, `e2e/testutil/bufbin.go` -- established patterns for server startup, buf execution, config generation
- `go.mod` -- dependency versions confirmed: connect-go v1.18.1, testify v1.8.4, protobuf v1.34.2

### Secondary (MEDIUM confidence)
- CONTEXT.md decisions D-01 through D-07 -- locked implementation strategy from phase discussion
- REQUIREMENTS.md -- NEW-01, NEW-02, HAND-03, HAND-04 requirement definitions

### Tertiary (LOW confidence)
- `buf dep update` RPC behavior in v1.69.0 [ASSUMED] -- not verified against buf source code; empirical discovery planned

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all dependencies verified from go.mod
- Architecture: HIGH - routing and handler behavior verified from source code
- Pitfalls: HIGH - based on verified source code analysis, with LOW items tagged as [ASSUMED]
- RPC discovery needs: MEDIUM - empirical approach is correct, but outcomes unknown until tests run

**Research date:** 2026-05-07
**Valid until:** 2026-06-07 (stable -- no fast-moving dependencies)
