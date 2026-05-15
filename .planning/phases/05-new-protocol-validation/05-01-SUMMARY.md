---
phase: 05-new-protocol-validation
plan: 01
subsystem: testing
tags: [buf, e2e, integration, connect-protocol, v1beta1]

requires:
  - phase: 04-old-protocol-validation
    provides: "Old protocol test patterns and testutil infrastructure"
provides:
  - "RunBufDepUpdate helper for buf dep update command execution"
  - "new_proto_test.go with TestNewProtocolBufModUpdate and TestNewProtocolBufDepUpdate"
  - "Debug logging findings: v1.69.0 uses v1beta1.CommitService/GetCommits path"
affects: [05-02]

tech-stack:
  added: []
  patterns: ["debug log level for RPC discovery"]

key-files:
  created: ["e2e/new_proto_test.go"]
  modified: ["e2e/testutil/server.go"]

key-decisions:
  - "Tests run with LogLevel=debug to capture full RPC call pattern from v1.69.0 CLI"
  - "Tests fail intentionally when RPCs are unhandled — failures provide input for Plan 05-02"

patterns-established:
  - "Debug-level integration tests for protocol discovery"

requirements-completed: [NEW-01, NEW-02]

duration: 3min
completed: 2026-05-07
---

# Phase 5 Plan 01 Summary

**RunBufDepUpdate helper and v1.69.0 integration tests — discovered modern buf uses v1beta1 protocol, not v1alpha1**

## Performance

- **Duration:** 3 min
- **Started:** 2026-05-07T20:09:00Z
- **Completed:** 2026-05-07T20:12:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- RunBufDepUpdate helper added to testutil following RunBufModUpdate pattern
- Two integration tests created with debug logging for v1.69.0
- Critical discovery: modern buf v1.69.0 uses completely different API surface

## Task Commits

1. **Task 1: Add RunBufDepUpdate helper** - `dd31fcf` (feat)
2. **Task 2: Create new_proto_test.go with debug logging** - `e4605ca` (test)

## Findings from Test Execution

Both tests fail with the same root cause:

**RPC Path Called:** `/buf.registry.module.v1beta1.CommitService/GetCommits`
- NOT the old v1alpha1 path (`/buf.alpha.registry.v1alpha1.ResolveService/GetModulePins`)
- The v1beta1 path is not registered in the proxy's connect mux
- Request falls through to rootHandler which returns `text/plain; charset=utf-8`
- Modern buf CLI rejects: `invalid content-type: "text/plain; charset=utf-8"; expecting "application/proto"`

**Request Headers from v1.69.0:**
- `Buf-Version: 1.69.0`
- `Connect-Protocol-Version: 1`
- `Content-Type: application/proto`
- `User-Agent: connect-go/1.19.2 (go1.26.2)`

**Note:** Only one RPC call was observed (`GetCommits`). The CLI fails immediately on content-type mismatch before making additional calls. Fixing this will likely reveal more RPCs the CLI needs.

## Decisions Made
- Tests intentionally left failing — they serve as investigation, not validation
- Findings feed directly into Plan 05-02 for targeted fixes

## Deviations from Plan
None — plan executed as written.

## Issues Encountered
- Parallel test execution caused buf binary download race (both tests download to same tmp path). Resolved by running tests sequentially.

## Next Phase Readiness
- Plan 05-02 has clear fix target: register v1beta1.CommitService/GetCommits handler or map to existing v1alpha1 handlers
- Additional RPCs may surface once GetCommits is handled
- Debug logging pattern established for ongoing discovery

---
*Phase: 05-new-protocol-validation*
*Completed: 2026-05-07*
