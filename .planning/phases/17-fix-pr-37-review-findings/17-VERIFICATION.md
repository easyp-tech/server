---
phase: 17-fix-pr-37-review-findings
verified: 2026-07-07T10:28:00Z
status: passed
score: 13/13 must-haves verified
overrides_applied: 0
overrides: []
gaps: []
deferred:
  - id: T-17-DEFER
    description: "probeCommitID reports a hit while registerResolved silently no-ops when sha isn't 40 or 64 hex (32-char hex path). Real-world impact is low (no known provider returns 32-char hex shas); deferred to a future phase per plan question #3."
human_verification: []
---

# Phase 17: Fix PR #37 review findings Verification Report

**Phase Goal:** Address pre-merge review findings from PR #37 (Phase 16) so the commit-id format cutover lands without undoing the v1.3 logging-quality work or breaking Bitbucket SHA-256 repositories
**Verified:** 2026-07-07T10:28:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | 17-01: A `computeB4Digest` failure from `GetFiles` or `computeB4DigestFromFiles` returns HTTP 502 (not 500) via `h.upstreamError` and logs the full ERR-05 context (server, protocol, request_id, error, status, error_class=upstream) | VERIFIED | `commits.go:174-186` and `commits.go:354-366` (after the Phase 17 edit) — the `else` branch of `errors.Is(err, errCommitUUIDContract)` calls `h.upstreamError(r, w, fmt.Sprintf("digest for %s/%s", ref.owner, ref.module), ...)` with `owner`, `module`, `repo`, `commit`, `upstream_error` attrs. `grep -c 'h\.upstreamError.*"digest for %s/%s"' internal/connect/commits.go` returns 2. `h.upstreamError` itself (defined elsewhere in the file) writes status 502. |
| 2   | 17-01: A `computeB4Digest` failure from the `commitUUID` check still returns 500 with `error_class=internal` via `logHandlerError` (genuine contract violation) | VERIFIED | `commits.go:765-780` — `computeB4Digest` wraps the inner `commitUUID(commit)` error via `fmt.Errorf("%w: %v", errCommitUUIDContract, uidErr)`. The two call sites (`commits.go:174-180`, `commits.go:354-360`) dispatch on `errors.Is(err, errCommitUUIDContract)` and call `h.logHandlerError(r, w, "internal error", http.StatusInternalServerError, slog.String("commit_id", cid), slog.String("upstream_error", err.Error()))`. `grep -c 'errors.Is(err, errCommitUUIDContract)' internal/connect/commits.go` returns 2. |
| 3   | 17-01: The `internalError` helper no longer exists in `commits.go` | VERIFIED | `grep -c 'func (h \*commitServiceHandler) internalError' internal/connect/commits.go` returns 0. All 5 former call sites (160, 176, 351, 356, 668 — line numbers pre-edit) refactored to call `h.logHandlerError` (3 contract-violation sites) or dispatch to `h.logHandlerError` / `h.upstreamError` (2 digest-error sites). `grep -c 'h\.internalError(' internal/connect/commits.go` returns 0. |
| 4   | 17-01: `errCommitUUIDContract` sentinel exists and is wired through `errors.Is` | VERIFIED | `commits.go:94-102` declares `var errCommitUUIDContract = errors.New("commitUUID contract violation")`. `grep -c 'errCommitUUIDContract' internal/connect/commits.go` returns 6: 1 declaration + 2 `errors.Is` checks + 1 wrap in `computeB4Digest` + 2 comment mentions. |
| 5   | 17-01: `commitUUID` accepts both 40-char and 64-char lowercase hex | VERIFIED | `commits_helpers.go:41-49` — `if len(gitSHA) != 40 && len(gitSHA) != 64 { return "", errors.New("commitUUID: input is not 40 or 64 lowercase hex characters") }`. Byte table (lines 48-58) reads `sha[0:6]`, `sha[6]`, `sha[7:14]` which are all within the 20-byte SHA-1 and 32-byte SHA-256 decoded buffers. `grep -c 'len(gitSHA) != 40 && len(gitSHA) != 64' internal/connect/commits_helpers.go` returns 1. |
| 6   | 17-01: A new `TestCommitUUID_SHA256_KnownSHA` test exercises 64-char inputs | VERIFIED | `commits_helpers_test.go:171-225` — 4 subtests: `all-zero 64-char SHA-256`, `all-ones 64-char SHA-256`, `64-char SHA-256 with 14-byte prefix matching 40-char fixture` (the load-bearing regression-guard for "function consumes SHA-256 bytes, not just the first 40 chars"), and `all-deadbeef 64-char SHA-256`. All 4 PASS under `go test -v -run TestCommitUUID_SHA256_KnownSHA`. |
| 7   | 17-01: `TestCommitUUID_InvalidInput` extended with 63-char and 65-char inputs | VERIFIED | `commits_helpers_test.go:236-237` — `{name: "63 chars", in: strings.Repeat("a", 63)}` and `{name: "65 chars", in: strings.Repeat("a", 65)}`. Both PASS. Total 8 subtests in `TestCommitUUID_InvalidInput`, all PASS. |
| 8   | 17-01: `preResolveForTest` lives in `commits_helpers_test.go` (not production) | VERIFIED | `grep -c 'func preResolveForTest' internal/connect/commits_helpers.go` returns 0 (deleted from production). `grep -c 'func preResolveForTest' internal/connect/commits_helpers_test.go` returns 1 (moved to test file with original 5-line doc comment). The `strings` import in `commits_helpers.go` is retained for `strings.SplitN` at line 288 (`parseModuleRefByID`). |
| 9   | 17-01: All 5 former `preResolveForTest` caller sites continue to resolve and pass | VERIFIED | `commits_helpers_test.go:272, 280, 289, 297, 313` — all 5 callers (TestPreResolveForTest subtests) PASS. `go test -run TestPreResolveForTest -v` shows 5/5 subtests PASS. |
| 10  | 17-01: The 400 not-found response message reads "unknown commit id: re-resolve via buf mod update / buf dep update" | VERIFIED | `commits.go:601` wire body: `h.badRequest(r, w, "unknown commit id: re-resolve via buf mod update / buf dep update", ...)`. `commits.go:418` inline comment quotes the same text. `grep -c 're-resolve via buf mod update / buf dep update' internal/connect/commits.go` returns 2. |
| 11  | 17-01: Both 400-message test assertions updated to the new substring | VERIFIED | `api_test.go:597-599` — `bytes.Contains(respBody, []byte("re-resolve via buf mod update / buf dep update"))`. `uuid_format_test.go:215-217` — `strings.Contains(string(body), "re-resolve via buf mod update / buf dep update")`. Both PASS under `go test -v -run 'TestBadRequest_OnUnknownCommitID|TestServeDownload_UnknownCommitID_ReturnsBadRequest'`. |
| 12  | 17-01: The "unknown commit id" prefix assertions remain valid (untouched) | VERIFIED | `api_test.go:590` and `uuid_format_test.go:209` retain `bytes.Contains(respBody, []byte("unknown commit id"))` and `strings.Contains(string(body), "unknown commit id")` respectively. Both PASS — the new wire body preserves the prefix. |
| 13  | Phase 17: ROADMAP Success Criteria (SC-1, SC-2, SC-3, SC-4, SC-5) are met in the codebase | VERIFIED | SC-1: 2 `errors.Is` dispatch sites + 2 `h.upstreamError` digest calls (verified). SC-2: `internalError` helper gone; 5 call sites use `logHandlerError`/`upstreamError` (verified). SC-3: `commitUUID` accepts 40 + 64 hex; byte table unchanged; new SHA-256 test with 4 cases (verified). SC-4: `preResolveForTest` in test file; `TestCommitUUID_SHA256_KnownSHA` exists in test file (verified). SC-5: wire body + 2 test assertions all use new `re-resolve via` text; old text removed from source (verified — only remaining hit is in `review.md`, the historical record of the finding). |

**Score:** 13/13 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/connect/commits.go` | `internalError` removed; `errCommitUUIDContract` sentinel; 5 call sites refactored; `computeB4Digest` 4-arg signature; softened 400 wire body; updated inline comment | VERIFIED | All 7 expectations met: helper at line 100-106 gone; sentinel at line 94-102; 3 `logHandlerError` direct calls (160, 351, 668) + 2 `errors.Is` dispatch blocks (174-186, 354-366); `computeB4Digest(r, ref, commit, cid)` at line 765; 400 wire body at line 601; inline comment at line 418. |
| `internal/connect/commits_helpers.go` | `commitUUID` accepts 40 or 64 hex; updated doc comment; updated error messages; `preResolveForTest` deleted | VERIFIED | Length check at line 41 (`!= 40 && != 64`); error message at line 42 + 46 (both say "40 or 64 lowercase hex characters"); doc comment at lines 16-37 mentions "40 or 64"; `preResolveForTest` deleted (0 hits). `strings` import retained (used by `strings.SplitN` at line 288). |
| `internal/connect/commits_helpers_test.go` | `TestCommitUUID_SHA256_KnownSHA` with 4 cases; `TestCommitUUID_InvalidInput` extended with 63/65-char; `preResolveForTest` pasted before `TestPreResolveForTest` | VERIFIED | `TestCommitUUID_SHA256_KnownSHA` at line 171-225 (4 subtests); `TestCommitUUID_InvalidInput` at line 227-256 (8 subtests including 63/65-char); `preResolveForTest` definition at line 263-272, `TestPreResolveForTest` at line 275-313. All tests PASS. |
| `internal/connect/api_test.go` | D-12 substring assertion updated to `re-resolve via` | VERIFIED | Line 597-599: `bytes.Contains(respBody, []byte("re-resolve via buf mod update / buf dep update"))` + matching `t.Errorf` text on line 598. `TestBadRequest_OnUnknownCommitID` (4 subtests) PASSES. |
| `internal/connect/uuid_format_test.go` | D-12 substring assertion updated to `re-resolve via` | VERIFIED | Line 215-217: `strings.Contains(string(body), "re-resolve via buf mod update / buf dep update")` + matching `t.Errorf` text on line 216. `TestServeDownload_UnknownCommitID_ReturnsBadRequest` PASSES. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `internal/connect/commits.go:765-780` (computeB4Digest) | `internal/connect/commits_helpers.go:38-58` (commitUUID) | contract-violation wrap | WIRED | `fmt.Errorf("%w: %v", errCommitUUIDContract, uidErr)` at line 778-779. The `%w` keeps `errors.Is` working; the `%v` exposes the inner error string for log lines. |
| `internal/connect/commits.go:174-186, 354-366` | `internal/connect/commits.go:94-102` (sentinel) | `errors.Is(err, errCommitUUIDContract)` dispatch | WIRED | 2 sites dispatch on the sentinel: true → `logHandlerError` 500, false → `upstreamError` 502. Pattern matches `errors.Is(err, errCommitUUIDContract)`. |
| `internal/connect/commits.go:601` (wire body) | `internal/connect/api_test.go:597` + `uuid_format_test.go:215` (assertions) | substring match on `re-resolve via buf mod update / buf dep update` | WIRED | Wire body literal matches the bytes.Contains / strings.Contains argument. Both assertions PASS. |
| `internal/connect/commits_helpers.go:38-58` | `internal/connect/commits_helpers_test.go:171-225` | `TestCommitUUID_SHA256_KnownSHA` exercises the 64-char length check | WIRED | 4 subtests call `commitUUID` with `strings.Repeat("0", 64)`, `strings.Repeat("f", 64)`, `strings.Repeat("0", 36)`-prefixed 28-char input, and `strings.Repeat("0", 36)`-prefixed deadbeef-prefixed input. All PASS. |
| `internal/connect/commits_helpers.go:60-70` (was) | `internal/connect/commits_helpers_test.go:263-272` (now) | `preResolveForTest` moved; 5 callers unchanged | WIRED | Function body, signature, and 5-line doc comment pasted verbatim before `TestPreResolveForTest`. All 5 callers (lines 285, 293, 302, 310, 326) continue to resolve through package-internal visibility. |

### Data-Flow Trace (Level 4)

N/A — Phase 17 is a refactor of pure error-wiring plus a length-check expansion plus a 4-byte string substitution. No new external data flows; the wire body and structured log lines flow through existing `h.badRequest` and `h.logHandlerError` / `h.upstreamError` helpers, and the existing `go test` coverage exercises both happy-path (40-char and 64-char) and contract-violation paths.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| `go build ./...` exits 0 from the repo root | `go build ./...` | exit 0, no output | PASS |
| `go vet ./...` exits 0 from the repo root | `go vet ./...` | exit 0, no output | PASS |
| `go test ./internal/connect/ -count=1` all PASS | `go test ./internal/connect/ -count=1` | `ok github.com/easyp-tech/server/internal/connect 0.276s` | PASS |
| `go test ./... -count=1` all packages pass | `go test ./... -count=1` | connect, artifactory, filter, multisource, reqid all PASS | PASS |
| `TestCommitUUID_SHA256_KnownSHA` all 4 subtests pass | `go test -run TestCommitUUID_SHA256_KnownSHA -v -count=1` | 4/4 PASS in 0.00s | PASS |
| `TestCommitUUID_InvalidInput` all 8 subtests pass (incl. 63/65-char) | `go test -run TestCommitUUID_InvalidInput -v -count=1` | 8/8 PASS in 0.00s | PASS |
| `TestPreResolveForTest` all 5 subtests pass (in new location) | `go test -run TestPreResolveForTest -v -count=1` | 5/5 PASS in 0.00s | PASS |
| `TestBadRequest_OnUnknownCommitID` (4 subtests) and `TestServeDownload_UnknownCommitID_ReturnsBadRequest` pass with new `re-resolve via` message | `go test -run 'TestBadRequest_OnUnknownCommitID\|TestServeDownload_UnknownCommitID_ReturnsBadRequest' -v -count=1` | all PASS, including the new substring in `TestServeDownload_UnknownCommitID_ReturnsBadRequest` log line: `error="unknown commit id: re-resolve via buf mod update / buf dep update"` | PASS |
| `internalError` helper no longer defined | `grep -c 'func (h \*commitServiceHandler) internalError' internal/connect/commits.go` | 0 | PASS |
| `errCommitUUIDContract` sentinel defined | `grep -c 'errCommitUUIDContract = errors.New' internal/connect/commits.go` | 1 | PASS |
| `errors.Is` dispatch in both digest-error sites | `grep -c 'errors.Is(err, errCommitUUIDContract)' internal/connect/commits.go` | 2 | PASS |
| `commitUUID` length check is `!= 40 && != 64` | `grep -n 'len(gitSHA) != 40 && len(gitSHA) != 64' internal/connect/commits_helpers.go` | line 41 | PASS |
| `preResolveForTest` lives in test file only | `grep -c 'func preResolveForTest' internal/connect/commits_helpers.go` (expect 0) and `commits_helpers_test.go` (expect 1) | 0 and 1 | PASS |
| Old `re-run` 400 message gone from `internal/connect/` | `grep -rn 're-run buf mod update / buf dep update' internal/connect/` (excluding review.md) | 0 hits | PASS |
| New `re-resolve via` 400 message in 2 commits.go locations | `grep -c 're-resolve via buf mod update / buf dep update' internal/connect/commits.go` | 2 (wire body + inline comment) | PASS |
| `computeB4Digest` signature takes 4 args with `cid` | `grep -n 'computeB4Digest(r \*http.Request, ref moduleRef, commit, cid string)' internal/connect/commits.go` | line 765 | PASS |

### Lint Spot-Check (informational)

The locally-installed `golangci-lint` is v2.12.2 but the repo's `.golangci.yml` is v1 format (v2 cannot load it). Spot-checks with `gofmt -l`, `gofumpt -d`, `goimports -l`, `wsl`, and `go vet` show all findings on the 5 modified files are pre-existing on `HEAD~5` (the PR #37 baseline). The Phase 17 diff introduced no new lint issues. The pre-existing lint debt (3 gofmt / 3 gofumpt / 3 goimports findings across `commits.go`, `api_test.go`, `uuid_format_test.go`; ~127 wsl findings in `commits.go`) is orthogonal cleanup that should land in a separate `gofmt -w` + `wsl --fix` pass.

### Cross-Phase Impact

- **Phase 16 (commit-id format cutover):** Phase 17 reuses the (string, error) `commitUUID` signature from Phase 16 and extends it to 64-char SHA-256. The format change and the SC-3 length expansion are backward-compatible additions — existing 40-char SHAs continue to mint the same UUIDs.
- **Phase 11–15 (Diagnostic Logging):** Phase 17 restores the v1.3 ERR-05 logging contract that the new `internalError` helper had bypassed. All 5xx handler-level errors now flow through `logHandlerError` with full `server`/`protocol`/`request_id`/`error`/`status`/`error_class` attributes, joinable on `request_id` against the rest of the request's decision-trace lines.

### Verifier Notes

- The phase is small and tightly scoped (1 plan, 4 tasks, 5 files modified, ~50 net lines of production code). All evidence is direct: structural greps, unit-test runs, and full-suite runs.
- The single deferred item (T-17-DEFER, the 32-char hex path on the probe/cache contract) is documented in the plan's `<questions>` and `<threat_model>` sections as an explicit non-action — no code change in this phase. It does not block shipping.
- The "new" 400 wire body text `re-resolve via buf mod update / buf dep update` is a substring of the old `re-run buf mod update / buf dep update`, so any operator searching logs for the old text will find the new lines via the shared `buf mod update / buf dep update` tail. The wire format is wire-compatible with buf clients (still a plain-text 400 body with the same status code).
- One non-trivial design choice: the dispatch on `errors.Is(err, errCommitUUIDContract)` was added at the two `computeB4Digest` call sites (174-186, 354-366) rather than inside `computeB4Digest` itself. This keeps the function returning a single error value and lets the caller decide the right response shape (500 vs 502) based on the cause. Future maintainers should not collapse this back into a single call site without preserving the dispatch.
