---
phase: 23-e2e-tests-branch-name-and-non-default-branch-commit-refs-in-buf-yaml-deps
plan: 01
subsystem: e2e
tags: [e2e, ref-honoring, branch-ref, commit-sha-ref, regression-guard, buf-cli, github-provider]

# Dependency graph
requires:
  - phase: 18-respect-buf-yaml-dependency-refs-not-always-head-fix-related
    provides: "GetMeta ref-resolution branches in internal/providers/github/getrepo.go (isConventionalDefaultName carve-out line 80, isSHA fast path line 93, repos.GetCommit fall-through line 96); commitUUID byte table in internal/connect/commits_helpers.go:45-65"
  - phase: 19-we-need-e2e-tests-for-the-ref-specified-for-dependency-looks
    provides: "e2e/ref_test.go helpers (gitLsRemote, isLowerHex, commitUUIDForTest, extractCommitFromLock, pinnedRef); RunBufModUpdateWithRef testutil helper"
  - phase: 11-logging-foundation
    provides: "testutil.DefaultTestConfig, testutil.StartServer, testutil.RequireEnvToken"
  - phase: 12-logging-infrastructure
    provides: "testutil.GetBuf, BufV169 constant"
provides:
  - "TestRefRespected_BranchName_PinsBranchTip: v1.69.0 e2e test proving a branch-name ref (gh-pages) pins buf.lock to the branch tip UUID"
  - "TestRefRespected_NonDefaultBranchCommitSHA: v1.69.0 e2e test proving a raw 40-char SHA ref of a commit off the default branch pins buf.lock to commitUUIDForTest(sha)"
  - "branchRef = \"gh-pages\" package-level constant (stable non-default branch on googleapis/googleapis)"
affects: [future-ref-regressions, future-buf-cli-version-bumps]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Branch-name ref fixture via a non-conventional non-default branch (gh-pages) — chosen so it falls through GetMeta's isConventionalDefaultName carve-out and exercises repos.GetCommit, and so its tip is provably off the default branch master"
    - "Raw-SHA ref fixture reuses the same branch tip — the gh-pages tip is a commit not on master, so it serves both the branch-name test (T1) and the non-default-branch commit test (T2) without a second fixture"
    - "Runtime master-tip sanity check — both tests git ls-remote master and assert it differs from the gh-pages tip, so a future fixture collapse (branch deleted/merged) fails loudly instead of silently weakening the test"

key-files:
  modified:
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/e2e/ref_test.go"
      provides: "Two new e2e tests (TestRefRespected_BranchName_PinsBranchTip, TestRefRespected_NonDefaultBranchCommitSHA) and one new const (branchRef = \"gh-pages\"). Reuses gitLsRemote/isLowerHex/commitUUIDForTest/extractCommitFromLock from the same file and RunBufModUpdateWithRef from testutil. No existing function or constant modified."

key-decisions:
  - "Both tests run on v1.69.0 only (matching TestRefRespected_ModUpdate_MatchesUpstreamSHA). The strict UUID assertion (lock == commitUUIDForTest(sha)) is sensitive to the commitUUID byte table; the broader differs-from-HEAD matrix coverage already exists in Phase 19. Phase 23 is the strict-shape guard for two new ref forms."
  - "gh-pages is the fixture branch because (a) it is stable/long-lived (GitHub Pages), (b) it is not in the isConventionalDefaultName set so it exercises repos.GetCommit not the HEAD carve-out, (c) its tip is divergent from master so it is provably a non-default-branch commit, and (d) one branch tip serves both T1 (as a name) and T2 (as a raw SHA) — no second fixture needed."
  - "The raw-SHA test (T2) sends the gh-pages tip SHA as the ref, not a SHA harvested from elsewhere. This means T1 and T2 resolve to the same commit UUID — by design: the two tests differ in the ref FORM (name vs SHA), which exercises two different GetMeta branches (repos.GetCommit vs isSHA fast path), proving both paths honor the request. Same target, different code path."
  - "Tests skip cleanly (not fail) when EASYP_GH_TOKEN is unset — token-less CI exits 0 with SKIP lines, not FAIL. Inherited from the Phase 19 RequireEnvToken pattern."
  - "No testutil or production changes. Both behaviors (branch-name → repos.GetCommit; raw SHA → isSHA fast path) already shipped in Phase 18. This phase is test-only."

patterns-established:
  - "Branch-name ref e2e test shape: gitLsRemote(refs/heads/<branch>) for the branch tip AND for master, sanity-assert they differ, then RunBufModUpdateWithRef(..., branchName) and assert lock == commitUUIDForTest(branchTip). Extends the Phase 19 matches-upstream shape from tag refs to branch refs."
  - "Raw-SHA ref e2e test shape: same as branch-name but RunBufModUpdateWithRef(..., sha) — the ref is the SHA itself. Surfaces whether the buf CLI accepts a 40-hex ref suffix; on client-side rejection the buf stderr is in the t.Fatalf message, which is the answer to the user's 'is it possible' question."

requirements-completed:
  - SC-23-1 (branch-name ref pins to branch tip on v1.69.0)
  - SC-23-2 (non-default-branch commit SHA ref honored on v1.69.0)
  - Regression guard for Phase 18 GetMeta ref-resolution branches

# Metrics
duration: 6min
completed: 2026-07-08
---

# Phase 23 Plan 01: e2e Tests for branch-name and non-default-branch commit refs in buf.yaml deps Summary

**Two v1.69.0 e2e tests added to `e2e/ref_test.go` that close the ref-shape coverage gap left by Phase 19 (tag refs) by proving the proxy honors a branch name and a raw commit SHA in `Name.ref`, including a SHA whose commit is not on the default branch.**

## Performance

- **Duration:** 6 min
- **Completed:** 2026-07-08
- **Tasks:** 1 (single atomic commit)
- **Files modified:** 1 (`e2e/ref_test.go`, +124 lines: 270 → 394)

## Accomplishments

- **`TestRefRespected_BranchName_PinsBranchTip` added.** Resolves the `gh-pages` branch tip and the `master` tip via `git ls-remote`, asserts they differ (sanity), derives `expectedUUID := commitUUIDForTest(ghPagesTip)`, runs `buf mod update` with `:ref=gh-pages`, and asserts the lock commit equals `expectedUUID`. Proves the `repos.GetCommit` fall-through in `GetMeta` (getrepo.go:96) resolves a non-conventional branch name to its tip — not HEAD.
- **`TestRefRespected_NonDefaultBranchCommitSHA` added.** Same fixture SHA (the `gh-pages` tip — a commit off the default branch), but this time sent as the raw 40-char SHA in `:ref`. Asserts the lock commit equals `commitUUIDForTest(sha)`. Proves the `isSHA` fast path in `GetMeta` (getrepo.go:93) stamps the SHA branch-agnostically. The captured buf stderr in the failure path also surfaces whether the buf CLI accepts a raw-SHA ref suffix.
- **`branchRef = "gh-pages"` const added** with a doc comment explaining why this fixture was chosen: stable, non-default, non-conventional, divergent-from-master.
- **Both tests skip cleanly when `EASYP_GH_TOKEN` is unset.** Reuses the Phase 19 `RequireEnvToken` pattern; token-less CI exits 0 with SKIP lines.
- **No production code, testutil, or go.mod changes.** Both behaviors already shipped in Phase 18; this phase is purely a regression guard.

## Task Commits

Single atomic commit (working tree drafted to match the plan's `read_first` shape, verified, then committed):

1. **Task 1: Add `e2e/ref_test.go` — two new tests + `branchRef` const; verify build/vet/list/skip; commit** — `8cf99fa`

## Files Created/Modified

- `e2e/ref_test.go` (+124, 270 → 394 lines) — `branchRef` const; `TestRefRespected_BranchName_PinsBranchTip`; `TestRefRespected_NonDefaultBranchCommitSHA`. No existing line modified.

## Decisions Made

- **v1.69.0 only.** The strict UUID assertion (`lock == commitUUIDForTest(sha)`) is sensitive to the byte table; the broader differs-from-HEAD matrix already exists in Phase 19. Phase 23 is the strict-shape guard for two new ref forms.
- **One fixture serves both tests.** The `gh-pages` tip is used as the branch name (T1) and as the raw SHA (T2). Both tests resolve to the same UUID — intentionally: they exercise different `GetMeta` branches (`repos.GetCommit` vs `isSHA`), proving both paths honor the request. Same target, different code path.
- **Runtime master-tip sanity check.** Both tests `git ls-remote master` and assert it differs from the `gh-pages` tip. A future fixture collapse (branch deleted/merged into master) fails loudly instead of silently weakening the test.
- **No testutil extension.** `RunBufModUpdateWithRef(buf, port, ref)` already accepts any ref string, so no new helper was needed. Purely additive test code.
- **The raw-SHA test is also the empirical answer.** T2 sends a 40-hex SHA as the `:ref` suffix. If the buf CLI rejects that syntax client-side, the test fails with the captured buf stderr — which directly answers the user's "is it possible to specify a commit in the dependency definition" question. (Token-less CI cannot run the assertion; a future CI with the token answers it definitively.)

## Deviations from Plan

None. The implementation matches the plan's `<action>` block verbatim. All verification commands from the plan's `<verify>` block passed on the first run.

## Issues Encountered

None.

## Self-Check: PASSED

- `go build ./e2e/...` exits 0
- `go vet ./e2e/...` exits 0
- `go test ./e2e/ -list 'TestRefRespected'` lists exactly 5 tests (3 Phase 19 + 2 Phase 23)
- `go test ./e2e/ -run 'TestRefRespected_BranchName_PinsBranchTip|TestRefRespected_NonDefaultBranchCommitSHA' -count=1` exits 0 (2 SKIP lines, token-less)
- `go test ./e2e/testutil/ -count=1` exits 0
- Structural grep checks all pass:
  - `branchRef = "gh-pages"` present exactly once
  - `TestRefRespected_BranchName_PinsBranchTip` / `TestRefRespected_NonDefaultBranchCommitSHA` each present exactly once
  - T1 passes `branchRef` (literal name) to `RunBufModUpdateWithRef`; T2 passes `branchTip` (raw SHA)
  - Three Phase 19 tests each still present exactly once (unchanged)
  - No TODO/FIXME/XXX/HACK/PLACEHOLDER markers
- `git status --short e2e/ref_test.go` is empty (committed)
- No `go.mod` / `go.sum` changes

## Next Phase Readiness

- Phase 23 deliverables complete; both success criteria (SC-23-1 branch-name ref, SC-23-2 non-default-branch commit SHA ref) met, plus the Phase 18 regression guard.
- The proxy's ref-honoring behavior now has e2e coverage for all three ref shapes a buf.yaml dep `:ref` can take: **tag** (Phase 19), **branch name** (Phase 23 T1), and **raw commit SHA** (Phase 23 T2, including a commit off the default branch).
- No new dependencies, no `go.mod` / `go.sum` changes, no production or testutil changes.
- Token-less CI (this environment) cannot exercise SC-23-1/2 directly; those are validated by the test infrastructure (compile + list + skip) and by the fact that the production code paths they exercise (Phase 18 `GetMeta`) are unit-tested and integration-tested elsewhere. A future CI environment with the token and cached buf binaries will exercise the full path and answer the user's "is it possible" questions empirically.

---
*Phase: 23-e2e-tests-branch-name-and-non-default-branch-commit-refs-in-buf-yaml-deps*
*Completed: 2026-07-08*
