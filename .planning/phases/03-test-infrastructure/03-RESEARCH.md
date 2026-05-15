# Phase 3: Test Infrastructure - Research

**Researched:** 2026-05-07
**Domain:** Go test helpers, subprocess lifecycle, binary download/caching, YAML config generation
**Confidence:** HIGH

## Summary

Phase 3 extracts the inline test infrastructure from `e2e/smoke_test.go` into a reusable `e2e/testutil/` package, then extends it with automated buf binary management. The existing smoke test already contains working implementations of all three core capabilities (server startup, buf execution, config generation). The task is primarily a refactor-and-extend operation, not greenfield development.

The server lifecycle pattern (allocate port, generate YAML config, start subprocess via `go run`, TCP poll for readiness, cleanup via context cancellation) is battle-tested in the Phase 2 smoke test. The buf binary management requires new code: downloading pinned versions from GitHub releases with platform-specific asset names, caching to `testdata/buf/`, and providing a path resolver for test consumers.

The Go testing ecosystem is well-suited to this work. The standard `testing` package provides `t.TempDir()`, `t.Cleanup()`, and `t.Parallel()` -- all of which are already used in the smoke test. The `stretchr/testify` package (already in `go.mod`) provides `require` for setup assertions. No new external dependencies are needed.

**Primary recommendation:** Extract the three helpers from `e2e/smoke_test.go` into `e2e/testutil/server.go`, `e2e/testutil/bufbin.go`, and `e2e/testutil/config.go`. Add GitHub release download logic to `bufbin.go`. Wire the existing smoke test to use the new package. Keep it simple -- this is infrastructure for Phases 4 and 5.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Subprocess approach -- start the proxy as a subprocess (via `go run` or pre-built binary). Tests the real compiled binary end-to-end, matching what users experience. Do not switch to in-process httptest.
- **D-02:** TCP poll for readiness -- keep the existing approach of polling the TCP port until the server accepts connections. Do not add readiness signals to production code.
- **D-03:** Config struct to YAML -- helper accepts a Go config struct and generates the YAML config file into `t.TempDir()`. This is what the existing smoke test does inline; formalize it into a reusable function.
- **D-04:** Auto-download from GitHub releases -- the helper downloads pinned buf versions from `github.com/bufbuild/buf/releases` if not found locally. Makes the test suite self-contained; no manual binary setup required.
- **D-05:** Cache in project `testdata/` -- downloaded binaries are stored in `testdata/buf/v{version}/buf` (or platform-specific subdirectory). Tests check cache first, download only if missing. Add `testdata/buf/` to `.gitignore`.
- **D-06:** Helpers live in `e2e/testutil/` -- separate package scoped to integration/e2e tests. Importable by Phases 4 and 5 test files.
- **D-07:** Split by concern -- three files: `server.go` (startServer, port allocation, config generation), `bufbin.go` (binary download, cache management, version assertion), `config.go` (test config struct and YAML generation). Each file has a single responsibility.

### Claude's Discretion
- Exact config struct field names and types -- follow existing `cmd/easyp/internal/config/config.go` patterns.
- Whether to use `go run` or pre-build the binary -- optimize for test speed as long as it's a subprocess.
- Platform detection logic for buf binary download (darwin/linux, amd64/arm64).
- Whether to verify checksums on downloaded binaries.
- How to handle the existing `e2e/smoke_test.go` -- refactor to use new helpers or leave as-is.

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TINF-01 | Test helper programmatically starts and stops the proxy server with TLS using `~/local-tls/server/` certs | server.go extraction from smoke_test.go (D-01, D-02, D-03) |
| TINF-02 | Buf binary v1.30.1 and v1.69.0+ pinned and managed for test execution (downloaded or path-configured) | bufbin.go with GitHub releases download (D-04, D-05) |
| TINF-03 | Test suite configured with GitHub API token for real API calls | config.go with env var pattern, `EASYP_GITHUB_TOKEN` |
| TINF-04 | Test GitHub repository identified/configured for test operations (repo with proto files) | config.go with `googleapis/googleapis` + `google/type/` path |
| TINF-05 | Tests can run in parallel without port conflicts or state interference | Port allocation via `net.Listen("tcp", "127.0.0.1:0")`, `t.Parallel()` |
| TINF-06 | Test configuration supports CI execution with environment-based setup | Env var tokens, env var TLS cert paths, auto-download binaries |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Server subprocess lifecycle | Test infrastructure | -- | Test helper manages `go run` subprocess; production code is unmodified |
| Port allocation | Test infrastructure | -- | OS assigns free port; no production code change |
| YAML config generation | Test infrastructure | -- | Generates config file into `t.TempDir()` for each test |
| Buf binary download/cache | Test infrastructure | -- | Downloads from GitHub releases, caches in testdata/ |
| TLS cert path resolution | Test infrastructure | -- | Reads from env var or home directory convention |
| GitHub token management | Test infrastructure | -- | Reads from env var, skips tests if missing |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go `testing` | 1.22 (go.mod) | Test framework, t.Helper, t.TempDir, t.Cleanup, t.Parallel | Standard library -- no alternatives considered |
| `stretchr/testify` | v1.8.4 (go.mod) | `require` for setup assertions, `assert` for test assertions | Already in go.mod, used in existing smoke test |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `net` (stdlib) | -- | Port allocation via `net.Listen("tcp", "127.0.0.1:0")` | Every test that starts a server |
| `os/exec` (stdlib) | -- | Subprocess management (`go run` for server, buf binary execution) | Server startup and buf commands |
| `runtime` (stdlib) | -- | OS/arch detection (`runtime.GOOS`, `runtime.GOARCH`) | Buf binary download platform resolution |
| `net/http` (stdlib) | -- | HTTP client for GitHub releases download | Buf binary auto-download |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `os/exec` + `go run` | `testcontainers` | testcontainers adds Docker dependency for no added benefit -- we test the real binary, not a containerized version |
| `net.Listen` port allocation | Hardcoded port range | Hardcoded ports cause flaky tests in parallel; zero-port allocation is the standard Go pattern |
| `net/http` GET for download | `go-getter` or similar | `net/http` is stdlib, sufficient for single-file GitHub release download; adding a dependency is not justified |

**Installation:**
No new dependencies required. All libraries are already in `go.mod` or are standard library.

## Architecture Patterns

### System Architecture Diagram

```
Test File (Phase 4/5)
  |
  |-- testutil.GetBuf(t, "v1.30.1") --> testdata/buf/v1.30.1/buf
  |       |                                  |
  |       |--- (cache miss) --> GitHub Releases API --> download to testdata/
  |       |                                  |
  |       |--- (cache hit) --> return cached path
  |
  |-- testutil.StartServer(t, TestConfig)
  |       |
  |       |-- Allocate free TCP port (net.Listen :0)
  |       |-- Generate YAML config into t.TempDir()
  |       |-- exec.Command("go", "run", "./cmd/easyp", "-cfg", path)
  |       |-- TCP poll until server accepts connections
  |       |-- t.Cleanup(kill subprocess)
  |       |-- Return (port, cleanup)
  |
  |-- exec.Command(bufPath, "mod", "update")
  |       |-- Runs buf CLI against 127.0.0.1:PORT
  |       |-- Validates protocol behavior
  |
  v
Proxy Server (subprocess, real binary)
  |
  |-- Reads YAML config from t.TempDir()
  |-- TLS via ~/local-tls/server/ certs
  |-- Connect RPC handlers
  |-- Real GitHub API calls (via EASYP_GITHUB_TOKEN)
  |
  v
GitHub API (api.github.com)
```

### Recommended Project Structure
```
e2e/
  testutil/
    server.go      # StartServer, port allocation, subprocess lifecycle
    bufbin.go      # GetBuf, download/cache, platform detection
    config.go      # TestConfig struct, YAML generation
  smoke_test.go    # Refactored to use testutil (or left as-is per discretion)
testdata/
  buf/             # Added to .gitignore
    v1.30.1/
      buf          # Downloaded binary (darwin-arm64)
    v1.69.0/
      buf          # Downloaded binary (darwin-arm64)
  cert.pem         # Existing test TLS cert
  key.pem          # Existing test TLS key
```

### Pattern 1: Test Helper with t.Helper() and t.Cleanup()
**What:** Go test helpers call `t.Helper()` so failure line numbers point to the calling test, not the helper. Use `t.Cleanup()` for resource teardown instead of returning cleanup functions.
**When to use:** Every exported function in `testutil/` that takes `*testing.T`.
**Example:**
```go
// Source: Go standard library pattern [VERIFIED: Context7 /golang/go]
func StartServer(t *testing.T, cfg TestConfig) int {
    t.Helper()

    port := allocatePort(t)
    cfgPath := generateConfig(t, cfg, port)
    cmd := startSubprocess(t, cfgPath)
    waitForReady(t, port)

    t.Cleanup(func() {
        cmd.Process.Kill()
        cmd.Wait()
    })

    return port
}
```

### Pattern 2: Platform-Specific Binary Path
**What:** Use `runtime.GOOS` and `runtime.GOARCH` to construct the GitHub release asset name following buf's naming convention.
**When to use:** Buf binary download in `bufbin.go`.
**Example:**
```go
// Buf release asset naming: buf-{OS}-{Arch}
// runtime.GOOS: "darwin", "linux", "windows"
// runtime.GOARCH: "arm64", "amd64"
// [VERIFIED: api.github.com/repos/bufbuild/buf/releases]
func assetName() string {
    os := runtime.GOOS   // "darwin"
    arch := runtime.GOARCH // "arm64"
    // Map Go arch to buf naming: "amd64" -> "x86_64", "arm64" stays "arm64"
    // Wait -- checking actual release assets...
    // v1.30.1 assets: buf-Darwin-arm64, buf-Darwin-x86_64, buf-Linux-aarch64, buf-Linux-x86_64
    // So: GOOS=darwin -> "Darwin", GOARCH=arm64 -> "arm64", GOARCH=amd64 -> "x86_64"
    return fmt.Sprintf("buf-%s-%s", capitalize(os), mapArch(arch))
}
```

### Pattern 3: Test Skip on Missing Prerequisites
**What:** Tests that require external resources (GitHub token, TLS certs) skip gracefully when those resources are unavailable.
**When to use:** Every test that calls `StartServer` or uses the GitHub token.
**Example:**
```go
// Source: Existing smoke_test.go pattern [VERIFIED: codebase]
func TestSomething(t *testing.T) {
    token := os.Getenv("EASYP_GITHUB_TOKEN")
    if token == "" {
        t.Skip("EASYP_GITHUB_TOKEN not set")
    }
    // ... test proceeds
}
```

### Anti-Patterns to Avoid
- **Returning cleanup functions instead of t.Cleanup():** The smoke test returns `(int, func())` -- prefer `t.Cleanup()` for automatic teardown ordering and cleaner test code. However, the existing pattern works and changing it is a style choice, not a correctness issue.
- **Global state in testutil:** Never use package-level variables for mutable state (ports, server processes). Every test should allocate its own resources via `t.TempDir()` and `net.Listen`.
- **Hardcoded binary paths:** Do not hardcode `~/go/bin/buf` or `/usr/local/bin/buf` in the helpers. Use the cache directory and auto-download. Existing hardcoded paths in smoke_test.go are the reason we need bufbin.go.
- **Skipping checksum verification without documentation:** If checksums are not verified (at Claude's discretion), document why in code comments.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| YAML serialization | Custom string formatting with `fmt.Sprintf` | `ghodss/yaml` (already in go.mod) or `gopkg.in/yaml.v3` | The server uses `ghodss/yaml` for deserialization. Using the same library for serialization ensures field naming consistency. However, the existing smoke test uses `fmt.Sprintf` and this works fine because the config is simple. The choice depends on complexity. |
| Port allocation | Random port number generation | `net.Listen("tcp", "127.0.0.1:0")` + close | OS assigns a free port atomically -- no race conditions |
| Temp directory management | Manual mkdir + rm | `t.TempDir()` | Auto-cleaned after test, no leak risk |
| HTTP file download | Custom TCP-based downloader | `net/http.Get` with `io.Copy` to file | Handles redirects (GitHub releases return 302), chunked transfer, etc. |
| Platform detection | Parsing `uname` output | `runtime.GOOS` / `runtime.GOARCH` | Stdlib, cross-platform, no subprocess |

**Key insight:** The existing smoke test already demonstrates that `fmt.Sprintf` for YAML config generation is adequate for this use case. The config is flat and simple. Switching to `ghodss/yaml.Marshal` would add type safety but is not strictly necessary -- this is at Claude's discretion per D-03.

## Common Pitfalls

### Pitfall 1: Port Reuse After Close
**What goes wrong:** After `listener.Close()`, another process grabs the same port before the test server starts.
**Why it happens:** There is a window between closing the listener and starting the subprocess where the port becomes available.
**How to avoid:** This is an accepted risk in Go testing. The window is tiny (< 1ms) and extremely unlikely on loopback. The existing smoke test uses this pattern successfully. No mitigation needed beyond documenting it.
**Warning signs:** Intermittent "address already in use" errors in parallel tests.

### Pitfall 2: GitHub API Rate Limiting on Downloads
**What goes wrong:** Repeated test runs trigger GitHub's unauthenticated API rate limit (60 requests/hour for API, but release downloads via redirect may differ).
**Why it happens:** Each `GetBuf` call might hit the GitHub API if cache validation uses HTTP HEAD requests.
**How to avoid:** The D-05 decision specifies cache-first: check `testdata/buf/v{version}/buf` exists locally before any network call. Only download on cache miss. Once cached, no network calls are needed.
**Warning signs:** Tests fail with HTTP 403 from GitHub during download.

### Pitfall 3: Subprocess Orphan on Test Panic
**What goes wrong:** If the test panics before `t.Cleanup` runs, the server subprocess keeps running and holds the port.
**Why it happens:** `t.Cleanup` runs even on panic in Go, so this is actually NOT a pitfall in Go's testing framework. However, if the test binary itself is killed (SIGKILL), cleanup cannot run.
**How to avoid:** Use `t.Cleanup()` (not deferred cleanup). Go's testing package guarantees cleanup runs even on panic. For SIGKILL scenarios, use process groups or PID-file-based cleanup in CI.
**Warning signs:** "address already in use" after a killed test run.

### Pitfall 4: Buf Binary Permission Denied
**What goes wrong:** Downloaded binary does not have execute permission.
**Why it happens:** `io.Copy` to a file preserves the file mode of the created file (default 0644 on most systems), not the execute bit.
**How to avoid:** After download, explicitly `os.Chmod(path, 0755)` to set execute permission.
**Warning signs:** "permission denied" when running buf commands.

### Pitfall 5: Config YAML Field Name Mismatch
**What goes wrong:** The test helper generates YAML with field names that don't match what the server's config parser expects.
**Why it happens:** The server uses `ghodss/yaml` which maps Go struct field names (or json tags) to YAML keys. The existing config struct uses `json` tags. The `fmt.Sprintf` approach in the smoke test uses the yaml key names directly.
**How to avoid:** Match the YAML key names used in the smoke test's `fmt.Sprintf` template exactly. The mapping is: `listen`, `domain`, `log.level`, `cache.type`, `tls.cert`, `tls.key`, `proxy.github[].token`, `proxy.github[].repo.owner`, `proxy.github[].repo.name`, `proxy.github[].repo.path`.
**Warning signs:** Server fails to start with config parse error.

### Pitfall 6: race Condition on testdata/buf/ Directory Creation
**What goes wrong:** Two parallel tests both try to create `testdata/buf/v1.30.1/` and download the binary simultaneously.
**Why it happens:** `testdata/` is shared across parallel test processes.
**How to avoid:** Use `os.MkdirAll` (idempotent) for directory creation. For the download itself, write to a temp file first, then `os.Rename` (atomic on same filesystem). Or accept the minor race: both downloads write the same content, and `os.Create` truncation is safe.
**Warning signs:** Corrupted binary file from concurrent writes (extremely rare).

## Code Examples

### Server Lifecycle (server.go)
```go
// Source: Derived from existing e2e/smoke_test.go [VERIFIED: codebase]
package testutil

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

// TestConfig holds the configuration for starting a test proxy server.
type TestConfig struct {
    TLSCertPath string
    TLSKeyPath  string
    GithubToken string
    RepoOwner   string
    RepoName    string
    RepoPaths   []string
    CacheType   string // "none" for tests
}

// StartServer starts the proxy as a subprocess and waits for readiness.
// Returns the allocated port. Server is cleaned up via t.Cleanup.
func StartServer(t *testing.T, cfg TestConfig) int {
    t.Helper()

    // Allocate free port
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatalf("allocating free port: %v", err)
    }
    port := listener.Addr().(*net.TCPAddr).Port
    listener.Close()

    // Generate config
    cfgPath := generateConfigYAML(t, cfg, port)

    // Start subprocess
    ctx, cancel := context.WithCancel(context.Background())
    t.Cleanup(cancel)

    cmd := exec.CommandContext(ctx, "go", "run", "./cmd/easyp", "-cfg", cfgPath)
    cmd.Dir = findProjectRoot(t)

    if err := cmd.Start(); err != nil {
        t.Fatalf("starting server: %v", err)
    }

    t.Cleanup(func() {
        cmd.Process.Kill()
        cmd.Wait()
    })

    // Poll for readiness
    addr := fmt.Sprintf("127.0.0.1:%d", port)
    deadline := time.Now().Add(30 * time.Second)
    for time.Now().Before(deadline) {
        conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
        if dialErr == nil {
            conn.Close()
            return port
        }
        time.Sleep(100 * time.Millisecond)
    }

    t.Fatalf("server not ready on port %d within 30s", port)
    return 0
}
```

### Buf Binary Management (bufbin.go)
```go
// Source: GitHub releases API pattern [VERIFIED: api.github.com]
package testutil

import (
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "runtime"
    "testing"
)

// Buf versions known to work with this proxy.
const (
    BufV130 = "v1.30.1"
    BufV169 = "v1.69.0"
)

// GetBuf returns the path to a pinned buf binary, downloading it if necessary.
// Binaries are cached at testdata/buf/{version}/buf.
func GetBuf(t *testing.T, version string) string {
    t.Helper()

    projectRoot := findProjectRoot(t)
    binDir := filepath.Join(projectRoot, "testdata", "buf", version)
    binPath := filepath.Join(binDir, "buf")

    // Check cache
    if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
        return binPath
    }

    // Download
    if err := os.MkdirAll(binDir, 0755); err != nil {
        t.Fatalf("creating buf cache dir: %v", err)
    }

    assetURL := fmt.Sprintf(
        "https://github.com/bufbuild/buf/releases/download/%s/buf-%s-%s",
        version, capitalizeOS(), mapArch(),
    )

    // Download to temp file first, then rename (atomic)
    tmpPath := binPath + ".tmp"
    if err := downloadFile(tmpPath, assetURL); err != nil {
        os.Remove(tmpPath)
        t.Fatalf("downloading buf %s: %v", version, err)
    }

    if err := os.Chmod(tmpPath, 0755); err != nil {
        os.Remove(tmpPath)
        t.Fatalf("chmod buf binary: %v", err)
    }

    if err := os.Rename(tmpPath, binPath); err != nil {
        os.Remove(tmpPath)
        t.Fatalf("renaming buf binary: %v", err)
    }

    return binPath
}

func capitalizeOS() string {
    switch runtime.GOOS {
    case "darwin":
        return "Darwin"
    case "linux":
        return "Linux"
    default:
        return runtime.GOOS
    }
}

func mapArch() string {
    switch runtime.GOARCH {
    case "amd64":
        return "x86_64"
    case "arm64":
        if runtime.GOOS == "linux" {
            return "aarch64"
        }
        return "arm64"
    default:
        return runtime.GOARCH
    }
}

func downloadFile(path, url string) error {
    resp, err := http.Get(url) //nolint:gosec // URL is constructed from known constants
    if err != nil {
        return fmt.Errorf("HTTP GET: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("HTTP %d", resp.StatusCode)
    }

    f, err := os.Create(path)
    if err != nil {
        return fmt.Errorf("create: %w", err)
    }
    defer f.Close()

    if _, err := io.Copy(f, resp.Body); err != nil {
        return fmt.Errorf("download: %w", err)
    }

    return nil
}
```

### Config Generation (config.go)
```go
// Source: Derived from existing smoke_test.go [VERIFIED: codebase]
package testutil

import (
    "fmt"
    "os"
    "path/filepath"
    "testing"
)

// generateConfigYAML writes a proxy config file and returns its path.
func generateConfigYAML(t *testing.T, cfg TestConfig, port int) string {
    t.Helper()

    tmpDir := t.TempDir()
    cfgPath := filepath.Join(tmpDir, "config.yml")

    content := fmt.Sprintf(`listen: "127.0.0.1:%d"
domain: "127.0.0.1:%d"
log:
  level: "info"
cache:
  type: "none"
tls:
  cert: %s
  key:  %s
proxy:
  github:
    - token: %s
      repo:
        owner: %s
        name:  %s
        path:
%s
`,
        port, port,
        cfg.TLSCertPath, cfg.TLSKeyPath,
        cfg.GithubToken,
        cfg.RepoOwner, cfg.RepoName,
        formatPaths(cfg.RepoPaths),
    )

    if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
        t.Fatalf("writing config: %v", err)
    }

    return cfgPath
}

func formatPaths(paths []string) string {
    var result string
    for _, p := range paths {
        result += fmt.Sprintf("          - %q\n", p)
    }
    return result
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `io/ioutil` for file I/O | `os` and `io` packages directly | Go 1.16 | `ioutil` is deprecated; use `os.ReadFile`, `os.WriteFile`, `os.MkdirAll` |
| Manual temp dir cleanup | `t.TempDir()` | Go 1.15 | Automatic cleanup, no more `defer os.RemoveAll` |
| Deferred cleanup only | `t.Cleanup()` | Go 1.14 | Ordered cleanup, runs even on panic |

**Deprecated/outdated:**
- `io/ioutil`: Deprecated since Go 1.16. Use `os.ReadFile`, `os.WriteFile` instead.
- `github.com/ghodss/yaml`: Uses `gopkg.in/yaml.v2` internally. The project already has it in go.mod. For generating YAML in tests, `fmt.Sprintf` is simpler and already proven.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | GitHub release binary download does not require authentication for public repos | bufbin.go | Download fails -- need to add token auth header |
| A2 | The buf binary naming convention (`buf-{OS}-{Arch}`) is stable across versions v1.30.1 through v1.69.0 | bufbin.go | Wrong asset name -- download fails |
| A3 | Linux arm64 uses "aarch64" in buf asset names (based on v1.30.1 release listing which showed `buf-Linux-aarch64`) | bufbin.go | Wrong asset name on Linux arm64 |
| A4 | The `http.Get` redirect follower handles GitHub's 302 release redirects correctly | bufbin.go | Download fails -- need manual redirect handling |
| A5 | The existing smoke test's YAML config template is complete and correct for server startup | config.go | Server fails to start with missing config fields |

**Risk assessment:**
- A1: LOW risk -- GitHub public releases are publicly downloadable. [VERIFIED: curl test returned 302, which standard http.Get follows]
- A2: LOW risk -- Verified both v1.30.1 and v1.69.0 have the same naming pattern. [VERIFIED: GitHub API]
- A3: MEDIUM risk -- Only verified for v1.30.1 Linux assets. [VERIFIED: GitHub API for v1.30.1]
- A4: LOW risk -- Go's `net/http` follows redirects by default. [VERIFIED: stdlib behavior]
- A5: LOW risk -- The smoke test runs successfully with this template. [VERIFIED: Phase 2 execution]

## Open Questions

1. **Should `fmt.Sprintf` or `ghodss/yaml.Marshal` generate config YAML?**
   - What we know: The smoke test uses `fmt.Sprintf` and it works. `ghodss/yaml` is in go.mod. The config struct uses json tags, and `ghodss/yaml` respects json tags.
   - What's unclear: Whether yaml.Marshal produces the exact format the server expects (field ordering, quoting).
   - Recommendation: Start with `fmt.Sprintf` (proven approach from smoke test). Switch to yaml.Marshal only if the config struct becomes complex. This is at Claude's discretion per D-03.

2. **Should the existing smoke_test.go be refactored to use testutil?**
   - What we know: The smoke test works as-is. Refactoring it would validate the new helpers immediately.
   - What's unclear: Whether refactoring introduces risk during a test-infra phase.
   - Recommendation: Refactor it. Using the new helpers in the smoke test is the best way to validate that the helpers work correctly before Phases 4 and 5 depend on them. This is at Claude's discretion.

3. **Should downloaded binaries be verified with checksums?**
   - What we know: GitHub releases do not publish checksums in a machine-readable format alongside the binary assets. Some releases have a `sha256.txt` file but this is not consistent.
   - What's unclear: Whether buf releases include a checksums file.
   - Recommendation: Skip checksum verification for now. The download is over HTTPS from GitHub's CDN, which provides transport security. Add checksum verification later if needed. This is at Claude's discretion.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build + `go run` server | Yes | go1.26.1 darwin/arm64 | -- |
| TLS certs (`~/local-tls/server/`) | Server startup | Yes | server-cert.pem, server-key.pem | -- |
| `EASYP_GITHUB_TOKEN` | GitHub API calls | Yes | Set in environment | Skip tests if missing |
| buf v1.30.1 binary | Old protocol tests | Yes | `~/go/bin/buf` | Auto-download via testutil |
| buf v1.69.0 binary | New protocol tests | Yes | `/usr/local/bin/buf` | Auto-download via testutil |
| GitHub releases API | Binary auto-download | Yes | Public, no auth needed | -- |
| Internet connectivity | Binary download + GitHub API | Yes | -- | Skip download-heavy tests offline |

**Missing dependencies with no fallback:**
- None -- all dependencies are available or have graceful skip behavior.

**Missing dependencies with fallback:**
- None -- all required dependencies are present.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + `stretchr/testify` v1.8.4 |
| Config file | none -- Go test framework uses convention (`*_test.go`) |
| Quick run command | `go test ./e2e/testutil/ -count=1 -timeout 120s -run TestHelper` |
| Full suite command | `go test ./e2e/... -count=1 -timeout 300s` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TINF-01 | StartServer starts proxy, returns port, cleans up | unit (integration) | `go test ./e2e/testutil/ -run TestStartServer -count=1` | Wave 0 |
| TINF-02 | GetBuf downloads/returns pinned buf binary path | unit | `go test ./e2e/testutil/ -run TestGetBuf -count=1` | Wave 0 |
| TINF-03 | Config includes GitHub token from env var | unit | `go test ./e2e/testutil/ -run TestConfigGeneration -count=1` | Wave 0 |
| TINF-04 | Config targets googleapis/googleapis repo | unit | `go test ./e2e/testutil/ -run TestConfigGeneration -count=1` | Wave 0 |
| TINF-05 | Parallel tests get different ports, no conflicts | integration | `go test ./e2e/ -run TestSmokeBufModUpdate -count=1` | Existing |
| TINF-06 | CI-compatible env-based config | manual | Verify env vars are the only config source | N/A |

### Sampling Rate
- **Per task commit:** `go test ./e2e/testutil/ -count=1 -timeout 120s`
- **Per wave merge:** `go test ./e2e/... -count=1 -timeout 300s`
- **Phase gate:** Full suite green + smoke test refactored to use testutil passes

### Wave 0 Gaps
- [ ] `e2e/testutil/server.go` -- covers TINF-01 (StartServer helper)
- [ ] `e2e/testutil/bufbin.go` -- covers TINF-02 (GetBuf helper)
- [ ] `e2e/testutil/config.go` -- covers TINF-03, TINF-04 (config generation)
- [ ] `e2e/testutil/testutil_test.go` -- internal validation tests for helpers

Note: The helpers themselves are test infrastructure, not application code. Their validation comes from:
1. Unit tests in `testutil_test.go` that exercise config generation and binary caching without full E2E
2. The refactored `smoke_test.go` which exercises the full integration path
3. Phases 4 and 5 which are the primary consumers

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
| Token exposure in test output | Information Disclosure | Use `t.Setenv` or env vars; never log the token value |
| Binary tampering (supply chain) | Tampering | HTTPS download from GitHub CDN; checksum verification deferred |
| Config file permissions | Information Disclosure | Write config with mode 0600 (already done in smoke test) |

## Sources

### Primary (HIGH confidence)
- Codebase: `e2e/smoke_test.go` -- existing working test infrastructure to refactor
- Codebase: `cmd/easyp/internal/config/config.go` -- production config struct definition
- Codebase: `cmd/easyp/main.go` -- server entry point and config loading
- Codebase: `cmd/easyp/internal/config/read.go` -- YAML config deserialization (uses `ghodss/yaml` + `os.ExpandEnv`)
- Codebase: `cmd/easyp/internal/config/cachetype/cachetype.go` -- cache type enum
- GitHub API: `api.github.com/repos/bufbuild/buf/releases` -- verified v1.30.1 and v1.69.0 release asset naming
- Context7: `/stretchr/testify` -- require/assert patterns
- Context7: `/golang/go` -- testing.T patterns (Helper, Cleanup, TempDir, Parallel)

### Secondary (MEDIUM confidence)
- Go stdlib: `runtime.GOOS`/`runtime.GOARCH` -- platform detection conventions
- Go stdlib: `net/http` redirect handling -- follows 302 by default

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all already in go.mod
- Architecture: HIGH -- extracting proven code from smoke test, patterns well-established
- Pitfalls: HIGH -- identified from real testing experience in Phase 2 and codebase analysis

**Research date:** 2026-05-07
**Valid until:** 2026-06-07 (stable -- Go testing patterns and GitHub release API are stable)
