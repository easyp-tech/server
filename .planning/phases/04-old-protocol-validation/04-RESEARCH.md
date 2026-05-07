# Phase 4: Old Protocol Validation - Research

**Researched:** 2026-05-07
**Domain:** Buf CLI v1.30.1 protocol compatibility, Go e2e testing with real binaries
**Confidence:** HIGH

## Summary

Phase 4 validates backward compatibility: confirming that the updated proxy still serves buf v1.30.1 clients correctly. The existing smoke test (`e2e/smoke_test.go`) already validates OLD-01 (`buf mod update` with v1.30.1). Phase 4's primary deliverable is OLD-02.

A critical finding from this research: **buf v1.30.1 does NOT have the `buf dep update` command**. The `buf dep` command family was introduced in buf v1.32.0. In v1.30.1, the only available command is `buf mod update`, which is functionally identical to `buf dep update` (the rename was a CLI restructuring, not a protocol change). This means OLD-02 as stated ("`buf dep update` succeeds against the proxy using buf v1.30.1 binary") cannot be fulfilled literally -- the command does not exist in that version.

The test infrastructure from Phase 3 (`e2e/testutil/`) provides all needed helpers: `StartServer`, `GetBuf`, `RunBufModUpdate`, `RequireEnvToken`, and `DefaultTestConfig`. No new dependencies are needed. The phase is a straightforward extension of the existing test patterns.

**Primary recommendation:** OLD-02 must be reinterpreted. Since v1.30.1 only has `buf mod update`, the valid backward compatibility test is running `buf mod update` twice (first run creates `buf.lock`, second run validates the lock file update path) or validating that `buf mod update` succeeds with an existing `buf.lock` present. The planner should flag this for user confirmation.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** OLD-01 is verified by the existing smoke test (`e2e/smoke_test.go` `TestSmokeBufModUpdate` with `buf_v1.30.1` subtest). No additional test needed.
- **D-02:** Two-step test -- first run `buf mod update` to create `buf.lock`, then run `buf dep update` on the same workspace. Both commands use v1.30.1 binary against the real TLS proxy.
- **D-03:** Validation is exit code 0 only -- keep tests simple. No buf.lock content inspection.
- **D-04:** On test failure, surface the server subprocess output (proxy logs) in the test failure message. The `StartServer` helper already captures stdout/stderr in a buffer.

### Claude's Discretion
- Test file location and naming (follow existing `e2e/` convention)
- Whether to add a `RunBufDepUpdate` helper to testutil or implement inline
- Whether OLD-02 test should share the test case structure from the smoke test or be standalone

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| OLD-01 | `buf mod update` succeeds against the proxy using buf v1.30.1 binary with real GitHub provider | Already verified by existing `TestSmokeBufModUpdate` smoke test (D-01). No new test needed. |
| OLD-02 | `buf dep update` succeeds against the proxy using buf v1.30.1 binary with real GitHub provider | **BLOCKED:** `buf dep update` does not exist in v1.30.1. See Critical Finding below. Equivalent test is `buf mod update` with existing `buf.lock`, or two successive `buf mod update` runs. Needs user clarification. |
</phase_requirements>

## Critical Finding: buf v1.30.1 Has No `buf dep update`

**Confidence:** HIGH (verified empirically)

buf v1.30.1 binary was tested directly:

```
$ ./testdata/buf/v1.30.1/buf dep update
unknown command "dep" for "buf"
```

The `buf dep` command family was introduced in **buf v1.32.0** [CITED: https://buf.build/blog/buf-cli-next-generation, https://buf.build/docs/migration-guides/migrate-v2-config-files]. In v1.30.1, the only available command is `buf mod update`, which is functionally identical:

| Command | Version Available | Status | Function |
|---------|-------------------|--------|----------|
| `buf mod update` | v1.30.1 and later | Deprecated since v1.32.0 | Updates buf.lock with latest dependency digests |
| `buf dep update` | v1.32.0 and later | Current canonical | Same function, renamed |

**Impact on OLD-02:** The requirement states "buf dep update succeeds against the proxy using buf v1.30.1 binary" -- this is impossible as written because v1.30.1 does not have this command.

**Options for the planner:**
1. **Reinterpret OLD-02 as "buf mod update with existing buf.lock"** -- run `buf mod update` twice: first creates `buf.lock`, second validates update on existing lock. This exercises the same RPC path (GetModulePins + DownloadManifestAndBlobs) that `buf dep update` would exercise.
2. **Change OLD-02 to use v1.69.0 binary** -- move the `buf dep update` test to Phase 5 (New Protocol Validation) where v1.69.0 has the command.
3. **Split OLD-02** -- OLD-02a tests `buf mod update` update path with v1.30.1, and create NEW-03 in Phase 5 for `buf dep update` with v1.69.0.

**Recommendation:** Option 1. Running `buf mod update` twice exercises the protocol path that matters (re-resolving module pins through the proxy). The command rename is cosmetic -- both commands call the same RPCs.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Test execution (buf binary invocation) | Test infrastructure | -- | e2e tests drive buf CLI subprocess via os/exec |
| Proxy server subprocess | Test infrastructure | -- | StartServer launches real proxy via `go run` |
| TLS termination | Proxy server | Test infrastructure (certs) | Proxy handles TLS; tests provide cert paths |
| RPC handling (GetModulePins, DownloadManifestAndBlobs) | Proxy server | -- | Production code, unchanged in this phase |
| GitHub API calls | GitHub (external) | Proxy server (delegates) | Proxy makes real API calls during tests |
| Test validation (exit code checking) | Test infrastructure | -- | Tests assert on buf command exit code |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go `testing` | 1.22 (go.mod) | Test framework, t.Helper, t.TempDir, t.Cleanup, t.Parallel | Standard library -- no alternative needed |
| `stretchr/testify` | v1.8.4 (go.mod) | `require` for setup, `assert` for checks | Already in go.mod, used throughout e2e tests |
| `os/exec` (stdlib) | -- | Subprocess management for buf CLI | Standard library, already used by RunBufModUpdate |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `net` (stdlib) | -- | TCP polling for server readiness | Every test that starts a server |
| `context` (stdlib) | -- | Timeout management for buf commands | Buf commands need 60s timeout |
| `fmt` (stdlib) | -- | Test failure messages with server output | Failure diagnostics (D-04) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `os/exec` for buf commands | `exec.Command` via helper function | Helper function is already the pattern in testutil -- use it |
| Inline buf execution in test | New `RunBufDepUpdate` helper in testutil | Helper is cleaner for reuse in Phase 5; inline is simpler for one-off |

**Installation:**
No new dependencies required. All libraries are already in `go.mod` or are standard library.

## Architecture Patterns

### System Architecture Diagram

```
Phase 4 Test File (e2e/)
  |
  |-- testutil.RequireEnvToken(t, "EASYP_GITHUB_TOKEN")
  |       |-- Skips test if env var not set
  |
  |-- testutil.GetBuf(t, BufV130)
  |       |-- Returns path to testdata/buf/v1.30.1/buf
  |
  |-- testutil.StartServer(t, cfg)
  |       |-- Allocates free port (net.Listen :0)
  |       |-- Generates YAML config into t.TempDir()
  |       |-- exec.Command("go", "run", "./cmd/easyp", "-cfg", path)
  |       |-- TCP polls until server accepts connections (30s timeout)
  |       |-- t.Cleanup kills subprocess
  |       |-- Captures server stdout/stderr for diagnostics
  |       |-- Returns port number
  |
  |-- testutil.RunBufModUpdate(t, bufPath, port)     [FIRST RUN]
  |       |-- Creates t.TempDir() workspace
  |       |-- Writes buf.yaml with dependency on proxy domain
  |       |-- Runs: buf mod update
  |       |-- Validates buf.lock created
  |       |-- Returns (exitCode, stderr)
  |
  |-- NEW: buf mod update on same workspace            [SECOND RUN]
  |       |-- Runs: buf mod update (again, on existing workspace)
  |       |-- Exercises update-with-existing-lock code path
  |       |-- Returns (exitCode, stderr)
  |
  v
Proxy Server (subprocess, real compiled binary)
  |
  |-- TLS via ~/local-tls/server/ certs
  |-- Connect RPC handlers (ResolveService, DownloadService, RepositoryService)
  |-- Real GitHub API calls (via EASYP_GITHUB_TOKEN)
  |
  v
GitHub API (api.github.com)
```

### Recommended Project Structure
```
e2e/
  testutil/
    server.go        # StartServer, RunBufModUpdate (existing)
    bufbin.go        # GetBuf, version constants (existing)
    config.go        # TestConfig, DefaultTestConfig, generateConfigYAML (existing)
    testutil_test.go # Internal validation tests (existing)
  smoke_test.go      # TestSmokeBufModUpdate (existing, validates OLD-01)
  old_proto_test.go  # NEW: TestOldProtocolBufDepUpdate (validates OLD-02)
testdata/
  buf/
    v1.30.1/buf      # Cached buf binary (existing)
```

### Pattern 1: Two-Step buf Command Test
**What:** Run `buf mod update` twice against the same workspace -- first creates `buf.lock`, second validates update on existing lock.
**When to use:** Validating OLD-02 (backward compatibility of dependency resolution).
**Example:**
```go
// Source: Derived from existing RunBufModUpdate pattern [VERIFIED: codebase]
func TestOldProtocolBufDepUpdate(t *testing.T) {
    token := testutil.RequireEnvToken(t, "EASYP_GITHUB_TOKEN")

    cfg := testutil.DefaultTestConfig()
    cfg.GithubToken = token

    t.Run("buf_v1.30.1", func(t *testing.T) {
        t.Parallel()

        bufPath := testutil.GetBuf(t, testutil.BufV130)
        port := testutil.StartServer(t, cfg)

        // Step 1: Run buf mod update to create buf.lock
        exitCode, stderr := testutil.RunBufModUpdate(t, bufPath, port)
        if exitCode != 0 {
            t.Fatalf("buf mod update (initial) failed (exit %d): %s", exitCode, stderr)
        }

        // Step 2: Run buf mod update again on same workspace
        // This exercises the same RPC path as "buf dep update" would
        exitCode, stderr = runBufModUpdateOnWorkspace(t, bufPath, port, workspace)
        if exitCode != 0 {
            t.Fatalf("buf mod update (re-run) failed (exit %d): %s", exitCode, stderr)
        }
    })
}
```

### Pattern 2: Failure Diagnostics with Server Output
**What:** On test failure, include the proxy server subprocess output in the failure message.
**When to use:** Every test that starts a server via StartServer.
**Example:**
```go
// Source: CONTEXT.md D-04 [VERIFIED: user decision]
// StartServer captures server output. To surface it in failures,
// the test needs access to the server output buffer.
// Option A: Modify StartServer to return the buffer
// Option B: Use a test-level variable that t.Cleanup can populate

// Simplest approach: check server logs from the captured buffer
if exitCode != 0 {
    t.Fatalf("buf command failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
        exitCode, serverOutput, stderr)
}
```

### Anti-Patterns to Avoid
- **Testing `buf dep update` with v1.30.1:** The command does not exist in that version. Always verify command availability before writing tests.
- **Sharing workspace between parallel subtests:** Each subtest must have its own temp directory. The existing `RunBufModUpdate` creates its own `t.TempDir()` internally, but for the two-step pattern, the workspace must persist between calls -- cannot use `RunBufModUpdate` as-is for the second step.
- **Hardcoding v1.30.1 in test logic:** Use the `testutil.BufV130` constant instead of string literals.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Workspace setup for buf commands | Custom temp dir + file writing | Extend `RunBufModUpdate` pattern or write a new helper that returns the workspace path | `RunBufModUpdate` creates workspace internally -- for two-step testing, need a variant that returns workspace path |
| Server output capture | Modify StartServer to return buffer | StartServer already captures stdout/stderr in `bytes.Buffer` -- need to expose it | The buffer exists but is local to StartServer; need to make it accessible for failure messages |

**Key insight:** The two changes needed (exposing server output buffer, workspace path from RunBufModUpdate) are small extensions to existing helpers, not new infrastructure.

## Common Pitfalls

### Pitfall 1: RunBufModUpdate Creates New Workspace Each Call
**What goes wrong:** Calling `RunBufModUpdate` twice creates two separate temp directories. The second call has no `buf.lock` from the first call.
**Why it happens:** `RunBufModUpdate` calls `t.TempDir()` internally and writes a fresh `buf.yaml`.
**How to avoid:** For OLD-02, either: (a) write a new helper that accepts an existing workspace directory, or (b) write inline test code that manages the workspace lifecycle directly. The CONTEXT.md says Claude's discretion on whether to add a helper or implement inline.
**Warning signs:** Test passes trivially because each invocation is independent -- not actually testing the "update existing lock" path.

### Pitfall 2: buf mod update Re-resolves All Dependencies on Every Run
**What goes wrong:** The second `buf mod update` call re-resolves dependencies, which hits GitHub API again. This is correct behavior but may fail if the API is rate-limited or flaky.
**Why it happens:** `buf mod update` always fetches latest digests, even when `buf.lock` exists.
**How to avoid:** This is expected behavior -- the test should handle it. Use a reasonable timeout (60s as in existing RunBufModUpdate).
**Warning signs:** Test flakes with timeout errors on slow network connections.

### Pitfall 3: Server Output Not Available on Test Failure
**What goes wrong:** Test fails but the failure message only shows buf's stderr, not the proxy server logs.
**Why it happens:** `StartServer` captures output in a local `bytes.Buffer` that is not returned to the caller.
**How to avoid:** Either: (a) modify `StartServer` to return the output buffer alongside the port, or (b) create a wrapper that captures the buffer. Per D-04, this is required for all Phase 4 tests.
**Warning signs:** Test failure messages only show "exit code 1" with no server-side context.

### Pitfall 4: Assuming buf dep update Exists in v1.30.1
**What goes wrong:** Writing a test that calls `buf dep update` with v1.30.1 binary produces "unknown command" error.
**Why it happens:** The `buf dep` family was introduced in v1.32.0. v1.30.1 only has `buf mod` subcommands.
**How to avoid:** Always verify command availability with `buf --help` or `buf <subcommand> --help` before writing tests.
**Warning signs:** Test fails immediately with "unknown command 'dep' for 'buf'".

## Code Examples

### OLD-02 Test with Two-Step Pattern (Inline Approach)
```go
// Source: Derived from existing smoke_test.go and RunBufModUpdate [VERIFIED: codebase]
package e2e

import (
    "bytes"
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "testing"

    "github.com/easyp-tech/server/e2e/testutil"
)

func TestOldProtocolBufModUpdateTwice(t *testing.T) {
    token := testutil.RequireEnvToken(t, "EASYP_GITHUB_TOKEN")

    cfg := testutil.DefaultTestConfig()
    cfg.GithubToken = token

    bufPath := testutil.GetBuf(t, testutil.BufV130)
    port := testutil.StartServer(t, cfg)

    // Create workspace
    tmpDir := t.TempDir()
    bufYAML := fmt.Sprintf(`version: v1
deps:
  - 127.0.0.1:%d/googleapis/googleapis
`, port)
    if err := os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600); err != nil {
        t.Fatalf("writing buf.yaml: %v", err)
    }

    // Step 1: buf mod update (creates buf.lock)
    runBuf := func() (int, string) {
        ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
        defer cancel()
        cmd := exec.CommandContext(ctx, bufPath, "mod", "update")
        cmd.Dir = tmpDir
        cmd.Env = os.Environ()
        var stderr bytes.Buffer
        cmd.Stderr = &stderr
        err := cmd.Run()
        if err != nil {
            if exitErr, ok := err.(*exec.ExitError); ok {
                return exitErr.ExitCode(), stderr.String()
            }
            return 1, stderr.String()
        }
        return 0, stderr.String()
    }

    if exitCode, stderr := runBuf(); exitCode != 0 {
        t.Fatalf("first buf mod update failed (exit %d): %s", exitCode, stderr)
    }

    // Verify buf.lock was created
    if _, err := os.Stat(filepath.Join(tmpDir, "buf.lock")); err != nil {
        t.Fatalf("buf.lock not created after first buf mod update: %v", err)
    }

    // Step 2: buf mod update again (updates existing buf.lock)
    if exitCode, stderr := runBuf(); exitCode != 0 {
        t.Fatalf("second buf mod update failed (exit %d): %s", exitCode, stderr)
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `buf mod update` | `buf dep update` | buf v1.32.0 | Same function, renamed. Old command still works but shows deprecation warning. |
| `buf mod clear-cache` | `buf registry cc` | buf v1.32.0 | Cache clearing moved to registry subcommand. |

**Deprecated/outdated:**
- `buf mod update`: Deprecated since v1.32.0, replaced by `buf dep update`. Still functional for backward compatibility.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Running `buf mod update` twice exercises the same RPC code path as `buf dep update` would | Critical Finding, Patterns | Test does not validate the intended behavior -- need to verify protocol calls are identical |
| A2 | The second `buf mod update` call re-resolves dependencies (hits GetModulePins + DownloadManifestAndBlobs RPCs) | Critical Finding | Test is trivially passing without exercising the protocol |
| A3 | No code changes to production server are needed for this phase | Summary | Phase scope expands unexpectedly |
| A4 | The `RunBufModUpdate` helper's workspace creation pattern is correct for OLD-02 two-step testing | Pitfall 1 | Need to write workspace management inline or extend the helper |

**Risk assessment:**
- A1: MEDIUM -- `buf mod update` and `buf dep update` are documented as functionally identical [CITED: buf.build docs]. Both call GetModulePins RPC. LOW risk of behavioral difference.
- A2: HIGH confidence -- `buf mod update` always resolves dependencies against the registry/proxy. The second run will hit the same RPCs.
- A3: HIGH confidence -- Phase 4 is validation only, confirmed by CONTEXT.md and REQUIREMENTS.md.
- A4: HIGH confidence -- the workspace pattern is straightforward Go; the pitfall is about `RunBufModUpdate`'s internal `t.TempDir()`, not the pattern itself.

## Open Questions

1. **How should OLD-02 be reinterpreted given v1.30.1 lacks `buf dep update`?**
   - What we know: v1.30.1 only has `buf mod update`. The `buf dep update` command was introduced in v1.32.0. Both commands are functionally identical (same RPC calls).
   - What's unclear: Whether the user intended OLD-02 to test a different code path than OLD-01, or simply to confirm that dependency operations work in general with v1.30.1.
   - Recommendation: Run `buf mod update` twice -- first creates `buf.lock`, second updates it. This validates a different execution path than the initial smoke test (fresh vs. existing lock). Flag for user confirmation.

2. **Should StartServer return the server output buffer?**
   - What we know: D-04 requires surfacing server output on failure. StartServer currently captures output in a local buffer.
   - What's unclear: Whether to modify StartServer's signature (breaking change to existing tests) or add a separate mechanism.
   - Recommendation: Add a `ServerOutput()` method or return a struct `{Port int; Output *bytes.Buffer}`. This is at Claude's discretion per the existing helper design.

3. **Should the OLD-02 test use table-driven subtests like the smoke test?**
   - What we know: The smoke test uses table-driven subtests for v1.30.1 and v1.69.0. OLD-02 is specifically v1.30.1 only (Phase 5 handles v1.69.0).
   - What's unclear: Whether to include v1.69.0 in the same test for completeness.
   - Recommendation: No -- Phase 5 handles NEW-01/NEW-02 with v1.69.0. Keep Phase 4 focused on v1.30.1 only.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build + `go run` server | Yes | go1.26.1 darwin/arm64 | -- |
| TLS certs (`~/local-tls/server/`) | Server startup | Yes | server-cert.pem, server-key.pem | -- |
| `EASYP_GITHUB_TOKEN` | GitHub API calls | No (not in shell env) | -- | test.env file exists with token; `source test.env` before running |
| buf v1.30.1 binary | Old protocol tests | Yes | `testdata/buf/v1.30.1/buf` | Auto-download via testutil GetBuf |
| buf v1.69.0 binary | NOT NEEDED this phase | No (not cached) | -- | Not required for Phase 4 |
| Internet connectivity | GitHub API calls | Yes | -- | Tests skip gracefully |

**Missing dependencies with no fallback:**
- `EASYP_GITHUB_TOKEN` is not set in the current shell environment. The `test.env` file contains the token but must be sourced before running tests. The planner should include a note about this, or tests should use `RequireEnvToken` which skips gracefully.

**Missing dependencies with fallback:**
- buf v1.69.0 not cached -- NOT needed for Phase 4 (Phase 5 only).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + `stretchr/testify` v1.8.4 |
| Config file | none -- Go test framework uses convention (`*_test.go`) |
| Quick run command | `go test ./e2e/ -count=1 -timeout 120s -run TestOldProtocol -v` |
| Full suite command | `go test ./e2e/... -count=1 -timeout 300s -v` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| OLD-01 | `buf mod update` succeeds with v1.30.1 against real proxy + GitHub | e2e (existing) | `go test ./e2e/ -run TestSmokeBufModUpdate/buf_v1.30.1 -count=1` | Yes: `e2e/smoke_test.go` |
| OLD-02 | `buf dep update` equivalent with v1.30.1 -- two-step buf mod update | e2e (new) | `go test ./e2e/ -run TestOldProtocol -count=1` | Wave 0: new file needed |

### Sampling Rate
- **Per task commit:** `go test ./e2e/ -count=1 -timeout 120s -run TestOldProtocol`
- **Per wave merge:** `go test ./e2e/... -count=1 -timeout 300s`
- **Phase gate:** Full suite green including OLD-01 (smoke test) and OLD-02 (new test)

### Wave 0 Gaps
- [ ] `e2e/old_proto_test.go` (or similar) -- covers OLD-02 with two-step buf mod update pattern
- [ ] Extend `StartServer` or `RunBufModUpdate` to support failure diagnostics (D-04)

*(If no other gaps: existing test infrastructure covers everything else)*

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | N/A -- test infrastructure, no user auth |
| V3 Session Management | no | N/A |
| V4 Access Control | no | N/A -- test-only code |
| V5 Input Validation | partial | Config generation validates required fields |
| V6 Cryptography | no | TLS certs are fixtures, not generated |

### Known Threat Patterns for Test Infrastructure

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Token exposure in test output | Information Disclosure | Use env vars; never log token value; config file mode 0600 |
| Token exposure in git | Information Disclosure | test.env is untracked; `.gitignore` excludes it |

## Sources

### Primary (HIGH confidence)
- Empirical verification: `testdata/buf/v1.30.1/buf dep update` returns "unknown command" [VERIFIED: local binary execution]
- Empirical verification: `testdata/buf/v1.30.1/buf mod update --help` shows available subcommands [VERIFIED: local binary execution]
- Codebase: `e2e/smoke_test.go` -- existing smoke test pattern [VERIFIED: codebase]
- Codebase: `e2e/testutil/` -- full helper package [VERIFIED: codebase]
- Codebase: `internal/connect/` -- handler implementations showing RPC paths [VERIFIED: codebase]

### Secondary (MEDIUM confidence)
- Buf CLI migration guide: `buf mod update` renamed to `buf dep update` in v1.32.0 [CITED: https://buf.build/docs/migration-guides/migrate-v2-config-files]
- Buf CLI blog: next generation CLI backwards compatibility [CITED: https://buf.build/blog/buf-cli-next-generation]
- Buf CLI docs: `buf dep update` description and usage [CITED: Context7 /websites/buf_build]

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all existing in go.mod
- Architecture: HIGH -- extending proven test patterns from Phase 3
- Pitfalls: HIGH -- critical pitfall (buf dep update missing in v1.30.1) verified empirically

**Research date:** 2026-05-07
**Valid until:** 2026-06-07 (stable -- buf v1.30.1 binary behavior will not change)
