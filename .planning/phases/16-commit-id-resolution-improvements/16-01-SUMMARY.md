---
phase: 16-commit-id-resolution-improvements
plan: 01
subsystem: api
tags: [connect, uuid, commit-id, buf-v1.69]

# Dependency graph
requires:
  - phase: "phase-15 (or earlier)"
    provides: "Existing commitUUID implementation with SHA-256 derivation in internal/connect/commits_helpers.go"
provides:
  - "commitUUID(string) (string, error) — derives 16-byte UUID from first 14 SHA bytes + UUID version/variant bits"
  - "preResolveForTest — test-only fixture that pads short hex strings to 40 chars"
  - "Updated test file with 4 new test cases per D-13 (known SHA, invalid input, inverse recovery, preResolveForTest)"
  - "Updated uuid_format_test.go call sites to new (string, error) signature"
affects:
  - "phase 16 plan 16-02 — will update 5 commits.go call sites and 400-message in ServeDownload per D-04/D-12"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Strict input contract: 40 lowercase hex chars only; signature change to (string, error) instead of empty-string sentinel"
    - "Inverse-recovery test pattern: decode UUID, skip bytes 6 and 8, recover sha[0..13] as regression-guard against accidental re-hashing"
    - "Test-only fixture pattern: preResolveForTest lives in production file but is consumed only by tests in the same package"

key-files:
  modified:
    - "internal/connect/commits_helpers.go — dropped crypto/sha256 import, added errors, rewrote commitUUID, added preResolveForTest"
    - "internal/connect/commits_helpers_test.go — updated existing tests to new signature, added 4 new test functions"
    - "internal/connect/uuid_format_test.go — updated 3 commitUUID call sites to new 2-return signature (deviation, see below)"

key-decisions:
  - "Used `errors.New(\"commitUUID: input is not 40 lowercase hex characters\")` for both length and hex-decode failure paths (concrete error, in line with D-03)"
  - "Test fixture truncates inputs >= 40 chars to first 40 (defensive against off-by-one in caller)"
  - "Mixed-case 40-char input is accepted (Go hex.DecoderString is case-insensitive) — plan's row was dropped per plan guidance: 'if the helper does not enforce case, drop this row'"
  - "Inverse-recovery test directly verifies bytes, not a separate uuidToSHA14BytesPrefix helper — keeps the test self-contained and the regression-guard explicit"

patterns-established:
  - "Test fixture naming: preResolveForTest makes the test-only intent obvious in code review"
  - "Test variant subtest naming: name each t.Run after the input class (empty/39/41/non-hex) for clear failure output"

requirements-completed: [SC-1]

# Metrics
duration: 9min
completed: 2026-07-06
---

# Phase 16 Plan 01: commitUUID rewrite + helper tests

**commitUUID rewritten to derive a 16-byte UUID from the first 14 SHA bytes plus UUID version/variant bits; new (string, error) signature; preResolveForTest test fixture added**

## Performance

- **Duration:** 9 min
- **Started:** 2026-07-06T13:07:25Z
- **Completed:** 2026-07-06T13:16:30Z (approx)
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- `commitUUID` now derives the 16-byte UUID from the first 14 bytes of the decoded 40-char SHA (positions 0..5, 7, 9..15) plus UUID version-4 (0x40) at position 6 and RFC 4122 variant (0x80) at position 8. The result is a 32-char dashless lowercase hex string, exactly the shape `buf.util.uuidutil.FromDashless` requires.
- The `crypto/sha256` path is gone. The new construction is byte-exact and trivially reversible: `uuidBytes[0..5] ++ uuidBytes[7] ++ uuidBytes[9..16]` recovers `sha[0..13]`.
- Signature is now `(string, error)` with a strict 40-lowercase-hex input contract. Any deviation returns `("", error)` — the empty-string-as-sentinel pattern was fragile and is replaced per D-03.
- `preResolveForTest` fixture added: right-pads short hex strings with `'0'` to 40 chars, or returns the first 40 chars for longer inputs. Used by `TestPreResolveForTest` to exercise the 7-byte and 14-byte short-SHA shapes that production callers never see directly.
- 4 new test functions added per D-13: `TestCommitUUID_KnownSHA` (5 SHAs, exact UUID bytes), `TestCommitUUID_InvalidInput` (6 invalid input shapes), `TestCommitUUID_InverseRecovery` (4 SHAs, byte-exact round-trip), `TestPreResolveForTest` (4 input classes + padding-collision guard).
- 3 call sites in `uuid_format_test.go` updated to handle the new 2-return signature so the test build does not break (deviation from plan's `files_modified` list — see Deviations).

## Task Commits

1. **Task 1: Rewrite commitUUID + add preResolveForTest fixture** — `2ff6157` (refactor)
2. **Task 2: Update existing commitUUID tests + add 4 new test cases** — `3a5db9c` (test)

## Files Created/Modified

- `internal/connect/commits_helpers.go` — dropped `crypto/sha256` import; added `errors`; rewrote `commitUUID` per D-01/D-02/D-03; added `preResolveForTest` per D-05. Doc comment updated to describe new contract.
- `internal/connect/commits_helpers_test.go` — updated 3 existing tests (Format, Determinism, Distinct) to call new 2-return signature; added 4 new test functions with 19 total sub-cases covering known SHAs, invalid inputs, inverse recovery, and the pre-resolve fixture.
- `internal/connect/uuid_format_test.go` — updated 3 `commitUUID` call sites to handle the new `(string, error)` signature. (Deviation, see below.)

## Decisions Made

- **Error message** — `commitUUID: input is not 40 lowercase hex characters` for both the length and hex-decode failure paths. Concrete and matches D-03's "concrete error" guidance; callers can match on the substring if they want to log a specific error class.
- **Mixed-case input is accepted** — Go's `hex.DecodeString` is case-insensitive, so an all-uppercase 40-char SHA decodes fine. Per the plan's "if the helper does not enforce case, drop this row" guidance, the mixed-case row was dropped from `TestCommitUUID_InvalidInput`. D-03's wording allows either case, and rejecting valid input would be needlessly strict.
- **Inverse-recovery test is byte-exact** — no `uuidToSHA14BytesPrefix` helper. The test inlines the inverse arithmetic, so a future regression that re-introduces a hash step fails the test with a clear byte-level diagnostic.
- **Padding-collision guard** — the pre-resolve test explicitly asserts that a 7-char prefix padded with zeros mints a different UUID than the full 40-char SHA. If the pre-resolve path ever silently produced a real-SHA UUID, the test would catch it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated 3 commitUUID call sites in uuid_format_test.go to new (string, error) signature**
- **Found during:** Task 2 (running `go test` to verify the new test functions)
- **Issue:** The plan's `files_modified` listed only `commits_helpers.go` and `commits_helpers_test.go`, but the broader `uuid_format_test.go` also calls `commitUUID` at lines 37, 116, and 333 with the old 1-return signature. After Task 1 changed the signature, the test build broke with "assignment mismatch: 1 variable but commitUUID returns 2 values". D-13 (in the locked decisions) explicitly says "Update existing tests in `internal/connect/commits_helpers_test.go` and `internal/connect/uuid_format_test.go` to match the new format" — so the test file was in scope per the decisions but missing from the plan's file list.
- **Fix:** Updated each of the 3 call sites to `id, err := commitUUID(sha); if err != nil { t.Fatalf(...) }`. No test logic was changed, only the call shape.
- **Files modified:** `internal/connect/uuid_format_test.go`
- **Verification:** `gofmt -e` clean; files re-read to confirm call shape matches the new signature.
- **Committed in:** `3a5db9c` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1: bug introduced by current task's signature change)
**Impact on plan:** Minimal — the deviation is a direct consequence of the helper signature change. D-13 already required both test files to be updated; the deviation is that `files_modified` was incomplete, not that scope was added.

## Issues Encountered

**Known: `commits.go` build is broken until plan 16-02 lands.**
The 5 production call sites in `internal/connect/commits.go` (lines 144, 336, 654, 737, 882) still expect the old 1-return signature. Per the plan split ("Call sites in `commits.go` are updated in plan 16-02; this plan only ships the helper and its tests"), these are intentionally untouched in this plan. `go build ./...` and `go test ./internal/connect/...` both fail until plan 16-02 is executed. Helper logic was verified via a standalone program run (see "Standalone verification" below).

## Standalone verification of helper logic

The package-level test build cannot run because of the commits.go call site break (see Issues Encountered). To prove the helper itself is correct, a standalone Go program was written at `/tmp/helper_verify/main.go` that copies the helper code (no package dependencies) and runs the 5-SHA known-value table, the 5-shape invalid input table, the 3-SHA inverse recovery check, the 4-shape preResolveForTest check, and the padding-collision guard. All checks pass.

Expected output (verified before temp file was cleaned up):

```
PASS non-zero: 81353411f7b0401080d5b9ebeb189906
PASS all-zero: 00000000000040008000000000000000
PASS all-ones: ffffffffffff40ff80ffffffffffffff
PASS leading-01: 0123456789ab40cd80ef0123456789ab
PASS deadbeef: deadbeefdead40be80efdeadbeefdead
PASS invalid(""): err=commitUUID: input is not 40 lowercase hex characters
PASS invalid("abc"): err=commitUUID: input is not 40 lowercase hex characters
PASS invalid("zzz..."): err=commitUUID: input is not 40 lowercase hex characters
PASS invalid("000...039"): err=commitUUID: input is not 40 lowercase hex characters
PASS invalid("000...041"): err=commitUUID: input is not 40 lowercase hex characters
PASS inverse for 81353411f7b010d5b9ebeb1899066aac18a36701
PASS inverse for 0000000000000000000000000000000000000000
PASS inverse for deadbeefdeadbeefdeadbeefdeadbeefdeadbeef
preResolveForTest(len=0)  = 0000...0000 (len=40)
preResolveForTest(len=3)  = abc0000...0000 (len=40)
preResolveForTest(len=40) = aaaa...aaaa (len=40, unchanged)
preResolveForTest(len=50) = aaaa...aaaa (len=40, truncated)
PASS padding: full=8135341 padded=8135341000...000 fullU=... paddedU=...
```

## Plan Verification Status

| Check | Result | Notes |
|------|--------|-------|
| `go build ./...` exits 0 | **FAIL (expected)** | 5 commits.go call sites broken; fix is in plan 16-02 |
| `go test ./internal/connect/ -run 'CommitUUID|PreResolveForTest' -v -count=1` all PASS | **FAIL (expected)** | Same as above; cannot build package. Standalone verification passes. |
| `grep -c 'crypto/sha256' internal/connect/commits_helpers.go internal/connect/commits_helpers_test.go` returns `0\t0` | **PASS** | `0\t0` |
| `grep -n 'func commitUUID' internal/connect/commits_helpers.go` shows the new 2-return signature | **PASS** | Line 38: `func commitUUID(gitSHA string) (string, error) {` |
| `grep -n 'func preResolveForTest' internal/connect/commits_helpers.go` shows the test fixture | **PASS** | Line 65: `func preResolveForTest(short string) string {` |

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Helper layer is complete and self-consistent. Plan 16-02 can:
  1. Update the 5 `commits.go` call sites to the new 2-return signature with the `log warn + return 500` pattern from D-04.
  2. Update the 400 message in `ServeDownload` per D-12.
  3. Add a CHANGELOG entry per D-11.
  4. Re-run the full test suite (`go test ./...`) — all unit tests should pass once commits.go is updated.
- No new external dependencies were introduced (Rule 3 is satisfied — no `go get` invocations).
- No blockers for plan 16-02. The worktree is clean and ready for the next plan to start from this branch state.

---
*Phase: 16-commit-id-resolution-improvements*
*Completed: 2026-07-06*
