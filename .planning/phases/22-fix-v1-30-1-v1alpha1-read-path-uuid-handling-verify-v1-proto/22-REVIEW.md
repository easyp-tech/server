---
phase: 22-fix-v1-30-1-v1alpha1-read-path-uuid-handling-verify-v1-proto
reviewed: 2026-07-08T00:00:00Z
depth: standard
files_reviewed: 4
files_reviewed_list:
  - internal/connect/blobs_test.go
  - internal/connect/blobs.go
  - internal/connect/api.go
  - internal/connect/commits.go
findings:
  critical: 1
  warning: 6
  info: 4
  total: 11
status: issues_found
---

# Phase 22: Code Review Report

**Reviewed:** 2026-07-08
**Depth:** standard
**Files Reviewed:** 4
**Status:** issues_found

## Summary

The phase-22 change wires the v1alpha1 `DownloadManifestAndBlobs` handler to the
Phase-18 commit-id resolution ladder (`commitMap` → single-module foreign-id
fallback → `probeCommitID`) via a new `CommitResolver` interface on `*api`. The
wiring itself is correct and the three new tests cover warm-cache, cold-cache
probe, and resolution-failure paths.

The ladder has, however, one real data-correctness defect
(`registerResolvedAlias` reuses a stale digest when it mutates an existing
`infoCache` entry) and several edge-case issues around multi-module behavior,
error status mapping, and dead defensive branches. The new code is also
inconsistent about whether resolution is scoped to the client-supplied
`owner`/`module` or ignores it.

## Critical Issues

### CR-01: `registerResolvedAlias` keeps a stale `digest` (and `filesMap`) when it mutates an existing `infoCache` entry to a different commit

**File:** `internal/connect/commits.go:1160-1182`
**Issue:**

`registerResolvedAlias` is called from `probeCommitID` after a UUID/SHA probe
succeeds. When the resolved `owner/module` already has an `infoCache` entry
(the common case — anything that has served a `GetCommits`/`GetGraph` request
previously), the update path preserves the previous `digest`:

```go
if existing, ok := h.infoCache[key]; ok {
    existing.commitID = id
    existing.commit = sha           // commit changed
    existing.ownerID = owner
    existing.moduleID = key
    h.infoCache[key] = existing     // existing.digest kept verbatim
}
```

`commitInfoCache.digest` is not refreshed, and `h.filesMap` still holds the old
`commitID → files` mapping (the new `id`/`sha` have no `filesMap` entry). When
the next request that reads `infoCache[key].digest` arrives — e.g.
`ServeGraph`'s cache-hit branch at `commits.go:326-334` (which emits
`digest: cached.digest`) — it returns the digest that belonged to the *old*
commit, not the one the probe just resolved. The same staleness flows into
every `commitMap[oldCommitID]` caller that consults `infoCache` for the digest.

Concrete trigger: any non-HEAD commit of the same module recovered through
the probe path (probe resolves UUID→SHA where SHA ≠ the cached HEAD commit)
immediately leaves a mismatched `(commit, digest)` pair in `infoCache`.

The fresh-branch in the same function (the `else`) omits `digest` entirely, so
fresh entries also report a zero digest until something else fills it in.

**Fix:**

Drop the `digest` from the mutated entry (force callers to recompute), or
explicitly clear it and the corresponding `filesMap` alias so subsequent
`ServeDownload`/`ServeGraph` paths take the cache-miss branch and recompute:

```go
if existing, ok := h.infoCache[key]; ok {
    existing.commitID = id
    existing.commit = sha
    existing.ownerID = owner
    existing.moduleID = key
    existing.digest = nil // force recompute; the old digest belonged to a different commit
    h.infoCache[key] = existing
}
// also invalidate any filesMap alias for the previous commitID, since the
// content for the new commit has not been fetched yet.
```

Alternatively, do not overwrite an existing entry unless `sha == existing.commit`.

## Warnings

### WR-01: `resolveCommitForRead` ignores its `owner`/`module` arguments, but `blobs.go` then calls `GetFiles` with the *client's* owner/repo + the resolved SHA

**File:** `internal/connect/commits.go:961-963`, `internal/connect/blobs.go:34-43`
**Issue:**

The new method receives `owner, module, id` but only `id` is consulted; the
resolution ladder (`commitMap` → `resolveForeignCommitID` → `probeCommitID`)
resolves to *some* module whose source owns the SHA, regardless of what the
client asked for. `blobs.go` then passes the resolved SHA back into:

```go
files, err := a.repo.GetFiles(ctx, req.Msg.GetOwner(), req.Msg.GetRepository(), ref)
```

In a multi-module deployment where the probe finds the SHA in module B but the
client asked for module A, `GetFiles(A_owner, A_repo, sha_from_B)` will fail
(git 404 / "commit not found") and the request 502s with a misleading
"upstream" error class — even though resolution itself succeeded. The
ServeDownload path does not have this problem because it uses the resolved
`ref.owner/ref.module`; the v1alpha1 path loses that information because
`resolveCommitForRead` returns only a string.

**Fix:**

Either have `CommitResolver.resolveCommitForRead` return the resolved
`(owner, module, sha)` so `blobs.go` can pass the resolved module to
`GetFiles`, or explicitly document and assert the single-module invariant
(reject the resolution when the resolved module ≠ the requested one) instead
of silently issuing a cross-module `GetFiles`.

### WR-02: Resolution failure is mapped to `CodeInternal` (HTTP 500) instead of a not-found code

**File:** `internal/connect/blobs.go:37-39`, `internal/connect/validate.go:47-55`
**Issue:**

When `resolveCommitForRead` cannot resolve the id it returns a wrapped error.
`blobs.go` passes that through `asConnectError`, which (because the error is
not an `IsValidationError` sentinel) maps it to `connect.CodeInternal` →
HTTP 500. The buf CLI treats 500 as a server failure and may retry; the
correct semantic for "no configured source owns this id" is a not-found /
invalid-argument. This is also inconsistent with the v1beta1
`ServeDownload` path, which surfaces the same condition as HTTP 400 with the
helpful message `"unknown commit id: re-resolve via buf mod update / buf dep
update"` (`commits.go:597`).

**Fix:**

Define the resolution miss as a validation error (or use
`connect.CodeNotFound`) and map it explicitly:

```go
if err != nil {
    return nil, connect.NewError(connect.CodeNotFound,
        fmt.Errorf("resolving commit uuid %q: %w", ref, err))
}
```

### WR-03: `resolveForeignCommitID` registers a `moduleRef` that loses the `ref` (branch) field

**File:** `internal/connect/commits.go:882-899`
**Issue:**

The alias registered into `commitMap` is constructed as
`moduleRef{owner: owner, module: module}` with no `ref`. The `moduleRef.ref`
field (`commits_helpers.go:11-19`) carries the branch/tag the client asked
for and is consulted by providers (HEAD vs. named branch). After this
fallback runs, every subsequent caller that pulls the alias out of
`commitMap` and forwards `ref.ref` upstream sees `""`, which providers
treat as HEAD. In single-module deployments this is masked because callers
fetch by the resolved SHA directly, but the loss is silent and
non-obvious — the alias is not equivalent to the entry that would have been
written by `ServeHTTP`/`ServeGraph`.

**Fix:**

Either carry the `ref` through from the prior `infoCache` entry, or make the
registration explicit that `ref=""` (and add an assertion/log so a future
caller that starts depending on `ref.ref` does not silently regress).

### WR-04: `resolveCommitForRead` reads `infoCache` without checking the `ok` flag, then drops the missing-entry case on the floor

**File:** `internal/connect/commits.go:965-984`
**Issue:**

In steps (a), (b), and (c) the code does:

```go
info := h.infoCache[ref.owner+"/"+ref.module]
if info.commit != "" {
    return info.commit, nil
}
```

If the entry is absent (`ok == false`), `info` is the zero value and
`info.commit == ""`, so the code silently falls through to the next step
without distinguishing "entry exists with empty commit" from "no entry at
all". In a code path whose entire purpose is correctness around partial
cache state, that ambiguity is a real defect magnet (e.g., a future caller
that pre-populates `commitMap` without `infoCache` would never hit the
probe, with no log line explaining why).

**Fix:**

Check `ok` explicitly and log the cache-miss branch so the trace shows
*why* the next ladder step was entered:

```go
if info, ok := h.infoCache[ref.owner+"/"+ref.module]; ok && info.commit != "" {
    return info.commit, nil
}
```

### WR-05: Dead defensive nil-guard `a.commitResolver != nil`

**File:** `internal/connect/blobs.go:33`
**Issue:**

`api.commitResolver` is always assigned in `NewWithConfig`
(`api.go:123`), and `New` simply delegates to `NewWithConfig`
(`api.go:66-73`). There is no production or test path on which
`a.commitResolver == nil`. The guard therefore never fires, and worse,
if it ever did (e.g., via a future test that constructs `*api` directly),
the handler would fall through to `GetFiles` with a 32-char UUID as the
commit argument — exactly the GitHub-404 bug phase 22 was created to fix.
A "defense-in-depth" branch whose failure mode is the bug being defended
against is worse than no guard.

**Fix:**

Drop the guard (`if isUUID(ref) {`), or panic if `commitResolver` is nil
when a UUID arrives, so a regression in wiring surfaces immediately
instead of silently re-introducing the original defect.

### WR-06: `probeCommitID` silently keeps only the first of multiple successful probes

**File:** `internal/connect/commits.go:1137-1144`
**Issue:**

The docstring claims "at most one source succeeds — no cross-module
ambiguity", but `commitUUIDInverse` recovers only the first 14 bytes (the
code's own comment acknowledges 2^112 collision surface), and a mirrored
SHA on two configured sources is plausible in real topologies. The loop

```go
for r := range results {
    h.registerResolvedAlias(id, r.commit, r.ref.owner, r.ref.module)
    ref := r.ref
    return &ref, true
}
```

returns the first result received, where "first" is determined by goroutine
scheduling (the channel is buffered to `len(sources)`, so send order is
nondeterministic). Two runs of the same request can therefore resolve to
different modules, with no log line flagging the ambiguity.

**Fix:**

Collect all successful results; if more than one distinct module matches,
either pick deterministically (e.g., the source ordered first in
`Repositories()`) or refuse the probe and surface an error. At minimum, log
when the assumption is violated so operators can detect mirror
configurations that break the unique-ownership invariant.

## Info

### IN-01: Misleading error message in `blobs.go`

**File:** `internal/connect/blobs.go:45`
**Issue:** Error is wrapped as `"a.repo.GetRepository: %w"` but the call is
to `a.repo.GetFiles`. Operators grepping logs for `GetFiles` failures will
miss this line.
**Fix:** `fmt.Errorf("a.repo.GetFiles: %w", err)`.

### IN-02: Duplicated doc-comment block on `probeCommitID`

**File:** `internal/connect/commits.go:1004-1039`
**Issue:** Two `// probeCommitID resolves ...` blocks appear back-to-back
above the function. The first describes the pre-UUID behavior, the second
extends it; they overlap and partially contradict each other (e.g., the
first says "A git sha is unique to one repo", the second introduces the
UUID/prefix path).
**Fix:** Merge into a single doc comment.

### IN-03: `commitUUIDInverse` allocates a 20-byte array it never fully uses

**File:** `internal/connect/commits_helpers.go:127-131`
**Issue:** `var sha [20]byte` is allocated, but only positions `0..13`
are written and `sha[:14]` is returned. The 20-byte size is correct for a
SHA-1 but misleading when the original SHA could have been 64 chars
(SHA-256). The function works correctly because only the first 14 bytes
are needed regardless of SHA length, but the choice of `[20]byte` reads as
"this is reconstructing a SHA-1", which is not what's happening.
**Fix:** Use `var sha [14]byte` to match the actual slice length, or
document why 20 was chosen.

### IN-04: Test mock accepts prefix lengths that production rejects

**File:** `internal/connect/api_test.go:99`
**Issue:** `mockSource.GetMeta` accepts any `commit` such that
`strings.HasPrefix(s.commit, commit)`, including 1-character prefixes.
Production GitHub requires short SHAs of ≥7 characters; the mock is more
permissive than the real provider, so a test could pass with a prefix that
would fail end-to-end. The current tests use 28-char prefixes so they
happen to be valid, but the mock's looseness is a latent test-reliability
hazard.
**Fix:** Tighten the mock to enforce a minimum prefix length (e.g.,
`len(commit) >= 7`) so a future test that uses a too-short prefix fails
loudly instead of passing mock-only.

---

_Reviewed: 2026-07-08_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
