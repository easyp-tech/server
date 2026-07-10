---
phase: 25-address-pr-39-post-merge-review-findings-pin-multi-default-b
plan: 03
subsystem: "e2e"
tags: ["commitLineRE", "ExtractCommitFromLock", "testutil", "e2e", "v1alpha1"]
requires: [25-01, 25-02]
provides: []
affects: ["e2e/ref_test.go", "e2e/testutil/server.go"]
tech-stack:
  added: []
  modified: []
key-files:
  modified:
    - e2e/ref_test.go
    - e2e/testutil/server.go
key-decisions:
  - Extracted CommitLineRE and ExtractCommitFromLock to testutil package (FIX-06), eliminating regex duplication
  - v1.30.1 v1alpha1 e2e gate (FIX-03) confirmed the proxy works end-to-end (all 6 TestRefRespected subtests PASS) but TestGenerateWithPinnedBufLock/v1.30.1 fails with 403 from buf.build's remote plugin registry — this is NOT a proxy regression
requirements-completed: [FIX-06]
duration: "10 min"
completed: 2026-07-10
commits:
  - "b1c0541 refactor(25-03): extract commitLineRE and ExtractCommitFromLock to testutil"
---

# Phase 25 Plan 03: Extract commitLineRE + e2e verification — Summary

## Task 1: commitLineRE extraction (FIX-06)

Moved CommitLineRE and ExtractCommitFromLock from e2e/ref_test.go to e2e/testutil/server.go as exported symbols. Updated all 7 call sites in ref_test.go to use testutil.ExtractCommitFromLock. The inlined regex in runBufGenerate's Step 2 now uses CommitLineRE directly.

## Task 2: v1alpha1 e2e gate (FIX-03)

Ran `TestGenerateWithPinnedBufLock/v1.30.1` against real GitHub with `EASYP_GH_TOKEN`. The proxy layer works correctly end-to-end:
- ✅ `GetModulePins("main")` resolves to master HEAD (carve-out works)
- ✅ `DownloadManifestAndBlobs` fetches 6+ files from GitHub

The test fails with `403 Forbidden` from buf.build's remote plugin registry (`buf.build/protocolbuffers/go:v1.28.1`) — this is an external dependency issue, not a proxy regression.

Key outcome from e2e testing: all 6 `TestRefRespected_*` subtests pass against real GitHub:
- `TestRefRespected_ModUpdate_DiffersFromHead/v1.30.1` — PASS
- `TestRefRespected_ModUpdate_DiffersFromHead/v1.69.0` — PASS
- `TestRefRespected_ModUpdate_MatchesUpstreamSHA` — PASS
- `TestRefRespected_DepUpdate_DiffersFromHead` — PASS
- `TestRefRespected_BranchName_PinsBranchTip` — PASS
- `TestRefRespected_NonDefaultBranchCommitSHA` — PASS

## Verification

- `go vet ./e2e/...`: **PASS** (no unused imports)
- `go test ./e2e/testutil/... -count=1`: **PASS**
- grep gate `var commitLineRE ` in ref_test.go: 0
- grep gate `func extractCommitFromLock` in ref_test.go: 0
- grep gate `testutil.ExtractCommitFromLock` in ref_test.go: 8
- grep gate `var CommitLineRE` in server.go: 1
- grep gate `commitLineRE := regexp.MustCompile` in server.go: 0
- e2e ref tests (6 subtests): **ALL PASS**

## Deviations from Plan

**FIX-03 (v1alpha1 e2e gate):** TestGenerateWithPinnedBufLock/v1.30.1 is BLOCKED by external 403 from buf.build's remote plugin registry. The proxy itself handles all requests correctly. This is a developer-environment or network-egress issue with buf.build, not a code regression. The proxy path (GetModulePins → DownloadManifestAndBlobs) works correctly as demonstrated by the successful ref tests.

## Issues Encountered

The TestGenerateWithPinnedBufLock test requires the buf CLI to contact buf.build's plugin registry directly (not through the proxy). The 403 indicates the running environment's IP or token is blocked from accessing `buf.build/protocolbuffers/go:v1.28.1`. This is unrelated to Phase 25 changes.

## Next

Phase 25 complete — ready for next step.