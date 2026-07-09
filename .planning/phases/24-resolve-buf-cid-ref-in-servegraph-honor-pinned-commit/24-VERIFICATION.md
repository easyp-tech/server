---
phase: 24-resolve-buf-cid-ref-in-servegraph-honor-pinned-commit
verified: 2026-07-09T18:47:00Z
status: human_needed
score: 6/6 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: none
  notes: "Initial verification"
human_verification:
  - test: "Run the Phase 24 e2e gate with EASYP_GH_TOKEN set, against the live proxy + GitHub upstream"
    expected: "TestGeneratePinnedCommit_NotHEAD PASSes for every cached buf version, asserting the proxy server log emits a serving-decision branch line carrying the PINNED SHA as a commit= attribute (not HEAD)."
    why_human: "The e2e test requires an externally-provisioned GitHub PAT (EASYP_GH_TOKEN) and live network access to github.com; cannot be exercised in this verifier sandbox. Without the token the test SKIPs by design (per PLAN), so the live-upstream integration is unverified here."
---

# Phase 24: Resolve buf cid ref in ServeGraph; honor pinned commit — Verification Report

**Phase Goal:** When a client sends a proxy-minted 32-hex buf commit_id as Name.ref (the normal buf.lock state), ServeGraph must resolve it to the real git SHA and return the pinned commit — never forward the cid to the upstream (422/502) and never serve a differently-cached commit (HEAD). Ports the Phase 18 commitUUIDInverse + prefix technique into ServeGraph, adds a cid→full-sha map populated at every mint site, gates infoCache hits on cid match, and makes ServeDownload's foreign-cid path prefer cid→sha over the owner/module infoCache.
**Verified:** 2026-07-09T18:47:00Z
**Status:** human_needed (live-token e2e unverified in this sandbox; all automated checks pass)
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | ServeGraph never forwards a 32-char buf-issued UUID (cid) to provider.GetMeta as a commit arg (PR-24-1) | ✓ VERIFIED | `internal/connect/commits.go:384-409` — `if isUUID(ref.ref) { resolved, ok := h.resolveUUIDRef(...); if ok { fetchRef = resolved } }` runs before the GetMeta call at `:410`. `resolveUUIDRef` (`:1036-1119`) returns either a cidSha hit, a prefix-probe result, or `("", false)` — never the raw cid. Test `TestServeGraph_BufCommitIDRefNotForwardedToUpstream` (`api_test.go:294`) iterates `repo.getMeta` and fails if any call equals the cid; PASS confirmed in fresh `go test` run. |
| 2 | A cid→full-git-sha map is populated at every mint site and consulted before any upstream call for a UUID ref (PR-24-2) | ✓ VERIFIED | `h.cidSha[cid] = meta.Commit` writes at: `commits.go:243` (GetCommits/ServeHTTP), `:483` (ServeGraph writeback), `:804` (ServeDownload fetch writeback), `:1116` (resolveUUIDRef probe success), `:1394` (registerResolvedAlias, guarded by `isUUID(id) && sha != ""`). Read side via `cidShaLookup` (`:1005-1010`) and consulted in `resolveUUIDRef:1042` and `ServeDownload:730`. Map is initialized in production (`api.go:111`) and test (`api_test.go:1926`). |
| 3 | On a cid→sha miss for a UUID ref, ServeGraph resolves via commitUUIDInverse → 28-hex prefix probe, not by sending the raw cid upstream (PR-24-3). resolveUUIDRef inherits probeCommitID defenses (probeSem, missCache, probeTimeout, isTransientErr) — CR-01 fix. | ✓ VERIFIED | `resolveUUIDRef` (`commits.go:1036-1119`) ladder: (a) `cidShaLookup` warm hit; (b) `h.missCached(cid)` early-out (`:1057`); (c) `commitUUIDInverse(cid)` → prefix (`:1066`); (d) non-blocking `h.probeSem` acquire (`:1076-1083`); (e) `context.WithTimeout(ctx, h.probeTimeout)` when > 0 (`:1091-1095`); (f) single `GetMeta(pctx, owner, module, prefix)` call — never the raw cid; (g) `isTransientErr` classification before `rememberMiss` (`:1101`); (h) `HasPrefix(meta.Commit, prefix)` validation (`:1106`); (i) cache writeback on success (`:1115-1117`). Tests `TestServeGraph_UUIDRefColdCache_ProbesWithInversePrefix` (asserts first GetMeta call == prefix) and `TestServeGraph_UUIDRefColdCache_NegativeCachesMiss` (asserts retry within TTL makes 0 new GetMeta calls) both PASS. |
| 4 | infoCache hit gated on cid match — symmetric (cidPinned flag, WR-01 fix) so neither direction poisons the other (PR-24-4) | ✓ VERIFIED | `commitInfoCache.cidPinned bool` field declared at `commits.go:41` with extensive comment explaining why commitID alone is insufficient. Symmetric gate at `:345`: `ok && ((!requestIsUUID && !cached.cidPinned) || (requestIsUUID && cached.commitID == ref.ref))`. `cidPinned: isUUID(ref.ref)` set at every write site (`:250`, `:490`, `:811`) and at registerResolvedAlias (`:1401`, `:1409`). Test `TestServeGraph_PinnedCidDoesNotPoisonHeadRequest` asserts HEAD request after pinned-cid prime re-resolves (>=1 new GetMeta call) and returns headCID — PASS. Test `TestServeGraph_InfoCacheMustNotServeWrongCommit` asserts pinned-after-HEAD direction — PASS. |
| 5 | ServeDownload's foreign-cid path prefers cid→sha over owner/module infoCache; repeat requests hit files-cache (WR-03) | ✓ VERIFIED | Files-cache hit gated on `cached.commitID == commitID` (`:698`); fetch path consults `cidShaLookup(commitID)` first, falls back to `cached.commit` only on miss+err (`:730-740`); fetch writeback populates commitMap+cidSha+infoCache(cidPinned)+filesMap (`:802-814`). Tests `TestServeDownload_PinnedCidNotServedFromWrongInfoCache` (asserts pinSHA's content served, HEAD's absent) and `TestServeDownload_PinnedCidRepeatHitsFilesCache` (asserts second identical request makes 0 new GetMeta/GetFiles calls) both PASS. |
| 6 | The two formerly-RED tests + new unit coverage + Phase-21-style e2e gate pass (PR-24-6) | ✓ VERIFIED (automated) / ⚠ human_needed (live e2e) | Unit: `go test ./internal/connect/...` → ok (0.278s); `go vet ./...` clean. All 8 Phase-24 tests PASS in fresh run (cache-cleared). E2e gate `TestGeneratePinnedCommit_NotHEAD` exists at `e2e/generate_test.go:119`, asserts via WR-04 strengthened log-attribute check (PINNED SHA on a serving-decision branch line, not bare substring). Skipped cleanly in this sandbox (no EASYP_GH_TOKEN); gated-by-design per PLAN — not a code gap, but the live-token run is routed to human verification. |

**Score:** 6/6 truths verified (automated evidence); 1 live-token e2e item routed to human verification.

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `internal/connect/commits.go` | cidSha map + resolveUUIDRef + ServeGraph UUID branch + infoCache cid-gating + ServeDownload cid→sha preference | ✓ VERIFIED | cidSha field (`:59`), cidShaLookup (`:1005`), resolveUUIDRef (`:1036-1119`), ServeGraph UUID branch (`:384-409`), symmetric cid gate (`:345`), ServeDownload files-cache cid-gate (`:698`) + cidSha preference (`:730`) + writeback (`:802-814`). |
| `internal/connect/api_test.go` | RED→GREEN + new coverage (short-circuit, cold-cache prefix probe, download wrong-cache, negative-cache, poison-HEAD, repeat-files-cache) | ✓ VERIFIED | All 8 tests present (`api_test.go:294, 336, 388, 442, 499, 544, 611, 709`) and PASS. |
| `internal/connect/api.go` | cidSha map init in production handler | ✓ VERIFIED | `api.go:111` — `cidSha: make(map[string]string)`. |
| `e2e/generate_test.go` | Phase-24 e2e gate asserting pinned SHA served (not HEAD) | ✓ VERIFIED | `TestGeneratePinnedCommit_NotHEAD` at `:119`; WR-04 fix strengthens assertion to require serving-branch log line carrying `commit=<pinnedSHA>`. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `commits.go::ServeGraph` | `commits.go::resolveUUIDRef` | `isUUID(ref.ref)` branch before GetMeta | ✓ WIRED | `:384-385` — `if isUUID(ref.ref) { resolved, ok := h.resolveUUIDRef(r.Context(), ref, ref.ref) ... }` before GetMeta at `:410`. |
| `commits.go::resolveUUIDRef` | `commits_helpers.go::commitUUIDInverse` | cold-cache fallback recovers 28-hex sha prefix | ✓ WIRED | `:1066` — `prefix, err := commitUUIDInverse(cid)`; prefix used in GetMeta at `:1096`. |
| `commits.go::ServeGraph` (writeback) | `commits.go::cidSha` map | `h.cidSha[cid] = meta.Commit` at every mint site | ✓ WIRED | Write sites at `:243`, `:483`, `:804`, `:1116`, `:1394` (registerResolvedAlias, guarded). |

### Data-Flow Trace (Level 4)

Not applicable — Phase 24 modifies in-process caching/resolution logic (no UI/dashboard rendering of dynamic data). All data-flow is verified via the unit-test assertions on GetMeta/GetFiles call counts and response-body content (e.g. `TestServeDownload_PinnedCidNotServedFromWrongInfoCache` asserts pinFile.Data present and headFile.Data absent in the response body — real data flows through the wiring).

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| All Phase 24 unit tests pass (clean cache) | `go clean -testcache && go test ./internal/connect/... -run 'TestServeGraph_BufCommitIDRefNotForwardedToUpstream\|TestServeGraph_InfoCacheMustNotServeWrongCommit\|TestServeGraph_UUIDRefShortCircuitsFromCidShaMap\|TestServeGraph_UUIDRefColdCache\|TestServeGraph_PinnedCidDoesNotPoisonHeadRequest\|TestServeDownload_PinnedCid' -v` | 8/8 PASS, "ok github.com/easyp-tech/server/internal/connect 0.278s" | ✓ PASS |
| Full connect package green | `go test ./internal/connect/...` | ok (cached) | ✓ PASS |
| go vet clean across module | `go vet ./...` | no output (clean) | ✓ PASS |
| E2e gate compiles + skips cleanly without token | `go test ./e2e/... -run TestGeneratePinnedCommit_NotHEAD` | ok (SKIP via RequireEnvToken) | ✓ PASS (skip-by-design) |

### Probe Execution

No `scripts/*/tests/probe-*.sh` probes declared by this phase. The Phase 24 gate is the Go e2e test `TestGeneratePinnedCommit_NotHEAD` (covered under Behavioral Spot-Checks above and routed to human verification for the live-token run).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| PR-24-1 | 24-01-PLAN.must_haves[0] | ServeGraph never forwards 32-hex cid to GetMeta | ✓ SATISFIED | commits.go:384-410; TestServeGraph_BufCommitIDRefNotForwardedToUpstream PASS |
| PR-24-2 | 24-01-PLAN.must_haves[1] | cid→sha map populated at every mint site + consulted before upstream | ✓ SATISFIED | 5 write sites (commits.go:243/483/804/1116/1394); cidShaLookup consulted in resolveUUIDRef + ServeDownload |
| PR-24-3 | 24-01-PLAN.must_haves[2] | Cold cache → commitUUIDInverse → 28-hex prefix probe (with probeCommitID defenses per CR-01) | ✓ SATISFIED | commits.go:1036-1119 has all 4 defenses (probeSem, missCache, probeTimeout, isTransientErr); tests PASS |
| PR-24-4 | 24-01-PLAN.must_haves[3] | infoCache gated on cid match (symmetric via cidPinned per WR-01) | ✓ SATISFIED | commits.go:345 symmetric gate; commitInfoCache.cidPinned at :41; 5 write sites set the flag; tests PASS |
| PR-24-5 | 24-01-PLAN.must_haves[4] | ServeDownload foreign-cid prefers cid→sha; repeat hits files-cache (WR-03) | ✓ SATISFIED | commits.go:730 (cidShaLookup) + :698 (files-cache cid-gate) + :802-814 (writeback); tests PASS |
| PR-24-6 | 24-01-PLAN.must_haves[5] | Formerly-RED tests + new unit coverage + e2e gate pass | ✓ SATISFIED (unit/vet) / ⚠ human (live e2e) | All 8 connect tests PASS; go vet clean; e2e gate exists and asserts via strengthened log-attribute check (WR-04); live-token run routed to human verification |

**REQUIREMENTS.md traceability note:** PR-24-1..6 IDs exist only in `24-01-PLAN.md` frontmatter — they are not enumerated in `.planning/REQUIREMENTS.md` (which scopes only v1.3 logging requirements FOUND/INFR/ERR/PROV/OPS, phases 11-15). No orphaned IDs, no mismatches: the phase-24 must_haves are plan-local and trace cleanly to the must_haves list above. Phase-24 commit also extends ROADMAP/STATE per recent `9270e6f docs(phase-23)` pattern (Phase 25 carries deferred persistence work per SUMMARY Next-Phase-Readiness).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| (none in phase-modified files) | — | — | — | — |

Targeted scans on `internal/connect/commits.go`, `internal/connect/api.go`, `internal/connect/api_test.go`, `e2e/generate_test.go` for TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER/placeholder/coming soon/not yet implemented/return null/return {}/return []/=> {}/hardcoded empty data yielded no debt markers and no stub returns in phase-modified code paths. All empty returns are explicit error-sentinel handling (e.g. `return nil, false` after `rememberMiss`), not stubs.

### Code Review Fix Verification (24-REVIEW.md)

| ID | Severity | Claimed Fix | Verified in Code |
| --- | --- | --- | --- |
| CR-01 | BLOCKER | resolveUUIDRef inherits probeSem + missCache + probeTimeout + isTransientErr | ✓ Verified at commits.go:1057 (missCached), :1066 (commitUUIDInverse), :1076-1083 (probeSem non-blocking acquire), :1091-1095 (WithTimeout guarded on probeTimeout>0), :1101 (isTransientErr classification), :1106 (HasPrefix validation). Test TestServeGraph_UUIDRefColdCache_NegativeCachesMiss asserts the negative-cache behavior. |
| WR-01 | WARNING | Symmetric cid-gate via cidPinned flag | ✓ Verified at commits.go:41 (field), :345 (symmetric gate), :250/:490/:811/:1401/:1409 (flag set at all write sites). Test TestServeGraph_PinnedCidDoesNotPoisonHeadRequest asserts the reverse direction. |
| WR-02 | WARNING | Per-call timeout in resolveUUIDRef | ✓ Verified via CR-01 fix at commits.go:1091-1095. |
| WR-03 | WARNING | ServeDownload fetch writeback to infoCache + filesMap | ✓ Verified at commits.go:802-814 (commitMap, cidSha, infoCache with cidPinned, filesMap). Test TestServeDownload_PinnedCidRepeatHitsFilesCache asserts second request makes 0 new upstream calls. |
| WR-04 | WARNING | e2e content assertion strengthened | ✓ Verified at e2e/generate_test.go:182-201 — requires serving-branch line carrying `commit=<pinnedSHA>`, not a bare substring match. |
| IN-01 | INFO | 2^112 uniqueness invariant documented | ✓ Verified via comments at commits.go:1060-1065 (resolveUUIDRef) and :1276-1278 (probeCommitID). |

All 6 review findings confirmed fixed in the live codebase, not just claimed.

### Human Verification Required

### 1. Live-token e2e gate run

**Test:** Set `EASYP_GH_TOKEN` (GitHub PAT with public-repo read) in the environment and run `go test ./e2e/... -run TestGeneratePinnedCommit_NotHEAD -v` against the current branch.
**Expected:** For every cached buf version under `testdata/buf/`, `TestGeneratePinnedCommit_NotHEAD` PASSes. The proxy server log captured by `srv.Output` contains a line that simultaneously (a) carries `commit=<pinnedSHA>` (the actual SHA of `refs/tags/common-protos-1_3_1` from `git ls-remote https://github.com/googleapis/googleapis`), and (b) is tagged with one of the serving-decision branches (`uuid_ref_resolved`, `info_cache_writeback`, `files_cache_hit`, `digest_b5_wrap`, `digest_b4_keep`, `commit_id_probe_hit`). HEAD's SHA must NOT appear in such a serving-branch line.
**Why human:** The verifier sandbox has no `EASYP_GH_TOKEN` and no live-egress path to github.com. The test SKIPs cleanly without the token by design (per PLAN user_setup); the live-upstream integration is therefore not exercised here. This is the only check that exercises the full chain (ServeGraph + resolveUUIDRef + commitUUIDInverse + real provider prefix resolution + ServeDownload + files-cache) against a real GitHub backend.

### Gaps Summary

No code gaps. All 6 must-have truths are verified against the live codebase with green unit tests, clean go vet, and the e2e gate present with the strengthened WR-04 assertion. The CR-01 BLOCKER and all four WR review findings are confirmed fixed in code (not just claimed in 24-REVIEW.md).

The single human-needed item is the live-token e2e run, which is gated by design on `EASYP_GH_TOKEN` and cannot be exercised in this sandbox. Per the verify_focus instructions, the SKIP-without-token behavior is explicitly NOT a code gap — but the underlying live-upstream check is genuine external-service integration and so is surfaced for human verification before declaring the phase fully production-ready.

---

_Verified: 2026-07-09T18:47:00Z_
_Verifier: Claude (gsd-verifier)_
