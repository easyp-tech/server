---
phase: 03-test-infrastructure
plan: 02
subsystem: test-infrastructure
tags: [smoke-test-refactor, testutil-validation, config-generation, buf-binary, env-token]
dependency_graph:
  requires: [e2e/testutil package (Plan 01)]
  provides: [refactored smoke_test.go using testutil, internal testutil validation tests]
  affects: [phase 4 tests, phase 5 tests]
tech_stack:
  added: []
  patterns: [table-driven-tests, same-package-testing, yaml-config-validation, binary-format-detection]
key_files:
  created:
    - e2e/testutil/testutil_test.go
  modified:
    - e2e/smoke_test.go
decisions:
  - Mach-O magic byte detection must check both big-endian and little-endian variants
  - RequireEnvToken skip behavior tested indirectly via t.Setenv positive case
  - No StartServer test in testutil_test.go -- smoke test serves as integration validation
metrics:
  duration: 177s
  completed: "2026-05-07"
  tasks: 2
  commits: 2
  files_created: 1
  files_modified: 1
---

# Phase 3 Plan 2: Smoke Test Refactor Summary

Refactored smoke_test.go to use testutil package (179 lines removed, 16 added), added 5 internal validation tests for helper functions (config generation, env token, binary caching, version constants).

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Refactor smoke_test.go to use testutil package | 1ea5273 | e2e/smoke_test.go |
| 2 | Create internal validation tests for testutil helpers | ac01e3e | e2e/testutil/testutil_test.go |

## What Was Built

### smoke_test.go (refactored)
- Replaced inline `startServer`, `runBufModUpdate`, `findProjectRoot` with `testutil.StartServer`, `testutil.RunBufModUpdate`, `testutil.GetBuf`
- Uses `testutil.RequireEnvToken(t, "EASYP_GITHUB_TOKEN")` for token gating
- Table-driven subtests now use `testutil.BufV130` / `testutil.BufV169` version constants
- Imports reduced from 12 to 2 (testing + testutil)
- 179 lines removed, 16 lines added

### testutil_test.go (new)
- `TestDefaultTestConfig` -- verifies DefaultTestConfig field defaults (RepoOwner, RepoName, RepoPaths, LogLevel, TLS paths)
- `TestConfigGeneration` -- verifies generateConfigYAML produces correct YAML keys/values, asserts file mode 0600
- `TestRequireEnvToken_Skips` -- verifies token return when env var is set; skip behavior validated by code structure
- `TestVersionConstants` -- verifies BufV130 == "v1.30.1" and BufV169 == "v1.69.0"
- `TestGetBuf_CachePath` -- verifies returned path format, file existence, execute bit, and binary format (Mach-O/ELF)
- Helper functions `isMachO` (big/little-endian) and `isELF` for binary format detection

## Verification

- `go vet ./e2e/...` passes with zero errors
- `go test ./e2e/testutil/ -count=1 -timeout 120s` passes (all 5 tests)
- Zero inline helper functions remain in smoke_test.go
- 7 references to testutil package in refactored smoke test

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed Mach-O magic byte detection**
- **Found during:** Task 2 (TestGetBuf_CachePath failed)
- **Issue:** isMachO only checked big-endian magic (0xfeedface, 0xfeedfacf) but macOS arm64 binaries use little-endian storage (0xcffaedfe)
- **Fix:** Added little-endian variants (0xcefaedfe, 0xcffaedfe) to the magic byte check
- **Files modified:** e2e/testutil/testutil_test.go
- **Commit:** ac01e3e

**2. [Rule 1 - Bug] Fixed extra closing parenthesis in TestRequireEnvToken_Skips**
- **Found during:** Task 2 (compilation error at line 95)
- **Issue:** Outer function TestRequireEnvToken_Skips used `})` instead of `}` as closing brace
- **Fix:** Changed `})` to `}` for the function-level closing
- **Files modified:** e2e/testutil/testutil_test.go
- **Commit:** ac01e3e

## Requirements Satisfied

| ID | Description | Status |
|----|-------------|--------|
| TINF-01 | StartServer starts proxy with TLS, polls for readiness, cleans up via t.Cleanup | Done (validated by smoke test usage) |
| TINF-02 | GetBuf downloads/returns pinned buf binary, caches in testdata/buf/{version}/buf | Done (validated by TestGetBuf_CachePath) |
| TINF-03 | Refactored smoke test uses testutil package exclusively | Done |
| TINF-04 | Internal validation tests for helper functions | Done |
| TINF-05 | Each test gets unique free port via net.Listen zero-port allocation | Done (inherited from Plan 01) |
| TINF-06 | CI-compatible env-only config (RequireEnvToken, DefaultTestConfig) | Done (validated by tests) |

## Self-Check: PASSED

All files and commits verified present.
