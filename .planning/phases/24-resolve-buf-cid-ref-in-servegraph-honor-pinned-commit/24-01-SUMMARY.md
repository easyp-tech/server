---
phase: 24-resolve-buf-cid-ref-in-servegraph-honor-pinned-commit
plan: 01
subsystem: api
tags: [buf, cid-resolution, servegraph, infocache, uuid, commitUUIDInverse]

# Dependency graph
requires:
  - phase: 18-respect-buf-yaml-dependency-refs-not-always-head-fix-related
    provides: commitUUIDInverse + 28-hex prefix probe technique (probeCommitID)
  - phase: 22-fix-v1-30-1-v1alpha1-read-path-uuid-handling-verify-v1-proto
    provides: isUUID branch shape for read paths
provides:
  - cidSha map storing cid -> full git SHA at every mint site
  - ServeGraph UUID-resolution branch (resolveUUIDRef) — never forwards raw cid upstream
  - infoCache cid-gating — cache hit requires commitID match when request pins a cid
  - ServeDownload files-cache cid-gating + cidSha preference over wrong infoCache entry
  - recordingProvider prefix-match support mirroring real GitHub/Bitbucket short-SHA resolution
affects:
  - 25-we-need-e2e-tests-for-the-ref-specified-for-dependency-looks
  - any future phase touching ServeGraph/ServeDownload cid handling

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "cidSha map: zero-round-trip cid->sha lookup populated at every mint site"
    - "resolveUUIDRef ladder: cidSha hit -> commitUUIDInverse -> 28-hex prefix probe"
    - "infoCache cid-gate: !isUUID(ref.ref) || cached.commitID == ref.ref"

key-files:
  created:
    - e2e/generate_test.go::TestGeneratePinnedCommit_NotHEAD (e2e not-HEAD gate)
  modified:
    - internal/connect/commits.go (cidSha map, resolveUUIDRef, ServeGraph UUID branch, infoCache gating, ServeDownload cidSha preference)
    - internal/connect/api.go (cidSha map init in production handler constructor)
    - internal/connect/api_test.go (recordingProvider prefix match; 3 new unit tests)

key-decisions:
  - "Gate-on-cid (not re-key infoCache): less invasive, no ripple to ServeGetModules/resolveForeignCommitID which read infoCache[owner/module]"
  - "resolveUUIDRef calls h.api.repo.GetMeta directly (not probeCommitID) — ServeGraph has owner/module from the request, so a single-source resolution suffices"
  - "cidSha populated at registerResolvedAlias only when isUUID(id) — raw-sha probe callers don't pollute the map"
  - "ServeDownload fetch path prefers cidSha, falls back to cached.commit (HEAD) only when cidSha misses — preserves prior single-module behavior for genuinely-foreign ids"

patterns-established:
  - "cidSha as the source-of-truth for cid->sha at all mint sites"
  - "infoCache cid-gate pattern for any owner/module-keyed cache read"

requirements-completed: [PR-24-1, PR-24-2, PR-24-3, PR-24-4, PR-24-5, PR-24-6]

# Metrics
duration: ~15 min
completed: 2026-07-09
---

# Phase 24 Plan 01: Resolve buf cid ref in ServeGraph; honor pinned commit Summary

**cidSha map + ServeGraph UUID-resolution branch (commitUUIDInverse -> 28-hex prefix probe) + infoCache cid-gating so a buf.lock-pinned cid resolves to its own commit, never forwarded upstream (422) and never served from a HEAD-keyed cache**

## Performance

- **Duration:** ~15 min
- **Tasks:** 3
- **Files modified:** 4 (commits.go, api.go, api_test.go, generate_test.go)

## Accomplishments
- Both Phase 24 RED tests turned GREEN: ServeGraph no longer forwards the raw 32-hex cid to GitHub, and infoCache no longer serves a HEAD entry for a pinned-cid request.
- cidSha map populated at every mint site (GetCommits, ServeGraph writeback, ServeDownload mint, registerResolvedAlias) — zero-round-trip cid->sha resolution for warm caches.
- ServeGraph UUID-resolution branch reuses the Phase 18 commitUUIDInverse -> 28-hex prefix technique: cold cache probes upstream with the prefix (never the cid), validates the response starts with the prefix, and caches the result.
- ServeDownload files-cache hit gated on cid match; fetch path prefers cidSha over the owner/module infoCache entry so a pinned cid fetches its own content.
- Three new unit tests + one e2e gate covering the warm-short-circuit, cold-cache-prefix-probe, and download wrong-infoCache paths.

## Task Commits

1. **Task 24-01-01: cid->sha map + ServeGraph UUID branch + infoCache cid-gating** — `6542e02` (fix)
2. **Task 24-01-02: new unit coverage (3 tests)** — `4837076` (test)
3. **Task 24-01-03: e2e gate (TestGeneratePinnedCommit_NotHEAD)** — `8b0e85f` (test)

## Files Created/Modified
- `internal/connect/commits.go` — cidSha map field + population at 4 mint sites; resolveUUIDRef + cidShaLookup helpers; ServeGraph UUID branch before GetMeta; infoCache cid-gate; ServeDownload files-cache cid-gate + cidSha-preference in fetch path
- `internal/connect/api.go` — cidSha map initialization in production commitServiceHandler constructor
- `internal/connect/api_test.go` — recordingProvider.GetMeta now mirrors real provider short-SHA prefix resolution; 3 new tests (TestServeGraph_UUIDRefShortCircuitsFromCidShaMap, TestServeGraph_UUIDRefColdCache_ProbesWithInversePrefix, TestServeDownload_PinnedCidNotServedFromWrongInfoCache); newTestCommitHandler inits cidSha
- `e2e/generate_test.go` — TestGeneratePinnedCommit_NotHEAD e2e gate asserting server log contains the pinned SHA (not just HEAD's)

## Decisions Made
- **Gate-on-cid over re-keying infoCache:** The plan offered two options for infoCache — gate-on-cid or re-key by (owner/module, cid). Chose gate-on-cid (`!isUUID(ref.ref) || cached.commitID == ref.ref`) because it's less invasive: ServeGetModules and resolveForeignCommitID read infoCache[owner/module] and would need updating if the key shape changed. The gate achieves the same correctness without ripple.
- **Direct GetMeta in resolveUUIDRef (not probeCommitID):** ServeGraph has the owner/module from the request, so a single `h.api.repo.GetMeta(ctx, owner, module, prefix)` call suffices. probeCommitID's fan-out is for the Download path where the module identity is unknown.
- **ServeDownload files-cache gate also added (Rule 2):** The plan mentioned ServeDownload's foreign-cid path but the files-cache hit branch at L677 also suffered the same cid-mismatch bug — it served filesMap[HEAD cid] for a pinned-cid request. Added `cached.commitID == commitID` to the gate.
- **Direct handler seeding for TestServeDownload_PinnedCidNotServedFromWrongInfoCache:** The HTTP-prime approach couldn't reproduce the wrong-infoCache state because ServeGraph's cache-hit gate (correctly) serves any non-UUID ref from the first entry. Seeded the handler directly via newTestCommitHandler to reproduce the exact prod state.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] recordingProvider.GetMeta didn't model short-SHA prefix resolution**
- **Found during:** Task 24-01-01 (turning the RED tests GREEN)
- **Issue:** The mock did exact `bySha[commit]` match. Real GitHub/Bitbucket providers resolve short-SHA prefixes (>=7 hex) via repos.GetCommit / the commit-fetch API — that's how the 28-hex prefix from commitUUIDInverse succeeds in production. Without prefix-match in the mock, the cold-cache path couldn't be exercised in tests.
- **Fix:** Added a prefix-match loop in recordingProvider.GetMeta that returns the entry whose full SHA starts with the requested commit. Mirrors the real provider behavior the 28-hex prefix path depends on.
- **Files modified:** internal/connect/api_test.go
- **Verification:** TestServeGraph_UUIDRefColdCache_ProbesWithInversePrefix passes (GetMeta receives the 28-hex prefix, not the cid); TestServeGraph_BufCommitIDRefNotForwardedToUpstream passes.
- **Committed in:** 6542e02 (Task 24-01-01)

**2. [Rule 2 - Missing Critical] ServeDownload files-cache hit branch not cid-gated**
- **Found during:** Task 24-01-01 (auditing ServeDownload for the same defect class as infoCache)
- **Issue:** The plan called out the infoCache cid-gate for ServeGraph (step 4) but the ServeDownload files-cache hit branch (`infoOK && len(cachedFiles) > 0`) had the same cid-mismatch bug — it served filesMap[HEAD cid] for a pinned-cid request because infoCache[owner/module] held HEAD's entry.
- **Fix:** Added `cached.commitID == commitID` to the files-cache hit gate. When the cids differ, falls through to the fetch path which resolves via cidSha.
- **Files modified:** internal/connect/commits.go
- **Verification:** TestServeDownload_PinnedCidNotServedFromWrongInfoCache passes.
- **Committed in:** 6542e02 (Task 24-01-01)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 missing critical)
**Impact on plan:** Both auto-fixes necessary for correctness — the mock fix makes the test exercisable; the ServeDownload files-cache gate closes the same defect class the plan targeted for ServeGraph. No scope creep.

## Issues Encountered
None beyond the auto-fixes above.

## User Setup Required
None — the e2e gate uses the same EASYP_GH_TOKEN environment variable already in use by the Phase 19/21/23 e2e suite. Without it, TestGeneratePinnedCommit_NotHEAD skips cleanly.

## Next Phase Readiness
- ServeGraph + ServeDownload cid handling is correct for the v1/v1beta1 GraphService + DownloadService paths exercised by `buf generate` on modern buf CLI.
- Persistence of cidSha across pod restart (artifactory) is tracked as a follow-up per the plan's out-of-scope note — the in-process map + inverse-prefix fallback unblocks `buf generate` within a warm pod.
- Phase 19/20/22/23 e2e tests (tag/branch/raw-SHA refs) continue to pass — the UUID branch triggers only on isUUID(ref.ref), so empty/branch/tag/full-SHA refs flow the existing path untouched.

## Known Stubs
None — all code paths are fully wired with real resolution logic.

## Self-Check: PASSED
