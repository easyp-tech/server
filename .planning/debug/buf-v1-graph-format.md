---
status: resolved
trigger: "buf 1.69.0 crashes with nil pointer dereference in graph_provider.go:140 when running `buf dep update` against the proxy."
created: 2026-06-01
updated: 2026-06-01
---

# Debug Session: buf-v1-graph-format

## Symptoms

- **Expected behavior:** `buf dep update` succeeds when pointing at the local proxy
- **Actual behavior phase 1:** panic with nil pointer dereference at `bufmoduleapi/graph_provider.go:140`
- **Actual behavior phase 2:** after fixing phase 1, `invalid_argument: unmarshal message: string field contains invalid UTF-8`
- **Actual behavior phase 3:** after fixing phase 2, `*** Digest verification failed ***`
- **Final result:** `buf dep update` exits with code 0

## Root Cause

Buf 1.69.0 uses `DigestTypeB5` (b5) as the default digest type. This causes it to use the **v1 API** (not v1beta1) for all module operations. The proxy was built for the v1beta1 API and had three separate issues.

### Issue 1: v1 GraphService request format

Buf's v1 `GetGraphRequest` uses `repeated ResourceRef` directly in field 1, while v1beta1 wraps each in `GetGraphRequest_ResourceRef { ResourceRef, Registry }`. The proxy's `ServeGraph` handler used a single parser (`parseGetGraphResourceRefs`) that only understood the v1beta1 format, causing v1 requests to return an empty graph → `response.Msg.Graph == nil` → nil pointer dereference.

### Issue 2: v1 GraphService response format

The v1 `Graph.commits` expects `repeated Commit` directly (no wrapper), while v1beta1 wraps each in `Graph_Commit { Commit, Registry }`. The proxy returned the v1beta1 format on both paths, causing buf to mis-parse the response → `invalid UTF-8`.

### Issue 3: B5 digest computation

The v1 `DigestType` enum has no B4 (`DIGEST_TYPE_B5 = 1` is the only valid type). Buf uses B5 digests, which are computed differently from B4:
- B4: `SHA3-Shake256(manifest_text)`
- B5: `SHA3-Shake256("shake256:" + hex(SHA3-Shake256(manifest_text)))` (wraps B4 hash as a string)

The proxy returned B4 digest values tagged as wire-type 1 (= B5 in v1), causing verification failure.

## Fix

### Changed files:

- `internal/connect/commits.go` — Three fixes:
  1. `ServeGraph`: detect v1 vs v1beta1 path, use appropriate request parser and response format
  2. `ServeHTTP` (CommitService): apply `toB5Digest()` on v1 path
  3. `ServeGraph`: apply `toB5Digest()` on v1 path
  4. `ServeDownload`: apply `toB5Digest()` on v1 path
  5. Added `toB5Digest()` helper function
- `internal/connect/commits_helpers.go` — Added `parseGetGraphResourceRefsV1` for v1 request format
- `internal/connect/api_test.go` — Updated tests to use correct request format for each path

### How:

**Request parsing:**
- v1beta1 path → uses `parseGetGraphResourceRefs` (existing, wrapped format)
- v1 path → uses `parseGetGraphResourceRefsV1` (new, direct ResourceRef format)

**Response format:**
- v1beta1 path → wraps each commit in `Graph_Commit { Commit, Registry }`
- v1 path → places `Commit` directly into `Graph.commits`

**Digest computation (`toB5Digest`):**
- Computes `SHA3-Shake256("shake256:" + hex(b4_hash))` to match buf's B5 algorithm
- Applied to all three v1 handler paths (CommitService, GraphService, DownloadService)

## Verification

- All existing tests pass
- `buf dep update` in `/Users/nil/DiskD/W/yadro/cyp-hardware-manager/api/proto` exits with code 0
- Generated buf.lock contains all 4 deps with correct B5 digests
