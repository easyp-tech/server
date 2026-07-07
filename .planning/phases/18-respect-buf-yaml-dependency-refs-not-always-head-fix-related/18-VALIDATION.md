---
phase: 18
slug: respect-buf-yaml-dependency-refs-not-always-head-fix-related
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-07
---

# Phase 18 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Phase 18 is a tightly-scoped bugfix + refactor: (1) honor `Name.ref` end-to-end, (2) replace the `commit != "" && commit != "main"` short-circuit in both providers with an `isSHA` gate, (3) add a `commitUUIDInverse` helper, (4) drop the prewarm startup fan-out. All changes are local to `internal/connect/`, `internal/providers/{bitbucket,github}/`, and `cmd/easyp/`. Verification is the existing `go test ./...` suite plus the new tests added in each task and 15 structural grep checks.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` (packages `connect`, `bitbucket`, `github`) |
| **Config file** | None — Go test discovery (`*_test.go` in package) |
| **Quick run command** | `go test ./internal/connect/ -count=1` |
| **Full suite command** | `go build ./... && go vet ./... && go test ./internal/connect/ -count=1 && go test ./internal/providers/bitbucket/ -count=1 && go test ./internal/providers/github/ -count=1` |
| **Estimated runtime** | ~5 seconds (existing suite is ~3s; new tests add <2s) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/connect/ -count=1` (full connect-package suite, ~3s)
- **After Task 1 commit (ref parsing + provider ref-resolution):** Run `go test ./internal/connect/ -run 'TestIsSHA|TestIsUUID|TestParseResourceRefName' -v -count=1` AND `go test ./internal/providers/bitbucket/ -run 'TestGetMeta' -v -count=1` AND `go test ./internal/providers/github/ -run 'TestGetMeta' -v -count=1` to confirm the new tests pass
- **After Task 2 commit (inverse helper):** Run `go test ./internal/connect/ -run 'TestCommitUUIDInverse' -v -count=1` to confirm the 8 subtests pass; also run `go test ./internal/connect/ -run 'TestCommitUUID' -v -count=1` to confirm no regression in the existing `commitUUID` tests
- **After Task 3 commit (prewarm removal + probe rewrite):** Run `go test ./internal/connect/ -run 'TestServeDownload_AfterRestart' -v -count=1` to confirm the new integration test passes; also run `go build ./...` to confirm `cmd/easyp/main.go` and `cmd/easyp/internal/config/config.go` are still clean after the prewarm removal
- **After all 3 tasks complete:** Run `go build ./... && go vet ./... && go test ./internal/connect/ -count=1 && go test ./internal/providers/bitbucket/ -count=1 && go test ./internal/providers/github/ -count=1` (full suite, ~5s) + the 15 structural grep checks
- **Max feedback latency:** ~5 seconds (one full suite run)

---

## Per-Task Verification Map

### Task 1 — Refs respected + provider ref-resolution

| Test | What it proves |
|------|----------------|
| `TestIsSHA` (6 subtests) | `isSHA` correctly accepts 40/64-char hex, rejects everything else |
| `TestIsUUID` (4 subtests) | `isUUID` correctly accepts 32-char hex, rejects everything else |
| `TestParseResourceRefName_ReadsRef` | proto field 3 (`ref`) is captured into `moduleRef.ref` |
| `TestParseResourceRefName_NoRef` | missing ref field is tolerated (old clients still work) |
| `bitbucket.TestGetMeta_Empty` | empty input still returns HEAD (regression guard) |
| `bitbucket.TestGetMeta_RawSHA_40` | 40-char SHA still bypasses ref-resolution (regression guard) |
| `bitbucket.TestGetMeta_RawSHA_64` | 64-char SHA still bypasses ref-resolution (regression guard) |
| `bitbucket.TestGetMeta_ResolvesRef` | non-SHA input routes to `/commits/{ref}` and gets the resolved SHA |
| `github.TestGetMeta_Empty` | empty input still returns HEAD (regression guard) |
| `github.TestGetMeta_RawSHA_40` | 40-char SHA still bypasses ref-resolution (regression guard) |
| `github.TestGetMeta_ResolvesRef` | non-SHA input routes to `repos.GetCommit` and gets the resolved SHA |

### Task 2 — `commitUUIDInverse` helper

| Test | What it proves |
|------|----------------|
| `TestCommitUUIDInverse` (8 subtests) | round-trip with 40-char and 64-char SHAs; error on empty, length-mismatched, and non-hex inputs |

### Task 3 — Prewarm removal + probe rewrite

| Test | What it proves |
|------|----------------|
| `TestServeDownload_AfterRestart_ProbeResolvesUUID` | a fresh handler with empty `commitMap` correctly recovers the module from a UUID via the probe path |
| `TestServeDownload_AfterRestart_ProbeMissesOnUnknownUUID` | a UUID whose first 14 bytes do not correspond to any source's SHA is correctly treated as a miss (400) |

### Phase 16/17 Regression Guards

| Test | What it proves |
|------|----------------|
| `TestServeHTTP_GetCommits_ReturnsDashlessUUID` | buf v1.69.0 wire format still works |
| `TestServeDownload_RoundTripWithMintedUUID` | UUID-pinned Download still works |
| `TestServeDownload_UnknownCommitID_ReturnsBadRequest` | 400 path still works |
| `TestCommitUUID_KnownSHA` | `commitUUID` byte-table still produces expected UUIDs |
| `TestCommitUUID_SHA256_KnownSHA` | 64-char SHA-256 inputs still work |
| `TestCommitUUID_InvalidInput` | 40/64-char gate still rejects bad inputs |

---

## Structural Grep Checks (15 checks)

All must pass at the end of the phase:

| Check | Expected | What it proves |
|-------|----------|----------------|
| `grep -rn 'prewarmHeads' .` | 0 hits | prewarmHeads is gone |
| `grep -rn 'registerResolved$' .` | 0 hits | old registerResolved is gone |
| `grep -rn 'PrewarmEnabled\|prewarmEnabled\|PrewarmConfig\|prewarmOnce\|prewarmTimeout' .` | 0 hits | all prewarm config is gone |
| `grep -c 'func commitUUIDInverse' internal/connect/commits_helpers.go` | 1 | inverse helper exists |
| `grep -c 'func (h \*commitServiceHandler) registerResolvedAlias' internal/connect/commits.go` | 1 | new alias helper exists |
| `grep -c 'commitUUIDInverse' internal/connect/commits.go` | >= 1 | probe uses the inverse |
| `grep -c 'strings.HasPrefix(meta.Commit, probeArg)' internal/connect/commits.go` | 1 | prefix-match validation exists |
| `grep -c 'isSHA(commit)' internal/providers/bitbucket/getrepo.go` | >= 1 | bitbucket uses the gate |
| `grep -c 'isSHA(commit)' internal/providers/github/getrepo.go` | >= 1 | github uses the gate |
| `grep -c 'isUUID' internal/connect/commits.go` | >= 2 | probe uses the UUID check |
| `grep -c 'num == 3 && typ == protowire.BytesType' internal/connect/commits_helpers.go` | 1 | ref field is parsed |
| `grep -c 'GetMeta(r.Context(), ref.owner, ref.module, ref.ref)' internal/connect/commits.go` | 2 | ServeHTTP and ServeGraph pass the ref |
| `grep -c 'Prewarm' cmd/easyp/main.go` | 0 | prewarm gone from main.go |
| `grep -c 'Prewarm' cmd/easyp/internal/config/config.go` | 0 | prewarm gone from config.go |
| `grep -c 'commit != "" && commit != "main"' internal/providers/{bitbucket,github}/getrepo.go` | 0 each | old short-circuit gone |

---

## Coverage Matrix (Dimension 8: Validation Coverage)

| SC | What it claims | What test proves it | What prevents silent breakage |
|----|----------------|---------------------|-------------------------------|
| SC-1 (refs respected) | `GetCommits` returns UUID from ref | `TestParseResourceRefName_ReadsRef` + `TestGetMeta_ResolvesRef` (both providers) | The provider mock returns a different SHA for HEAD vs the ref input; the test asserts the UUID matches the ref-resolved SHA, not HEAD |
| SC-2 (related bug fixed) | non-SHA inputs route through ref-resolution | `TestGetMeta_ResolvesRef` (both providers) | The provider mock returns a 404 for non-SHA inputs that don't match a real ref; the test asserts the function returns an error, not a silent fallthrough to HEAD |
| SC-3 (prewarm removed) | `prewarmHeads` / `registerResolved` are gone | Structural grep + `go build ./...` clean | The 7 structural grep checks (`prewarmHeads`, `registerResolved`, `PrewarmConfig`, etc.) must all return 0 hits; `go build ./...` is the final compile-time guard |
| SC-4 (inverse helper) | `commitUUIDInverse` recovers the 28-char SHA prefix | `TestCommitUUIDInverse` (8 subtests) | The 4 success subtests use real `commitUUID(sha)` calls (not hardcoded UUIDs) so a future change to `commitUUID`'s byte-table automatically propagates |
| SC-5 (no regression) | Phase 16/17 tests still pass | `go test ./internal/connect/ -count=1` + 6 targeted tests | The full connect-package suite is the regression guard; the 6 targeted tests are the named-property guards (UUID wire format, SHA-256 path, etc.) |

---

## Known Gaps

- The `mockProvider` in `api_test.go` is constructed at test setup with a single configured source. The new `TestServeDownload_AfterRestart_*` tests rely on this single-source shape. A multi-source test (verifying that the probe correctly identifies the right source among several) is a future phase, not in this one.
- The `isSHA` / `isUUID` helpers are simple linear scans. For a 64-char SHA-256 input, this is 64 character comparisons. A future optimization could use a precomputed lookup table, but the linear scan is fast enough for the proxy's request rate (~hundreds of requests per second at most).
- The Bitbucket `/commits/{ref}` endpoint is documented to accept short SHAs (>= 7 chars) in addition to full SHAs and ref names. The new `getCommit` helper does not test the short-SHA case explicitly (the existing `TestGetMeta_RawSHA_40` covers the full-SHA case). A future test could add a 7-char short-SHA case.

---

## Rollback Plan

If the prewarm removal causes a regression in production, the rollback is:

1. Revert the commit that deletes `prewarmHeads` and `registerResolved`.
2. Revert the `probeCommitID` rewrite to its pre-task-3 shape.
3. Re-add the `PrewarmConfig` block in `config.go`.
4. Re-add the `PrewarmEnabled` / `PrewarmTimeout` fields in `CommitResolution` and the goroutine launch in `api.go:111-113`.

The prewarm removal is a single atomic commit (Task 3 is one `<task>` block, but it can be split into Task 3a "probe rewrite" and Task 3b "prewarm deletion" if a more granular rollback is desired — for the initial plan, the atomic version is fine).

The ref-respect changes (Task 1) and the inverse helper (Task 2) are independent of the prewarm removal. If only those need to be rolled back, the rollback is the inverse of Task 1 + Task 2.
