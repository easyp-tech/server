---
phase: 24-resolve-buf-cid-ref-in-servegraph-honor-pinned-commit
reviewed: 2026-07-09T00:00:00Z
depth: standard
files_reviewed: 4
files_reviewed_list:
  - internal/connect/commits.go
  - internal/connect/api.go
  - internal/connect/api_test.go
  - e2e/generate_test.go
findings:
  critical: 1
  warning: 4
  info: 1
  total: 6
status: issues_found
---

# Phase 24: Code Review Report

**Reviewed:** 2026-07-09
**Depth:** standard
**Files Reviewed:** 4
**Status:** issues_found

## Summary

Phase 24 introduces a `cidSha` map and a `resolveUUIDRef` branch in `ServeGraph`
so that requests pinning a buf-issued 32-hex cid are no longer forwarded to
upstream as raw SHAs (which GitHub 422s) and no longer served from a stale
HEAD-minted `infoCache` entry. The `ServeDownload` files-cache hit is now
cid-gated, and the fetch path prefers `cidSha` over `cached.commit`. The
direction is correct and the unit tests cover the warm-cache, cold-cache
(prefix-probe), and wrong-infoCache scenarios well.

One BLOCKER remains: the new cold-cache prefix-probe path in `resolveUUIDRef`
was added **outside** the carefully-built `probeCommitID` sandbox. It bypasses
`probeSem` (the only bound on upstream probe fan-out), `missCache` (the only
defense against repeated probing of known-bogus ids), and `probeTimeout` (the
only per-call bound on hanging upstreams). The Phase 18 comment on `probeSem`
explicitly calls out "an attacker can trigger by flooding distinct unknown
shas" — Phase 24 re-opens that exact hole for 32-hex cids on the `ServeGraph`
path.

Secondary concerns: an asymmetric cid-gate that lets a pinned-cid resolution
silently poison subsequent HEAD/SHA requests; an upstream probe with no
timeout; and a weak content assertion in the e2e gate.

## Critical Issues

### CR-01: `resolveUUIDRef` prefix-probe bypasses `probeSem`, `missCache`, and `probeTimeout`

**File:** `internal/connect/commits.go:1004-1037` (called from `ServeGraph` at `:371`)

**Issue:**
The new cold-cache branch of `resolveUUIDRef` recovers a 28-hex SHA prefix via
`commitUUIDInverse` and calls `h.api.repo.GetMeta(ctx, ref.owner, ref.module, prefix)`
directly. This is the same shape of upstream probe that `probeCommitID` performs,
but it skips every safety mechanism `probeCommitID` was instrumented with:

- **No `probeSem` acquire** (`commits.go:1212-1219`). `probeSem` (cap
  `maxConcurrentProbes = 4`) is the explicit bound on simultaneous upstream
  probes. An unauthenticated client flooding `GraphService/GetGraph` with
  distinct 32-hex cids causes one upstream GetMeta per request, with no
  concurrency cap and no negative caching — exactly the amplification the
  `probeSem` comment at `:71-74` says it exists to prevent.
- **No `missCache` / `rememberMiss`** (`commits.go:1041-1052`). Every retry of
  an unknown cid re-probes upstream forever; the TTL-based negative cache that
  `probeCommitID` populates is never consulted on this path.
- **No `context.WithTimeout(ctx, h.probeTimeout)`** (contrast `:1243`). A slow
  or hung upstream stalls the request for as long as the client's request
  context allows. `probeTimeout` exists precisely for this and is ignored.
- **No transient-error classification** (`isTransientErr`, `:1331`). A
  network blip is indistinguishable from a definitive not-found, so the cid is
  permanently treated as "no upstream owner" until the next request retries.

The cold path is reachable by any client — `ServeGraph` is on the unauthenticated
v1/v1beta1 mux — and the cost per request is one upstream round-trip per
distinct cid, unbounded.

**Fix:** route the cold-cache prefix probe through `probeCommitID` (which
already handles UUID inputs via `commitUUIDInverse` at `:1194-1201` and writes
`cidSha[id] = sha` via `registerResolvedAlias` at `:1307-1309`), or replicate
the `probeSem` acquire, the `missCache` check, the `context.WithTimeout`, and
the `transient` classification inside `resolveUUIDRef`. Concretely:

```go
func (h *commitServiceHandler) resolveUUIDRef(ctx context.Context, ref moduleRef, cid string) (string, bool) {
    if cid == "" {
        return "", false
    }
    if sha, ok := h.cidShaLookup(cid); ok && sha != "" {
        return sha, true
    }
    if h.missCached(cid) {
        return "", false
    }
    prefix, err := commitUUIDInverse(cid)
    if err != nil {
        h.rememberMiss(cid)
        return "", false
    }
    if h.probeSem != nil {
        select {
        case h.probeSem <- struct{}{}:
            defer func() { <-h.probeSem }()
        default:
            return "", false
        }
    }
    pctx, cancel := context.WithTimeout(ctx, h.probeTimeout)
    defer cancel()
    meta, err := h.api.repo.GetMeta(pctx, ref.owner, ref.module, prefix)
    if err != nil {
        if !isTransientErr(err) {
            h.rememberMiss(cid)
        }
        return "", false
    }
    if meta.Commit == "" || !strings.HasPrefix(meta.Commit, prefix) {
        h.rememberMiss(cid)
        return "", false
    }
    h.commitMu.Lock()
    h.cidSha[cid] = meta.Commit
    h.commitMu.Unlock()
    return meta.Commit, true
}
```

## Warnings

### WR-01: Asymmetric infoCache cid-gate — pinned-cid resolution poisons subsequent HEAD/SHA/tag requests

**File:** `internal/connect/commits.go:331`

**Issue:**
The new gate is `ok && (!isUUID(ref.ref) || cached.commitID == ref.ref)`. It
correctly prevents a HEAD-minted cache entry from being served to a
pinned-cid request, but the reverse direction is unprotected. Trace:

1. Client pins cid X for `owner/module`. `ServeGraph` resolves, then writes
   `infoCache["owner/module"] = {commitID: X, commit: shaX, ...}` at `:467-477`.
2. A later request for the same module with `ref.ref == ""` (HEAD) or a
   40-hex SHA arrives. `isUUID("")` is false → `!isUUID(ref.ref)` is true →
   the gate short-circuits to HIT and serves cid X's content/digest.

Once any pinned cid has been resolved for a module, the proxy is sticky to
that commit for every subsequent HEAD/SHA/tag request on that module until
the process restarts. For `googleapis/googleapis` (the documented primary
fixture) this means `buf dep update` against HEAD silently returns the
previously-pinned tag's digest. The existing unit test
`TestServeGraph_InfoCacheMustNotServeWrongCommit` only exercises the
pinned-after-HEAD ordering; the HEAD-after-pinned ordering is untested and
broken.

**Fix:** make the gate symmetric — also miss when the request ref is non-cid
(empty/SHA/tag) but the cached entry was minted for a cid:

```go
cachedIsUUID := isUUID(cached.commitID)
requestIsUUID := isUUID(ref.ref)
if ok && (!requestIsUUID && !cachedIsUUID || requestIsUUID && cached.commitID == ref.ref) {
    // serve cached
}
```

Add a regression test that primes with a cid and then asserts a HEAD request
re-resolves rather than serving the cached cid's content.

### WR-02: `resolveUUIDRef` GetMeta call has no per-call timeout

**File:** `internal/connect/commits.go:1021`

**Issue:**
Even setting CR-01 aside, the GetMeta in `resolveUUIDRef` uses only `ctx` (the
request context). `probeCommitID` wraps every per-source GetMeta with
`context.WithTimeout(ctx, h.probeTimeout)` at `:1243` precisely because request
contexts alone are an insufficient bound on upstream hangs (proxies behind
proxies, idle-pinned connections, etc.). The new path regressed that defense.

**Fix:** wrap the call as in the snippet in CR-01.

### WR-03: `ServeDownload` fetch path does not refresh `infoCache` after resolving a pinned cid

**File:** `internal/connect/commits.go:752-754`

**Issue:**
After resolving a pinned cid and computing `cid = commitUUID(meta.Commit)`,
the fetch path writes only `h.cidSha[cid] = meta.Commit`. It does NOT update
`infoCache[owner/module]` (still holds the stale HEAD entry that triggered
the cache-miss) and does NOT update `filesMap[cid]`. As a result every
subsequent Download for the same pinned cid:

- hits `commitMap` (or foreign fallback) and lands at the cid-gate at `:683`,
- sees `cached.commitID == commitID` fail (cache still holds the old cid),
- falls through to the fetch path,
- re-runs `cidShaLookup` (hit) → GetMeta → GetFiles → re-computes digest,
  every time.

The Phase 24 unit test `TestServeDownload_PinnedCidNotServedFromWrongInfoCache`
happens to assert correctness on a single request and so does not surface
this. The fix is to write back both `infoCache` and `filesMap` after a
successful pinned-cid fetch (mirroring `:752-754` in `ServeGraph` at `:467-477`
and in `ServeHTTP` at `:231-241`).

**Fix:**
```go
h.commitMu.Lock()
h.cidSha[cid] = meta.Commit
h.infoCache[ref.owner+"/"+ref.module] = commitInfoCache{
    commitID: cid,
    commit:   meta.Commit,
    ownerID:  cached.ownerID,
    moduleID: cached.moduleID,
    digest:   digest,
}
h.filesMap[cid] = files
h.commitMu.Unlock()
```

(Note: this also closes the WR-01 asymmetry from the ServeDownload side, since
the next pinned-cid request will then hit the files-cache directly.)

### WR-04: `TestGeneratePinnedCommit_NotHEAD` content assertion is too weak to detect HEAD serving

**File:** `e2e/generate_test.go:160-169`

**Issue:**
The test asserts every generated file contains `package google.type`. That
package marker exists in `googleapis/googleapis` at HEAD, at
`common-protos-1_3_1`, and at every tag in between. A bug where the proxy
served HEAD's content under the pinned cid would still produce files
containing `package google.type` and pass this assertion. The only
load-bearing check is the indirect server-log substring check at `:179`
(`strings.Contains(srvOut, pinnedSHA)`) — which proves the pinned SHA was
*logged*, not that its content was *served*.

Combined with WR-01 (where a prior pinned-cid request poisons HEAD cache
entries) a regression could slip through this e2e gate.

**Fix:** pin the assertion to a file or symbol that differs between HEAD and
the pinned tag. Either (a) generate to a temp dir and diff against a fresh
`buf generate` run pointed directly at GitHub (the ground-truth content for
the same pinned SHA), or (b) assert the server log carries the pinned SHA in
a `commit` attribute of a `files_cache_hit`/`info_cache_writeback` decision
line (proving it was the resolution target), not just anywhere in the log.

## Info

### IN-01: `commitUUIDInverse` relies on 28-hex prefix uniqueness inside one repo

**File:** `internal/connect/commits_helpers.go:116-132` (used at `commits.go:1017`)

**Issue:**
`commitUUIDInverse` recovers only the first 14 bytes of the original SHA. The
prefix-match validation in `resolveUUIDRef` (`:1025`) and `probeCommitID`
(`:1259`) trusts that at most one commit in the upstream repo starts with
those 28 hex chars. For SHA-1 the collision space is 2^112 — acceptable — but
the assumption is implicit and undocumented at the call site. If a future
provider returns the *wrong* commit among two that share the prefix, the
cid→sha cache will silently pin the wrong content.

**Fix:** add a one-line invariant comment at `:1017` and `:1195` noting the
2^112 assumption, and consider logging at debug level when the prefix probe
returns a commit so an operator can correlate if a collision is ever
suspected. Low priority — the probability is negligible; this is
documentation hardening, not a defect.

---

_Reviewed: 2026-07-09_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
