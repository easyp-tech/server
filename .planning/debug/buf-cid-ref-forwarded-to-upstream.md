---
slug: buf-cid-ref-forwarded-to-upstream
status: diagnosed
trigger: "Production `buf generate` fails for grpc-ecosystem/grpc-gateway pinned in buf.lock at commit e91b8a68fe214081808d79f1a1a4f09e. Client error: 'no content returned for commit ID e91b8a68-fe21-4081-808d-79f1a1a4f09e'. User surprised: the upstream git commit e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9 exists (https://github.com/grpc-ecosystem/grpc-gateway/commit/e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9)."
created: 2026-07-09
updated: 2026-07-09
goal: find_and_fix
specialist_dispatch_enabled: true
---

# Symptoms

- **Expected:** `buf generate` resolves the grpc-gateway dep pinned in buf.lock and succeeds.
- **Actual:** GraphService/GetGraph returns 502 (fresh pod) OR returns a graph for the wrong commit (warm pod) → client: "could not get module data ... no content returned for commit ID e91b8a68-fe21-4081-808d-79f1a1a4f09e".
- **Error:** prod log: `resolving ref "e91b8a68fe214081808d79f1a1a4f09e": GET https://api.github.com/repos/grpc-ecosystem/grpc-gateway/commits/e91b8a68fe214081808d79f1a1a4f09e: 422 No commit found for SHA`.
- **Timeline:** persists across PR #39 (foreign-cid fallback). Fallback papered over Download only; Graph still broken.
- **Reproduction:** any buf.lock that pins a proxy-minted commit_id (the normal state after `buf dep update`), served by a pod that lacks an in-memory cid→sha mapping for it.

# Evidence

- buf.yaml deps have NO ref; buf.lock pins `commit: e91b8a68fe214081808d79f1a1a4f09e` (proxy-minted 32-hex cid). Client sends this cid back as Name.ref.
- `commitUUID("e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9") == "e91b8a68fe214081808d79f1a1a4f09e"` (verified vs commits_helpers.go:45). cid is lossy: drops sha bytes 14..19.
- Prod log `prod-buf-proxy-...-6cln8...log` line 2216: GetMeta called with `commit:"e91b8a68fe214081808d79f1a1a4f09e"` (the 32-hex cid). Line 2222: github 422. Line 2223: ServeGraph 502.
- Same pod line 2712-2718: when ServeDownload's probe retried with the FULL 40-hex sha `e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9`, GetMeta succeeded and GetFiles returned 32394 bytes — PROOF the commit exists and is fetchable.
- Prod log `prod-buf-proxy-...-fwdnr...log` 12:24:41: ServeDownload `commit_id_lookup` for the cid → `files_cache_lookup` resolved to main HEAD `34a6674c` (info_cache_hit) — wrong commit served under the pinned cid.
- Commits.go:310-335 ServeGraph reads `infoCache[owner/module]` and returns cached commitID/commit/digest **ignoring ref.ref**. Commits.go:344 miss path calls `GetMeta(owner, module, ref.ref)` with ref.ref verbatim — the 32-hex cid.
- Confirming tests (RED, exact repros):
  - `TestServeGraph_BufCommitIDRefNotForwardedToUpstream` — GetMeta receives the cid, status 502.
  - `TestServeGraph_InfoCacheMustNotServeWrongCommit` — primed HEAD cache → pinned-cid request returns HEAD cid.

# Root Cause

Two defects, same origin: **the proxy mints a lossy 32-hex buf commit_id from a git SHA but never retains a cid→full-git-SHA map, so it cannot honor a commit-pinned (buf.lock) dependency.**

1. **ServeGraph forwards the buf cid to the upstream as a git SHA.** When the client sends the cached cid as Name.ref (standard buf.lock behavior), ServeGraph passes it verbatim to `GetMeta`/GitHub. GitHub rejects the 32-hex string (`422 No commit found for SHA`) because the real SHA is 40-hex. ServeGraph has no recovery (unlike ServeDownload's probe path).

2. **infoCache is keyed by owner/module only.** Once any commit for a module is cached (HEAD, a tag, …), a later request pinning a different cid returns the cached wrong commit's id+digest. Client receives a valid graph/download for the wrong commit → buf image build fails → "no content returned for commit ID".

commitUUID is intentionally lossy (drops 6 of 20 SHA bytes), so the full SHA cannot be recovered from the cid alone — the proxy MUST store cid→full-SHA at mint time and never re-derive or forward the cid as a SHA.

# Fix (proposed)

1. **Persist cid→full-git-SHA.** Add `commit string` to the value stored alongside every minted cid (commitMap value / a dedicated `cidSha map[string]string`), populated at all mint sites: ServeGraph writeback (commits.go:416), the GetCommits path (commits.go:223), and ServeDownload's mint (commits.go:681). Survive across the request that minted it at minimum; ideally persist to the artifactory cache alongside the b5 digest so it survives pod restart (the prod failure is partly a cold-cache/restart problem).

2. **ServeGraph: resolve a cid ref before touching upstream.** When `isUUID(ref.ref)`: look up cid→sha; if present, use the full SHA for GetMeta (or short-circuit and emit the cached commit entry directly). Only on a true miss fall through. Never forward a 32-hex cid to `GetMeta`.

3. **infoCache: key by (owner/module, cid) — or check that the cached entry's cid matches the requested ref** before serving. A request pinning cid X must not be answered from an entry minted for cid Y.

4. **ServeDownload foreign-id path:** prefer the cid→sha map over the ambiguous owner/module infoCache (the 12:24 prod log shows the current fallback happily served main HEAD for a pinned-commit request).

Tests already added (RED); they go GREEN once 1+2+3 land. Add: cid→sha survives a simulated restart (persist + reload), and ServeGraph short-circuits without calling upstream when the cid is known.

# Files

- internal/connect/commits.go — ServeGraph (264-444), mint/writeback sites (149/223/352/416/681), ServeDownload foreign path (498-620).
- internal/connect/commits_helpers.go — commitUUID (45), commitUUIDInverse (116), isUUID (87).
- internal/connect/api_test.go — two confirming tests + helpers (buildV1GetGraphRequestWithRef, recordingProvider).
