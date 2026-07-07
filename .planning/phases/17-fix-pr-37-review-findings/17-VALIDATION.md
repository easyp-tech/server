---
phase: 17
slug: fix-pr-37-review-findings
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-07
---

# Phase 17 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Phase 17 is a tightly-scoped refactor of the v1beta1 commit-id error path. All changes are local to `internal/connect/` and verified by the existing `go test ./internal/connect/...` suite plus 4 structural grep checks (helper deletion, sentinel presence, length-check update, message-text update).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` (package `connect`) |
| **Config file** | None — Go test discovery (`*_test.go` in package) |
| **Quick run command** | `go test ./internal/connect/ -count=1` |
| **Full suite command** | `go build ./... && go vet ./... && go test ./internal/connect/ -count=1` |
| **Estimated runtime** | ~3 seconds (existing suite is ~0.3s; new test adds <1s) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/connect/ -count=1` (full connect-package suite, ~0.3s)
- **After Task 2 commit (new test added):** Run `go test ./internal/connect/ -run 'TestCommitUUID_SHA256_KnownSHA|TestCommitUUID_InvalidInput' -v -count=1` to confirm the new test passes
- **After Task 3 commit (message + assertions updated):** Run `go test ./internal/connect/ -run 'TestBadRequest_OnUnknownCommitID|TestServeDownload_UnknownCommitID_ReturnsBadRequest' -v -count=1` to confirm the 400 path tests pass with the new message
- **After Task 4 commit (function moved):** Run `go test ./internal/connect/ -run 'TestPreResolveForTest' -v -count=1` to confirm all 5 subtests pass in the new location
- **After all 4 tasks complete:** Run `go build ./... && go vet ./... && go test ./internal/connect/ -count=1` (full suite, ~3s) + the structural grep checks
- **Max feedback latency:** ~3 seconds (one full suite run)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 17-01-T1 | 01 | 1 | SC-1, SC-2 | T-17-01, T-17-03 | `computeB4Digest` upstream errors return 502 with full ERR-05 attrs; `internalError` helper gone | unit + structural | `go test ./internal/connect/ -count=1` + `grep -c 'func (h \*commitServiceHandler) internalError' internal/connect/commits.go` (= 0) | ✅ | ⬜ pending |
| 17-01-T2 | 01 | 1 | SC-3, SC-4 (test part) | T-17-01, T-17-04 | `commitUUID` accepts 40 and 64 hex; new SHA-256 test covers 4 cases; extended invalid-input test covers 63/65-char boundaries | unit | `go test ./internal/connect/ -run 'TestCommitUUID_SHA256_KnownSHA|TestCommitUUID_InvalidInput' -v -count=1` | ❌ W0 (new test in T2) | ⬜ pending |
| 17-01-T3 | 01 | 1 | SC-5 | T-17-02 | 400 wire body + 2 test assertions all use the new `re-resolve via` text | unit + structural | `go test ./internal/connect/ -run 'TestBadRequest_OnUnknownCommitID|TestServeDownload_UnknownCommitID_ReturnsBadRequest' -v -count=1` + `grep -rn 're-run buf mod update / buf dep update' .` (= 0 hits) | ✅ | ⬜ pending |
| 17-01-T4 | 01 | 1 | SC-4 (move part) | T-17-06 | `preResolveForTest` lives in `_test.go` only; production binary excludes it | unit + structural | `go test ./internal/connect/ -run 'TestPreResolveForTest' -v -count=1` + `grep -c 'func preResolveForTest' internal/connect/commits_helpers.go` (= 0) + `grep -c 'func preResolveForTest' internal/connect/commits_helpers_test.go` (= 1) | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/connect/commits_helpers_test.go` — add `TestCommitUUID_SHA256_KnownSHA` with 4 subcases (all-zero 64-char, all-ones 64-char, 14-byte-prefix match, all-deadbeef 64-char) — **W0 in Task 2**
- [ ] `internal/connect/commits_helpers_test.go` — extend `TestCommitUUID_InvalidInput` cases slice with 63-char and 65-char entries — **W0 in Task 2**
- [ ] `internal/connect/commits.go` — add `errCommitUUIDContract` sentinel declaration (new package-level var) — **W0 in Task 1**
- [ ] `internal/connect/commits_helpers_test.go` — paste `preResolveForTest` definition (with doc comment) before `TestPreResolveForTest` — **W0 in Task 4**

*Existing infrastructure covers: all other tests (400 path, commitUUID format, inverse recovery, preResolveForTest 5 subtests, full suite).*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| (none) | — | All phase behaviors have automated verification via `go test` and `grep` checks | — |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies (Task 1, 3, 4: pure verification; Task 2: W0 in-task)
- [x] Sampling continuity: every task ends with `go test ./internal/connect/ -count=1`; no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (3 new test additions + 1 new var + 1 moved function, all listed above)
- [x] No watch-mode flags
- [x] Feedback latency < 5s (full suite is ~3s, grep checks are <1s)
- [ ] `nyquist_compliant: true` set in frontmatter (set after Tasks 1-4 each pass their per-task verify)

**Approval:** pending
