---
gsd_state_version: 1.0
milestone: v1.3
milestone_name: Diagnostic Logging — In Progress
status: executing
last_updated: "2026-07-10T07:43:08.135Z"
last_activity: 2026-07-10 -- Phase 25 planning complete
progress:
  total_phases: 15
  completed_phases: 14
  total_plans: 20
  completed_plans: 17
  percent: 85
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-10)

**Core value:** The proxy must correctly serve both old (v1.30.1) and modern (v1.69.0+) Buf CLI clients simultaneously

**Current focus:** Phase 24 — resolve-buf-cid-ref-in-servegraph-honor-pinned-commit

## Current Position

Phase: 24 (resolve-buf-cid-ref-in-servegraph-honor-pinned-commit) — EXECUTING
Plan: 1 of 1
Status: Ready to execute
Last activity: 2026-07-10 -- Phase 25 planning complete

Progress: [█████████░] 94%

## Performance Metrics

**Velocity:**

- Total plans completed: 6 (this milestone)
- Average duration: ~10 min
- Total execution time: ~50 min

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 16    | 3     | -     | -        |
| 17    | 1     | -     | 8 min    |
| 18    | 1     | -     | 18 min   |
| 22 | 1 | - | - |

**Recent Trend:**

- (milestone just started)

## Accumulated Context

### Decisions

Decision log maintained in PROJECT.md Key Decisions table.

Recent decisions affecting current work:

- [Roadmap]: 5 phases for v1.3, numbered 11-15 (continuing from v1.2)
- [Roadmap]: Phase ordering follows dependency chain — Foundation before Infrastructure before handler logging
- [Roadmap]: OPS-01 (panic recovery) placed in its own phase since it's a distinct infrastructure concern with no handler-level dependency
- [Roadmap]: Phase 16 (Commit ID Resolution Improvements) added 2026-07-06 — three items bundled into one phase: drop SHA-256 derivation in favor of first-16-bytes of git SHA (incl. short-sha support), probe all configured repos on cache miss, clearer not-found error response and log

### Pending Todos

None yet.

### Blockers/Concerns

None yet.

### Roadmap Evolution

- Phase 16 added: предлагаю изменения — use first 16 bytes of git commit id, probe all repos on miss, fix unclear not-found error message
- Phase 17 added: Fix PR #37 review findings — address pre-merge issues from Phase 16 PR: re-route digest errors through `logHandlerError`/`upstreamError` (not `internalError`), remove `internalError` helper that bypassed `ERR-05`, accept SHA-256 Bitbucket commits (regression at commits_helpers.go:39), move `preResolveForTest` to a `_test.go` file
- Phase 18 added: respect buf.yaml dependency refs (not always HEAD); fix related bug; remove prewarm logic now that buf id → git id is derivable
- Phase 18 planned: 1 plan (18-01-PLAN.md) with 3 tasks — (1) honor `Name.ref` end-to-end + add `isSHA`/`isUUID`/`commitUUIDInverse` helpers + provider ref-resolution; (2) add `commitUUIDInverse` + table-driven tests; (3) delete `prewarmHeads`/`registerResolved`/`PrewarmConfig` and rewrite `probeCommitID` to use the inverse for 32-char UUID inputs. See `.planning/phases/18-respect-buf-yaml-dependency-refs-not-always-head-fix-related/18-{RESEARCH,01-PLAN,VALIDATION}.md`
- Phase 19 added: e2e tests for the `ref` specified for dependency — `looks like it does not work`. Phase 18 introduced end-to-end honoring of `Name.ref` in `buf.yaml` dependencies (plus `commitUUIDInverse` for 32-char UUID inputs); need a real-server e2e test that exercises the path where the `ref` resolves through buf cache/git to prove it works (or surfaces the bug). See `e2e/ref_test.go` and `.planning/phases/19-we-need-e2e-tests-for-the-ref-specified-for-dependency-looks/`.
- Phase 19 executed: e2e tests committed in `e15927b feat(19-01): add e2e tests for ref-honoring in buf.yaml deps`. Tests pass token-less (skip cleanly). With `EASYP_GH_TOKEN` set, tests revealed 2 real proxy regressions: (a) v1.30.1 no-ref path tries to resolve `"main"` as a ref → GitHub 422 (also breaks the pre-existing `TestSmokeBufModUpdate`); (b) v1.69.0 ref-pinned run returns HEAD's SHA instead of the ref's SHA on both `buf mod update` and `buf dep update`. The e2e tests successfully caught the regression; the proxy fixes are deferred to Phase 20.
- Phase 20 added: Fix ref-honoring regressions discovered in Phase 19 — restore the no-ref (HEAD) path for buf v1.30.1 and fix the v1beta1 path where the ref is being ignored on `buf dep update`. See `.planning/phases/20-fix-the-problem-with-refs-discovered-on-phase-19/`.
- Phase 21 added: e2e test for `buf generate` with `buf.lock` pointing to a valid-but-not-latest commit. This is a valid situation (the lock is intentionally pinned to an older commit) and should work with both v1 and v2 buf CLIs. See `.planning/phases/21-we-need-another-e2e-test-we-are-doing-buf-generate-with-buf-`.
- Phase 21 executed: `TestGenerateWithPinnedBufLock` committed in `8df1f54 test(21-01): add TestGenerateWithPinnedBufLock matrix test`. With `EASYP_GH_TOKEN` set, the test caught two issues: (a) a deprecated `remote:` field in the plan's buf.gen.yaml — fixed in `e0c79b1 fix(21-01): use 'plugin:' not 'remote:' in generated buf.gen.yaml` (the alpha-remote-generation API was removed in v1.69.0+ and is deprecated in v1.30.1); (b) a real proxy regression in the v1.30.1 v1alpha1 `DownloadManifestAndBlobs` read-path: the proxy passes 32-char buf UUIDs directly to GitHub's `/git/trees/<id>` API which 404s, because the v1alpha1 `Download` chain does not apply `commitUUIDInverse` (only the v1beta1 path from Phase 18 does). The v1.69.0 subtest hits a persistent TLS handshake timeout fetching HEAD's tree from `raw.githubusercontent.com` (and is not actually testing the pinned-UUID path — the v1.69.0 client ignores the `buf.lock` for `buf generate` and just asks the proxy for HEAD). The v1.30.1 fix is deferred to a follow-up phase (proposed `22-fix-v1alpha1-download-uuid-handling`); the v1.69.0 subtest redesign is deferred to another follow-up.
- Phase 22 proposed: Fix the v1.30.1 v1alpha1 read-path to apply `commitUUIDInverse` on 32-char buf-issued UUIDs in the `DownloadManifestAndBlobs` handler chain, then re-run `TestGenerateWithPinnedBufLock` to confirm both subtests pass. This is the same class of bug Phase 18 fixed for the v1beta1 path; Phase 19/20 e2e tests only exercised the v1beta1 path.
- Phase 22 added: Fix v1.30.1 v1alpha1 read-path UUID handling; verify v1 protocol works. Depends on Phase 21. Scope: (1) apply `commitUUIDInverse` in the v1alpha1 `DownloadManifestAndBlobs` handler chain so 32-char buf UUIDs resolve to git SHAs before hitting GitHub's tree API; (2) re-run `TestGenerateWithPinnedBufLock` to confirm both subtests pass; (3) broader verification that the v1 (v1alpha1) protocol path works end-to-end (not just `buf generate` — also `buf mod update` and the existing smoke test that Phase 19 revealed was broken for v1.30.1). See `.planning/phases/22-fix-v1-30-1-v1alpha1-read-path-uuid-handling-verify-v1-proto/`.
- Phase 23 added: e2e tests for branch-name and non-default-branch commit refs in buf.yaml deps. Closes the ref-shape coverage gap from Phase 19 (which covered tag refs only). Two v1.69.0 tests: `TestRefRespected_BranchName_PinsBranchTip` (gh-pages branch ref → `repos.GetCommit` fall-through pins branch tip) and `TestRefRespected_NonDefaultBranchCommitSHA` (raw 40-char SHA of a gh-pages commit → `isSHA` fast path stamps it branch-agnostically). Both behaviors already shipped in Phase 18 — test-only phase. Test code in `8cf99fa`; docs in separate commit. See `.planning/phases/23-e2e-tests-branch-name-and-non-default-branch-commit-refs-in-buf-yaml-deps/`.
- Phase 24 added: Resolve buf cid ref in ServeGraph; honor pinned commit. Prod `buf generate` failed for grpc-ecosystem/grpc-gateway pinned in buf.lock at `e91b8a68fe214081808d79f1a1a4f09e` — ServeGraph forwarded the 32-hex cid to GitHub (422→502) and infoCache keyed by owner/module served HEAD for the pinned cid. Diagnosis + two RED confirming tests in `.planning/debug/buf-cid-ref-forwarded-to-upstream.md`. Fix: cid→sha map at every mint site, ServeGraph UUID-resolution branch (commitUUIDInverse + 28-hex prefix probe, ported from Phase 18), infoCache cid-gating, ServeDownload cid→sha preference. See `.planning/phases/24-resolve-buf-cid-ref-in-servegraph-honor-pinned-commit/`.
- Phase 25 added: Address PR #39 post-merge review findings. Post-merge reviews of PR #39 (Phases 19–22, branch `buf-proto-update-3` → `main`, MERGED, 6184+/68-) raised findings of varying severity. Verified real problems to fix: (1) `isConventionalDefaultName` carve-out silently changes resolution for repos with a real branch named `main`/`master`/`develop`/`trunk` that is NOT the default — a real behavioral regression (`internal/providers/{github,bitbucket}/getrepo.go`); (2) misleading error wrap at `internal/connect/blobs.go:45` says `GetRepository` but calls `GetFiles` (pre-existing, adjacent to new code); (3) PR-22-4 live e2e was never run against real GitHub (TLS issue, now resolved by `df02ff0`) — residual risk for v1alpha1 read path; (4) `isConventionalDefaultName` (and `isSHA`) duplicated verbatim across two providers with identical 17-line doc comment; (5) dangling proto path `api/proto/buf/registry/module/v1beta1/resource.proto` cited in `commits_helpers_test.go:375` does not exist in repo (verified by `find`); (6) `commitLineRE` regex duplicated between `e2e/ref_test.go:42` and `e2e/testutil/server.go:285`; (7) post-construction mutation of `api.commitResolver` at `api.go:124` is fragile — a future alternate constructor that forgets to wire it silently degrades. Lower-priority items acknowledged in reviews as deliberate trade-offs (e.g. `commitUUIDForTest` byte-table mirror, `resolveCommitForRead` logging asymmetry) are NOT in this phase. See `.planning/phases/25-address-pr-39-post-merge-review-findings-pin-multi-default-b/`.

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| UAT | Phase 03 human UAT (1 pending smoke test) | From v1.1 | 2026-05-07 |
| Verification | Phase 05 human verification (E2E with GitHub token) | From v1.1 | 2026-05-07 |
| v2 features | Performance, new endpoints | Future milestone | 2026-05-10 |

## Session Continuity

Last session: 2026-07-09
Stopped at: Phase 23 complete (local). Commit 8cf99fa ships the two tests; a separate docs commit lands RESEARCH/PLAN/SUMMARY. STATE.md + PROJECT.md evolved to reflect P23. Ref-shape e2e coverage now complete: tag (P19) + branch name (P23-T1) + raw SHA off default branch (P23-T2). No PR opened. Live run of the new tests (with EASYP_GH_TOKEN + cached buf binaries) still pending — would definitively answer whether the buf CLI accepts a raw 40-char SHA as a `:ref` suffix.
Resume file: `.planning/phases/23-e2e-tests-branch-name-and-non-default-branch-commit-refs-in-buf-yaml-deps/23-01-SUMMARY.md`
