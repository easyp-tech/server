---
phase: 22
plan: 01
subsystem: connect
tags: [v1alpha1, uuid-resolution, commit-map, probe, bugfix]
requires:
  - "Phase 18: commitUUIDInverse / isUUID / probeCommitID ladder (the machinery this plan wires into)"
  - "Phase 21: TestGenerateWithPinnedBufLock e2e gate (the regression that caught the bug)"
provides:
  - "CommitResolver interface on *api (internal/connect/api.go) bridging v1alpha1 handlers to the commit-resolution ladder"
  - "resolveCommitForRead wrapper on *commitServiceHandler (internal/connect/commits.go) for commitMap → resolveForeignCommitID → probeCommitID resolution"
  - "UUID-resolution branch in DownloadManifestAndBlobs (internal/connect/blobs.go) fixing PR-22-1/2/3"
affects:
  - "internal/connect/blobs.go (DownloadManifestAndBlobs handler)"
  - "internal/connect/api.go (*api struct, NewWithConfig)"
  - "internal/connect/commits.go (new resolveCommitForRead method)"
tech-stack:
  added: []
  patterns:
    - "Thin-wrapper resolver delegating to existing ladder (no reimplementation)"
    - "Interface decoupling (*api → CommitResolver → *commitServiceHandler)"
key-files:
  created:
    - internal/connect/blobs_test.go
  modified:
    - internal/connect/blobs.go
    - internal/connect/api.go
    - internal/connect/commits.go
decisions:
  - "Named the interface CommitResolver with unexported method resolveCommitForRead — keeps the interface same-package-only (both *api and *commitServiceHandler live in package connect) and the name matches the Phase 18 ladder terminology"
  - "Added nil-guard at the call site (isUUID(ref) && a.commitResolver != nil) as defense-in-depth per T-22-04, even though NewWithConfig always sets the pointer"
  - "countingProvider wrapper defined in blobs_test.go (not modifying mockProvider in api_test.go) to assert zero GetFiles calls in the resolution-error test"
---

# Phase 22 Plan 01: Fix v1alpha1 Read-Path UUID Handling Summary

Fixed the v1.30.1 v1alpha1 `DownloadManifestAndBlobs` read-path so 32-char buf-issued UUID references are resolved to 40-char git SHAs via the existing Phase 18 ladder (`commitMap` → `resolveForeignCommitID` → `probeCommitID`) before being passed to `provider.GetFiles`.

## What Changed

### Interface chosen

`CommitResolver` with unexported method `resolveCommitForRead(ctx, owner, module, id) (sha, err)` in `internal/connect/api.go`. The method is unexported because both `*api` and `*commitServiceHandler` are in package `connect` — no cross-package implementation needed.

### Exact insertion lines

- **api.go**: `CommitResolver` interface declared after the `provider` interface (line ~23). `commitResolver CommitResolver` field added to `*api` struct. `a.commitResolver = commitHandler` assignment in `NewWithConfig` immediately after the `commitServiceHandler` literal closes (before the `sweepMisses` goroutine launch).
- **commits.go**: `resolveCommitForRead` method on `*commitServiceHandler` inserted between `sweepMisses` (ends ~line 945) and `probeCommitID` (starts ~line 1010 after insertion), so the ladder ordering is visually adjacent.
- **blobs.go**: UUID-resolution branch inserted before the `a.repo.GetFiles` call. Captures `req.Msg.GetReference()` into local `ref`, resolves when `isUUID(ref) && a.commitResolver != nil`, passes the resolved `ref` to `GetFiles`.

### Method body shape

`resolveCommitForRead` mirrors `ServeDownload`'s decision ladder:
1. **commitMap lookup** (RLock → read → RUnlock)
2. **resolveForeignCommitID** (single-module fallback; handles its own locking)
3. **probeCommitID** (if `probeEnabled`; handles UUID inputs via `commitUUIDInverse` + prefix-match internally)
4. **Error return** on total miss — never falls back to HEAD

## Test Results (PR-22-N)

| PR ID | Test | Status | Command |
|-------|------|--------|---------|
| PR-22-1 | `TestDownloadManifestAndBlobs_ResolveUUID` | PASS | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ResolveUUID -count=1` |
| PR-22-2 | `TestDownloadManifestAndBlobs_ProbeFallback` | PASS | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ProbeFallback -count=1` |
| PR-22-3 | `TestDownloadManifestAndBlobs_ResolveError` | PASS | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ResolveError -count=1` |
| PR-22-4 | `TestGenerateWithPinnedBufLock/v1.30.1` | SKIP (no token) | `go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1'` — EASYP_GH_TOKEN not set; test SKIPs cleanly (PASS, no FAIL). A token-bearing run is required before marking the phase fully verified. |
| PR-22-5 | Phase 18 guards (`TestCommitUUIDInverse`, `TestIsUUID`, `TestIsSHA`, `TestProbeCommitID`) | PASS | `go test ./internal/connect/ -run 'TestCommitUUIDInverse|TestIsUUID|TestIsSHA|TestProbeCommitID' -count=1` |

### Full suite

- `go test ./internal/connect/ -count=1` — PASS (all tests green)
- `go test ./... -count=1` — PASS (all packages green)
- `go build ./...` — PASS (exit 0)
- `go vet ./...` — PASS (exit 0)

## Commits

| Task | Commit | Message |
|------|--------|---------|
| 1 (RED) | `f49fa87` | `test(22-01): add failing tests for v1alpha1 UUID resolution` |
| 2 (GREEN) | `71fbfeb` | `fix(22-01): resolve UUID refs in v1alpha1 DownloadManifestAndBlobs` |

## TDD Gate Compliance

- RED gate: `test(22-01)` commit exists (`f49fa87`) — three tests verified failing against the unmodified handler.
- GREEN gate: `fix(22-01)` commit exists (`71fbfeb`) — same three tests now pass.
- No REFACTOR commit needed — the implementation was minimal (thin wrapper, no cleanup required).

## Deviations from Plan

### Worktree base correction (Rule 3 — blocking issue)

- **Found during:** Task 2 build
- **Issue:** The worktree's `worktree-agent-*` branch was based on `8069e17` (Phase 16) instead of `924fda9` (Phase 22 planning tip). Phase 18 helpers (`isUUID`, `commitUUIDInverse`, UUID-aware `probeCommitID`) were missing, causing `go build` to fail with "undefined: isUUID".
- **Fix:** `git reset --hard 924fda9` to the correct base, re-applied the test file, and re-executed Task 2 edits against the Phase 18+ source. Root cause: the `<worktree_branch_check>` merge-base assertion was only partially run at agent startup (HEAD_REF/branch checks passed, but the merge-base reset step was skipped).
- **Commit:** No separate deviation commit — the reset happened before Task 2 and the Task 1/2 commits are on the correct base.

### connect.CodeOK removal (Rule 1 — bug in test)

- **Found during:** Task 1 `go vet`
- **Issue:** `connect.CodeOK` does not exist in `connectrpc.com/connect`. The Connect error codes start at `CodeCanceled`; there is no OK sentinel.
- **Fix:** Removed the `connect.CodeOf(err) == connect.CodeOK` assertion from `TestDownloadManifestAndBlobs_ResolveError`. The load-bearing assertion is `err != nil` (handler surfaced a real error) plus `getFilesCalls == 0` (provider never touched).

## Known Stubs

None — all code paths are fully wired to real resolution logic.

## Threat Flags

None — no security-relevant surface beyond what the plan's threat model already covers (T-22-01 through T-22-05 are all mitigated by the implementation).

## Self-Check: PASSED

- `internal/connect/blobs_test.go` — FOUND (created)
- `internal/connect/blobs.go` — FOUND (modified, isUUID branch present)
- `internal/connect/api.go` — FOUND (modified, CommitResolver interface + field + assignment)
- `internal/connect/commits.go` — FOUND (modified, resolveCommitForRead method present)
- Commit `f49fa87` — FOUND in git log
- Commit `71fbfeb` — FOUND in git log
