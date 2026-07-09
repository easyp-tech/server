---
phase: 23-e2e-tests-branch-name-and-non-default-branch-commit-refs-in-buf-yaml-deps
subsystem: e2e
tags: [e2e, ref-honoring, branch-ref, commit-sha-ref, regression-guard, buf-cli, github-provider]
---

# Phase 23 Research: e2e Tests for branch-name and non-default-branch commit refs in buf.yaml deps

## Question

The user asked for two more e2e tests:

1. Is it possible to specify the **branch name** in the dependency definition in `buf.yaml`? The proxy must return the latest commit for this branch in this case.
2. Is it possible to specify a **commit NOT from the default branch** in the dependency definition in `buf.yaml`?

Both questions are about the `Name.ref` field that the buf CLI sends over the wire when `buf.yaml` declares a dep with a `:ref` suffix (e.g. `deps: - host:port/owner/repo:gh-pages`). Phases 18–22 built and fixed the ref-honoring path; Phase 19 added the regression-guard e2e tests for **tag** refs. Phase 23 closes the gap for the two remaining ref shapes: **branch names** and **raw commit SHAs**.

## Production behavior (already shipped — no code change in this phase)

Both behaviors already exist in `internal/providers/github/getrepo.go` `GetMeta`:

| Ref shape sent in `Name.ref` | GetMeta branch taken | Result |
|---|---|---|
| `""` (no ref) | `commit == ""` | HEAD of default branch |
| `main` / `master` / `develop` / `trunk` or the repo's `DefaultBranch` | `isConventionalDefaultName` carve-out | HEAD of default branch (no second round-trip) |
| 40/64-char lowercase hex (SHA) | `isSHA(commit)` fast path | `meta.Commit = commit` — the SHA is stamped **as-is**, branch-agnostic |
| anything else (tag name, **branch name**, short SHA) | `repos.GetCommit(ctx, owner, repoName, commit, nil)` | GitHub resolves the ref name to its tip SHA; `meta.Commit = rc.GetSHA()` |

So:

- **T1 (branch name ref):** a non-conventional branch name (not main/master/develop/trunk, not the repo's `DefaultBranch`) falls through to `repos.GetCommit`, which resolves the branch tip. ✓ Supported.
- **T2 (non-default-branch commit SHA ref):** any 40-char SHA hits the `isSHA` fast path; the SHA is stamped regardless of which branch it lives on. ✓ Supported, branch-agnostic.

This phase is therefore **test-only**: it adds regression guards proving the two ref shapes work end-to-end through a real buf CLI + real GitHub API. No production code is modified.

## Fixture choice

The fixture repo (matching Phases 19/21) is `googleapis/googleapis`. Its default branch is `master`. `git ls-remote --heads` confirms a stable non-default branch `gh-pages` whose tip differs from `master`'s tip:

```
gh-pages → 84e3d1702766972d360fb02a52602f154bccbbb1
master   → 99f54e6513f09d8df1707a6c553b0a3c6ef9b5fb   (default branch HEAD)
```

`gh-pages` is GitHub Pages — a long-lived branch certain to remain present. It is not in the `isConventionalDefaultName` set, so it exercises the `repos.GetCommit` branch. Its tip is a commit on a non-default branch, so the same SHA serves double duty as the T2 fixture (raw SHA ref whose commit is not on the default branch).

`gh-pages` tip is **not** reachable from `master` (it is a divergent docs branch), so "commit not from the default branch" holds by construction. The tests additionally assert the SHA differs from the `master` tip as a runtime sanity check.

## Test design

Both tests reuse the Phase 19 helpers already in `e2e/ref_test.go`: `gitLsRemote`, `isLowerHex`, `commitUUIDForTest`, `extractCommitFromLock`, and the `RunBufModUpdateWithRef` testutil helper. No new testutil code is required — `RunBufModUpdateWithRef(buf, port, ref)` already accepts any ref string.

Both tests run on **v1.69.0 only** (matching `TestRefRespected_ModUpdate_MatchesUpstreamSHA`), because the assertion compares the lock's pinned commit against `commitUUIDForTest(upstreamSHA)` — the strict UUID contract that is sensitive to the byte table. The broader "differs from HEAD" matrix coverage already exists in Phase 19; Phase 23 is the strict-shape guard for two new ref forms.

### T1: `TestRefRespected_BranchName_PinsBranchTip`

1. `RequireEnvToken` → skip cleanly when unset.
2. `gitLsRemote(googleapis, "refs/heads/gh-pages")` → `ghPagesTip` SHA.
3. Sanity: `ghPagesTip != gitLsRemote(googleapis, "refs/heads/master")` (the branch is genuinely not the default).
4. `expectedUUID := commitUUIDForTest(ghPagesTip)`.
5. `RunBufModUpdateWithRef(bufV169, port, "gh-pages")`.
6. Assert lock commit == `expectedUUID`.

Proves: branch name in `Name.ref` → proxy pins to that branch's tip, not HEAD.

### T2: `TestRefRespected_NonDefaultBranchCommitSHA`

1. `RequireEnvToken` → skip cleanly when unset.
2. `gitLsRemote(googleapis, "refs/heads/gh-pages")` → `ghPagesTip` SHA (a commit that lives on `gh-pages`, not on `master`).
3. Sanity: `ghPagesTip != master tip`.
4. `expectedUUID := commitUUIDForTest(ghPagesTip)`.
5. `RunBufModUpdateWithRef(bufV169, port, ghPagesTip)` — the ref is the **raw 40-char SHA**.
6. Assert lock commit == `expectedUUID`.

Proves: a commit SHA that is not on the default branch, sent in `Name.ref`, is honored (the `isSHA` fast path stamps it regardless of branch).

## Unknowns the tests will resolve

- Whether the **buf CLI** accepts a raw 40-char SHA as the `:ref` suffix in `buf.yaml` deps (`host:port/owner/repo:<40hex>`). If buf rejects the syntax client-side, T2 fails with the buf stderr captured — which is itself the answer to the user's question. The proxy-side `isSHA` path is already correct; the test surfaces any buf-client-side restriction.
- Whether buf accepts `gh-pages` (hyphenated branch name) as the `:ref` suffix. Tag names worked in Phase 19; branch names are the same wire field, so this is expected to work, but the test confirms it.

Both unknowns are exactly what the user asked ("is it possible"). A passing test = yes; a failing test with captured stderr = no, with the reason.

## Dependencies on prior phases

- **Phase 18** — ref-honoring code path (shipped).
- **Phase 19** — `e2e/ref_test.go` helpers (`gitLsRemote`, `isLowerHex`, `commitUUIDForTest`, `extractCommitFromLock`) + `RunBufModUpdateWithRef` testutil helper. Reused, not modified.
- **Phase 11/12** — `testutil.DefaultTestConfig`, `StartServer`, `RequireEnvToken`, `GetBuf`, `BufV169`.

## Out of scope

- Production code changes (none needed; behaviors already shipped).
- The matrix "differs from HEAD" variant for these ref shapes (Phase 19's matrix already covers the diff guarantee generically).
- v1.30.1 v1alpha1 coverage (the strict UUID assertion is v1.69.0-only by design; v1.30.1 coverage exists in Phase 19's matrix test).
- BitBucket / local-git providers (project scope: GitHub-only testing).
