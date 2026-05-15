---
phase: 03-test-infrastructure
plan: 01
subsystem: test-infrastructure
tags: [testutil, test-helpers, subprocess, buf-binary, config-generation]
dependency_graph:
  requires: [go-1.22, stretchr/testify, ghodss/yaml]
  provides: [e2e/testutil package with StartServer, GetBuf, RunBufModUpdate, TestConfig]
  affects: [e2e/smoke_test.go (future refactor target), phase 4 tests, phase 5 tests]
tech_stack:
  added: []
  patterns: [subprocess-lifecycle, tcp-poll-readiness, github-releases-download, cache-first-binary-mgmt]
key_files:
  created:
    - e2e/testutil/config.go
    - e2e/testutil/server.go
    - e2e/testutil/bufbin.go
  modified:
    - .gitignore
decisions:
  - fmt.Sprintf for YAML config generation (proven in Phase 2 smoke test, matches json tags)
  - Config file mode 0600 to prevent world-readable token (T-03-02 mitigation)
  - Checksum verification deferred for buf binary downloads (HTTPS provides transport integrity)
  - t.Cleanup for subprocess lifecycle (guaranteed to run even on panic)
metrics:
  duration: 161s
  completed: "2026-05-07"
  tasks: 3
  commits: 3
  files_created: 3
  files_modified: 1
---

# Phase 3 Plan 1: Testutil Package Summary

Reusable test helper package (e2e/testutil/) with server subprocess lifecycle, buf binary management, and config generation extracted from the Phase 2 smoke test.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Create testutil package with config.go and shared findProjectRoot | 1317ba0 | e2e/testutil/config.go |
| 2 | Create server.go with StartServer and bufbin.go with GetBuf | ea75708 | e2e/testutil/server.go, e2e/testutil/bufbin.go |
| 3 | Update .gitignore for testdata/buf/ cache directory | a9e4301 | .gitignore |

## What Was Built

### config.go
- `TestConfig` struct with fields: TLSCertPath, TLSKeyPath, GithubToken, RepoOwner, RepoName, RepoPaths, LogLevel
- `DefaultTestConfig()` populates from env vars (HOME for TLS paths, EASYP_GITHUB_TOKEN for token)
- `generateConfigYAML()` writes YAML config with mode 0600 into t.TempDir(), matching production json tags
- `findProjectRoot()` resolves project root via runtime.Caller with correct 3-level path depth for e2e/testutil/

### server.go
- `StartServer(t, TestConfig) int` allocates free port, generates config, starts subprocess via `go run ./cmd/easyp`, TCP polls for 30s readiness, registers t.Cleanup with 5s graceful shutdown
- `RunBufModUpdate(t, bufBinary, port) (int, string)` creates temp buf module, runs buf mod update with 60s timeout, validates buf.lock creation

### bufbin.go
- `GetBuf(t, version) string` checks testdata/buf/{version}/buf cache, downloads from GitHub Releases on miss with atomic file placement
- `RequireEnvToken(t, envVar) string` reads env var, calls t.Skip if empty
- Platform detection: darwin->Darwin, linux->Linux, amd64->x86_64, arm64->arm64 (darwin) / aarch64 (linux)
- Version constants: BufV130 ("v1.30.1"), BufV169 ("v1.69.0")

### .gitignore
- Added `testdata/buf/` to exclude downloaded binary cache from version control

## Verification

- `go vet ./e2e/testutil/` passes with zero errors
- `go build ./e2e/testutil/` compiles successfully
- All 8 planned exports present: TestConfig, DefaultTestConfig, StartServer, RunBufModUpdate, GetBuf, BufV130, BufV169, RequireEnvToken
- No new external dependencies added to go.mod

## Deviations from Plan

None - plan executed exactly as written.

## Requirements Satisfied

| ID | Description | Status |
|----|-------------|--------|
| TINF-01 | StartServer starts proxy with TLS, polls for readiness, cleans up via t.Cleanup | Done |
| TINF-02 | GetBuf downloads/returns pinned buf binary, caches in testdata/buf/{version}/buf | Done |
| TINF-05 | Each test gets unique free port via net.Listen zero-port allocation | Done |
| TINF-06 | CI-compatible env-only config (RequireEnvToken, DefaultTestConfig from env vars) | Done |

## Self-Check: PASSED

All files and commits verified present.
