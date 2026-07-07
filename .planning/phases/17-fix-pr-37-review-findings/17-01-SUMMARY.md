---
phase: 17-fix-pr-37-review-findings
plan: 01
subsystem: api
tags: [error-routing, logging, sha-256, bitbucket, test-fixtures]

# Dependency graph
requires:
  - phase: 16-commit-id-resolution-improvements
    provides: "the new (string, error) commitUUID signature and internalError helper that this phase rewires"
provides:
  - "computeB4Digest errors routed through upstreamError (502) vs logHandlerError (500) via errCommitUUIDContract sentinel"
  - "internalError helper removed; all 500s flow through logHandlerError with full ERR-05 attrs"
  - "commitUUID accepts both 40-char SHA-1 and 64-char SHA-256 hex (Bitbucket compat)"
  - "400 not-found message softened to 're-resolve via buf mod update / buf dep update' for foreign-id miss class"
  - "preResolveForTest moved to test file (excluded from production build)"
affects: [any future phase touching commits.go / commits_helpers.go error paths]

# Tech tracking
tech-stack:
  added: []
  patterns: [errors.Is sentinel dispatch for context-dependent error routing, 40/64 length allow-list for sha inputs]

key-files:
  created: []
  modified:
    - internal/connect/commits.go
    - internal/connect/commits_helpers.go
    - internal/connect/commits_helpers_test.go
    - internal/connect/api_test.go
    - internal/connect/uuid_format_test.go

key-decisions:
  - "Used a sentinel (errCommitUUIDContract) with fmt.Errorf('%w: %v', sentinel, inner) so errors.Is works alongside existing %v log lines"
  - "Threaded the caller-minted cid into computeB4Digest instead of re-running commitUUID — caller has already validated, the re-derivation was wasted work and the redundant call was the source of finding #5"
  - "Kept the 'unknown commit id:' prefix on the 400 message so the two existing 'unknown commit id' substring assertions (api_test.go:590, uuid_format_test.go:209) continue to pass unchanged"
  - "Replaced 're-run' with 're-resolve via' in the 400 message so foreign-id misses do not get the stale-lockfile-flavored command that wouldn't help"
  - "Retained the strings import in commits_helpers.go — strings.SplitN at line 288 (parseModuleRefByID) still uses it; do NOT remove even after preResolveForTest is moved out"

patterns-established:
  - "Pattern: error routing via sentinel + errors.Is dispatch in the caller — wraps a generic 5xx handler (logHandlerError) and an upstream 502 handler (upstreamError) behind one error type, letting the error source decide which response to emit"
  - "Pattern: when a helper's call site is refactored, thread already-computed values as parameters rather than re-deriving — eliminates the dead-path that the old code defended against (here, the redundant commitUUID inside computeB4Digest)"

requirements-completed: [SC-1, SC-2, SC-3, SC-4, SC-5]

# Metrics
duration: 8min
completed: 2026-07-07
---

# Phase 17: Fix PR #37 Review Findings Summary

**All four in-scope PR #37 review findings resolved atomically: error routing restored to ERR-05, 64-char SHA-256 accepted for Bitbucket, 400 message softened for foreign-id misses, test fixture moved to test file.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-07-07T10:14:00Z
- **Completed:** 2026-07-07T10:22:00Z
- **Tasks:** 4
- **Files modified:** 5

## Accomplishments

- `internalError` helper removed; all 5 former call sites now flow through `logHandlerError` (3) or `upstreamError` (2) with full ERR-05 attributes (`server`, `protocol`, `request_id`, `error`, `status`, `error_class`)
- `errCommitUUIDContract` sentinel introduced; `errors.Is` dispatch in both digest-error sites routes 500 (contract violation) vs 502 (upstream outage) correctly
- `commitUUID` now accepts 40 or 64 lowercase hex chars; SHA-256 Bitbucket Server repos no longer 500
- 400 message softened from `re-run` to `re-resolve via` to cover both stale-lockfile and foreign-id miss classes
- `preResolveForTest` relocated to test file; production binary no longer carries the fixture
- Targeted tests added: `TestCommitUUID_SHA256_KnownSHA` (4 cases) + `TestCommitUUID_InvalidInput` extended with 63/65-char inputs (8 cases total)

## Task Commits

Each task was committed atomically:

1. **Task 1: Refactor internalError + thread cid + sentinel (findings #1, #2, #5)** — `d355bac`
2. **Task 2: commitUUID accepts 64-char SHA-256 (finding #3)** — `1c1a404`
3. **Task 3: Soften 400 message + update 2 test assertions (finding #4)** — `3ff1786`
4. **Task 4: Move preResolveForTest to test file (finding #7)** — `01ed747`

## Files Created/Modified

- `internal/connect/commits.go` — `internalError` helper deleted; `errCommitUUIDContract` sentinel added; 5 call sites refactored; `computeB4Digest` signature extended with `cid` parameter; digest-error sites dispatch on `errors.Is`; 400 wire body updated; comment at line 399 (now 418) updated
- `internal/connect/commits_helpers.go` — `commitUUID` length check changed from `len != 40` to `len != 40 && len != 64`; error messages updated to "40 or 64 lowercase hex characters"; doc comment updated to call out the SHA-256 / Bitbucket driver; `preResolveForTest` deleted (5-line doc + function body); `strings` import retained (still used by `strings.SplitN`)
- `internal/connect/commits_helpers_test.go` — `TestCommitUUID_SHA256_KnownSHA` added (4 cases: all-zero, all-ones, 14-byte-prefix-match, deadbeef); `TestCommitUUID_InvalidInput` extended with 63-char and 65-char inputs; `preResolveForTest` pasted in before `TestPreResolveForTest` with its original 5-line doc comment
- `internal/connect/api_test.go` — D-12 substring assertion at line 597 updated to `re-resolve via buf mod update / buf dep update`; error message text in `t.Errorf` updated
- `internal/connect/uuid_format_test.go` — D-12 substring assertion at line 215 updated; error message text updated

## Decisions Made

- Used `fmt.Errorf("%w: %v", errCommitUUIDContract, uidErr)` for the contract-violation wrap so `errors.Is(err, errCommitUUIDContract)` works and the existing `%v` log lines still get the underlying error string. This is the canonical Go wrap pattern.
- Threaded the caller-minted `cid` into `computeB4Digest` (Task 1, finding #5) — eliminated the redundant `commitUUID(commit)` re-derivation, kept the contract-violation guard via a separate `commitUUID(commit)` call wrapped with the sentinel, and let `h.filesMap[cid] = files` use the parameter directly.
- Kept the "unknown commit id:" prefix on the 400 message — preserves grep-friendly hook for the existing `bytes.Contains` / `strings.Contains` substring assertions and the implicit log-correlation. Only the prescriptive tail changed.
- Did NOT touch `registerResolved` warn-and-return at `commits.go:901-911` — it logs without an HTTP request so `h.hlog`+`context.Background()` shape is intentional. Finding #6 is deferred per plan question #3.

## Deviations from Plan

None - plan executed exactly as written. The verbatim "deferred" finding (probe/cache contract divergence at `commits.go:901-911` vs `1101-1103`) was acknowledged in the plan as a non-action item and is noted in `<threat_model>` as T-17-DEFER.

## Issues Encountered

None. The IDE diagnostic errors that fired between commits were the expected intermediate state (the 5 call sites still referencing the deleted `internalError` helper, then `computeB4Digest` callers with the old 3-arg signature). Each was resolved by the next edit in the planned sequence.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- v1.3 milestone now has all 7 phases either complete or in-progress per the ROADMAP
- 5 ROADMAP success criteria (SC-1 through SC-5) all pass
- The deferred finding (32-char hex sha cache miss path) is documented in `<threat_model>` T-17-DEFER for a future phase; no action required for current milestone
- Phase 17 is ready for `verify-phase` (the next step in the milestone close path)
