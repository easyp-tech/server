---
phase: 22-fix-v1-30-1-v1alpha1-read-path-uuid-handling-verify-v1-proto
verified: 2026-07-08T15:35:00Z
status: human_needed
score: 4/5 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Run TestGenerateWithPinnedBufLock/v1.30.1 with EASYP_GH_TOKEN set (token-bearing live GitHub gate)"
    expected: "buf v1.30.1 generate exits 0 and produces gen/go/google/type/*.pb.go containing 'package google.type' (PR-22-4)"
    why_human: "E2E test SKIPs cleanly when EASYP_GH_TOKEN is unset (current run condition). Requires live GitHub api.github.com egress + a personal access token. Cannot be exercised programmatically without credentials."
---

# Phase 22: Fix v1.30.1 v1alpha1 Read-Path UUID Handling Verification Report

**Phase Goal:** Fix the v1.30.1 v1alpha1 read-path so that a 32-char buf-issued UUID carried in DownloadManifestAndBlobs is resolved to its 40-char git SHA (via the existing Phase 18 probeCommitID / commitUUIDInverse ladder) BEFORE being passed to provider.GetFiles / github.GetFiles / git.GetTree, then verify the v1 protocol works by re-running the Phase 21 TestGenerateWithPinnedBufLock/v1.30.1 e2e gate.
**Verified:** 2026-07-08T15:35:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth                                                                                                                                   | Status     | Evidence                                                                                                                                                                                                                                                                |
| --- | --------------------------------------------------------------------------------------------------------------------------------------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | A v1alpha1 DownloadManifestAndBlobs request whose reference is a 32-char buf-issued UUID is resolved to the 40-char git SHA before the handler calls provider.GetFiles (PR-22-1) | ✓ VERIFIED | `blobs.go:33-41` contains `if isUUID(ref) && a.commitResolver != nil { resolved, err := a.commitResolver.resolveCommitForRead(...); ref = resolved }` before `a.repo.GetFiles` at line 43. `TestDownloadManifestAndBlobs_ResolveUUID` PASSes (exit 0).                |
| 2   | On a commitMap miss for a UUID input, the resolution reaches probeCommitID (Phase 18 path), not just the in-session cache (PR-22-2)     | ✓ VERIFIED | `commits.go:986-998` step (c) calls `h.probeCommitID(ctx, id)` after commitMap and resolveForeignCommitID miss. `TestDownloadManifestAndBlobs_ProbeFallback` PASSes with `sourceCalls.Load() >= 1` assertion (proves probe fan-out ran).                                 |
| 3   | When the UUID cannot be resolved, DownloadManifestAndBlobs returns a Connect error and does NOT fall back to HEAD or serve wrong content (PR-22-3) | ✓ VERIFIED | `commits.go:1000-1001` returns `fmt.Errorf("commit id %q not resolved by any configured source", id)` on total miss. `blobs.go:37-39` wraps via `asConnectError`. `TestDownloadManifestAndBlobs_ResolveError` PASSes with `getFilesCalls == 0` assertion (no HEAD fb). |
| 4   | The v1.30.1 subtest of TestGenerateWithPinnedBufLock passes against the live proxy with EASYP_GH_TOKEN set (PR-22-4)                     | ? UNCERTAIN | E2E test SKIPs cleanly without EASYP_GH_TOKEN (verified: "EASYP_GH_TOKEN ... not set -- skipping test"). Unit-level evidence (PR-22-1/2/3) proves the handler-level fix; the e2e gate requires a token-bearing run. Routed to human verification.                       |
| 5   | The existing commitUUIDInverse / isUUID / isSHA helpers and their tests are unchanged (PR-22-5)                                         | ✓ VERIFIED | `commits_helpers.go` still defines `commitUUID` (L45), `isSHA` (L72), `isUUID` (L87), `commitUUIDInverse` (L116). Phase 18 guard suite `TestCommitUUIDInverse|TestIsUUID|TestIsSHA|TestProbeCommitID` PASSes. Diff shows ServeDownload NOT refactored.                   |

**Score:** 4/5 truths verified (5th requires live-token run)

### Required Artifacts

| Artifact                          | Expected                                                                        | Status    | Details                                                                                                                                                                                              |
| --------------------------------- | ------------------------------------------------------------------------------- | --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/connect/blobs_test.go`  | Unit tests TestDownloadManifestAndBlobs_ResolveUUID/_ProbeFallback/_ResolveError | ✓ VERIFIED | File exists; `grep -c 'func TestDownloadManifestAndBlobs_'` returns 3. All three tests PASS (run independently).                                                                                     |
| `internal/connect/blobs.go`       | DownloadManifestAndBlobs with UUID-resolution branch before a.repo.GetFiles      | ✓ VERIFIED | `isUUID(ref) && a.commitResolver != nil` branch at L33; calls `resolveCommitForRead` at L34; resolved `ref` passed to `a.repo.GetFiles` at L43.                                                     |
| `internal/connect/api.go`         | CommitResolver interface + commitResolver field + NewWithConfig assignment       | ✓ VERIFIED | Interface declared at L32-34; `commitResolver CommitResolver` field on `*api` at L56; `a.commitResolver = commitHandler` assignment at L123.                                                        |
| `internal/connect/commits.go`     | resolveCommitForRead wrapper reusing commitMap/resolveForeignCommitID/probeCommitID | ✓ VERIFIED | Method at L961-1002; reuses ladder verbatim (commitMap L966 → resolveForeignCommitID L977 → probeCommitID L990); returns error on total miss L1001; does NOT reimplement probeCommitID internals. |

### Key Link Verification

| From                                                | To                                                  | Via                                                                | Status  | Details                                                                                                                                  |
| --------------------------------------------------- | --------------------------------------------------- | ------------------------------------------------------------------ | ------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| blobs.go::DownloadManifestAndBlobs                  | commits.go::resolveCommitForRead                    | `a.commitResolver.resolveCommitForRead` when isUUID(reference)     | ✓ WIRED | `blobs.go:33-36` invokes `a.commitResolver.resolveCommitForRead(ctx, owner, repo, ref)`. Test PASS proves end-to-end.                    |
| api.go::NewWithConfig                               | commits.go::commitServiceHandler                    | `a.commitResolver = commitHandler` after commitHandler constructed | ✓ WIRED | `api.go:123` — single-line assignment after commitHandler literal closes (L106-118). Guard nil-check at call site (blobs.go:33).         |
| commits.go::resolveCommitForRead                    | commits.go::probeCommitID                           | delegates UUID case to Phase 18 probe                              | ✓ WIRED | `commits.go:990` — `h.probeCommitID(ctx, id)` invoked under `h.probeEnabled` gate. probeCommitID unchanged at L1040.                     |

### Data-Flow Trace (Level 4)

| Artifact                      | Data Variable | Source                                                     | Produces Real Data | Status    |
| ----------------------------- | ------------- | ---------------------------------------------------------- | ------------------ | --------- |
| blobs.go::DownloadManifestAndBlobs | `ref`         | req.Msg.GetReference() → resolveCommitForRead → GetFiles   | Yes                | ✓ FLOWING |
| commits.go::resolveCommitForRead  | `info.commit` | h.infoCache[owner/module].commit written by probeCommitID  | Yes                | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior                                                                | Command                                                                                                                                                          | Result                                                    | Status |
| ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | ------ |
| PR-22-1: warm-cache UUID resolves to SHA, GetFiles returns content      | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ResolveUUID -count=1`                                                                            | PASS (exit 0; fixture uuid=f00dcafe000040008000000000000000) | ✓ PASS |
| PR-22-2: cold-cache UUID reaches probeCommitID (sourceCalls > 0)        | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ProbeFallback -count=1`                                                                          | PASS (exit 0; fixture uuid=81353411f7b0401080d5b9ebeb189906) | ✓ PASS |
| PR-22-3: unresolvable UUID surfaces Connect error, GetFiles never called | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ResolveError -count=1`                                                                           | PASS (exit 0; getFilesCalls == 0)                         | ✓ PASS |
| PR-22-5: Phase 18 guards unchanged                                      | `go test ./internal/connect/ -run 'TestCommitUUIDInverse\|TestIsUUID\|TestIsSHA\|TestProbeCommitID' -count=1`                                                   | PASS (exit 0)                                             | ✓ PASS |
| Full internal/connect suite                                             | `go test ./internal/connect/ -count=1`                                                                                                                          | PASS (exit 0; 0.324s)                                     | ✓ PASS |
| Build clean                                                            | `go build ./...`                                                                                                                                                | exit 0                                                    | ✓ PASS |
| Vet clean                                                              | `go vet ./...`                                                                                                                                                  | exit 0                                                    | ✓ PASS |
| PR-22-4 e2e gate (no token)                                            | `go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1`                                                                                          | SKIP ("EASYP_GH_TOKEN ... not set -- skipping test")      | ? SKIP |

### Probe Execution

Not applicable — this phase has no `scripts/*/tests/probe-*.sh` probes; verification uses Go test commands directly.

### Requirements Coverage

| Requirement | Source Plan | Description                                                                          | Status      | Evidence                                                                                                                                |
| ----------- | ----------- | ------------------------------------------------------------------------------------ | ----------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| PR-22-1     | 22-01-PLAN  | UUID resolved to SHA before GetFiles                                                 | ✓ SATISFIED | blobs.go:33-41 + TestDownloadManifestAndBlobs_ResolveUUID PASS                                                                          |
| PR-22-2     | 22-01-PLAN  | Cache-miss UUID reaches probeCommitID                                                | ✓ SATISFIED | commits.go:989-998 + TestDownloadManifestAndBlobs_ProbeFallback PASS (sourceCalls > 0)                                                  |
| PR-22-3     | 22-01-PLAN  | Resolution failure surfaces Connect error, no HEAD fallback                          | ✓ SATISFIED | commits.go:1001 + blobs.go:37-39 + TestDownloadManifestAndBlobs_ResolveError PASS (getFilesCalls == 0)                                  |
| PR-22-4     | 22-01-PLAN  | v1.30.1 TestGenerateWithPinnedBufLock e2e gate passes with EASYP_GH_TOKEN            | ? NEEDS HUMAN | Test SKIPs cleanly without token (current run condition). Unit tests PR-22-1/2/3 prove the handler-level fix; live gate awaits token.   |
| PR-22-5     | 22-01-PLAN  | Phase 18 helpers (commitUUIDInverse / isUUID / isSHA) unchanged                      | ✓ SATISFIED | commits_helpers.go unchanged; TestCommitUUIDInverse/TestIsUUID/TestIsSHA/TestProbeCommitID PASS; ServeDownload NOT refactored (additive) |

**REQUIREMENTS.md traceability note:** PR-22-* IDs are NOT registered in `.planning/REQUIREMENTS.md` (which only formally tracks Phase 11-15 IDs). This matches the established pattern for later phases (Phase 19 SC-19-*, Phase 21 SC-21-*) whose requirement IDs are derived in ROADMAP.md + PLAN frontmatter. Not an orphaned-requirement gap — the ROADMAP explicitly authorizes the IDs ("derived from RESEARCH.md Test Map; see 22-01-PLAN.md").

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER markers found in phase-modified files | ℹ️ Info | No debt markers introduced |

**Notes from 22-REVIEW.md (informational, not blockers):**
- CR-01 (registerResolvedAlias stale digest): pre-existing Phase 18 behavior; not introduced by this phase. ServeDownload has the same pattern. Out of scope.
- WR-01..WR-06: edge-case concerns (multi-module scoping, CodeInternal vs CodeNotFound mapping, dead nil-guard, etc.). None block the phase goal (single-module deployment is the tested topology; the v1.30.1 gate exercises exactly one module). Surface as follow-up considerations for a future hardening phase.

### Human Verification Required

### 1. Live E2E Regression Gate (PR-22-4)

**Test:** Set `EASYP_GH_TOKEN=<github-read-public-repos-token>` and run `go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1 -v`
**Expected:** buf v1.30.1 client runs `buf generate` against the proxy with `buf.lock` pinning a non-HEAD UUID; buf generate exits 0; generated `gen/go/google/type/*.pb.go` exists and contains the string `package google.type`.
**Why human:** The test SKIPs cleanly without the token (no FAIL) — confirmed by direct run: "EASYP_GH_TOKEN ... not set -- skipping test". Exercising the live gate requires GitHub api.github.com egress and a personal access token that only the operator can provide. The unit-level wiring (PR-22-1/2/3) is GREEN, so the handler-level fix is proven; the e2e is the end-to-end confirmation.

### Gaps Summary

No code-level gaps. All four unit-level truths (PR-22-1, PR-22-2, PR-22-3, PR-22-5) are VERIFIED against the actual codebase with passing tests, wired artifacts, and traced data flow. Build and vet are clean; full `internal/connect` suite is green; ServeDownload was NOT refactored (diff is purely additive).

The single UNCERTAIN item is PR-22-4, the live e2e regression gate, which SKIPs cleanly without `EASYP_GH_TOKEN`. Per the phase's explicit design (RESEARCH.md Environment Availability + PLAN user_setup), this is the expected run condition without a token — NOT a code defect. The handler-level fix that the e2e is meant to validate is independently proven by PR-22-1/2/3 unit tests. A token-bearing run is required before marking the phase fully complete.

---

_Verified: 2026-07-08T15:35:00Z_
_Verifier: Claude (gsd-verifier)_
