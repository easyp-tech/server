# Phase 2: Handler Adaptation - Research

**Researched:** 2026-05-07
**Domain:** Connect RPC handler adaptation for regenerated proto types
**Confidence:** HIGH

## Summary

Phase 2 is a verification-heavy phase with minimal code changes. The handler layer (`internal/connect/`) already compiles against the regenerated v1.69.0 proto types because the `api` struct embeds all three `Unimplemented*Handler` types which were expanded by the codegen. The project passes both `go build ./...` and `go vet ./...` with zero errors. The primary work is writing E2E smoke tests that prove the server starts, serves RPCs, and handles both old (buf v1.30.1) and modern (buf v1.69.0) CLI clients.

The four HAND requirements break down as follows: HAND-01 (embed new Unimplemented types) is already satisfied -- no code changes needed. HAND-02 (existing RPCs work) needs E2E verification. HAND-03 (manifest_digest) is deferred per user decision D-02. HAND-04 (GetSDKInfo) is deferred per user decision D-01. The real deliverable is E2E smoke tests that prove the proxy works end-to-end with both buf CLI versions.

**Primary recommendation:** Write E2E smoke tests as the primary Phase 2 deliverable. Zero handler code changes are required. The tests must start a real TLS server, point both buf CLI versions at it, and verify `buf mod update` succeeds.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Leave GetSDKInfo as Unimplemented (returns `CodeUnimplemented`). Phase 5 will discover through empirical testing whether a real implementation is needed.
- **D-02:** Leave `manifest_digest` field empty in ModulePin responses. Phase 5 will discover through empirical testing whether modern buf CLI requires it populated.
- **D-03:** Include E2E smoke tests for BOTH old and modern buf CLI versions in Phase 2: start the TLS proxy server, run `buf mod update` with buf v1.30.1 AND buf v1.69.0+ against the proxy, verify both succeed.
- **D-04:** The E2E smoke tests should use minimal test infrastructure -- TLS server startup using `~/local-tls/server/` certs, both buf binaries available on PATH, GitHub API token from environment variable. Phase 3 will formalize this into reusable test helpers.
- **D-05:** HAND-01 (handler structs embed new Unimplemented* types) is effectively already complete from Phase 1. No handler struct changes needed.

### Claude's Discretion
- Exact E2E test structure and helper functions -- Claude can choose the simplest approach that validates the server works.
- How to structure the test file (table-driven, single test function, etc.).

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| HAND-01 | Handler structs updated to embed new `Unimplemented*` types from regenerated code | **Already satisfied.** The `api` struct in `internal/connect/api.go:18-24` embeds all three Unimplemented types. The project compiles and passes `go vet`. No changes needed. |
| HAND-02 | Existing RPC logic works correctly with new generated types | **Compilation verified.** All four implemented RPCs (`GetModulePins`, `DownloadManifestAndBlobs`, `GetRepositoryByFullName`, `GetRepositoriesByFullName`) use types from the regenerated proto packages. E2E smoke tests will prove runtime correctness. |
| HAND-03 | `manifest_digest` field populated on `ModulePin` responses if modern buf CLI requires it | **Deferred (D-02).** The `ModulePin` struct at `gen/proto/buf/alpha/module/v1alpha1/module.pb.go:446` has a `ManifestDigest string` field. It is left empty (zero value). Phase 5 will determine empirically if population is needed. |
| HAND-04 | `GetSDKInfo` RPC returns appropriate response or `CodeUnimplemented` | **Deferred (D-01).** The `UnimplementedResolveServiceHandler.GetSDKInfo` at `resolve.connect.go:408-410` returns `connect.NewError(connect.CodeUnimplemented, ...)`. This satisfies the requirement for Phase 2. Phase 5 will determine if a real response is needed. |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Handler interface satisfaction | API / Backend | -- | Connect RPC handler structs own interface implementation |
| TLS server startup for testing | API / Backend | -- | Server binary is the test target; tests start it |
| E2E test orchestration | Test runner | -- | External `buf` CLI invokes the proxy; test validates exit code |
| Proto type compatibility | Generated code | API / Backend | Proto types are generated; handler code consumes them |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| connectrpc.com/connect | v1.18.1 | Connect RPC framework | Project dependency ceiling (v1.19.x requires Go 1.24) [VERIFIED: go.mod] |
| google.golang.org/protobuf | v1.34.2 | Protobuf runtime | Paired with connect-go v1.18.1 [VERIFIED: go.mod] |
| stretchr/testify | v1.8.4 | Test assertions | Already in go.mod as indirect dependency [VERIFIED: go.mod] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| net/http (stdlib) | Go 1.22 | HTTP server | Starting TLS server in tests |
| os/exec (stdlib) | Go 1.22 | Execute buf CLI | E2E test invokes buf binary |
| testing (stdlib) | Go 1.22 | Test framework | All test files |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| stretchr/testify | No assertions (manual checks) | testify reduces boilerplate; already in go.mod |
| External buf CLI via os/exec | Connect client in-process | External CLI is the real-world usage; validates actual protocol compatibility |

**Installation:**
No new packages needed. All dependencies already in go.mod.

## Architecture Patterns

### System Architecture Diagram

```
E2E Test (Go test runner)
  |
  |-- 1. Write temp config.yml with TLS certs + GitHub token
  |-- 2. Start server binary: `go run cmd/easyp/main.go -cfg temp.yml`
  |-- 3. Wait for server ready (TCP probe on 127.0.0.1:PORT)
  |
  |--+-- Test Case A: buf v1.30.1
  |     |-- `buf mod update` pointing at TLS proxy
  |     |-- Verify: exit code 0, buf.lock created
  |
  |--+-- Test Case B: buf v1.69.0
  |     |-- `buf mod update` pointing at TLS proxy
  |     |-- Verify: exit code 0, buf.lock created
  |
  |-- 4. Kill server process
  |-- 5. Cleanup temp files
```

### Handler Interface Satisfaction (Current State)

The `api` struct at `internal/connect/api.go:18-24`:

```go
type api struct {
    log *slog.Logger
    connect.UnimplementedRepositoryServiceHandler    // 21 RPCs
    connect.UnimplementedResolveServiceHandler       // 10 RPCs
    connect.UnimplementedDownloadServiceHandler      // 2 RPCs
    repo   provider
    domain string
}
```

Methods explicitly overridden (4 total):
- `GetModulePins` -- `internal/connect/modulepins.go:13`
- `DownloadManifestAndBlobs` -- `internal/connect/blobs.go:17`
- `GetRepositoryByFullName` -- `internal/connect/bynames.go:32`
- `GetRepositoriesByFullName` -- `internal/connect/bynames.go:15`

Methods from Unimplemented embedding (29 total, including new v1.69.0 RPCs):
- ResolveService: GetSDKInfo, GetGoVersion, GetSwiftVersion, GetMavenVersion, GetNPMVersion, GetPythonVersion, GetCargoVersion, GetNugetVersion, GetCmakeVersion -- all return `CodeUnimplemented`
- RepositoryService: 19 RPCs -- all return `CodeUnimplemented`
- DownloadService: `Download` -- returns `CodeUnimplemented`

### Pattern 1: Connect RPC Handler Override
**What:** Embed `Unimplemented*Handler`, override only the RPCs you implement.
**When to use:** For all Connect service handlers. The embedding pattern ensures forward compatibility when new RPCs are added to the proto definition.
**Example:**
```go
// From internal/connect/api.go
type api struct {
    connect.UnimplementedResolveServiceHandler
    // ... override GetModulePins method
}
func (a *api) GetModulePins(ctx context.Context, req *connect.Request[registry.GetModulePinsRequest]) (
    *connect.Response[registry.GetModulePinsResponse], error) {
    // implementation
}
```

### Pattern 2: E2E Smoke Test Structure
**What:** Start real server, invoke real buf CLI, verify exit code.
**When to use:** When you need to prove actual protocol compatibility, not just type compatibility.
**Example:**
```go
func TestE2EBufModUpdate(t *testing.T) {
    // 1. Create temp config with TLS paths and GitHub token
    // 2. Start server as subprocess: go run ./cmd/easyp -cfg temp.yml
    // 3. Wait for TCP listener to accept connections
    // 4. Create temp buf module with buf.yaml referencing proxy
    // 5. Run: buf mod update --verify
    // 6. Assert exit code 0 and buf.lock exists
    // 7. Kill server, cleanup
}
```

### Anti-Patterns to Avoid
- **In-process Connect client testing:** Using `connect.NewClient` in tests only proves type compatibility, not wire protocol compatibility with the actual buf CLI. Must use external buf binary.
- **Skipping TLS:** buf CLI requires TLS for registry communication. Tests must use the real TLS setup.
- **Hardcoded ports:** Tests must use dynamic port allocation to avoid conflicts when run in parallel.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Server lifecycle in tests | Custom process manager | `os/exec.Command` + `cmd.Process.Kill()` | stdlib handles subprocess management correctly |
| Port allocation | Hardcoded ports | `net.Listen("tcp", "127.0.0.1:0")` to get free port | Avoids flaky port conflicts |
| Temp files | Manual file creation/cleanup | `t.TempDir()` from testing package | Auto-cleaned on test completion |
| Wait for server ready | `time.Sleep` | TCP dial loop with timeout | Reliable and deterministic |

## Common Pitfalls

### Pitfall 1: buf CLI requires specific TLS cert trust
**What goes wrong:** buf CLI rejects self-signed certs that are not in the system trust store.
**Why it happens:** buf uses Go's TLS stack which checks system CA certs by default.
**How to avoid:** The `~/local-tls/server/` certs are already set up and the local CA was added to the system trust store. Tests should reference these paths directly.
**Warning signs:** `x509: certificate signed by unknown authority` errors in test output.

### Pitfall 2: buf mod update needs a valid buf.yaml with dependencies
**What goes wrong:** Running `buf mod update` in an empty directory or without a valid `buf.yaml` that references the proxy will fail with a non-protocol error.
**Why it happens:** buf mod update resolves dependencies listed in buf.yaml against the configured registry.
**How to avoid:** Test must create a minimal buf.yaml with at least one dependency referencing the proxy's domain (e.g., `buf.build/owner/repo` mapped to the proxy).
**Warning signs:** `no buf.yaml found` or `no dependencies` errors.

### Pitfall 3: Server startup race condition
**What goes wrong:** Test runs buf CLI before the server is listening, causing connection refused.
**Why it happens:** Server startup is asynchronous -- `go run` returns immediately but the server takes time to bind the port.
**How to avoid:** Poll the TCP port in a loop with a timeout (e.g., 5 seconds). Only proceed when a TCP connection succeeds.
**Warning signs:** `connection refused` errors in test output, tests passing when re-run (server already up from previous attempt).

### Pitfall 4: GitHub API token not available
**What goes wrong:** Server starts but handler calls to GitHub API fail with 401.
**Why it happens:** The GitHub token is in the YAML config file, not an environment variable. Tests must write a valid config file.
**How to avoid:** Read the token from an environment variable (e.g., `EASYP_GITHUB_TOKEN`) and write it into a temp config file. Fail the test with a skip message if the env var is not set.
**Warning signs:** `401 Unauthorized` from GitHub API in server logs.

### Pitfall 5: buf.lock format differences between versions
**What goes wrong:** buf v1.30.1 and v1.69.0 may produce different buf.lock file formats, leading to false test failures.
**Why it happens:** The lock file format may have changed between versions.
**How to avoid:** Verify only that `buf mod update` exits with code 0 and a `buf.lock` file exists. Do not assert on file contents.
**Warning signs:** Tests fail on lock file content assertions but the command succeeds.

## Code Examples

### E2E Test Skeleton (Verified pattern)
```go
// File: e2e/smoke_test.go
package e2e

import (
    "context"
    "fmt"
    "net"
    "os"
    "os/exec"
    "path/filepath"
    "testing"
    "time"
)

func TestBufModUpdate(t *testing.T) {
    token := os.Getenv("EASYP_GITHUB_TOKEN")
    if token == "" {
        t.Skip("EASYP_GITHUB_TOKEN not set, skipping E2E test")
    }

    // Allocate free port
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatalf("allocating port: %v", err)
    }
    port := listener.Addr().(*net.TCPAddr).Port
    listener.Close()

    // Create temp config
    tmpDir := t.TempDir()
    cfgPath := filepath.Join(tmpDir, "config.yml")
    cfgContent := fmt.Sprintf(`
listen: "127.0.0.1:%d"
domain: "127.0.0.1:%d"
tls:
  cert: %s/server-cert.pem
  key:  %s/server-key.pem
proxy:
  github:
    - token: %s
      repo:
        owner: googleapis
        name:  googleapis
        path:
          - google/type/
`, port, port, os.Getenv("HOME")+"/local-tls/server", os.Getenv("HOME")+"/local-tls/server", token)
    os.WriteFile(cfgPath, []byte(cfgContent), 0600)

    // Start server
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    cmd := exec.CommandContext(ctx, "go", "run", "./cmd/easyp", "-cfg", cfgPath)
    cmd.Dir = projectRoot(t)
    if err := cmd.Start(); err != nil {
        t.Fatalf("starting server: %v", err)
    }
    defer cmd.Process.Kill()

    // Wait for server ready
    waitForPort(t, port, 5*time.Second)

    // Run buf mod update (old version)
    testBufModUpdate(t, "buf", port, tmpDir) // or path to specific version
}
```

### Server Wiring (Existing, verified at cmd/easyp/main.go)
```go
// Lines 52-53: Handler creation and wiring
handler = connect.New(log, storage, cfg.Domain)
serve = func() error { return http.ListenAndServe(cfg.Listen.String(), loggingMiddleware(log, handler)) }
```

### ModulePin Construction (Existing, verified at internal/connect/modulepins.go:51-56)
```go
// manifest_digest is left empty per decision D-02
return &module.ModulePin{ //nolint:exhaustruct
    Remote:     a.domain,
    Owner:      v.GetOwner(),
    Repository: v.GetRepository(),
    Commit:     repo.Commit,
}, nil
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| ResolveService had only GetModulePins | ResolveService has GetModulePins + 9 version RPCs | v1.69.0 proto | New RPCs are Unimplemented -- handled by embedding |
| DownloadService had only DownloadManifestAndBlobs | DownloadService has Download + DownloadManifestAndBlobs | v1.69.0 proto | New Download RPC is Unimplemented -- handled by embedding |
| RepositoryService had ~15 RPCs | RepositoryService has 21 RPCs (3 new group RPCs) | v1.69.0 proto | New RPCs are Unimplemented -- handled by embedding |
| ModulePin had no manifest_digest | ModulePin has ManifestDigest field | v1.69.0 proto | Left empty per D-02 |

**Deprecated/outdated:**
- `Download` RPC on DownloadService: marked "NOTE: Newer clients should use DownloadManifestAndBlobs instead" in proto comments [VERIFIED: download.connect.go:61-62]

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go runtime | Server build | Yes | 1.22 | -- |
| buf CLI v1.30.1 | E2E tests (old protocol) | Yes | 1.30.1 at `~/go/bin/buf` | -- |
| buf CLI v1.69.0 | E2E tests (modern protocol) | Yes | 1.69.0 at `/usr/local/bin/buf` (Homebrew) | -- |
| TLS certs | E2E server startup | Yes | `~/local-tls/server/` (cert+key) | -- |
| GitHub API token | E2E handler calls | No env var set | -- | Must be provided via env var at test time |
| `go build` | Compilation verification | Yes | passes clean | -- |
| `go vet` | Interface satisfaction | Yes | passes clean | -- |

**Missing dependencies with no fallback:**
- GitHub API token: Not set as environment variable. The test must read it from an env var (e.g., `EASYP_GITHUB_TOKEN`) and write it into a temp config file. Tests should `t.Skip()` if not available.

**Missing dependencies with fallback:**
- None -- all other dependencies are available.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + stretchr/testify v1.8.4 |
| Config file | none -- see Wave 0 |
| Quick run command | `go test ./e2e/ -run TestSmoke -v -count=1` |
| Full suite command | `go test ./e2e/ -v -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| HAND-01 | Handler structs embed Unimplemented types | compilation (already verified) | `go build ./...` | N/A -- zero code change needed |
| HAND-02 | Existing RPCs compile and serve correctly | E2E smoke | `go test ./e2e/ -run TestSmokeBufModUpdate -v` | Wave 0 |
| HAND-02 | GetModulePins returns correct ModulePin | E2E smoke (implicit) | Covered by buf mod update success | Wave 0 |
| HAND-02 | DownloadManifestAndBlobs returns blobs | E2E smoke (implicit) | Covered by buf mod update success | Wave 0 |
| HAND-02 | RepositoryService RPCs work | E2E smoke (implicit) | Covered by buf mod update success | Wave 0 |
| HAND-03 | manifest_digest left empty | compilation (verified) | `go vet ./...` | N/A -- no code change |
| HAND-04 | GetSDKInfo returns Unimplemented | compilation (verified) | `go build ./...` | N/A -- Unimplemented embedding |

### Sampling Rate
- **Per task commit:** `go build ./... && go vet ./...`
- **Per wave merge:** `go test ./... -v` (unit + E2E if token available)
- **Phase gate:** E2E smoke test with both buf versions passes

### Wave 0 Gaps
- [ ] `e2e/smoke_test.go` -- E2E smoke test for buf mod update with both CLI versions
- [ ] E2E test directory: `e2e/` does not exist yet

## Planning Guidance

### Recommended Approach

The implementation is ordered by dependency and risk:

1. **Verify compilation baseline** (5 min) -- Confirm `go build ./...` and `go vet ./...` pass clean. This is already verified but should be the first task to establish a baseline.

2. **Create E2E test infrastructure** (15 min) -- Create `e2e/` directory and `e2e/smoke_test.go`. Implement:
   - Test helper: allocate free port, write temp config with TLS paths and GitHub token from env var
   - Test helper: start server subprocess (`go run ./cmd/easyp -cfg temp.yml`)
   - Test helper: wait for TCP port to be listening
   - Test helper: create minimal buf module (buf.yaml with dependency)
   - Test helper: run `buf mod update` and verify exit code 0

3. **Run E2E smoke test with buf v1.30.1** (10 min) -- Use `~/go/bin/buf` (v1.30.1). Verify `buf mod update` succeeds. This validates HAND-02 for old protocol.

4. **Run E2E smoke test with buf v1.69.0** (10 min) -- Use `/usr/local/bin/buf` (v1.69.0). Verify `buf mod update` succeeds. This validates HAND-02 for modern protocol.

### Key Decisions for Planner

1. **Test file location:** `e2e/smoke_test.go` as a separate package (not `internal/connect/`) because it tests the full server binary, not internal packages.

2. **GitHub token source:** Read from `EASYP_GITHUB_TOKEN` environment variable. Skip test if not set. This allows CI configuration while keeping the token out of the test code.

3. **Buf binary paths:**
   - Old: `~/go/bin/buf` (v1.30.1)
   - Modern: `/usr/local/bin/buf` (v1.69.0, Homebrew)
   - Or make paths configurable via env vars: `BUF_OLD_PATH`, `BUF_NEW_PATH`

4. **Test config pattern:** Write a YAML config file to `t.TempDir()` with the test-specific port, TLS cert paths, and GitHub token. Use the existing `config.Config` YAML structure.

5. **Module dependency for test:** Use a known public GitHub repo already in the config (e.g., `googleapis/googleapis` with path `google/type/`). The test buf.yaml should reference this as a dependency via the proxy domain.

### Risk Areas

1. **buf mod update may require specific buf.yaml format** -- Different buf versions may expect different buf.yaml fields. Mitigation: use the simplest valid buf.yaml format and iterate if needed.

2. **TLS cert trust** -- If the self-signed cert is not trusted by the buf CLI, the test will fail with x509 errors. Mitigation: certs at `~/local-tls/server/` are already trusted (added to system CA store previously).

3. **Modern buf CLI may call GetSDKInfo** -- If buf v1.69.0 calls GetSDKInfo during `buf mod update`, it will receive `CodeUnimplemented`. This could cause the test to fail. Per decision D-01, this is acceptable and will be discovered empirically. If it fails, we escalate to the user rather than implementing a stub.

4. **Modern buf CLI may require manifest_digest** -- If buf v1.69.0 requires the `manifest_digest` field populated in ModulePin responses, the test will fail. Per decision D-02, this is acceptable and will be discovered empirically.

5. **Server startup time** -- `go run` compiles before starting, which adds latency. Mitigation: use a generous timeout (10s) for the TCP readiness check.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The `~/local-tls/server/` CA cert is already trusted by the system | Environment Availability | Tests fail with x509 errors -- need to add CA to trust store |
| A2 | `buf mod update` is the correct command to test end-to-end protocol flow | Planning Guidance | May need `buf dep update` or other commands per version |
| A3 | A minimal buf.yaml with a single dependency is sufficient for `buf mod update` to trigger GetModulePins + Download RPCs | Planning Guidance | Test may not exercise the full RPC chain |
| A4 | The test does not need to handle mTLS (no CA cert in config) | Environment Availability | Server may require client certs -- check config TLS section |

## Open Questions

1. **buf.yaml format for proxy testing**
   - What we know: buf.yaml needs at least one `dep` entry pointing to the proxy domain
   - What's unclear: Exact format that works with both v1.30.1 and v1.69.0 buf CLI
   - Recommendation: Use `buf.build` domain format with proxy DNS override, or configure buf registry URL directly

2. **Whether modern buf CLI calls GetSDKInfo during mod update**
   - What we know: GetSDKInfo is in the v1.69.0 proto but the proxy returns Unimplemented
   - What's unclear: Whether buf v1.69.0 actually calls it during `buf mod update`
   - Recommendation: Run the test and observe. If it fails, log the error and report to user as a Phase 5 escalation.

## Sources

### Primary (HIGH confidence)
- `go.mod` -- verified dependency versions
- `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/resolve.connect.go` -- generated handler interfaces
- `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/repository.connect.go` -- generated handler interfaces
- `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/download.connect.go` -- generated handler interfaces
- `gen/proto/buf/alpha/module/v1alpha1/module.pb.go` -- ModulePin type with ManifestDigest field
- `internal/connect/api.go` -- handler struct definition
- `internal/connect/modulepins.go` -- GetModulePins implementation
- `internal/connect/blobs.go` -- DownloadManifestAndBlobs implementation
- `internal/connect/bynames.go` -- RepositoryService implementations
- `cmd/easyp/main.go` -- server entry point

### Secondary (MEDIUM confidence)
- `cmd/easyp/internal/config/config.go` -- configuration structure
- `internal/https/https.go` -- TLS server implementation
- `internal/providers/content/repo.go` -- provider interface types
- `local.config.yml` -- example configuration file

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all verified from go.mod and generated code
- Architecture: HIGH -- handler code and generated interfaces fully read and analyzed
- Pitfalls: MEDIUM -- based on project-specific analysis and Go ecosystem knowledge; E2E test pitfalls are well-understood patterns

**Research date:** 2026-05-07
**Valid until:** 2026-06-07 (stable -- no fast-moving dependencies)

---

*Phase: 02-handler-adaptation*
*Research completed: 2026-05-07*
