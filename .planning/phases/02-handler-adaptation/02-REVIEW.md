---
phase: 02-handler-adaptation
reviewed: 2026-05-07T00:00:00Z
depth: standard
files_reviewed: 2
files_reviewed_list:
  - e2e/smoke_test.go
  - go.mod
findings:
  critical: 1
  warning: 3
  info: 2
  total: 6
status: issues_found
---

# Phase 2: Code Review Report

**Reviewed:** 2026-05-07T00:00:00Z
**Depth:** standard
**Files Reviewed:** 2
**Status:** issues_found

## Summary

Reviewed `e2e/smoke_test.go` (E2E smoke test for buf mod update through the TLS proxy) and `go.mod` (module dependency manifest). The test file has one critical issue: the GitHub OAuth token is written in cleartext to a config file on disk, which is a secret exposure risk. There are also warnings around a port race condition and unreachable code after `t.Fatalf`.

## Critical Issues

### CR-01: Secret token written to disk in cleartext config file

**File:** `e2e/smoke_test.go:81-96`
**Issue:** The GitHub OAuth token (`EASYP_GITHUB_TOKEN`) is interpolated directly into a YAML config file written to `t.TempDir()` (line 81-96). While `t.TempDir()` is cleaned up after the test, the token exists as plaintext on disk for the duration of the test. Any process on the machine can read it during that window. The config file permissions are set to `0600` (line 98), which limits exposure to the current user, but the token is still present in cleartext on the filesystem and visible in `/proc` or similar on Linux.

**Fix:** This is an E2E test running against a local proxy, so the risk is limited to the development machine. Consider passing the token via an environment variable or stdin to the server process instead of embedding it in a config file. If the config file approach is necessary, document that this is a test-only pattern and ensure `t.TempDir()` cleanup runs promptly.

## Warnings

### WR-01: Port race condition between listener close and server bind

**File:** `e2e/smoke_test.go:68-72`
**Issue:** The code allocates a free port by listening on `:0`, extracting the port number, then immediately closing the listener (line 72). There is a window between `listener.Close()` and the server subprocess binding to the same port where another process on the machine could claim that port. This is a known TOCTOU (time-of-check-time-of-use) race that causes intermittent test failures, especially on CI systems with high port contention.

**Fix:** There is no perfect fix in Go without FD passing. Options: (1) Accept the race as a known limitation and retry with a new port on `EADDRINUSE`, or (2) use a higher port range and retry logic. Document the trade-off in a comment.

### WR-02: Unreachable code after `t.Fatalf`

**File:** `e2e/smoke_test.go:138-139`
**Issue:** Line 139 (`return port, cleanup`) is unreachable because `t.Fatalf` on line 138 terminates the goroutine. While harmless (the Go compiler and `t.Fatalf` guarantee this), it is dead code that signals the author may not realize `t.Fatalf` calls `runtime.Goexit()`.

**Fix:** Remove line 139, or restructure to avoid the unreachable return:
```go
// Server didn't start in time
t.Fatalf("server did not become ready on port %d within 30s. Output:\n%s", port, serverOutput.String())
// No return needed -- t.Fatalf terminates the goroutine
```

### WR-03: Hardcoded buf binary paths are fragile and environment-specific

**File:** `e2e/smoke_test.go:36-41`
**Issue:** The two buf binary paths are hardcoded to specific locations (`~/go/bin/buf` and `/usr/local/bin/buf`). These paths will not exist on many developer machines or CI environments, causing the test to fail with `require.NoError` rather than a meaningful skip message. The test should check binary existence before entering the subtest and skip gracefully if a binary is not found, rather than failing.

**Fix:** Check binary existence outside the subtest loop and skip individual cases with `t.Skipf` instead of failing with `require.NoError`:
```go
if _, err := os.Stat(tc.bufBinary); err != nil {
    t.Skipf("buf binary not found at %s: %v", tc.bufBinary, err)
}
```

## Info

### IN-01: `go.mod` declares Go 1.22 but uses only basic features

**File:** `go.mod:3`
**Issue:** The module declares `go 1.22` but the reviewed test file does not use any Go 1.22-specific features. This is informational -- the version directive may be required by other files in the module.

**Fix:** No action needed for this file alone.

### IN-02: Config template uses string interpolation instead of YAML marshaling

**File:** `e2e/smoke_test.go:81-96`
**Issue:** The YAML config is constructed via `fmt.Sprintf` with manual indentation and formatting. If any interpolated value (e.g., a file path) contains characters meaningful to YAML (like `:` or `#`), the resulting config could be malformed. For a test with controlled inputs this is acceptable, but it is fragile.

**Fix:** Consider using `ghodss/yaml` (already in go.mod) to marshal a struct to YAML for robustness.

---

_Reviewed: 2026-05-07T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
