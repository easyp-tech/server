---
slug: download-foreign-commit-id
status: awaiting_human_verify
trigger: "Production DownloadService/Download returns HTTP 400 'unknown commit id: must call CommitService/GetCommits first' even after PR #32 write-back fix. Client sends a cached/foreign commit_id that the proxy never minted."
created: 2026-06-24
updated: 2026-06-24
goal: find_and_fix
specialist_dispatch_enabled: true
commit: bd4f0ae04051163f6971f49bd419fa48a212bd16
---

# Symptoms

- **Expected:** `DownloadService/Download` succeeds (200) for module googleapis/googleapis after GetGraph resolved it in the same session.
- **Actual:** HTTP 400 `unknown commit id: must call CommitService/GetCommits first`. `ref_found=false` in ServeDownload.
- **Error:** `error_class=bad_request status=400`, `commit_id=2d1654c2cc02a6e7f3bbea2d06fc1c59`. Repeated 3x in log (15:59:33, 16:00:39, 16:02:28) from two client IPs.
- **Timeline:** Persisted across PR #32 (`6610ee9` write-back in ServeGraph). #32 did not change symptom.
- **Reproduction:** Any buf CLI v1 client with a locally cached commit_id for googleapis/googleapis (e.g. pinned in buf.lock from real buf.build) issues Download directly with cached id.

# Evidence

- Log: `logs/prod-buf-proxy-buf-proxy-6f899c7c68-v4jc4-buf-proxy.log` lines 418-419, 512-513, 664-665.
- Same client `10.78.116.13` at 16:00:39: GetGraph (.886) mints commit_id `60558023ae758022bef83ecc648b3a9c` for googleapis@e57bae6e; Download (.889) sends `2d1654c2cc02a6e7f3bbea2d06fc1c59`.
- Verified `deterministicID("e57bae6efbd075a925978a79bb9b997beb4ecc19")` == `60558023…` (matches GetGraph output).
- `2d1654c2cc02a6e7f3bbea2d06fc1c59` matches no natural input to deterministicID (commit, owner, module, digest, combos). Git history: minting input `meta.Commit` unchanged since `5e1d204`. So id is foreign (client-cached), never minted by this proxy.
- Zero CommitService/GetCommits traffic in 760-line log.

# Root Cause (candidate, strong)

ServeDownload lookup is strict in-memory `h.commitMap[commitID]` keyed only by ids this proxy mints ([commits.go:436-444](internal/connect/commits.go#L436-L444)). Foreign/cached commit_id from client → `ref_found=false` → 400. PR #32 write-back registers only GetGraph-minted id; irrelevant when client ignores it.

# Current Focus

- **hypothesis:** ServeDownload's strict `h.commitMap[commitID]` lookup rejects any commit_id this proxy never minted (e.g. a buf.lock-cached id from real buf.build). Falling back to resolving the module identity via infoCache before returning 400 eliminates the rejection for any module the proxy actually serves.
- **test:** Update `TestBadRequest_OnUnknownCommitID` to reflect new semantics (400 only when module truly unresolvable). Add `TestServeDownload_ForeignCommitID_FallbackKnownModule` (single infoCache entry, unknown commit_id -> 200) and `TestServeDownload_ForeignCommitID_TrulyUnknownModule` (empty infoCache, unknown commit_id -> 400).
- **expecting:** Fallback test returns 200 with module content; truly-unknown test still returns 400 "unknown commit id".
- **next_action:** Implement fallback in ServeDownload (commits.go ~471-480): on unknown commit_id, if infoCache has exactly one entry, resolve it, register commit_id alias in commitMap, continue serving. Only 400 when no resolvable module.
- **reasoning_checkpoint:**
  hypothesis: "ServeDownload returns 400 for foreign/cached commit_ids because commitMap is keyed only by proxy-minted ids; falling back to single-entry infoCache resolution (the proxy serves a single active module per deployment in the failing prod case) serves the content the client wants."
  confirming_evidence:
    - "commits.go:436-444 — commitID lookup is strict h.commitMap[commitID]; miss -> ref=nil -> 400 at 476-479"
    - "Prod log shows the foreign id 2d1654c2... was never an output of deterministicID for any natural input; client cached it from real buf.build"
    - "infoCache keyed by 'owner/module' holds commitInfoCache{commitID, commit, ownerID, moduleID, digest} — enough to serve the download"
  falsification_test: "If after the fix a Download with an unknown commit_id but a populated single-entry infoCache still returns 400, the fallback path is wrong."
  fix_rationale: "Registers the foreign commit_id as an alias of the resolved module's ref in commitMap, so subsequent identical requests are served directly. Addresses the root cause (strict lookup) rather than papering over the 400."
  blind_spots: "Multi-module deployments — when infoCache has >1 entry we cannot tell which module a foreign id refers to, so we still 400 (safer than guessing). Real buf clients may also send ResourceRef.name; we do not parse it (buildDownloadRequest only sets id), but a future enhancement could."

# Eliminated

(none yet)

# Resolution

- **root_cause:** ServeDownload's commitMap lookup is strict — keyed only by ids this proxy mints. A foreign/cached commit_id (e.g. one a buf CLI client cached from real buf.build and pinned in buf.lock) never matches, so the handler returns 400 "unknown commit id" even though the proxy serves the module the client wants.
- **fix:** Added a foreign-commit_id fallback in ServeDownload. On commitMap miss, before returning 400, attempt to resolve the module identity: if infoCache has exactly one entry (the common single-module deployment), use it as the active module and register the foreign commit_id as an alias of that module's ref in commitMap (so subsequent identical requests are served directly). Only return 400 when infoCache is empty / the module is truly unresolvable. Added `resolveForeignCommitID` + `splitOwnerModule` helpers.
- **verification:** `go test ./internal/connect/...` passes (incl. updated TestBadRequest_OnUnknownCommitID and new TestServeDownload_ForeignCommitID_FallbackKnownModule, _FallbackAliasCachesMapping, _TrulyUnknownModule). `go vet ./internal/connect/...` clean. `go test ./...` — all packages pass.
- **files_changed:**
  - internal/connect/commits.go (fallback branch + helpers)
  - internal/connect/api_test.go (updated test docstring + 3 new tests)
