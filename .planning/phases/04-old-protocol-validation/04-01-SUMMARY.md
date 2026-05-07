---
phase: 04-old-protocol-validation
plan: 01
subsystem: testing
tags: [e2e, buf-cli, backward-compatibility, tls, go-testing]

# Dependency graph
requires:
  - phase: 03-test-infrastructure
    provides: "e2e/testutil package (StartServer, GetBuf, RunBufModUpdate, RequireEnvToken)"
provides:
  - "ServerResult struct exposing server output buffer for failure diagnostics"
  - "TestOldProtocolBufModUpdateTwice validating OLD-02 backward compatibility"
  - "Updated StartServer API returning ServerResult instead of bare int"
affects: [05-new-protocol-validation]

# Tech tracking
tech-stack:
  added: []
  patterns: ["ServerResult struct for exposing subprocess output in test diagnostics", "Inline workspace management for multi-step buf command tests"]

key-files:
  created:
    - e2e/old_proto_test.go
  modified:
    - e2e/testutil/server.go
    - e2e/smoke_test.go

key-decisions:
  - "StartServer returns ServerResult{Port, Output} instead of bare int -- enables D-04 failure diagnostics"
  - "OLD-02 reinterpreted as two-step buf mod update since buf dep update does not exist in v1.30.1"
  - "Inline workspace management in old_proto_test.go -- RunBufModUpdate creates new workspace each call"

patterns-established:
  - "ServerResult pattern: test helpers return structs with diagnostics buffers, callers use .Port and .Output"
  - "Two-step buf test pattern: create workspace once, run buf command twice to exercise update-with-existing-lock path"

requirements-completed: [OLD-01, OLD-02]

# Metrics
duration: 7min
completed: 2026-05-07
---

# Phase 4 Plan 01: Old Protocol Validation Summary

**Two-step buf mod update test with v1.30.1 and ServerResult diagnostics for OLD-01/OLD-02 backward compatibility**

## Performance

- **Duration:** 7 min
- **Started:** 2026-05-07T15:06:14Z
- **Completed:** 2026-05-07T15:13:04Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- StartServer API upgraded to return ServerResult with Port and Output fields, enabling D-04 failure diagnostics
- OLD-01 smoke test updated to use srv.Port and include srv.Output in failure messages
- OLD-02 validated via TestOldProtocolBufModUpdateTwice -- two-step buf mod update with v1.30.1 passes against real proxy + GitHub API

## Task Commits

Each task was committed atomically:

1. **Task 1: Expose server output buffer from StartServer** - `36cf296` (feat)
2. **Task 2: Create two-step buf mod update test for OLD-02** - `d185ed0` (feat)

## Files Created/Modified
- `e2e/old_proto_test.go` - NEW: Two-step buf mod update test for OLD-02 backward compatibility
- `e2e/testutil/server.go` - Added ServerResult struct, changed StartServer return type from int to ServerResult
- `e2e/smoke_test.go` - Updated to use srv.Port and srv.Output in failure diagnostics

## Decisions Made
- StartServer returns ServerResult{Port, Output} instead of bare int -- callers now have access to server subprocess output for failure messages (D-04)
- OLD-02 implemented as two-step buf mod update since buf dep update does not exist in v1.30.1 (command introduced in v1.32.0)
- Inline workspace management in old_proto_test.go rather than extending RunBufModUpdate -- RunBufModUpdate creates a new temp dir each call, incompatible with two-step testing

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- Intermittent TLS handshake timeouts to raw.githubusercontent.com during parallel test execution. This is a pre-existing environmental issue (network latency to GitHub CDN), not caused by code changes. Both OLD-01 and OLD-02 pass in isolation. The v1.69.0 smoke test subtest is also affected when it needs to download the buf binary.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 5 (New Protocol Validation) can reuse ServerResult pattern for its tests
- StartServer API is stable and ready for v1.69.0 test cases
- buf v1.69.0 binary may need pre-caching due to intermittent download timeouts from GitHub CDN

## Self-Check: PASSED

All claimed files exist. Both task commits (36cf296, d185ed0) found in git log.

---
*Phase: 04-old-protocol-validation*
*Completed: 2026-05-07*
