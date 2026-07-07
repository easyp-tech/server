---
phase: 16-commit-id-resolution-improvements
plan: 02
subsystem: api
tags: [connect, error-handling, commit-uuid, d-04, d-06, d-12, internal-error]

# Dependency graph
requires:
  - phase: 16-commit-id-resolution-improvements
    plan: 01
    provides: "commitUUID(string) (string, error) signature from plan 16-01"
provides:
  - "internalError helper in commits.go (D-04): warn log with error_class=internal, commit_id, upstream_error; 500 response via http.Error"
  - "5 commits.go call sites of commitUUID wired to the new (string, error) signature with structured 500 handling"
  - "Updated 400 message in ServeDownload (commits.go:582) to D-12 text 'unknown commit id: re-run buf mod update / buf dep update'"
  - "Updated test assertions in api_test.go and uuid_format_test.go to assert the new D-12 substring"
affects:
  - "internal/connect/commits.go - error-handling shape and one user-facing 400 message"
  - "internal/connect/api_test.go + uuid_format_test.go - 400-message assertions and mock commit fixtures"
  - "Future bug investigations: 'internal: commitUUID failure' warn line is now a single grep target for upstream-bad-SHA contract violations"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "internalError helper pattern: request-scoped hlog + http.Error + structured slog.Attr trio, mirrors existing logHandlerError shape"
    - "Loop-shadow avoidance: rename loop's err to cidErr when introducing a new commitUUID check inside an existing for loop"
    - "Background-path logging fallback: when no http.ResponseWriter is in scope (registerResolved), use h.api.log + context.Background() instead of h.hlog(r)"

key-files:
  modified:
    - "internal/connect/commits.go - new internalError helper; 5 call sites updated; 400 message + comment updated"
    - "internal/connect/api_test.go - 4 mock commit literals padded to 40 hex chars; new D-12 assertion added"
    - "internal/connect/uuid_format_test.go - new D-12 assertion added"

key-decisions:
  - "internalError helper signature is (w, r, commitID, upstreamErr) per plan D-04 explicit order, even though the package convention is (r, w, ...); used http.Error rather than introducing a new JSON encoder (D-04)"
  - "computeB4Digest callers (ServeHTTP and ServeGraph) call internalError (500) instead of upstreamError (502) on commitUUID error - the caller cannot distinguish a commitUUID failure from an upstream GetFiles failure, so a clean 500-with-warn-class is the simpler contract; genuine upstream failures from this path now also map to 500, a documented trade-off"
  - "registerResolved logs with h.api.log + context.Background() because the function is called from both prewarmHeads (background) and probeCommitID (request-scoped) contexts; there is no http.ResponseWriter in scope and using hlog(r) would require a request the function does not have"
  - "Deviation: padded 4 mockProvider mock commits in api_test.go from 6/8 chars to 40 hex chars (Rule 1); the new commitUUID helper rejects non-40-char input per D-03, so the old short fixtures caused 6 tests to 500 with 'internal error' instead of exercising their real assertions. Padded literals preserve test intent (same prefix, same owner/module)"

patterns-established:
  - "When adopting a (string, error) signature change inside existing loops that already use `err`, rename the new error to a specific name (cidErr) rather than shadowing in if-init, so the rest of the loop body can still use the cid variable"
  - "Padding test mock commits to 40 hex chars is a routine maintenance step now that commitUUID enforces D-03's strict input contract"

requirements-completed: [SC-2, SC-3]

# Metrics
duration: 9min
completed: 2026-07-06
---

# Phase 16 Plan 02: commitUUID error wiring + 400 message update Summary

**Wired the new 2-return commitUUID into all 5 callsites in commits.go, added the internalError helper for 500-with-structured-warn, and updated the ServeDownload 400 message to the D-12 text that points operators at `buf mod update / buf dep update`.**

## Performance

- **Duration:** 9 min
- **Started:** 2026-07-06T13:21:28Z
- **Completed:** 2026-07-06T13:30:42Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- New `internalError(w, r, commitID, upstreamErr string)` helper added near the top of `commits.go`. Logs a structured warn via `h.hlog(r)` with `error_class=internal`, `commit_id`, and `upstream_error` attributes; writes `http.Error(w, "internal error", http.StatusInternalServerError)`. Mirrors the existing `logHandlerError` shape (no JSON encoder, plain text body, 500 status) so the two error-reporting paths are visually consistent in the codebase.
- All 5 `commitUUID` call sites in `commits.go` now handle the new `(string, error)` signature:
  - **Site 1 (line 158, ServeHTTP loop):** renamed local to `cidErr` to avoid shadowing the loop's existing `err` from `GetMeta`. On failure, `return` from the handler (one bad ref must not silently succeed for siblings in the same response).
  - **Site 2 (line 354, ServeGraph loop):** same `cidErr` rename + same handler-`return` semantics.
  - **Site 3 (line 670, ServeDownload):** reused the existing `err` variable from the prior `GetMeta`/`GetFiles` block; helper failure still `return`s from the handler.
  - **Site 4 (line 763, computeB4Digest):** propagates the helper error as `return nil, err` (no `http.ResponseWriter` in scope). Both callers of `computeB4Digest` (ServeHTTP, ServeGraph) were updated to call `internalError` instead of `upstreamError`, so any computeB4Digest failure now returns 500 with a `error_class=internal` warn log line.
  - **Site 5 (line 911, registerResolved):** background path uses `h.api.log.LogAttrs(context.Background(), ...)` (no request scope available) and `return`s from the function. Future call from `probeCommitID` and `prewarmHeads` both have the same failure path.
- 400 message in `ServeDownload` (commits.go:582) changed from `"unknown commit id: must call CommitService/GetCommits first"` to `"unknown commit id: re-run buf mod update / buf dep update"` per D-12. The surrounding `slog.String("commit_id", commitID), slog.Int("body_bytes", len(body))` attributes and 400 status code are unchanged.
- Comment at commits.go:399 (was 387 pre-Phase-16) that quoted the old 400 message verbatim updated to reference the new text, so the comment does not drift from the code.
- Test assertion added to `TestBadRequest_OnUnknownCommitID` in `api_test.go` that explicitly asserts the body contains `"re-run buf mod update / buf dep update"`, so a future regression that drops the D-12 substring fails loudly.
- Test assertion added to `TestServeDownload_UnknownCommitID_ReturnsBadRequest` in `uuid_format_test.go` with the same substring check.
- The `probeCommitID` call site in `ServeDownload` (line 555) is unchanged: still runs unconditionally when `probeEnabled` is true and the fast-path `resolveForeignCommitID` miss falls through. D-07/D-08 preserved.
- All `internal/connect` tests pass (`go test ./internal/connect/ -count=1` returns ok). `go build ./...` exits 0. `go vet ./internal/connect/...` exits 0.

## Task Commits

1. **Task 1: Add internalError helper; update 5 commitUUID call sites in commits.go** — `fe4dade` (feat)
2. **Task 2: Update 400 message; update 2 test files** — `d601748` (feat)

## Files Created/Modified

- `internal/connect/commits.go`
  - New `internalError` helper (after `protocolLabel`, before `ServeHTTP`).
  - Site 1 (ServeHTTP loop) and Site 2 (ServeGraph loop) now use `cid, cidErr := commitUUID(meta.Commit)` with handler-level `return` on failure.
  - Site 3 (ServeDownload) uses `cid, err = commitUUID(meta.Commit)` reusing the existing `err` variable.
  - Site 4 (computeB4Digest) uses `cid, err := commitUUID(commit)` and propagates the error; both callers (ServeHTTP, ServeGraph) call `internalError` instead of `upstreamError`.
  - Site 5 (registerResolved) uses `uuid, err := commitUUID(sha)` with `h.api.log + context.Background()` warn line and `return`.
  - 400 message at the `ref == nil` branch updated to D-12 text.
  - Comment that quoted the old 400 message updated to quote the new text.
- `internal/connect/api_test.go`
  - 4 short hex commit literals padded to 40 chars (deviation, see below).
  - New D-12 substring assertion in `TestBadRequest_OnUnknownCommitID`.
- `internal/connect/uuid_format_test.go`
  - New D-12 substring assertion in `TestServeDownload_UnknownCommitID_ReturnsBadRequest`.

## Decisions Made

- **Helper signature follows the plan's explicit (w, r, ...) order rather than the package's existing (r, w, ...) convention.** The plan was explicit that the helper takes `(w http.ResponseWriter, r *http.Request, commitID, upstreamErr string)`, and 5 call sites use that order. The codebase's `logHandlerError` family uses (r, w, ...) so this is a minor inconsistency; the plan's choice was honored verbatim.
- **Both `upstreamError` callers of `computeB4Digest` were switched to `internalError` (500) per the plan's "verify the caller is updated" instruction.** The caller cannot distinguish a `commitUUID` failure from an upstream `GetFiles` failure, so genuine upstream errors that flow through `computeB4Digest` also map to 500 with `error_class=internal`. This is a documented trade-off — the alternative would have required wrapping the `commitUUID` error in a sentinel type and having the caller switch on it, which the plan did not request. Operators alerting on `error_class=internal` should be aware that this specific path can also fire on transient upstream hiccups.
- **`registerResolved` uses `h.api.log + context.Background()` because the function has no `http.Request` in scope.** It is called from `prewarmHeads` (background, no request) and from `probeCommitID` (request-scoped, but the request is not threaded through). Using `h.hlog(r)` would have required a parameter change, which the plan did not request. The log line shape is identical to the helper's; an operator can grep for `"internal: commitUUID failure"` and see both paths.
- **Loop-shadow avoidance via `cidErr` rename, not if-init shadowing.** The plan offered both; the rename was chosen so the `cid` variable stays accessible to the rest of the loop body without extra assignment lines. (The if-init form would have left `cid` scoped to the if, requiring a follow-up `cid = ...` outside the if for the downstream code at line ~163+ to see the value.)
- **Deviation: padded 4 short mock commits in `api_test.go` from 6/8 chars to 40 hex chars.** The new `commitUUID` helper rejects non-40-char input per D-03, so without padding 6 tests would have returned 500 `"internal error"` instead of exercising their real assertions. Padded literals preserve the test's intent (same prefix, same owner/module, same scenario). The other `mockProvider` instances in api_test.go (lines 944, 1018) use the same short `"deadbeef"` literal but pair it with `err: errUpstream`, which causes `GetMeta` to fail before `commitUUID` is called — those tests are unaffected and were not modified.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Padded 4 short mock commit literals in api_test.go to 40 hex chars**
- **Found during:** Task 2 (`go test ./internal/connect/ -count=1` after the 400 message change)
- **Issue:** The plan's `files_modified` list for Task 2 did not include fixture updates, but the new `commitUUID` helper (introduced in 16-01 and now wired into all 5 callsites in Task 1) rejects non-40-char input per D-03. The pre-existing test mocks used 6/8-char hex literals (`"abc123"`, `"deadbeef"`, `"cafe1234"`, `"f00dcafe"`, `"aaa111"`, `"bbb222"`, `"cafef00d"`) that previously worked only because `commitUUID` silently accepted them under the old SHA-256 implementation. With the strict 40-char contract, those literals would cause the test handler to return 500 with `"internal error"`, breaking 6 distinct tests:
  - `TestV1RoutesNotReachingRootHandler` (Commit `"abc123"`)
  - `TestCommitServiceV1ReturnsProtobuf` (Commit `"deadbeef"`)
  - `TestGraphServiceV1ReturnsProtobuf` (Commit `"cafe1234"`)
  - `TestDownloadServiceV1ReturnsProtobuf` (Commit `"f00dcafe"`)
  - `TestPrewarmHeads_PopulatesCommitMap` (mockSource commits `"aaa111"`, `"bbb222"`)
  - `TestProbeCommitID_HitResolvesAndCaches` (mockSource commits `"deadbeef"`, `"cafef00d"`, probe arg `"deadbeef"`)
- **Fix:** Padded each short literal out to exactly 40 lowercase hex chars by appending `'0'` characters. For example `"deadbeef"` (8 chars) became `"deadbeef00000000000000000000000000000000"` (40 chars). Each `commitMap[...]` lookup key and each `probeCommitID` argument that referenced the old literal was updated to the new padded form. The 2 short literals that pair with `err: errUpstream` (lines 944, 1018) were left alone — `GetMeta` returns the error before `commitUUID` is called, so those tests are unaffected.
- **Files modified:** `internal/connect/api_test.go` (6 mock commit strings + 4 lookup-key strings, in 6 distinct test functions)
- **Verification:** All `internal/connect` tests pass; `go build ./...` exits 0; `go vet ./internal/connect/...` exits 0.
- **Committed in:** `d601748` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1: test fixture regression caused by the upstream strict-validation contract change)
**Impact on plan:** Minimal — the deviation is a direct consequence of the 16-01 helper change. The fix is mechanical (padding, not rewriting), preserves the test scenario's intent, and was necessary to satisfy the Task 2 acceptance criterion "all `internal/connect` tests pass".

## Threat Surface Scan

The plan's `<threat_model>` was reviewed against the changes. The new `internalError` helper logs `commit_id` (the bad input) and `upstream_error` (the Go error string) — neither contains secrets. The new 400 message is identical in disclosure profile to the old one (single sentence, no internal identifiers). No new network endpoints, no new auth paths, no new file access patterns, no schema changes at trust boundaries. The threat register in PLAN.md is fully covered by the implementation.

## Plan Verification Status

| Check | Result | Notes |
|------|--------|-------|
| `go build ./...` exits 0 | **PASS** | repo root build clean |
| `go test ./internal/connect/ -count=1` all PASS | **PASS** | full package test suite ok |
| `grep -c 'commitUUID(' internal/connect/commits.go` returns `5` | **PASS** | 5 |
| `grep -n 'error_class.*internal' internal/connect/commits.go` shows helper + registerResolved | **PASS** | line 102 (helper), line 908 (registerResolved), plus doc comment |
| `grep -n 'h.hlog(r)' internal/connect/commits.go` shows helper uses request-scoped logger | **PASS** | line 101 inside the new helper |
| `grep -n 'http.Error' internal/connect/commits.go` shows helper uses http.Error | **PASS** | line 105 inside the new helper, line 799 in logHandlerError |
| `grep -rn 'must call CommitService/GetCommits first' internal/connect/` returns 0 hits | **PASS** | 0 |
| `grep -rn 're-run buf mod update / buf dep update' internal/connect/` returns at least 3 hits | **PASS** | 6 hits (production message + comment + 2 test assertion files × 2 lines each) |
| `grep -n 'probeCommitID(' internal/connect/commits.go` is unchanged | **PASS** | call site at line 555 in ServeDownload is preserved per D-07/D-08 |

## Self-Check: PASSED

- All 3 modified files present at expected paths.
- Both task commits present in branch: `fe4dade` (Task 1: 5 callsite rewires + helper), `d601748` (Task 2: 400 message + test updates + fixture padding).
- No modifications to shared orchestrator files (`STATE.md`, `ROADMAP.md`, `REQUIREMENTS.md`).
- `gofmt -e` clean on all 3 modified source files.
- All `internal/connect` tests pass.
- Probe contract (D-07/D-08) verified preserved by grep.

## User Setup Required

None - no external service configuration required. No new dependencies introduced. No new env vars, no new config keys, no new routes, no migration steps.

## Next Phase Readiness

- Phase 16 is complete: 16-01 (helper rewrite), 16-02 (this plan, error wiring + 400 message), 16-03 (CHANGELOG entry).
- Operators upgrading the proxy will see a single grep target — `"internal: commitUUID failure"` — for the "upstream sent us a non-conforming SHA" class of bug.
- The buf client will see a 400 with the new D-12 message when its cached `buf.lock` contains pre-Phase-16 commit ids; clients recover by running `buf mod update` or `buf dep update` per the message and the CHANGELOG entry.
- No further plans in this phase.

---
*Phase: 16-commit-id-resolution-improvements*
*Completed: 2026-07-06*
