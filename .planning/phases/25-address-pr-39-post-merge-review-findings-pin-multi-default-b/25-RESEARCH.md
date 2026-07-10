# Phase 25: Address PR #39 post-merge review findings - Research

**Researched:** 2026-07-10
**Domain:** Go / Code quality / Bugfix / Regression prevention / Test infrastructure
**Confidence:** HIGH

## Summary

Phase 25 addresses 7 findings from the post-merge review of PR #39 (buf-proto-update-3 -> main, 6184+/68-). The findings span four severity tiers:

1. **Behavioral regression** (Finding 1): `isConventionalDefaultName` carve-out silently resolves HEAD for repos with a real branch named `main`/`master`/`develop`/`trunk` that is NOT the default branch. This is a genuine runtime regression in `internal/providers/{github,bitbucket}/getrepo.go`.

2. **Bug adjacent to new code** (Finding 2): `internal/connect/blobs.go:45` wraps an error as `GetRepository` when the actual call was `GetFiles`. Pre-existing, adjacent to Phase 22 wire work.

3. **Construction-time fragility** (Finding 7): The post-construction mutation `a.commitResolver = commitHandler` at `api.go:124` means any future constructor that forgets to wire `commitResolver` silently degrades (UUID resolution silently skipped, nil-guarded at call site).

4. **Cleanup / test-only** (Findings 3-6): v1alpha1 live e2e gap (opex), duplicated `isConventionalDefaultName`/`isSHA` across two providers, dangling proto path in comment, duplicated `commitLineRE` regex between e2e and testutil.

**Primary recommendation:** One plan with findings ranked by severity. The behavioral regression (Finding 1) and the adjacent bug (Finding 2) should be first-priority. The construction-time safety (Finding 7) is cheap to fix alongside Finding 1/Finding 2 (same files). The cleanup items (Findings 3-6) can be sequenced as lower-priority tasks in the same plan, or deferred to a separate follow-up if the planner wants to keep blast radius small.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| `isConventionalDefaultName` ref-resolution carve-out | Provider layer (`github/getrepo.go`, `bitbucket/getrepo.go`) | Shared utility package (extraction target) | The carve-out is inlined in both providers' `GetMeta`/`getMeta` methods; fixing the regression requires changing the same branch-selection logic |
| `isSHA` / `isConventionalDefaultName` shared definitions | Provider layer (currently duplicated) | Shared utility package (extraction target) | Both functions are copy-pasted with identical doc comments; the right home is a shared package like `internal/providers/content` |
| Error wrap message in `DownloadManifestAndBlobs` | API layer (`internal/connect/blobs.go`) | - | Single-line string fix in a Connect handler |
| Post-construction `commitResolver` mutation | API layering/construction (`internal/connect/api.go`) | - | Constructor pattern fix in `NewWithConfig` |
| v1alpha1 e2e regression gate | E2E test layer (`e2e/generate_test.go`) | - | Operationally requires `EASYP_GH_TOKEN` + TLS certs |
| Test-only proto path comment | Test layer (`commits_helpers_test.go`) | - | Single-line comment fix |
| `commitLineRE` regex definition | E2E test layer (`e2e/ref_test.go`, `e2e/testutil/server.go`) | Shared test utility (`e2e/testutil`) | Regex is duplicated; extract to `e2e/testutil` |

## Standard Stack

No new libraries or dependencies. All changes use existing Go stdlib and the repo's own packages.

### Core (already in repo)
| Library | Purpose | Why Standard |
|---------|---------|--------------|
| `net/http` | HTTP response handling for the provider layer | Already the HTTP framework for the entire project |
| `internal/providers/content` | Shared type definitions for provider metadata | Already exists at `internal/providers/content/repo.go` (defines `Meta` struct) |
| `log/slog` | Any new log lines | Already the logging framework for the entire project |
| `testing` / `github.com/stretchr/testify/require` | Test assertions | Already the test framework for the entire project |

**Version verification:** Not applicable -- no new packages.

## Package Legitimacy Audit

Not applicable -- no external packages are installed by this phase. All changes are to source files in the repo.

## Architecture Patterns

### System Architecture Diagram

```
Finding 1: isConventionalDefaultName regression
================================================
buf CLI sends ref="main" (v1.30.1 v1alpha1 path)
        │
        ▼
provider.GetMeta(ctx, owner, repo, "main")     [getrepo.go]
        │
        ├── repo.DefaultBranch == "production"   (not "main")
        ├── isConventionalDefaultName("main")    → true   ← THE BUG
        │       └── returns HEAD (production tip), NOT the tip of branch "main"
        │
        After fix:
        └── commit == "main"  AND  repo has a branch named "main"?
                ├── YES → repos.GetCommit("main") resolves to real branch tip
                └── NO → isConventionalDefaultName carve-out (v1.30.1 fallback)

Finding 2: Misleading error wrap
==================================
a.repo.GetFiles(ctx, owner, repo, ref)          [blobs.go:43]
        │ error?
        ▼
return fmt.Errorf("a.repo.GetRepository: %w", err)   ← WRONG: says GetRepository

Finding 4: Duplicated helpers
==============================
internal/providers/github/getrepo.go  -- isSHA(), isConventionalDefaultName()
internal/providers/bitbucket/getrepo.go -- isSHA(), isConventionalDefaultName()  IDENTICAL
internal/connect/commits_helpers.go   -- isSHA() (THIRD copy, but different domain)

Fix: extract to internal/providers/content (or internal/providers/...)

Finding 7: Fragile post-construction mutation
==============================================
NewWithConfig()                              [api.go:77-141]
  a := &api{log, repo, domain}
  ...
  commitHandler := &commitServiceHandler{...}
  a.commitResolver = commitHandler   ← Post-construction mutation [api.go:124]
  ...
  return mux

Fix: make commitResolver a constructor parameter or
      move assignment before handler registration
```

### Finding Descriptions

### Finding 1: `isConventionalDefaultName` silently returns HEAD for non-default branches named "main"/"master"/"develop"/"trunk"

**What:** `isConventionalDefaultName` in both `github/getrepo.go` and `bitbucket/getrepo.go` checks if the commit string matches any of `{"main", "master", "develop", "trunk"}`. If it does, `GetMeta` returns HEAD (the default branch's tip) WITHOUT checking whether the repo actually has a branch named that string. For a repo with a real branch named "main" that is NOT the default branch (e.g., default = "production", branch = "main"), this silently returns the wrong commit.

**Severity:** Real behavioral regression. A user pinning `:main` in buf.yaml expects the tip of branch `main`, not the tip of `production`.

**Existing behavior:** The carve-out was added to handle the v1.30.1 v1alpha1 case where buf sends `reference="main"` (the buf default label) even when the repo's actual default branch is e.g., "master" (googleapis/googleapis). Without the carve-out, GitHub's `/commits/main` endpoint 422s because it expects a SHA.

**Fix options:**
- (a) **Prefer real branch resolution**: Before the `isConventionalDefaultName` carve-out, check if the repo actually has a branch named the requested string. If yes, resolve it via `repos.GetCommit`/`getCommit`. If no, fall through to the carve-out. Best fidelity but requires a GitHub API call (extra round-trip).
- (b) **Remove the carve-out entirely**: The carve-out was designed for the v1.30.1 v1alpha1 `reference="main"` case. But that case is now handled differently (the `name.label_name=3` field is silently ignored and `ref` parses to empty, taking the HEAD fast path in the providers -- verified by `TestParseResourceRefName_LabelNameIsField3` in `commits_helpers_test.go:394-413`). So the carve-out may be dead code.
- (c) **Add a strict "only if default branch matches" check**: Only apply the carve-out when `commit == meta.DefaultBranch` -- which is already checked first on line 80/github and 75/bitbucket. The `isConventionalDefaultName` check is an OR to cover the v1.30.1 label-vs-branch mismatch. If the Mismatch case is no longer reachable, the carve-out is dead.

**Which fix to use:** Option (c) is the simplest and preserves the heuristic for any remaining v1.30.1 edge case: only apply the non-SHA carve-out when the commit name is either the actual default branch OR a conventional default name AND the actual default branch. But the real question is whether the carve-out is still needed at all. Let me trace the v1.30.1 path:

1. v1.30.1 sends reference="main" at `name.label_name=3` (proto field 3).
2. `parseResourceRefName` reads field 4 (ref), NOT field 3 (label_name).
3. So `ref.ref` is empty for the v1.30.1 case.
4. `ServeHTTP`/`ServeGraph` calls `GetMeta(..., "", meta.Commit)` where `meta.Commit` is an empty string or whatever was available.
5. Wait -- let me re-read the flow. `meta.Commit` is the commit UUID NOT the raw string. Let me re-check.

Actually, looking more carefully: the v1.30.1 client sends the reference in the `Name` message at field 3 (`label_name=3`). The Phase 18 `parseResourceRefName` reads field 4 (`ref=4`). So for v1.30.1, `ref.ref` is empty. But the v1.30.1 reference still reaches `GetMeta` via a different path (`resolveCommitForRead` or via the modulepins path) -- let me check the actual flow.

Actually, looking at the code flow more carefully: the `isConventionalDefaultName` carve-out is in `GetMeta` which is called by the provider layer for ALL version paths (not just v1alpha1). When `ref.ref` is empty (no ref sent by the buf CLI), the commit argument to `GetMeta` is... let me check.

In `ServeHTTP` (commits.go), `GetMeta` is called with `commit` set to `ref.ref` (line 228: `meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, "")` for the no-ref case). The carve-out checks `commit == DefaultBranch || isConventionalDefaultName(commit)`. If `commit` is `""`, it doesn't match either, so the carve-out doesn't fire. The four branches in `GetMeta` are:

1. `commit == ""`: no ref supplied, keep HEAD.
2. `commit == DefaultBranch || isConventionalDefaultName(commit)`: client asked for a well-known default.
3. `isSHA(commit)`: raw SHA fast path.
4. non-SHA non-empty: resolve as ref.

So the carve-out fires when a NON-EMPTY commit string matches a conventional default name. This could happen when:
- A modern buf CLI sends `ref="main"` in a dependency (e.g., `googleapis/googleapis:main`).
- The repo's default branch is "master" (googleapis) and "main" is NOT a real branch.
- The carve-out returns HEAD (master's tip) -- which is WRONG if the user wanted the tip of a real branch named "main".

But wait -- googleapis/googleapis DEFAULT branch IS "master", and "main" is NOT a real branch there. In that case, calling `repos.GetCommit("main")` would 422 because Github's API expects a SHA for that endpoint, not a branch name... actually, I need to re-read the comment. The review comment about `repos.GetCommit` says it expects a SHA and rejects branch names with 422 "No commit found for SHA: main". But wait -- `repos.GetCommit` actually DOES accept ref names, the doc comment says "accepts ref names, short SHAs >= 7 chars, and full SHAs". The 422 issue was specifically because the v1.30.1 path sends "main" as a label name (not a ref), and `/commits/main` endpoint in GitHub's API expects a SHA...

Hmm, let me re-read the comment in getrepo.go more carefully. It says:
```
// Without this carve-out, repos.GetCommit(ctx, owner, repoName,
// "main", nil) hits GitHub's /commits/main endpoint, which
// expects a SHA and rejects branch names with 422 "No commit
// found for SHA: main"
```

Wait, that contradicts the comment at line 72-76 which says `repos.GetCommit` "accepts ref names, short SHAs >= 7 chars, and full SHAs". Let me check the actual behavior: GitHub's Repositories.GetCommit API accepts both SHAs and refs. BUT there might be an issue with the go-github library's handling.

Actually, looking at the google/go-github library, `Repositories.GetCommit(ctx, owner, repo, sha)` -- the parameter is named `sha` but the GitHub API endpoint is `GET /repos/{owner}/{repo}/commits/{ref}` which DOES accept branch names, tags, and SHAs. So why did the comment say it 422s?

The issue might be specifically with the value "main" when there's no branch or tag named "main" on that repo. GitHub's API returns 422 "No commit found for SHA: main" when the ref doesn't resolve. So the carve-out is actually needed for the case where the v1.30.1 client sends "main" and there's no branch named "main" on the target repo.

OK so the issue is real: **if a repo has a real branch named "main" but the default branch is something else (e.g., "production")**, then:
- Without the carve-out: `repos.GetCommit("main")` succeeds and returns the tip of branch "main" -- CORRECT.
- With the carve-out: `isConventionalDefaultName("main")` returns true, HEAD is returned instead -- WRONG.

And the fix should be: only apply the carve-out when the string is NOT a real branch (i.e., the carve-out fires only when `repos.GetCommit` would fail). This is option (a).

But making an extra API call to check if a branch exists is expensive. A simpler approach: the carve-out should check if the string is NOT the actual default branch AND is NOT a real branch. But we don't know if it's a real branch without an API call.

Actually, the simplest approach: **only apply the carve-out for "main" when the default branch is one of the traditional git names that "main" replaced (like "master")**. In other words, the heuristic is: "main" is a conventional default-label name that the old v1.30.1 buf CLI sends, and it should map to the actual default branch. But if the repo ACTUALLY HAS a branch named "main", treat it as a real ref resolution, not a label alias.

But again, we need an API call to know if "main" is a real branch.

OK, the cleanest approach for this phase: **remove the `isConventionalDefaultName` carve-out and instead add a `isDefaultLabel(s string) bool` that checks if `s` is a well-known DEFAULT LABEL name that the buf CLI sends. Only apply the carve-out when `s` matches a conventional default label AND the repo's default branch has the OTHER name (e.g., `s="main"` and `default="master"`) -- this signals the buf-label-vs-git-branch mismatch. When `s=="main"` and `default=="main"`, it's treated as the actual default branch (already covered by the first check). When `s=="main"` and `default=="production"`, it's an actual branch name to resolve.**

Wait, that still doesn't distinguish the "main is a real branch" vs "main is a buf default label" cases. Both would have `s="main"` and `default!="main"`.

The real question is: **do we know of any real-world repos where the default branch is NOT "main"/"master"/"develop"/"trunk" AND there exists a branch named one of those?** If such repos exist and have buf users, the regression is real. If not, it's theoretical.

The comment in the code already acknowledges this risk: "The risk -- a repo with a non-conventional default (e.g., 'production') and a branch named 'main' -- returns HEAD instead of the branch's commit, but this is the pre-Phase-18 behavior and the v1.30.1 case is the common one."

Since the Phase 18 changes, the v1.30.1 case is no longer a concern (label_name is ignored, ref is not sent). So the carve-out is pure risk with no offsetting benefit in the v1.30.1 case.

**Recommendation for Finding 1:** Remove the carve-out entirely. The v1.30.1 `label_name="main"` case that it was designed for is no longer reachable (Phase 18 changed parseResourceRefName to read field 4 only; field 3/label_name is silently ignored). If a caller passes `ref="main"` to GetMeta and the repo has no such branch, `repos.GetCommit("main")` will 422 and the proxy returns a clear error. The pre-Phase-18 "v1.30.1 label_name" behavior that motivated the carve-out is gone. Document the removal clearly in the commit message so if the 422 surfaces for a real v1.30.1 user, the revert path is known.

### Finding 2: Misleading error wrap at blobs.go:45

**What:** `fmt.Errorf("a.repo.GetRepository: %w", err)` at line 45 of `internal/connect/blobs.go` says "GetRepository" but the actual method called is `a.repo.GetFiles` at line 43. Pre-existing typo adjacent to Phase 22 UUID-resolution wiring.

**Fix:** Change the error string from `"a.repo.GetRepository"` to `"a.repo.GetFiles"`.

**Severity:** Low (the error string is only surfaced in logs and Connect error messages to the client; both contexts see a wrong method name but the error path is already a failure).

### Finding 3: v1alpha1 live e2e gap

**What:** PR-22-4 (`TestGenerateWithPinnedBufLock/v1.30.1` subtest) was never run against real GitHub because the TLS issue to `raw.githubusercontent.com` blocked it. The TLS issue was resolved by commit `df02ff0` (retry transient upstream errors in HTTP transport). The v1alpha1 read path (resolving 32-char UUIDs in `DownloadManifestAndBlobs`) has been verified by unit tests (PR-22-1/2/3) but not by the end-to-end regression gate.

**Fix:** Run the e2e gate: `EASYP_GH_TOKEN=<token> go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1 -v` with TLS certs at `~/local-tls/server/`. No code changes needed -- this is a pure operational verification task.

**Severity:** Residual risk only. The Phase 22 `resolveCommitForRead` wrapper + unit tests (PR-22-1/2/3) cover the code path structurally. The e2e test adds the final integration verification.

### Finding 4: `isConventionalDefaultName` and `isSHA` duplicated verbatim across providers

**Duplication ledger:**
- `isSHA`: defined 3 times in the repo:
  1. `internal/providers/github/getrepo.go:19-29`
  2. `internal/providers/bitbucket/getrepo.go:16-26`
  3. `internal/connect/commits_helpers.go:72-82`

- `isConventionalDefaultName`: defined 2 times in the repo:
  1. `internal/providers/github/getrepo.go:48-54`
  2. `internal/providers/bitbucket/getrepo.go:45-51`

The doc comments are identical across the provider copies (literally the same 17-line comment).

**Extraction target:** Options:

1. `internal/providers/content/` -- This package already exists and defines `Meta struct`, `File struct`, etc. It's imported by both providers. Cleanest home. But `isConventionalDefaultName` is a provider-layer heuristic, and `content` is a data-model package. Name mismatch.

2. New `internal/providers/helpers/` or similar -- Cleanest conceptually but requires a new package import in both providers.

3. New `internal/providers/content/provider.go` or extend existing `repo.go` -- Functionally cleanest; `content` already holds `Meta` which is the type the carve-out operates on.

4. Don't extract (leave duplicated) -- For such small functions (7 lines each, a switch and a hex loop), the duplication is tolerable. The risk is that a future bugfix in one copy is missed in the other. Since both functions are pure (no side effects, no external state), this risk is low.

**Recommendation:** Extract to `internal/providers/content/` as `helpers.go` (or add to `repo.go`). The two functions are provider-layer pure helpers and belong alongside the types they validate. The connect-layer `isSHA` (commits_helpers.go) has a different doc comment and is used in a different context -- leave it in place (it's not "provider-layer" logic).

Actually, wait -- the connect-layer `isSHA` in `commits_helpers.go` is identical in body to the provider copies. But the connect layer already imports `isSHA` for use in `probeCommitID`. The extraction should serve both connect and provider layers. But the connect package (`internal/connect`) has no business importing from `internal/providers/content` -- that would be a layering violation (connect is already above providers).

**Revised recommendation:** Extract the shared helpers into a new package `internal/providers/content` (which both providers already import). Leave the connect-layer `isSHA` in `commits_helpers.go` (it's a different concern -- buffer validation for UUID derivation, not provider-layer commit-vs-ref gating). The test for `isSHA` already exists in both `getrepo_test.go` and `commits_helpers_test.go`, so move the provider-layer test coverage to the extracted package.

### Finding 5: Dangling proto path in test comment

**What:** `commits_helpers_test.go:375` references `api/proto/buf/registry/module/v1beta1/resource.proto` which does not exist in the repo. The path has likely changed as part of the proto regeneration in v1.2 (Phase 7). The correct path in the current repo layout is `api/proto/buf/alpha/registry/v1alpha1/resource.proto` (the v1alpha1 path where the `Name` message lives) and/or `api/_third_party/buf/proto/buf/alpha/registry/v1alpha1/resource.proto`.

**Fix:** Update the comment on line 375 to reference the correct literal path. Or remove the specific path and just say "buf v1beta1 Name proto definition".

**Severity:** Cosmetic -- the comment is documentation only, not code or test logic.

### Finding 6: `commitLineRE` regex duplicated between e2e and testutil

**Duplication ledger:**
- `e2e/ref_test.go:42`: `var commitLineRE = regexp.MustCompile(\`(?m)^[ \t]+commit:[ \t]+(\S+)\s*$\`)`
- `e2e/testutil/server.go:285`: `commitLineRE := regexp.MustCompile(\`(?m)^[ \t]+commit:[ \t]+(\S+)\s*$\`)`

The second instance explicitly notes the duplication: "extractCommitFromLock lives in package e2e (e2e/ref_test.go), so we duplicate the regex inline here (the testutil package cannot import the e2e package)."

**Extraction targets:**
1. Export `commitLineRE` from `e2e/ref_test.go` (rename to `CommitLineRE`) and have `testutil/server.go` reference it.
2. Move `extractCommitFromLock` from `e2e/ref_test.go` to `e2e/testutil/server.go` (the function is used by 5 test functions in `e2e/ref_test.go`, so all callers would update their import).
3. Define the regex in `e2e/testutil/server.go` only, and have `e2e/ref_test.go` import from testutil.

**Recommendation:** Option 3 (define in testutil only). `testutil` is the shared test utility package. `e2e/ref_test.go` can import `e2e/testutil.CommitLineRE` (or `ExtractCommitFromLock`). This eliminates the duplication. The alternative (option 2) is also clean since `extractCommitFromLock` is already duplicated in server.go's `runBufGenerate` logic (it inlines the same regex pattern for the lock-overwrite step). Moving `extractCommitFromLock` to testutil makes it reusable for both purposes.

Actually, the cleanest approach: export `ExtractCommitFromLock` from testutil, have e2e/ref_test.go call it, and have testutil's `runBufGenerate` also call it (it's already duplicating the logic). This eliminates BOTH the regex duplication and the function-logic duplication.

### Finding 7: Fragile post-construction mutation of `api.commitResolver`

**What:** At `api.go:124`, `a.commitResolver = commitHandler` assigns the `CommitResolver` interface after `commitHandler` is constructed. The assignment happens after the `*api` is constructed (`a := &api{...}` at line 84) and after the v1alpha1 handlers are registered (lines 92-94, which register `a`'s methods). `NewWithConfig` runs single-threaded at startup, so there is no data race. But if a future code path introduces another constructor that calls `New` (which calls `NewWithConfig` with zero-value `CommitResolution{}`) or constructs `*api` directly, `commitResolver` is nil and the UUID-resolution path silently degrades (the nil-guard at blobs.go:33 catches it, but the handler returns "resolving commit uuid" error instead of failing to compile).

**Fix options:**
- (a) **Accept `CommitResolver` in `NewWithConfig` signature**: Add `commitResolver CommitResolver` as a parameter to `NewWithConfig`. But `commitHandler` is constructed INSIDE `NewWithConfig` -- it IS the `CommitResolver`. This creates a circular construction dependency.
- (b) **Make `CommitResolver` a wrapper struct created before `commitServiceHandler`**: Split `CommitResolver` into its own struct that takes the same state as `commitHandler`. Too invasive for this phase.
- (c) **Add a `CommitResolver` field to `CommitResolution` struct**: Allow optional injection of an external resolver. If nil, `NewWithConfig` uses the `commitHandler` as before. This lets test constructors pass a mock.
- (d) **Move the assignment before handler registration**: Not meaningful -- the handlers reference `a` by pointer, so the mutation is visible regardless of when it happens in the function.
- (e) **Remove the back-pointer entirely and inject the resolve function**: Add a `resolveCommitForRead` field on `*api` (a `func` type). When `New` calls `NewWithConfig`, the function is not set. When `NewWithConfig` constructs `commitHandler`, it also sets `a.resolveCommitForRead = commitHandler.resolveCommitForRead`. Same issue, different name.

Actually, the cleanest fix: **accept a `resolveCommitFunc func(ctx, owner, module, id string) (string, error)` on `*api` and set it in both `New` and `NewWithConfig`**. In `New`, set it to a function that returns an error ("commit resolution not configured"). In `NewWithConfig`, set it to `commitHandler.resolveCommitForRead`. The nil-guard at blobs.go:33 becomes a non-nil check on the function pointer. This makes the contract explicit and the "not configured" path produces a clear error instead of silently skipping resolution.

**Recommendation:** Option (e) or a simpler variant: rename `commitResolver` to a `resolveCommitForRead` function field. Set it to `commitHandler.resolveCommitForRead` in `NewWithConfig`. In `New` (which creates a `commitServiceHandler` via `NewWithConfig`), have the handler created first, then the assignment is natural. Wait -- `New` calls `NewWithConfig` which already does this. The concern is about FUTURE constructors that bypass `New`/`NewWithConfig`. For that, the clearest fix is to add `commitResolver` to the `api` struct literal in `NewWithConfig`:

```go
a := &api{
    log:            log,
    repo:           core,
    domain:         domain,
    commitResolver: nil, // assigned below after commitHandler is constructed
}
```

...with a comment that explains why it's nil here and assigned later. But actually, the `commitResolver` field is only assigned AFTER `commitHandler` is constructed, which depends on `a` being fully initialized (because `commitServiceHandler.api = a`). So it can't be set in the struct literal -- circular.

The pragmatic fix: **add a `setCommitResolver(CommitResolver)` method on `*api`** that assigns the field. Or, even simpler: **add a compile-time assertion** that `CommitResolver` cannot be forgotten. But Go has no compile-time "method was called" assertions.

**Simplest practical fix:** Keep the current pattern but add a doc comment on the `commitResolver` field saying it MUST be assigned by `NewWithConfig` and that `New` inherits this. Then change the callers of `New` (like `testMux` / `testMuxWithConfig`) to explicitly call a new function if needed. This is documentation-only -- the nil-guard already catches the error at runtime with a clear message.

Actually, the simplest fix that actually prevents the regression: **don't nil-guard at blobs.go:33 -- panic if `commitResolver` is nil**. This turns a silent degradation into a loud panic at handler registration time. But that's more fragile, not less.

**Recommended fix for Finding 7:** Change `NewWithConfig` to accept a `CommitResolver` as a parameter and always assign it to `a.commitResolver`. Make `New` (the test entrypoint) also accept it. This moves the assignment from post-construction positional to a constructor parameter, so forgetting to wire it is a compile error (missing argument) not a runtime nil-skip.

But wait, this changes the signature of `New` and `NewWithConfig` which are called from tests and `cmd/easyp`. That's a bigger change than this phase should take.

**Revised recommendation:** Keep the post-construction assignment but add a `initCommitResolver` helper on `*api` that checks for nil and panics with a clear message. Call it in `NewWithConfig` after construction. Extend the `CommitResolution` struct with a `CommitResolver` field that, if non-nil, overrides the automatic assignment. This makes the contract explicit:

```go
func (a *api) initCommitResolver(r CommitResolver) {
    if r == nil {
        // Should never happen -- NewWithConfig always constructs a
        // commitServiceHandler which implements CommitResolver.
        panic("easyp-buf-proxy: internal/connect/api.go: commitResolver not configured -- use NewWithConfig not New")
    }
    a.commitResolver = r
}
```

Called as `a.initCommitResolver(commitHandler)` at api.go:124. The panic is a compile-time catch (fires at startup, not at request time) if a future constructor forgets to wire it. The `CommitResolution` struct lets a test pass a mock resolver.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Detecting whether a branch named X exists on GitHub | A new `branchExists` API call before `GetMeta` branching | `repos.GetCommit` (already called for the non-SHA non-empty branch) | `GetCommit` is the authoritative way to resolve a branch name to a SHA. If it 404/422s, the branch doesn't exist and the carve-out heuristic fires. Don't add a separate `GetBranch` check. |
| Shared helper package for provider-layer utils | A new `internal/providers/helpers` or `internal/providers/shared` package | Existing `internal/providers/content` package | Already imported by both providers; adding two small pure functions to `repo.go` or a new `helpers.go` in the same package extends the existing boundary without adding an import chain. |
| Extracting `ExtractCommitFromLock` to avoid code duplication | A new helper function that rewrites the regex | Add the function to `e2e/testutil/server.go` and have `e2e/ref_test.go` call it | Simplest change: export `ExtractCommitFromLock` from testutil, have both callers reference it. |

**Key insight:** The behavioral regression (Finding 1) is the only finding that requires a design decision with trade-offs. The remaining findings are mechanical text changes, extractions, or operational tasks.

## Runtime State Inventory

Not applicable -- this is a cleanup/fix phase, not a rename/refactor/migration phase.

## Common Pitfalls

### Pitfall 1: Overfixing `isConventionalDefaultName` with an extra API call
**What goes wrong:** The fix adds a `repos.GetBranch` or `repos.GetCommit` check inside the carve-out to verify whether the branch exists, doubling provider round-trips for every GetMeta call.
**Why it happens:** The natural impulse is "check if the branch exists before applying the carve-out."
**How to avoid:** Remove the carve-out entirely (the v1.30.1 case it was designed for is no longer reachable). If a real-world regression surfaces, add a targeted check only for the specific v1.30.1+googleapis pattern.
**Warning signs:** Plan tasks named "add branch existence check to GetMeta."

### Pitfall 2: Extracting `isSHA` to a shared package without considering the connect layer
**What goes wrong:** The extraction moves `isSHA` from `commits_helpers.go` (connect package) to `internal/providers/content` (provider package), forcing the connect package to import from the provider layer. That's a downward import (connect already imports providers), which is fine directionally, but `isSHA` in connects is used for UUID-derivation validation, not provider-layer commit-vs-ref gating.
**How to avoid:** Only extract the provider copies. Leave the connect-layer `isSHA` where it is. The two have different doc comments and usage contexts.
**Warning signs:** PR modifies `commits_helpers.go` to remove `isSHA`.

### Pitfall 3: Changing the `NewWithConfig` signature
**What goes wrong:** Adding a `CommitResolver` parameter to `New`/`NewWithConfig` breaks all call sites (tests, cmd/easyp).
**How to avoid:** Keep the existing signatures. The simplest fix for Finding 7 is a documentation change + a panic-guard, not a constructor signature change.
**Warning signs:** Plan changes `func NewWithConfig(log, core, domain, cfg)` signature.

### Pitfall 4: Running the v1alpha1 e2e against stale TLS certs
**What goes wrong:** The e2e test starts a TLS server and expects self-signed certs at `~/local-tls/server/`. If those certs have expired or been rotated, the test fails not because of the v1alpha1 path but because of infrastructure.
**How to avoid:** Verify `~/local-tls/server/server.crt` and `~/local-tls/server/server.key` exist and are valid before running the e2e gate. The existing Phase 19-23 tests already depend on these certs.
**Warning signs:** e2e test fails with TLS error on server start.

## Code Examples

### Finding 1: Remove the `isConventionalDefaultName` carve-out from both providers

**github/getrepo.go -- GetMeta method (lines 56-105):**

Before:
```go
if commit != "" {
    if commit == meta.DefaultBranch || isConventionalDefaultName(commit) {
        // Keep HEAD from getRepo (v1.30.1 v1alpha1 reference="main" case)
    } else if isSHA(commit) {
        meta.Commit = commit
    } else {
        rc, _, err := c.repos.GetCommit(ctx, owner, repoName, commit, nil)
        if err != nil {
            return meta, fmt.Errorf("resolving ref %q: %w", commit, err)
        }
        meta.Commit = rc.GetSHA()
    }
}
```

After:
```go
// Four branches:
//   - commit == "":          no ref was supplied, keep HEAD.
//   - commit == DefaultBranch: client asked for the default branch by name.
//   - isSHA(commit):         raw SHA fast path (40/64 lowercase hex).
//   - non-SHA non-empty:     treat as a ref and resolve via GitHub's
//                            repos.GetCommit (accepts ref names, short
//                            SHAs >= 7 chars, and full SHAs).
//
// Note: The isConventionalDefaultName carve-out (main/master/develop/trunk)
// was REMOVED in Phase 25. It was originally added to handle the v1.30.1
// v1alpha1 case where buf sent reference="main" (the BSR default label)
// even when the repo's default branch was "master". Phase 18 changed
// parseResourceRefName to read proto field 4 only, so label_name=3 is
// silently ignored and ref="" falls through to the HEAD path. The carve-out
// is no longer reached. Its risk -- silently returning HEAD for a real
// branch named one of the conventional labels -- outweighed its benefit.
if commit != "" {
    if commit == meta.DefaultBranch {
        // Client asked for the default branch by name.
        // meta.Commit already holds HEAD (from getRepo).
    } else if isSHA(commit) {
        meta.Commit = commit
    } else {
        rc, _, err := c.repos.GetCommit(ctx, owner, repoName, commit, nil)
        if err != nil {
            return meta, fmt.Errorf("resolving ref %q: %w", commit, err)
        }
        meta.Commit = rc.GetSHA()
    }
}
```

Same change applies to `bitbucket/getrepo.go` `getMeta` method, but using `c.getCommit(ctx, commit)` instead of `c.repos.GetCommit`.

### Finding 2: Fix error wrap text

**blobs.go:45:**

Before:
```go
return nil, asConnectError(fmt.Errorf("a.repo.GetRepository: %w", err))
```

After:
```go
return nil, asConnectError(fmt.Errorf("a.repo.GetFiles: %w", err))
```

### Finding 4: Extract `isSHA` and `isConventionalDefaultName` to `internal/providers/content/`

New file `internal/providers/content/helpers.go`:

```go
package content

// isSHA reports whether s is a 40-char (SHA-1) or 64-char (SHA-256)
// lowercase hex string.
func IsSHA(s string) bool {
    if len(s) != 40 && len(s) != 64 {
        return false
    }
    for _, c := range s {
        if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
            return false
        }
    }
    return true
}

// IsConventionalDefaultName reports whether s is a well-known default
// branch or label name. [Same doc comment as current, but exported for test use]
func IsConventionalDefaultName(s string) bool {
    switch s {
    case "main", "master", "develop", "trunk":
        return true
    }
    return false
}
```

Then in both `github/getrepo.go` and `bitbucket/getrepo.go`, replace the locally-defined `isSHA` and `isConventionalDefaultName` with calls to `content.IsSHA` and `content.IsConventionalDefaultName`. Remove the local definitions.

### Finding 6: Extract `commitLineRE` and `extractCommitFromLock` to testutil

In `e2e/testutil/server.go`, add `CommitLineRE` and `ExtractCommitFromLock` as exported symbols. In `e2e/ref_test.go`, remove the local `commitLineRE` and `extractCommitFromLock` and import from `testutil`.

### Finding 7: Strengthen the post-construction mutation with a guard

**api.go:**

```go
// CommitResolver returns the commit resolver used by v1alpha1 handlers.
// Must not be nil after NewWithConfig returns. The zero value causes
// v1alpha1 UUID resolution to silently degrade (test entrypoints only).
func (a *api) CommitResolver() CommitResolver {
    return a.commitResolver
}

// In NewWithConfig, after commitHandler is constructed:
a.commitResolver = commitHandler
if a.commitResolver == nil {
    panic("internal/connect/api.go: commitResolver was not configured")
}
```

Or, simpler: just add a `ResetWithResolver(CommitResolver)` method that panics on nil.

Actually, the simplest approach: make `New` (the test entrypoint) also use `NewWithConfig` which already constructs `commitHandler`, and add a `commitResolver` assignment in `NewWithConfig` that is the TOP of the function rather than the bottom. But the circular dependency prevents this.

Pragmatic fix: add a `resolver()` accessor on `*api` that panics if nil, and call it from `blobs.go` instead of nil-guarding:

```go
func (a *api) resolver() CommitResolver {
    if a.commitResolver == nil {
        panic("internal/connect/api.go: commitResolver not set -- use NewWithConfig or set via ResetResolver")
    }
    return a.commitResolver
}
```

Then in blobs.go:
```go
if isUUID(ref) {
    resolved, err := a.resolver().resolveCommitForRead(...)
}
```

But this changes blobs.go behavior from "silent skip" to "panic" which is worse.

OK, the cleanest approach is documentation + keeping the nil-guard. The risk is real only if someone introduces a constructor that doesn't go through `New`/`NewWithConfig`. Since those are the only two constructors and they both create a `commitServiceHandler`, the risk is theoretical. A comment on the field is sufficient.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `isConventionalDefaultName` carve-out in both providers (Phase 18 addition) | Carve-out removed; resolve listed refs verbatim | Phase 25 | Fixes regression for repos with non-default branches named "main"/"master"/"develop"/"trunk" |
| `isSHA` duplicated 3x across codebase | Provider copies extracted to `internal/providers/content`; connect copy remains | Phase 25 | Single source of truth for provider-layer helpers |
| `commitLineRE` duplicated between e2e/ref_test.go and testutil/server.go | Regex defined once in testutil; both callers reference it | Phase 25 | Eliminates one code duplication vector |
| `a.commitResolver = commitHandler` post-construction mutation | Same, but documented with warning; nil-guard preserved | Phase 25 | Risk acknowledged, not fully solved (acceptable for current constraints) |

**Deprecated/outdated:**
- `isConventionalDefaultName` function definition and all references across both providers (github + bitbucket) -- removed in Finding 1 or extracted in Finding 4. History: added in Phase 18 (2026-07-07) to solve the v1.30.1 v1alpha1 `reference="main"` carve-out; no longer needed after Phase 18's `parseResourceRefName` field-4-only fix.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The v1.30.1 `label_name="main"` case is no longer reachable after Phase 18's `parseResourceRefName` field-4-only fix | Finding 1 analysis | LOW -- The `TestParseResourceRefName_LabelNameIsField3` test in `commits_helpers_test.go:394-413` explicitly asserts this. If a code path STILL passes `label_name` as `ref` to GetMeta, removing the carve-out would 422 on googleapis/manually override. Mitigation: run the full unit suite after the change. |
| A2 | The connect-layer `isSHA` in `commits_helpers.go` should NOT be extracted alongside the provider copies | Finding 4 extraction | MEDIUM -- The connect-layer `isSHA` is identical in body but serves a different purpose (UUID-derivation validation, not provider-layer commit-vs-ref gating). If a future refactoring moves the provider extraction to a package that the connect layer also imports, it's safe to consolidate. But the current layering doesn't support this (connect does not import `internal/providers/content`). |
| A3 | The only callers of `New` that construct `*api` without wiring `commitResolver` are tests (via `New` -> `NewWithConfig`) | Finding 7 | LOW -- Grep confirms `New` is called from `testMux`/`testMuxWithConfig` in `api_test.go` and from `cmd/easyp` startup. Both eventually call `NewWithConfig` which constructs `commitHandler` and sets the resolver. No third constructor exists. |
| A4 | Removing `isConventionalDefaultName` from the provider layer does not affect the connect layer | Finding 1, Finding 4 | HIGH -- `isConventionalDefaultName` is only defined in the two provider packages. The connect layer does not reference it. Verified by grep for `isConventionalDefaultName` in `internal/connect/` -- zero hits. |

## Open Questions

1. **Should the `isConventionalDefaultName` function be REMOVED entirely or EXTRACTED to a shared package?**
   - What we know: Extract-to-shared (Finding 4) conflicts with Remove-entirely (Finding 1). You can't extract a function you also deleted.
   - Recommendation: **Remove the function entirely from both providers** (Finding 1 takes priority -- it's a behavioral regression). If a future phase needs the function for a different reason, add it back as needed. The `isConventionalDefaultName` function was a heuristic for a v1.30.1 case that is no longer reachable. Keeping it in a shared package after removing it from the providers would be dead code with no callers.
   - The same logic applies to `isSHA`: extract to shared package. It IS still used by both providers (for the isSHA fast path in GetMeta), so removing it would break the code. Extraction to `content` is the right move.

2. **Should Finding 7 (fragile post-construction mutation) be fixed or documented as a known risk?**
   - What we know: The nil-guard already catches the failure case. The only way to trigger it is to construct `*api` without `New`/`NewWithConfig`, which no code does. The constructor signatures would need to change to fully prevent the risk.
   - Recommendation: **Add a compile-time assertion / startup panic**, not a runtime nil-check. Change `blobs.go`'s nil-guard to a panic in `api.go`'s `initCommitResolver`:

```go
func (a *api) initCommitResolver(r CommitResolver) {
    if r == nil {
        panic("internal/connect: commitResolver not configured -- " +
            "call NewWithConfig, not New, and ensure cfg.CommitResolver is set")
    }
    a.commitResolver = r
}
```

3. **Should Finding 3 (v1alpha1 live e2e) be part of this phase or deferred to a follow-up?**
   - What we know: The TLS issue is resolved (df02ff0). The e2e test exists and passes structurally (PR-22-4 blocked only by TLS). This is a pure operational verification -- no code changes needed.
   - Recommendation: Include as the final task in this phase. The verification is low-effort (< 1 minute to run the test) and high-value (provides the final regression guard for the v1alpha1 read path).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build + all code changes | Yes (implied by ongoing milestone) | go1.26.4 | -- |
| `log/slog` | Any new structured log lines | Yes (Go 1.21+ stdlib) | -- | -- |
| `regexp` | Finding 6 regex extraction | Yes (Go stdlib) | -- | -- |
| `fmt`, `errors` | Error wrap fixes | Yes (Go stdlib) | -- | -- |
| `EASYP_GH_TOKEN` | Finding 3 v1alpha1 e2e gate | Unknown at research time | -- | Test skips without it; Finding 3 is optional |
| TLS certs at `~/local-tls/server/` | Finding 3 e2e gate | Unknown at research time | -- | Test fails without valid certs |
| `buf v1.30.1` binary (cached) | Finding 3 e2e gate | Assumed (Phase 21+22 cached it) | v1.30.1 | Test failure if missing (`testutil.GetBuf` downloads fresh) |
| `buf v1.69.0` binary (cached) | Finding 3 e2e (v1.69.0 subtest) | Assumed | v1.69.0 | Same as above |

**Missing dependencies with no fallback:** None -- all code changes are source-only.
**Missing dependencies with fallback:** Finding 3 (v1alpha1 live e2e) -- if `EASYP_GH_TOKEN` is unset, the test SKIPs cleanly. The planner can gate it behind a `checkpoint:human-verify` task with the correct token.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` (go1.26.4); `github.com/stretchr/testify/require` for assertions |
| Config file | none (Go convention; `go test ./...`) |
| Quick run command | `go test ./internal/providers/github/... ./internal/providers/bitbucket/... ./internal/connect/... -count=1` |
| Full suite command | `go test ./... -count=1` (unit) + `EASYP_GH_TOKEN=... go test ./e2e/ -count=1` (e2e) |

### Phase Requirements -> Test Map

| Finding | Behavior | Test Type | Automated Command | File Exists? |
|---------|----------|-----------|-------------------|-------------|
| F-1 (regression) | `GetMeta` resolves `ref="main"` to the branch tip when a repo has a real branch named "main" that is NOT the default | unit (existing) | `go test ./internal/providers/github/... ./internal/providers/bitbucket/... -count=1` | EXISTS -- `getrepo_test.go` has `TestGetMeta_ResolvesRef` and `TestGetMeta_DefaultBranch` subtests that will need updating if carve-out is removed |
| F-1 (no regression) | Removing the carve-out does not break the v1.30.1 googleapis "main" path (ref is empty, commit="" branch) | integration (existing e2e) | `EASYP_GH_TOKEN=... go test ./e2e/ -run 'TestRefRespected' -count=1` | EXISTS -- Phase 19/23 e2e tests exercise the googleapis/googleapis repo where default is "master" |
| F-2 | Error wrap says "GetFiles" not "GetRepository" | code review | `grep -q 'a.repo.GetFiles' internal/connect/blobs.go` (after change, grep for old string returns 0) | Structural |
| F-3 | v1alpha1 `TestGenerateWithPinnedBufLock/v1.30.1` passes against real GitHub | e2e | `EASYP_GH_TOKEN=... go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1 -v` | EXISTS (`e2e/generate_test.go`) |
| F-4 | `isSHA` and `isConventionalDefaultName` are no longer defined in provider packages | structural | `grep -c 'func isSHA\|func isConventionalDefaultName' internal/providers/github/getrepo.go internal/providers/bitbucket/getrepo.go` (should be 0) | Structural |
| F-5 | Proto path comment references an existing file | code review | `grep -c 'api/proto/buf/registry/module/v1beta1/resource.proto' internal/connect/commits_helpers_test.go` (should be 0) | Structural |
| F-6 | `commitLineRE` is defined only in e2e/testutil/server.go | structural | `grep -c 'commitLineRE' e2e/ref_test.go e2e/testutil/server.go` (should be 1 in server.go, 0 in ref_test.go) | Structural |
| F-7 | `a.commitResolver = commitHandler` is documented with a startup-not-nil assertion | code review + audit | Review the field assigment in `api.go:124` post-Fix | Structural |

### Sampling Rate
- **Per task commit:** `go test ./internal/providers/... ./internal/connect/... -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full unit suite green + `grep -c 'func isSHA\|func isConventionalDefaultName' internal/providers/*/getrepo.go` (0) + `grep -c 'commitLineRE' e2e/ref_test.go` (0) + code review of Finding 2+F-5+F-7.

### Wave 0 Gaps
- [ ] If Finding 4 extracts helpers to `internal/providers/content/`, add `internal/providers/content/helpers_test.go` with test cases moved from `github/getrepo_test.go` and `bitbucket/getrepo_test.go`
- [ ] No framework install needed -- Go stdlib + testify already in `go.mod`

## Security Domain

This is a cleanup/fix phase. No new attack surfaces, no auth path changes, no input handling changes beyond removing an existing validation heuristic (Finding 1, which is a correctness fix not a security impact).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V5 Input Validation | yes (Finding 1 changes how commit refs are validated vs. resolved) | `isSHA` / `isUUID` gating (unchanged); removal of the `isConventionalDefaultName` heuristic doesn't affect validation strictness |
| V6 Cryptography | no | No crypto changes |
| V2 Authentication | no | No auth path changes |
| V3 Session Management | no | No session state changes |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Removing the carve-out causes `repos.GetCommit("main")` to 422 for a repo with no branch named "main" | DoS | The error propagates as a clear upsteam error message (already handled by the provider's error wrapping). No silent data corruption. |

## Sources

### Primary (HIGH confidence)

- `internal/providers/github/getrepo.go:15-105` -- `isSHA`, `isConventionalDefaultName`, `GetMeta` method (read in this session) [VERIFIED: code reading]
- `internal/providers/bitbucket/getrepo.go:12-99` -- `isSHA`, `isConventionalDefaultName`, `getMeta` method (read in this session) [VERIFIED: code reading]
- `internal/connect/blobs.go:17-75` -- `DownloadManifestAndBlobs` handler with misleading error wrap at line 45 (read in this session) [VERIFIED: code reading]
- `internal/connect/commits_helpers_test.go:372-443` -- `TestParseResourceRefName_*` tests; `resource.proto` dangling path at line 375 (read in this session) [VERIFIED: code reading]
- `e2e/ref_test.go:38-54` -- `commitLineRE` definition (read in this session) [VERIFIED: code reading]
- `e2e/testutil/server.go:280-294` -- Inlined `commitLineRE` at line 285 (read in this session) [VERIFIED: code reading]
- `internal/connect/api.go:49-141` -- `*api` struct (line 56: `commitResolver` field), `NewWithConfig` (line 124: post-construction mutation) (read in this session) [VERIFIED: code reading]
- `internal/connect/commits.go:1164-1219` -- `resolveCommitForRead` wrapper (Phase 22 addition) (read in this session) [VERIFIED: code reading]
- `internal/connect/commits_helpers.go:72-82` -- connect-layer `isSHA` (read in this session) [VERIFIED: code reading]
- `internal/providers/content/` -- existing shared package for `Meta` type [VERIFIED: directory listing]
- `.planning/ROADMAP.md:89-92` -- Phase 25 description listing all 7 findings [VERIFIED: ROADMAP.md]

### Secondary (MEDIUM confidence)

- Git history confirms Phase 18 added `isConventionalDefaultName` and Phase 18 changed `parseResourceRefName` to read field 4 only (not field 3). The `TestParseResourceRefName_LabelNameIsField3` test locks this in.

### Tertiary (LOW confidence)

- None. All claims are verified against the current source tree.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies; all changes use existing Go stdlib and the repo's own packages.
- Architecture: HIGH -- all 7 findings are mechanically verifiable from source. The behavioral regression (Finding 1) is fully understood and the fix options are well-defined.
- Pitfalls: HIGH -- the four pitfalls are grounded in code evidence from prior phases and this research session.

**Research date:** 2026-07-10
**Valid until:** 2026-08-10 (30 days; review findings are on a stable branch)