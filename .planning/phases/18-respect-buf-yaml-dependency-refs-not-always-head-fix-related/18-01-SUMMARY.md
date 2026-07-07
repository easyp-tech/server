---
phase: 18-respect-buf-yaml-dependency-refs-not-always-head-fix-related
plan: 01
subsystem: connect
tags: [buf, grpc, dependency-resolution, probe, prewarm-removal, uuid-inverse, refs, prefix-match]

# Dependency graph
requires:
  - phase: 16-commit-id-resolution-improvements
    provides: "commitUUID mint helper, prewarmHeads, registerResolved, probeCommitID, CommitResolution struct"
  - phase: 17-fix-pr-37-review-findings
    provides: "Phase 16 fixes (dashless UUID format, prewarm-enabled by default)"
provides:
  - "moduleRef.ref field (buf BSR Name.ref, proto field 3) plumbed end-to-end"
  - "parseResourceRefName reads field 3"
  - "ServeHTTP and ServeGraph pass ref.ref to GetMeta (was \"\")"
  - "isSHA / isUUID pure-function helpers in commits_helpers.go"
  - "Bitbucket provider: new getCommit(ctx, ref) helper that calls /commits/{ref} and returns the resolved SHA"
  - "GitHub provider: GetMeta routes non-SHA inputs through repos.GetCommit(ctx, owner, repo, ref, nil)"
  - "commitUUIDInverse(uuid) (string, error) — 14-byte recovery from a 32-char dashless UUID"
  - "probeCommitID rewritten to use commitUUIDInverse for 32-char UUID inputs with prefix-match validation (fail-closed)"
  - "registerResolvedAlias helper that takes the buf-issued id (UUID) as primary key and the resolved SHA as alias"
  - "prewarmHeads, registerResolved, prewarmOnce, prewarmEnabled, prewarmTimeout, PrewarmEnabled, PrewarmTimeout, PrewarmConfig all removed"
affects: [19, future-buf-v1-cli-tests, future-foreign-id-handling]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "isSHA / isUUID duplicated as a tiny 8-line helper in each provider package (avoided the unexported-connect-to-provider dependency; pure functions, low cost)"
    - "prefix-match validation in probeCommitID: when probe arg is a UUID-derived prefix, the source's meta.Commit MUST start with the prefix (fail-closed) — closes review.md finding #6"
    - "registerResolvedAlias takes the original client-sent id (UUID) as primary key, not the resolved SHA — matches the buf v1.69.0 commit_id contract"

key-files:
  created:
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/providers/bitbucket/getrepo_test.go"
      provides: "4 new TestGetMeta_* cases (Empty, RawSHA_40, RawSHA_64, ResolvesRef) for the Bitbucket provider"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/providers/github/getrepo_test.go"
      provides: "3 new TestGetMeta_* cases (Empty, RawSHA_40, ResolvesRef) for the GitHub provider"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/providers/github/mockrepos_test.go"
      provides: "new mockRepos mock with GetCommit plumbing and call-count tracking for the GitHub provider tests"
  modified:
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/commits_helpers.go"
      provides: "moduleRef gains ref; parseResourceRefName reads field 3; isSHA + isUUID + commitUUIDInverse helpers"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/commits_helpers_test.go"
      provides: "TestIsSHA (8 cases), TestIsUUID (6 cases), TestParseResourceRefName_ReadsRef, TestParseResourceRefName_NoRef, TestCommitUUIDInverse (8 cases)"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/commits.go"
      provides: "ServeHTTP/ServeGraph pass ref.ref; probeCommitID rewritten to use commitUUIDInverse + prefix-match; prewarmHeads/registerResolved deleted; prewarm fields deleted"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/api.go"
      provides: "CommitResolution loses PrewarmEnabled/PrewarmTimeout; NewWithConfig drops the goroutine launch"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/api_test.go"
      provides: "TestServeDownload_AfterRestart_ProbeResolvesUUID + TestServeDownload_AfterRestart_ProbeMissesOnUnknownUUID; mockSource accepts HasPrefix; TestPrewarmHeads_PopulatesCommitMap removed; TestProbeCommitID_* tests updated for the isSHA gate"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/uuid_format_test.go"
      provides: "stale 'registerResolved' comment updated to 'registerResolvedAlias'"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/providers/bitbucket/getrepo.go"
      provides: "getMeta uses isSHA(commit) gate; new getCommit(ctx, ref) helper"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/providers/bitbucket/client.go"
      provides: "new tmplGetCommit template for /commits/{{.id}}"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/providers/github/getrepo.go"
      provides: "GetMeta uses isSHA(commit) gate; non-SHA inputs route through repos.GetCommit"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/cmd/easyp/internal/config/config.go"
      provides: "PrewarmConfig struct deleted; Connect struct retains Probe only"
    - path: "/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/cmd/easyp/main.go"
      provides: "handler factory drops cc.Prewarm.* references"

key-decisions:
  - "isSHA and isUUID are unexported in the connect package; each provider package defines its own 8-line local isSHA. Avoids the unexported-cross-package import; trivial duplication."
  - "registerResolvedAlias is a new helper, not a rename of registerResolved. It takes the original client-sent id (typically a buf-issued UUID) as the primary key, matching the buf v1.69.0 commit_id contract."
  - "probeCommitID fails closed on a prefix mismatch (a source whose meta.Commit does not start with the recovered 28-char prefix is treated as a miss, not a hit). Closing the wrong-source-match security/correctness risk in review.md finding #6."
  - "Inputs that are neither SHA-shaped (40/64 hex) nor UUID-shaped (32 hex) are negative-cached and return early in probeCommitID — no upstream fan-out. The 'bogus000' fixture from Phase 16's probe tests was updated to a 40-char SHA to match the new gate."
  - "The prewarm removal is observable to the client: a buf build immediately after a proxy restart will incur one extra upstream round-trip per module (the probe fan-out). The probeSem cap (maxConcurrentProbes=4) and per-source probeTimeout (8s default) bound the worst case."

patterns-established:
  - "Three-branch commit gate in providers: commit == \"\" → HEAD; isSHA(commit) → fast path; else → ref-resolution API call"
  - "Provider tests use httptest.NewServer (Bitbucket) or a configurable Repositories mock (GitHub) rather than a real network call"
  - "TDD with RED-then-GREEN: failing tests committed first, then the production change in a separate commit"

requirements-completed:
  - SC-1 (refs respected: GetCommits/GetGraph return UUID from ref, not HEAD)
  - SC-2 (related bug fixed: provider short-circuit replaced with isSHA gate)
  - SC-3 (prewarm removed: prewarmHeads, registerResolved, related fields, config block)
  - SC-4 (inverse helper: commitUUIDInverse + round-trip tests)
  - SC-5 (no regression: Phase 16/17 tests still pass)

# Metrics
duration: 18min
completed: 2026-07-07
---

# Phase 18 Plan 01: Respect buf.yaml Dependency Refs Summary

**End-to-end ref support for buf.yaml deps + isSHA-gated provider ref-resolution + prewarm removal replaced by commitUUIDInverse-based probe**

## Performance

- **Duration:** 18 min
- **Started:** 2026-07-07T13:58:32Z
- **Completed:** 2026-07-07T14:16:20Z
- **Tasks:** 3
- **Files modified:** 11

## Accomplishments

- **`moduleRef` plumbs the `ref` field through `parseResourceRefName` → `ServeHTTP`/`ServeGraph` → provider `GetMeta`.** A `buf.yaml` dep like `cyp/cyp-net-listeners:main/v2` now returns the SHA at `main/v2`, not HEAD. Backward-compatible: clients that omit `ref` (older buf CLI, v1beta1 callers) still resolve to HEAD.
- **The `commit != "" && commit != "main"` short-circuit in both providers is replaced with an `isSHA(commit)` gate** (40/64 lowercase hex). Non-SHA inputs route through the providers' commit-fetch APIs: Bitbucket's `/commits/{ref}` returns the resolved id, GitHub's `repos.GetCommit(ctx, owner, repo, ref, nil)` returns the resolved SHA. The new path is one extra round-trip per ref-resolved request; HEAD-only requests still skip resolution.
- **`prewarmHeads` and the related startup fan-out are completely removed.** The probe is rewritten to derive a 28-char SHA prefix from a 32-char buf-issued UUID via the new `commitUUIDInverse` helper, then validate that the source's `meta.Commit` actually starts with the recovered prefix (prefix-match validation, fail-closed on mismatch). This closes the review.md finding #6 (probe/register cache contract divergence) — a wrong-source match is now treated as a miss, not a silent hit.

## Task Commits

Each task was committed atomically with RED → GREEN TDD:

1. **Task 1: Honor `Name.ref` end-to-end + provider ref-resolution** - `b16fd2d` (test, RED) → `6a49450` (feat, GREEN)
2. **Task 2: Add `commitUUIDInverse` and round-trip tests** - `6fc909d` (test, RED) → `372bb15` (feat, GREEN)
3. **Task 3: Delete prewarm + rewrite `probeCommitID` to use the inverse** - `1d5fe33` (feat)

## Files Created/Modified

- `internal/connect/commits_helpers.go` - `moduleRef` gains `ref`; `parseResourceRefName` reads field 3; new `isSHA`, `isUUID`, `commitUUIDInverse` helpers
- `internal/connect/commits_helpers_test.go` - `TestIsSHA` (8), `TestIsUUID` (6), `TestParseResourceRefName_ReadsRef`, `TestParseResourceRefName_NoRef`, `TestCommitUUIDInverse` (8)
- `internal/connect/commits.go` - `ServeHTTP`/`ServeGraph` pass `ref.ref`; `probeCommitID` rewritten with `commitUUIDInverse` + prefix-match; `prewarmHeads`/`registerResolved`/prewarm fields deleted
- `internal/connect/api.go` - `CommitResolution` loses `PrewarmEnabled`/`PrewarmTimeout`; `NewWithConfig` drops the goroutine launch
- `internal/connect/api_test.go` - `TestServeDownload_AfterRestart_ProbeResolvesUUID` + `TestServeDownload_AfterRestart_ProbeMissesOnUnknownUUID`; `mockSource` accepts `HasPrefix`; `TestPrewarmHeads_PopulatesCommitMap` removed
- `internal/connect/uuid_format_test.go` - stale `registerResolved` comment updated
- `internal/providers/bitbucket/getrepo.go` - `getMeta` uses `isSHA(commit)` gate; new `getCommit(ctx, ref)` helper
- `internal/providers/bitbucket/client.go` - new `tmplGetCommit` template for `/commits/{{.id}}`
- `internal/providers/bitbucket/getrepo_test.go` (new) - 4 TestGetMeta_* cases via `httptest.NewServer`
- `internal/providers/github/getrepo.go` - `GetMeta` uses `isSHA(commit)` gate; non-SHA inputs route through `repos.GetCommit`
- `internal/providers/github/getrepo_test.go` (new) - 3 TestGetMeta_* cases
- `internal/providers/github/mockrepos_test.go` (new) - new `mockRepos` mock with `GetCommit` plumbing
- `cmd/easyp/internal/config/config.go` - `PrewarmConfig` deleted; `Connect` retains `Probe` only
- `cmd/easyp/main.go` - handler factory drops `cc.Prewarm.*` references

## Decisions Made

- **isSHA/isUUID are unexported in the connect package; each provider has its own 8-line local `isSHA`.** Avoids an unexported-cross-package import path; the helpers are pure functions, ~8 lines each, and the cost of one duplication is far less than the cost of exporting them and widening the public API.
- **`registerResolvedAlias` is a new helper, not a rename of `registerResolved`.** It takes the original client-sent id (typically a buf-issued UUID) as the primary key and the resolved SHA as the alias. The old `registerResolved` did the inverse (took the SHA, called `commitUUID` to mint a UUID, stored both). The new helper is needed because in the post-restart probe path, the buf-issued id is already known — no need to re-derive it.
- **probeCommitID fails closed on a prefix mismatch.** A source whose `meta.Commit` does not start with the recovered 28-char prefix is treated as a miss, not a hit. This closes the review.md finding #6: a wrong-source match would silently alias a real commit id to a wrong module and serve wrong content. Failing closed (miss → 400 to client) lets the client re-run `buf mod update` and recover.
- **Inputs that are neither SHA-shaped nor UUID-shaped are negative-cached and return early in probeCommitID.** No upstream fan-out for unresolvable inputs. The 'bogus000' fixture in Phase 16's probe tests was updated to a 40-char SHA to match the new gate.
- **The prewarm removal is observable to the client.** A `buf build` immediately after a proxy restart will incur one extra upstream round-trip per module (the probe fan-out). The `probeSem` cap (`maxConcurrentProbes=4`) and per-source `probeTimeout` (8s default) bound the worst case at `4 × 8s = 32s` per request in the absolute worst case; in practice the fan-out is parallel and completes in < 8s.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed test data length for the 64-char SHA case in TestCommitUUIDInverse**
- **Found during:** Task 2 (initial test run after the GREEN commitUUIDInverse implementation)
- **Issue:** The plan's fixture for the 64-char SHA case was `0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00` (66 chars, not 64) — the `commitUUID` function rejected it with "input is not 40 or 64 lowercase hex characters". The fixture was hand-derived and miscounted.
- **Fix:** Truncated to 64 chars: `0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`. The recovered 28-char prefix is still `0123456789abcdef0123456789ab` (the first 14 bytes of either length input produce the same UUID).
- **Files modified:** `internal/connect/commits_helpers_test.go`
- **Verification:** `TestCommitUUIDInverse/64-char_SHA_round-trip` passes; the test computes `mustCommitUUID(t, sha)` inline so the recovery is automatic.
- **Committed in:** `372bb15` (Task 2 GREEN commit)

**2. [Rule 1 - Bug] Fixed test expectation length (28 vs 32 chars) in TestCommitUUIDInverse**
- **Found during:** Task 2 (initial test run after the GREEN commitUUIDInverse implementation)
- **Issue:** The plan's `want` values for 3 of the 4 success cases were 32 chars (the full SHA) but the function correctly returns 28 chars (the first 14 bytes, the recoverable portion). The plan author's hand-derivation of "expected prefix" was off — `hex(sha[0:14])` is 28 chars, not the full 40/64.
- **Fix:** Trimmed the 3 affected `want` values to 28 chars: `0000000000000000000000000000` (28 zeros), `ffffffffffffffffffffffffffff` (28 f's), `0123456789abcdef0123456789ab` (28 hex chars). The function output is unchanged.
- **Files modified:** `internal/connect/commits_helpers_test.go`
- **Verification:** `TestCommitUUIDInverse` passes all 8 subtests; `len(got) == 28` for the 4 success cases.
- **Committed in:** `372bb15` (Task 2 GREEN commit)

**3. [Rule 1 - Bug] Updated existing TestProbeCommitID_* tests for the new isSHA gate**
- **Found during:** Task 3 (full test suite run after probe rewrite)
- **Issue:** The existing `TestProbeCommitID_MissNegativeCaches` and `TestProbeCommitID_TransientNotNegativeCached` used short placeholder commits like `"bogus000"` (8 chars) and `"real"` (4 chars) which now bypass the probe entirely via the new `!isSHA(id)` early-return branch (negative-cached before any GetMeta call). The test assertions (1 GetMeta call, retry after transient) no longer held.
- **Fix:** Updated both tests to use 40-char lowercase hex strings (`aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` for the source's owned commit, `bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb` for the bogus test input). The transient-error test now uses the same 40-char shape. The test semantics (negative-cache, transient non-caching) are preserved.
- **Files modified:** `internal/connect/api_test.go`
- **Verification:** Both tests pass; `calls.Load() == 1` on first miss, `== 1` on negative-cache hit; `calls.Load() == 2` after retry on transient.
- **Committed in:** `1d5fe33` (Task 3 commit)

**4. [Rule 2 - Missing Critical] Updated mockSource.GetMeta to accept a SHA prefix**
- **Found during:** Task 3 (RED test for TestServeDownload_AfterRestart_ProbeResolvesUUID)
- **Issue:** The existing `mockSource.GetMeta` returns an error if `commit != "" && commit != s.commit`. With the new probe path, `commit` will be a 28-char SHA prefix (the `commitUUIDInverse` output), and the mock must accept a prefix match (`strings.HasPrefix(s.commit, commit)`) so the post-restart probe can resolve. Without this update, the test would fail (the probe would never see a hit), but production behavior is correct (real providers accept short SHAs).
- **Fix:** Added a `!strings.HasPrefix(s.commit, commit)` clause to the `mockSource.GetMeta` mismatch check. The mock now accepts HEAD, exact SHA match, and SHA prefix match.
- **Files modified:** `internal/connect/api_test.go`
- **Verification:** `TestServeDownload_AfterRestart_ProbeResolvesUUID` passes; the probe fans out to the source with the 28-char prefix, the source returns the full SHA, the prefix-match validation succeeds.
- **Committed in:** `1d5fe33` (Task 3 commit)

---

**Total deviations:** 4 auto-fixed (3 bug fixes, 1 missing critical test infrastructure)
**Impact on plan:** All auto-fixes were test-data or test-infrastructure corrections that did not change the production code shape. The plan's structural intent (end-to-end ref support, isSHA gate, prewarm removal, commitUUIDInverse-based probe) is preserved exactly.

## Issues Encountered

- The plan's hand-derived test fixtures for `commitUUIDInverse` (the `want` values) were 32 chars but the function correctly returns 28 chars. This is a 4-char off-by-one in the plan's expected output. The implementation is correct; the test expectations were fixed. Documented as deviation #2.
- The plan's 64-char SHA fixture had 66 chars (off-by-two). The implementation rejected it with a clear "input is not 40 or 64 lowercase hex characters" error message; truncated to 64 chars and re-ran. Documented as deviation #1.
- The prewarm removal means `TestPrewarmHeads_PopulatesCommitMap` (Phase 16) no longer applies — the test is removed as part of the deletion of `prewarmHeads`. Not a deviation; the plan explicitly noted this in Step A.

## Next Phase Readiness

- Phase 18 deliverables complete; all 5 ROADMAP success criteria met.
- The proxy is now ref-aware: a `buf.yaml` with `cyp/cyp-net-listeners:main/v2` returns the SHA at `main/v2`, not HEAD.
- The prewarm removal is a behavioral change observable to the client: the first `Download` after a process restart now incurs one upstream round-trip per source (the probe fan-out) instead of zero (prewarm). Operators should be aware of this latency shift; the `probeSem` cap and `probeTimeout` bound the worst case.
- No new dependencies, no go.mod / go.sum changes.
- All Phase 16/17 regression tests still pass.

## Self-Check: PASSED

- All commits exist (`git log --oneline | grep -E 'b16fd2d|6a49450|6fc909d|372bb15|1d5fe33'` — 5 commits)
- All created files exist on disk (3 new test files)
- All modified files exist on disk (8 modified files)
- `go build ./...` exits 0
- `go vet ./...` exits 0
- All 3 test packages pass: `go test ./internal/connect/`, `./internal/providers/bitbucket/`, `./internal/providers/github/` — all with `-count=1`
- Targeted tests pass: `TestCommitUUIDInverse|TestIsSHA|TestIsUUID|TestParseResourceRefName|TestServeDownload_AfterRestart`
- Phase 16/17 regression guards pass
- All structural grep checks pass (prewarmHeads, registerResolved$ — 0 hits; commitUUIDInverse, registerResolvedAlias, prefix-match validation — present; isSHA, isUUID, num==3, GetMeta(ref.ref) — correct hit counts; no Prewarm in main.go/config.go)

---
*Phase: 18-respect-buf-yaml-dependency-refs-not-always-head-fix-related*
*Completed: 2026-07-07*
