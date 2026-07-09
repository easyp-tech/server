# Phase 24 Research — Resolve buf cid ref in ServeGraph; honor pinned commit

## Origin

Prod `buf generate` failure for `grpc-ecosystem/grpc-gateway` pinned in buf.lock at
`commit: e91b8a68fe214081808d79f1a1a4f09e`. Client error:
`could not get module data ... no content returned for commit ID e91b8a68-fe21-4081-808d-79f1a1a4f09e`.
Full diagnosis in `.planning/debug/buf-cid-ref-forwarded-to-upstream.md`.

Two defects, same origin: proxy mints a lossy 32-hex buf commit_id from a git SHA but
never retains a cid→full-git-SHA map, so it cannot honor a commit-pinned (buf.lock) dep.

## Facts established

- `commitUUID(sha)` ([commits_helpers.go:45](../../../internal/connect/commits_helpers.go#L45)) is lossy:
  keeps sha[0:6], overwrites byte 6 with `0x40` (UUID v4 version), keeps sha[6] at byte 7,
  overwrites byte 8 with `0x80` (RFC4122 variant), keeps sha[7:14] at bytes 9-15, drops sha[14:20].
  ⇒ raw 32-hex cid is NOT a sha prefix (byte 6 differs); GitHub returns 422.
- `commitUUIDInverse(cid)` ([commits_helpers.go:116](../../../internal/connect/commits_helpers.go#L116))
  recovers the real sha[0:14] = **28 hex**. 28 ≥ 7 (GitHub short) and ≥ 11 (Bitbucket short) ⇒
  valid short SHA on both providers. Already used by `probeCommitID`
  ([commits.go:1059-1066](../../../internal/connect/commits.go#L1059-L1066)) in the ServeDownload path.
- Prod proof the pinned commit is fetchable: 6cln8 log L2712-2718 — when ServeDownload's probe
  retried with the full 40-hex sha, GetMeta 200 + GetFiles 32394 bytes.
- Prod proof of wrong-commit serve: fwdnr log 12:24:41 — ServeDownload resolved the pinned cid to
  main HEAD `34a6674c` via the owner/module infoCache.

## Defect 1 — ServeGraph forwards cid to upstream

[commits.go:344](../../../internal/connect/commits.go#L344) `GetMeta(ctx, owner, module, ref.ref)`.
Client sends cached cid as `ref.ref` (buf.lock standard). ServeGraph has no `isUUID`/inverse branch
(unlike ServeDownload) → raw 32-hex to GitHub → 422 → 502.

Reproduced RED by `TestServeGraph_BufCommitIDRefNotForwardedToUpstream` (api_test.go):
GetMeta receives `e91b8a68fe214081808d79f1a1a4f09e`, status 502.

## Defect 2 — infoCache keyed by owner/module only

[commits.go:310-335](../../../internal/connect/commits.go#L310-L335) serves cached commitID/commit/digest
ignoring `ref.ref`. Request pinning cid X can be answered from an entry minted for cid Y (HEAD, tag, …).

Reproduced RED by `TestServeGraph_InfoCacheMustNotServeWrongCommit` (api_test.go): primed HEAD →
pinned-cid request returns HEAD cid `34a6674c253f4028807533e8e904d89e`.

## Fix layers (not either-or)

1. **cid→full-sha map** stored at every mint site (deterministic, zero round-trip, persistable).
   Primary path. Mint sites: ServeGraph writeback [L416-423](../../../internal/connect/commits.go#L416),
   GetCommits [L223](../../../internal/connect/commits.go#L223), ServeDownload [L681](../../../internal/connect/commits.go#L681).
2. **ServeGraph UUID branch**: `isUUID(ref.ref)` → look up cid→sha; on hit use full sha for GetMeta
   (or short-circuit emit cached entry). On miss fall through to `commitUUIDInverse` → 28-hex prefix
   probe (port probeCommitID's technique). Never forward raw cid to GetMeta.
3. **infoCache**: key by `(owner/module, cid)` OR gate the cache hit on `cached.commitID == requested cid`.
   A pinned-cid request must not be served from a differently-minted entry.
4. **ServeDownload foreign path**: prefer cid→sha map over the ambiguous owner/module fallback
   (fwdnr 12:24 served HEAD for a pinned cid).

## Out of scope

- Persistence of cid→sha across pod restart (artifactory). Valuable but separable; the in-process map
  + inverse-prefix fallback already unblock `buf generate` within a warm pod. Persistence tracked as
  a follow-up if cold-pod failures recur after this phase.
- v1alpha1 read-path (covered by Phase 22). This phase targets the v1/v1beta1 GraphService +
  DownloadService Download path exercised by `buf generate` on modern buf CLI.

## Tests

Already RED in api_test.go (confirming defect):
- `TestServeGraph_BufCommitIDRefNotForwardedToUpstream`
- `TestServeGraph_InfoCacheMustNotServeWrongCommit`

To add (green after fix):
- `TestServeGraph_UUIDRefShortCircuitsFromCidShaMap` — cid→sha known ⇒ no upstream call, correct cid returned.
- `TestServeGraph_UUIDRefColdCache_ProbesWithInversePrefix` — cid→sha unknown ⇒ GetMeta called with
  28-hex prefix (not the cid), resolves, caches.
- `TestServeDownload_PinnedCidNotServedFromWrongInfoCache` — pinned cid served with its own sha, not HEAD's.
- e2e: extend `e2e/ref_test.go` / Phase 21 `TestGenerateWithPinnedBufLock` to assert the generated
  content matches the pinned commit (not HEAD).
