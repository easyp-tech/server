---
phase: 16-commit-id-resolution-improvements
verified: 2026-07-06T13:35:00Z
status: passed
score: 13/13 must-haves verified
overrides_applied: 0
overrides: []
gaps: []
deferred: []
human_verification: []
---

# Phase 16: Commit ID Resolution Improvements Verification Report

**Phase Goal:** Make commit-id resolution more robust (accept short git SHAs, fall back to upstream probe on cache miss) and the not-found failure mode diagnosable (clear error response and structured log line)
**Verified:** 2026-07-06T13:35:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | 16-01: `commitUUID` returns a 16-byte UUID derived from the first 14 SHA bytes plus UUID version/variant bits | VERIFIED | `internal/connect/commits_helpers.go:38-58` — signature `(string, error)`, byte table: `result[0..6]=sha[0..6]`, `result[6]=0x40` (version-4), `result[7]=sha[6]`, `result[8]=0x80` (RFC 4122 variant), `result[9..16]=sha[7..14]`; total 16 bytes; hex-encoded to 32 lowercase chars. |
| 2   | 16-01: `commitUUID` returns `("", error)` for inputs that are not exactly 40 valid hex characters | VERIFIED | `commits_helpers.go:39-44` — explicit length check + `hex.DecodeString` check returning the same `errors.New("commitUUID: input is not 40 lowercase hex characters")`. `TestCommitUUID_InvalidInput` covers empty/39/41/non-hex/mixed/1-char inputs and passes. |
| 3   | 16-01: Existing tests in `commits_helpers_test.go` are updated to the new expected UUIDs for their known SHAs | VERIFIED | `TestCommitUUIDFormat:23` asserts `want = "81353411f7b0401080d5b9ebeb189906"` for the reference SHA. `TestCommitUUIDDeterminism`, `TestCommitUUIDDistinct` updated to 2-return signature. All three pass under `go test -run CommitUUID -v`. |
| 4   | 16-01: New tests cover known-SHA mapping, invalid input, inverse recovery, and `preResolveForTest` | VERIFIED | `TestCommitUUID_KnownSHA` (5 cases), `TestCommitUUID_InvalidInput` (6 cases), `TestCommitUUID_InverseRecovery` (4 cases), `TestPreResolveForTest` (5 subtests incl. padding-collision guard) — all PASS. |
| 5   | 16-02: All 5 call sites of `commitUUID` in `commits.go` handle the new `(string, error)` return | VERIFIED | `grep -c 'commitUUID(' internal/connect/commits.go` returns 5. Sites: line 158 (ServeHTTP), line 349 (ServeGraph), line 666 (ServeDownload), line 753 (computeB4Digest), line 901 (registerResolved). All read 2-return form. |
| 6   | 16-02: When `commitUUID` returns an error, the request fails with HTTP 500 and a structured warn log tagged `error_class=internal, commit_id, upstream_error` | VERIFIED | `internalError` helper at `commits.go:100-106`: `h.hlog(r).LogAttrs(... slog.String("error_class", "internal"), slog.String("commit_id", commitID), slog.String("upstream_error", upstreamErr))` + `http.Error(w, "internal error", http.StatusInternalServerError)`. All 4 HTTP call sites (158, 349, 666) call `h.internalError`; `registerResolved` (line 901) logs with `h.api.log + context.Background()`; `computeB4Digest` (line 753) propagates the error and its callers (line 176, 356) call `h.internalError`. |
| 7   | 16-02: The 400 response at the `ref == nil` branch in `ServeDownload` carries the new D-12 message | VERIFIED | `commits.go:582`: `h.badRequest(r, w, "unknown commit id: re-run buf mod update / buf dep update", ...)` (400 status preserved). Comment at `commits.go:399` also updated to quote the new text. |
| 8   | 16-02: Test files that hard-coded the old 400 message are updated to the new string | VERIFIED | `api_test.go:597-599` and `uuid_format_test.go:215-217` add explicit `bytes.Contains` / `strings.Contains` assertions for the D-12 substring; both pass. Old `grep -rn 'must call CommitService/GetCommits first' internal/connect/` returns 0 hits. |
| 9   | 16-02: `probeCommitID` call site in `ServeDownload` is unchanged (still runs unconditionally when `probeEnabled` is true and the fast-path `resolveForeignCommitID` miss falls through to it) | VERIFIED | `commits.go:555` still calls `h.probeCommitID(r.Context(), commitID)` inside the `ServeDownload` `ref == nil` branch, ahead of the `badRequest` 400. Probe contract D-07/D-08 preserved. |
| 10  | 16-02: `internalError` helper exists and is used | VERIFIED | `commits.go:100-106` defines the helper. Called at lines 160, 351, 668, 176, 356. |
| 11  | 16-03: `CHANGELOG.md` exists at the repo root and contains a new entry under the current unreleased section | VERIFIED | `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/CHANGELOG.md` exists with `# Changelog` / `## [Unreleased]` / `### Changed` structure. |
| 12  | 16-03: The new entry contains the literal sentence specified in D-11 | VERIFIED | `CHANGELOG.md:6` reads verbatim: `commit-id format change: the proxy now mints the first 16 bytes of the git SHA (with UUID version/variant bits) instead of the SHA-256 of the git SHA. Existing `buf.lock` entries are invalidated; clients must re-run `buf mod update` or `buf dep update` after upgrading.` |
| 13  | Phase 16: ROADMAP Success Criteria (SC-1, SC-2, SC-3) are met in the codebase | VERIFIED | SC-1: format change in `commits_helpers.go` + 5-SHA `TestCommitUUID_KnownSHA` + 4-SHA `TestCommitUUID_InverseRecovery` exercise both full-40-char and 7-char/14-byte shapes via `preResolveForTest`. SC-2: `probeCommitID` preserved at `commits.go:555`. SC-3: 400 response at `commits.go:582` names the offending `commit_id` in both wire body and structured log line via `slog.String("commit_id", commitID)`. |

**Score:** 13/13 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/connect/commits_helpers.go` | `func commitUUID` (string, error) per D-01..D-05 | VERIFIED | Line 38. Imports `errors`, no `crypto/sha256`. `preResolveForTest` at line 65. |
| `internal/connect/commits_helpers_test.go` | Updated + 4 new test functions | VERIFIED | `TestCommitUUIDFormat`, `TestCommitUUIDDeterminism`, `TestCommitUUIDDistinct` updated to 2-return form. New: `TestCommitUUID_KnownSHA`, `TestCommitUUID_InvalidInput`, `TestCommitUUID_InverseRecovery`, `TestPreResolveForTest`. |
| `internal/connect/commits.go` | 5 call sites updated; `internalError` helper; 400 message updated | VERIFIED | `internalError` at line 100. 5 `commitUUID(` call sites at lines 158, 349, 666, 753, 901. 400 message at line 582. |
| `internal/connect/api_test.go` | Test assertions for the new 400 message | VERIFIED | D-12 substring assertion at line 597. Mock commit fixtures padded to 40 hex chars. |
| `internal/connect/uuid_format_test.go` | Test assertions for the new 400 message | VERIFIED | D-12 substring assertion at line 215. 3 `commitUUID` call sites updated to 2-return form (Rule 1 deviation from 16-01, documented in SUMMARY). |
| `CHANGELOG.md` | User-visible release note describing the commit-id format change and `buf.lock` invalidation | VERIFIED | Single bullet under `## [Unreleased] / ### Changed`, verbatim D-11 sentence. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `internal/connect/commits_helpers.go` | `encoding/hex` | `hex.DecodeString` of the 40-char git SHA | WIRED | Line 42. |
| `internal/connect/commits_helpers_test.go` | `internal/connect/commits_helpers.go` | package-internal test calling `commitUUID` and `preResolveForTest` | WIRED | `preResolveForTest` referenced at lines 243, 251, 260, 268, 284. |
| `internal/connect/commits.go` | `internal/connect/commits_helpers.go` | calls `commitUUID` and propagates the error | WIRED | 5 call sites, all 2-return form. |
| `internal/connect/commits.go` | `h.api.log` / `h.hlog(r)` | `slog/warn` with `error_class=internal`, `commit_id`, `upstream_error` attrs (D-04) | WIRED | `internalError` helper (line 101) uses `h.hlog(r)`; `registerResolved` (line 907) uses `h.api.log + context.Background()`. Both include the 3 required attributes. |
| `CHANGELOG.md` | `internal/connect/commits_helpers.go` | release note describes the behavior change in 16-01/16-02 | WIRED | `commit-id format change:` substring present in `CHANGELOG.md:6`. |

### Data-Flow Trace (Level 4)

N/A — Phase 16 is a behavior / contract change to a pure function (`commitUUID`) plus a documentation entry. No dynamic data is rendered from an external source. The data flowing through `commitUUID` is the literal input `gitSHA` string, and the function's correctness is exercised by the byte-exact `TestCommitUUID_KnownSHA` table and the byte-exact `TestCommitUUID_InverseRecovery` round-trip.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| `go build ./...` exits 0 from the repo root | `go build ./...` | exit 0, no output | PASS |
| `go vet ./...` exits 0 from the repo root | `go vet ./...` | exit 0, no output | PASS |
| `go test ./internal/connect/ -count=1` all PASS | `go test ./internal/connect/ -count=1` | `ok github.com/easyp-tech/server/internal/connect 0.267s` | PASS |
| `go test ./internal/connect/ -run 'CommitUUID\|PreResolveForTest' -v -count=1` all PASS | same | 4 parent tests + 19 subtests, all PASS in 0.246s | PASS |
| `commitUUID` returns the expected UUID for the reference SHA `81353411f7b010d5b9ebeb1899066aac18a36701` | `TestCommitUUIDFormat` | expects `81353411f7b0401080d5b9ebeb189906`, PASS | PASS |
| `commitUUID("")` returns `("", error)` | `TestCommitUUID_InvalidInput/empty` | PASS | PASS |
| `commitUUID` inverse recovery recovers first 14 SHA bytes | `TestCommitUUID_InverseRecovery` (4 SHAs) | PASS | PASS |
| `internalError` helper exists with `h.hlog(r)` + `http.Error` | `grep -n 'func (h \*commitServiceHandler) internalError' internal/connect/commits.go` | line 100 | PASS |
| 400 message at `ServeDownload` is the new D-12 text | `grep -n 're-run buf mod update / buf dep update' internal/connect/commits.go` | line 582 | PASS |
| Old 400 message is gone from `internal/connect/` | `grep -rn 'must call CommitService/GetCommits first' internal/connect/` | 0 hits | PASS |
| `CHANGELOG.md` contains the verbatim D-11 sentence | `grep -c 'commit-id format change: the proxy now mints the first 16 bytes of the git SHA' CHANGELOG.md` | 1 | PASS |
| `probeCommitID` call site in `ServeDownload` is preserved | `grep -n 'probed, ok := h.probeCommitID' internal/connect/commits.go` | line 555 | PASS |
| 5 `commitUUID(` call sites remain in `commits.go` | `grep -c 'commitUUID(' internal/connect/commits.go` | 5 | PASS |
| `crypto/sha256` is no longer imported in the helper or its test | `grep -c 'crypto/sha256' internal/connect/commits_helpers.go internal/connect/commits_helpers_test.go` | 0 / 0 | PASS |

### Probe Execution

N/A — no `scripts/*/tests/probe-*.sh` declared by any plan in this phase. The conventional probe-discovery pattern finds none.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| SC-1 | 16-01, 16-03 | Mint commit id from first 16 bytes of git SHA; unit test verifies full-40 and short (7-char) SHAs round-trip through `commitUUID` deterministically | SATISFIED | `commits_helpers.go:38-58` constructs the 16-byte UUID from `sha[0..13] + 0x40 + 0x80`. `TestCommitUUID_KnownSHA` (5 SHAs) and `TestCommitUUID_InverseRecovery` (4 SHAs) exercise the deterministic mapping. `TestPreResolveForTest` covers the 7-char padding path. CHANGELOG documents the cutover. |
| SC-2 | 16-02 | When `DownloadService/Download` carries a commit id not in `commitMap` and the proxy serves multiple modules, the handler probes every configured source; single-source deployments keep `resolveForeignCommitID` fast path | SATISFIED | `commits.go:555` still calls `h.probeCommitID(r.Context(), commitID)` unconditionally inside the `ref == nil` branch of `ServeDownload`. Probe contract D-07/D-08 preserved (this code was already on the branch pre-Phase-16 and is untouched in 16-02). |
| SC-3 | 16-02 | The 400 response for an unresolvable commit id names the id itself in both wire body and structured log line | SATISFIED | `commits.go:582` calls `h.badRequest(r, w, "unknown commit id: re-run buf mod update / buf dep update", slog.String("commit_id", commitID), slog.Int("body_bytes", len(body)))`. The `commit_id` is in both the body substring and the structured log line. D-12 substring asserted by `api_test.go:597` and `uuid_format_test.go:215`. |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | — | — | No `TBD`, `FIXME`, `XXX`, `TODO`, `HACK`, `PLACEHOLDER` markers in any of the 4 modified source files. No empty-function stubs. No hardcoded empty-data props. No console-log-only handlers. |

### Human Verification Required

None. The phase's deliverables are pure-function logic (byte-exact) and a static documentation entry; both are fully exercised by the unit-test suite. The HTTP error response shape and structured log attributes are also covered by integration tests in `api_test.go` and `uuid_format_test.go`.

### Gaps Summary

None. All 13 must-haves across the three plans are verified, all three ROADMAP Success Criteria are satisfied, all three named tests pass, the `go build` / `go vet` / `go test` triple is clean, and no anti-patterns or regressions were introduced.

---

_Verified: 2026-07-06T13:35:00Z_
_Verifier: Claude (gsd-verifier)_
