# Phase 22: Fix v1.30.1 v1alpha1 read-path UUID handling; verify v1 protocol - Research

**Researched:** 2026-07-08
**Domain:** Go / Connect RPC / buf BSR registry protocol / GitHub git-trees API
**Confidence:** HIGH

## Summary

Phase 21's live e2e test (`TestGenerateWithPinnedBufLock` / `v1.30.1` subtest) caught a real proxy regression: when `buf v1.30.1` runs `buf generate` against a `buf.lock` pinning a 32-char buf-issued UUID, the v1alpha1 `DownloadManifestAndBlobs` handler passes the UUID verbatim to `a.repo.GetFiles(...)` → `multisource.GetFiles` → `github.GetFiles` → `c.git.GetTree(ctx, owner, repo, UUID, true)`, which GitHub rejects with 404 because the git-trees API expects a 40-char git SHA, not a 32-char buf UUID. Phase 18 already solved the identical class of bug for the v1beta1 / `ServeDownload` path by adding `commitUUIDInverse` in `internal/connect/commits_helpers.go:116-132` and wiring it into `probeCommitID` (`internal/connect/commits.go:983-1078`); the v1alpha1 `DownloadManifestAndBlobs` handler was simply never wired to that resolution machinery.

The fix is narrow and surgical: detect a 32-char UUID in the incoming `reference`, run it through the existing `commitMap` → `resolveForeignCommitID` → `probeCommitID` (which already handles UUIDs via `commitUUIDInverse` + prefix-probe + prefix-match validation), and pass the resolved 40-char SHA to `GetFiles`. The architectural blocker is that the v1alpha1 handler lives on `*api` (`blobs.go`) while the resolution machinery lives on `*commitServiceHandler` (`commits.go`), and `*api` currently has no back-reference to the commit handler — only the reverse pointer (`commitServiceHandler.api *api`) exists. The cleanest fix adds a back-pointer (or a narrow resolver interface) on `*api`, set in `NewWithConfig` immediately after `commitHandler` is constructed, plus a small resolution method on `*commitServiceHandler` that wraps the existing `commitMap`/`probeCommitID` path for the UUID case.

**Primary recommendation:** Add a back-pointer from `*api` to `*commitServiceHandler` (or a `commitResolver` interface), expose `(*commitServiceHandler).resolveCommitForRead(ctx, owner, module, id) (sha string, err error)` reusing the existing `commitMap` + `probeCommitID` logic, and call it from `blobs.go::DownloadManifestAndBlobs` when `isUUID(reference)`. Gate verification with a new unit test in `internal/connect/` (no unit test currently covers `DownloadManifestAndBlobs`) plus the Phase 21 e2e regression (`TestGenerateWithPinnedBufLock` with `EASYP_GH_TOKEN`).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| v1alpha1 `DownloadManifestAndBlobs` RPC handling | API / Backend (Connect handler on `*api`, `blobs.go`) | — | Connect RPC handler; buf v1.30.1 calls this directly with the UUID from `buf.lock` |
| buf UUID → git SHA resolution | API / Backend (`*commitServiceHandler`, `commits.go`) | — | Owns `commitMap`, `resolveForeignCommitID`, `probeCommitID` (already UUID-aware via Phase 18) |
| File-tree fetch | Provider layer (`github.GetFiles` → `git.GetTree`) | — | Expects a 40-/64-char SHA or ref; 32-char UUIDs are not acceptable inputs |
| Regression guard | E2E test (`e2e/generate_test.go`) | — | Phase 21 matrix test; v1.30.1 subtest is the authoritative gate |

## Standard Stack

No new dependencies. This phase modifies existing Go code only.

### Core (already in repo)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `connectrpc.com/connect` | (pinned in go.mod) | v1alpha1 RPC handlers (`*api` implements `DownloadServiceHandler`) | Already the framework for `blobs.go` |
| `github.com/easyp-tech/server/internal/connect` | local | `commitUUIDInverse`, `isUUID`, `commitMap`, `probeCommitID` | Phase 18 deliverables; reused as-is |
| `github.com/google/go-github/v59` | (pinned) | `git.GetTree` (the failing call site) | Unchanged — the fix must keep 32-char UUIDs from reaching it |

**Version verification:** Not applicable — no new packages. All referenced symbols verified by grepping the current source tree.

## Package Legitimacy Audit

Not applicable — no external packages are installed by this phase. All code lives in `github.com/easyp-tech/server/internal/...`.

## Architecture Patterns

### System Architecture Diagram

```
buf v1.30.1 "buf generate" (buf.lock pins 32-char UUID)
        │
        ▼
POST /buf.alpha.registry.v1alpha1.ResolveService/DownloadManifestAndBlobs
        {owner, repository, reference = <32-char UUID>}
        │
        ▼
(*api).DownloadManifestAndBlobs            [blobs.go:17]   ◀── BUG: passes UUID verbatim
        │
        ├── (FIX) if isUUID(reference): resolve UUID → 40-char SHA
        │       via (*commitServiceHandler).resolveCommitForRead
        │           ├── commitMap[id]                              (fast path)
        │           ├── resolveForeignCommitID(id)                 (single-module fallback)
        │           └── probeCommitID(id)                          (Phase 18 path)
        │                   ├── commitUUIDInverse(id) → 28-char prefix
        │                   ├── fan-out source.GetMeta(prefix)     (all configured sources)
        │                   └── prefix-match validation            (close review.md #6)
        │
        ▼
a.repo.GetFiles(ctx, owner, repo, <resolved 40-char SHA>)
        │
        ▼
multisource.GetFiles → github.GetFiles → c.git.GetTree(..., SHA, true)
        │
        ▼
200 OK: manifest + blobs → buf client → codegen → gen/go/google/type/*.pb.go
```

### Recommended Project Structure
```
internal/connect/
├── api.go                  # *api struct — ADD back-pointer to *commitServiceHandler (or resolver interface)
├── blobs.go                # DownloadManifestAndBlobs — ADD UUID-resolution branch before GetFiles
├── commits.go              # *commitServiceHandler — ADD resolveCommitForRead method (wraps existing logic)
├── commits_helpers.go      # commitUUIDInverse / isUUID / isSHA — UNCHANGED (reused)
└── blobs_test.go (NEW) or api_test.go — ADD unit test for DownloadManifestAndBlobs UUID path
e2e/
└── generate_test.go        # TestGenerateWithPinnedBufLock — UNCHANGED (it is the gate; just re-run with token)
```

### Pattern 1: Back-pointer from `*api` to `*commitServiceHandler`
**What:** The `*api` struct currently has no link to the commit-resolution state. `NewWithConfig` (`api.go:51-95`) constructs `a := &api{...}` first, then `commitHandler := &commitServiceHandler{api: a, ...}`. The fix adds a field on `*api` (e.g. `commitResolver CommitResolver` or `commitHandler *commitServiceHandler`) and assigns it after `commitHandler` is built but before the mux is returned (a one-line `a.commitHandler = commitHandler` after construction is safe — `a` is a pointer, the mutation is visible to the registered handlers).
**When to use:** Whenever a v1alpha1 Connect handler (method on `*api`) needs the commit-resolution machinery that already lives on `*commitServiceHandler`.
**Tradeoff note:** A narrow `CommitResolver` interface is preferable to a concrete `*commitServiceHandler` pointer — it keeps `blobs.go` decoupled from the handler's full surface and makes the unit test in `api_test.go` able to inject a fake resolver (the existing `testMux` helper uses `New` / `NewWithConfig`, both of which must continue to compile).

### Pattern 2: Reuse Phase 18's `probeCommitID` — do NOT re-implement
**What:** `(*commitServiceHandler).probeCommitID(ctx, id)` (`commits.go:983`) already accepts a 32-char UUID, derives the 28-char SHA prefix via `commitUUIDInverse`, fans out `source.GetMeta(prefix)` across all configured sources, validates the result via prefix-match, and registers the resolved alias in `commitMap`. The new `resolveCommitForRead` helper should be a thin wrapper that runs the SAME sequence (`commitMap` lookup → `resolveForeignCommitID` → `probeCommitID`) and returns the resolved git SHA, NOT a fresh re-implementation.
**Why:** Phase 18 locked in the prefix-match validation to close review.md finding #6 (a wrong-source match silently aliasing a real commit id to a wrong module). Re-implementing the probe in `blobs.go` would risk dropping that validation.

### Anti-Patterns to Avoid
- **Re-implementing `commitUUIDInverse` / the probe fan-out in `blobs.go`.** Duplicates the byte-table and the prefix-match validation; guarantees future drift. The existing `probeCommitID` is the single source of truth.
- **Putting UUID resolution in the provider layer (`multisource` / `github`).** The providers are currently clean of buf-id semantics; `commitUUIDInverse` is a connect-layer concern. Pushing buf logic into `github/getfiles.go` pollutes the provider boundary and breaks the pattern Phase 18 established.
- **Gating the v1alpha1 fix behind `probeEnabled = false`.** If the back-pointer's resolution method declines when `probeEnabled` is false AND the commitMap is cold, the v1.30.1 read path stays broken whenever probe is disabled. The fix should at minimum consult `commitMap` (cheap) and only fall through to `probeCommitID` (which itself respects `probeEnabled`). Note: the e2e harness starts the proxy with no `connect:` block in YAML, which defaults `Probe.Enabled = true` (`config.go:106-108`), so the gate passes — but the fix must not introduce a new "only works with probe enabled" cliff for direct callers.
- **Returning HEAD when resolution fails.** A silent HEAD fallback would mask the regression and break the Phase 21 content assertion (`package google.type`). A failed resolution must surface as an error, not as a 200 with wrong content.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| UUID → git SHA recovery | Re-derive byte table or SHA-1 inverse | `commitUUIDInverse` (`commits_helpers.go:116`) | Phase 18 locked the byte table + tests (`TestCommitUUIDInverse`) |
| Source fan-out + prefix-match validation | New probe loop in `blobs.go` | `(*commitServiceHandler).probeCommitID` (`commits.go:983`) | Already handles UUIDs; already enforces review.md #6 prefix-match |
| Detecting UUID shape | New 32-hex checker | `isUUID` (`commits_helpers.go:87`) | `TestIsUUID` already pins the contract |

**Key insight:** Phase 18 already built and tested every primitive this fix needs. The only missing piece is a wiring change so the v1alpha1 `DownloadManifestAndBlobs` handler can reach them.

## Common Pitfalls

### Pitfall 1: Forgetting that the e2e harness pins a NON-HEAD commit
**What goes wrong:** A fix that only consults `commitMap` (the in-session cache) passes unit tests but fails the e2e gate.
**Why it happens:** `runBufGenerate` runs `buf mod update` first (which populates `commitMap` with the HEAD UUID), then **overwrites** the lock with a DIFFERENT UUID derived from `refs/tags/common-protos-1_3_1` (a non-HEAD tag). The overwritten UUID is never in `commitMap`.
**How to avoid:** The resolution path MUST reach `probeCommitID` for the e2e scenario to pass. Verify the fix exercises `probeCommitID` (the planner's unit test should cover the cache-miss → probe branch, not just the cache-hit branch).
**Warning signs:** Unit test passes, e2e `v1.30.1` subtest still 404s.

### Pitfall 2: Dropping the prefix-match validation
**What goes wrong:** A hand-rolled probe accepts a `GetMeta` result whose SHA does NOT start with the recovered 28-char prefix, silently aliasing a foreign commit to the wrong module.
**Why it happens:** The prefix from `commitUUIDInverse` is 14 bytes / 112 bits — collision-resistant but not collision-proof across disjoint commit spaces. Phase 18 added the `strings.HasPrefix(meta.Commit, probeArg)` check (`commits.go:1067`) specifically to close this (review.md finding #6).
**How to avoid:** Route through `probeCommitID` verbatim; do not write a new probe loop. If a separate helper is unavoidable, it MUST replicate the `isUUID(id) && !strings.HasPrefix(meta.Commit, probeArg) → miss` branch.
**Warning signs:** A "simpler" probe helper that omits the prefix check.

### Pitfall 3: Treating this as a `buf generate`-only bug
**What goes wrong:** The fix is scoped to `buf generate` and misses that `buf mod update` step 2 (re-update against an existing lock) and any other v1alpha1 client RPC that sends a UUID also hit the same `DownloadManifestAndBlobs` handler.
**Why it happens:** Phase 21's test name (`TestGenerateWithPinnedBufLock`) suggests a generate-specific scope.
**How to avoid:** Frame the fix as "`DownloadManifestAndBlobs` mishandles UUIDs", not "buf generate mishandles UUIDs". The unit test should target the handler, not the CLI command. `TestOldProtocolBufModUpdateTwice` (`e2e/old_proto_test.go`) exercises the same RPC and is a secondary regression signal.
**Warning signs:** Plan tasks named "fix buf generate path".

### Pitfall 4: Breaking the v1.69.0+ path
**What goes wrong:** Refactoring `DownloadManifestAndBlobs` to live on `*commitServiceHandler` (instead of adding a back-pointer) breaks the Connect handler registration (`v1alpha1connect.NewDownloadServiceHandler(a, ...)` requires the method on the registered handler struct).
**Why it happens:** Tempting to "unify" the two Download handlers.
**How to avoid:** Keep `DownloadManifestAndBlobs` on `*api`; reach the resolver via back-pointer/interface only. The v1.69.0+ client uses the raw-HTTP v1beta1 `ServeDownload` path (`commits.go`) which is untouched by this fix.
**Warning signs:** Plan moves `blobs.go` into `commits.go`.

## Code Examples

### Current failing code (blobs.go:17-27)
```go
// Source: internal/connect/blobs.go (current)
func (a *api) DownloadManifestAndBlobs(
	ctx context.Context,
	req *connect.Request[registry.DownloadManifestAndBlobsRequest],
) (
	*connect.Response[registry.DownloadManifestAndBlobsResponse],
	error,
) {
	files, err := a.repo.GetFiles(ctx, req.Msg.GetOwner(), req.Msg.GetRepository(), req.Msg.GetReference())
	if err != nil {
		return nil, asConnectError(fmt.Errorf("a.repo.GetRepository: %w", err))
	}
	// ... build manifest + blobs ...
}
```

### Sketch of the fix (illustrative — final shape is the planner's call)
```go
// Source: internal/connect/blobs.go (after fix)
func (a *api) DownloadManifestAndBlobs(
	ctx context.Context,
	req *connect.Request[registry.DownloadManifestAndBlobsRequest],
) (
	*connect.Response[registry.DownloadManifestAndBlobsResponse],
	error,
) {
	ref := req.Msg.GetReference()
	// v1alpha1 path (buf v1.30.1): buf.lock pins a 32-char buf-issued UUID.
	// GitHub's git-trees API rejects it with 404. Recover the git SHA via
	// the same commitMap / probeCommitID path Phase 18 wired for ServeDownload.
	if isUUID(ref) && a.resolver != nil {
		resolved, err := a.resolver.resolveCommitForRead(ctx, req.Msg.GetOwner(), req.Msg.GetRepository(), ref)
		if err != nil {
			return nil, asConnectError(fmt.Errorf("resolving commit uuid %q: %w", ref, err))
		}
		ref = resolved
	}
	files, err := a.repo.GetFiles(ctx, req.Msg.GetOwner(), req.Msg.GetRepository(), ref)
	// ... unchanged ...
}
```

`resolveCommitForRead` on `*commitServiceHandler` should reuse the exact decision ladder `ServeDownload` already uses (`commits.go:~500-590`): `commitMap` hit → `resolveForeignCommitID` → `probeCommitID`. Returning the resolved SHA (looked up from `infoCache[owner/module].commit` after a probe hit) is the value `GetFiles` needs.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| v1beta1 `ServeDownload` only path that resolves UUIDs | `probeCommitID` UUID-aware (Phase 18) | 2026-07-07 (commit `4fc6f28` / Phase 18) | v1beta1 read-path works; v1alpha1 `DownloadManifestAndBlobs` was never wired |
| `prewarmHeads` / `registerResolved` eager pre-warm | Removed; `probeCommitID` resolves on demand | Phase 18 | Cold-start `commitMap` is expected; the e2e test starts a fresh server, so resolution MUST go through the probe |

**Deprecated/outdated:**
- The buf.gen.yaml `remote:` field (alpha-remote-generation) — Phase 21 already replaced it with `plugin:`. This phase inherits that fix; nothing to do.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `buf v1.30.1 generate` sends the 32-char UUID directly to `DownloadManifestAndBlobs` without a prior `GetCommits` in the same session | Summary / Architecture | If the client ALSO calls `GetCommits` (or `GetModulePins`) first, `commitMap` would be warm and a commitMap-only fix would suffice — but Phase 21 logs explicitly attribute the UUID to `DownloadManifestAndBlobs`, so this is well-supported. `[ASSUMED]` only because the exact RPC trace was not re-captured in this research session. |
| A2 | The back-pointer mutation `a.commitHandler = commitHandler` after `commitHandler` is constructed is safe (no goroutine reads it before `mux` is returned) | Pattern 1 | LOW — `NewWithConfig` runs single-threaded at startup; all handler invocations happen after the function returns. Verifiable by inspection. |
| A3 | A narrow `CommitResolver` interface on `*api` is preferable to a concrete `*commitServiceHandler` pointer | Pattern 1 / Anti-patterns | MEDIUM — design preference; the existing code uses concrete pointers (`commitServiceHandler.api *api`), so a concrete pointer is consistent with house style. Planner's call. |

**Note on `[ASSUMED]` tagging:** Phase 21's live-test log is the authoritative source for A1 (it explicitly identifies `DownloadManifestAndBlobs` as the failing handler and the UUID as the incoming reference). The claim is tagged `[ASSUMED]` only because the RPC trace was not re-captured in this session, not because Phase 21's evidence is weak.

## Open Questions (RESOLVED)

1. **Should the fix also cover the `reference == ""` and `reference == "main"` cases on this handler?** — RESOLVED: keep narrowly scoped to `isUUID(reference)`; `reference == "main"` deferred to a separate follow-up (proposed Phase 23). Implemented in 22-01-PLAN.md Task 2 Step 5 (forbids the `reference == "main"` branch).
   - What we know: `GetModulePins` (modulepins.go) already handles those for the v1alpha1 path. `DownloadManifestAndBlobs` receiving `"main"` would also fail at `GetTree`, but Phase 21 only surfaced the UUID case.
   - Recommendation: Keep the fix narrowly scoped to `isUUID(reference)`. A `reference == "main"` branch would conflate concerns; if it is a real path, surface it as a separate follow-up (proposed Phase 23).

2. **Should `resolveCommitForRead` be a new public-ish method, or should the planner refactor `ServeDownload`'s decision ladder into a shared helper first?** — RESOLVED: do NOT refactor `ServeDownload` this phase; add a small `resolveCommitForRead` helper mirroring the three-step ladder, return `(sha, error)`. Implemented in 22-01-PLAN.md Task 2 Step 5 (forbids ServeDownload refactor) + Step 3 (adds the thin wrapper).
   - What we know: `ServeDownload` (commits.go:~500-590) currently inlines the `commitMap` → `resolveForeignCommitID` → `probeCommitID` ladder.
   - Recommendation: Do NOT refactor `ServeDownload` in this phase (out of scope, higher blast radius). Instead, add a small `resolveCommitForRead` helper that runs the same three steps and returns `(sha, error)`. A future phase can DRY up `ServeDownload` to call the same helper.

3. **Unit test injection: does the new `*api` back-pointer need a nil-safe path for `testMux`?** — RESOLVED: include the nil guard (`a.commitResolver != nil` branch). Implemented in 22-01-PLAN.md Task 2 Step 4.
   - What we know: `testMux` → `New` → `NewWithConfig` always constructs a `commitServiceHandler`, so the back-pointer is never nil in tests. But a nil-check (`a.resolver != nil`) keeps `blobs.go` robust if a future caller constructs `*api` without the commit handler.
   - Recommendation: Include the nil guard; it costs one branch and prevents a nil-deref in hypothetical embedded use.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build + unit tests | ✓ | go1.26.4 darwin/arm64 | — |
| `buf v1.30.1` binary | e2e regression gate (cached) | ✓ (per Phase 21 summary: `testutil.BufV130`, fetched via `testutil.GetBuf`) | v1.30.1 | — |
| `buf v1.69.0` binary | e2e matrix (v1.69.0 subtest) | ✓ (per Phase 21) | v1.69.0 | — |
| `EASYP_GH_TOKEN` | e2e live gate | unknown at research time (env-scoped) | — | Test skips cleanly without it (`testutil.RequireEnvToken`) |
| TLS certs at `~/local-tls/server/` | `testutil.DefaultTestConfig` | ✓ (per Phases 19-21) | — | — |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** If `EASYP_GH_TOKEN` is unset on the verification run, the e2e test SKIPs cleanly (no FAIL) but the regression gate is not exercised — the planner must require a token-bearing run as the phase gate.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` (go1.26.4); `github.com/stretchr/testify/require` for assertions |
| Config file | none (Go convention; `go test ./...`) |
| Quick run command | `go test ./internal/connect/ -count=1` |
| Full suite command | `go test ./... -count=1` (unit) + `EASYP_GH_TOKEN=... go test ./e2e/ -run TestGenerateWithPinnedBufLock -count=1` (e2e gate) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PR-22-1 | `DownloadManifestAndBlobs` resolves a 32-char UUID reference to the 40-char SHA before calling `GetFiles` | unit | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ResolveUUID -count=1` | NEW — Wave 0 must create `internal/connect/blobs_test.go` (or extend `api_test.go`) |
| PR-22-2 | UUID cache-miss reaches `probeCommitID` (not just `commitMap`) | unit | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ProbeFallback -count=1` | NEW — same file |
| PR-22-3 | Resolution failure surfaces an error (no silent HEAD fallback) | unit | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ResolveError -count=1` | NEW — same file |
| PR-22-4 | v1.30.1 `buf generate` against a pinned `buf.lock` exits 0 and produces `package google.type` content | e2e (regression gate) | `EASYP_GH_TOKEN=... go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1` | ✅ exists (`e2e/generate_test.go`) — no harness change needed |
| PR-22-5 | Existing `commitUUIDInverse` / `isUUID` / `probeCommitID` helpers unchanged | unit (guard) | `go test ./internal/connect/ -run 'TestCommitUUIDInverse|TestIsUUID|TestIsSHA' -count=1` | ✅ exists (`commits_helpers_test.go`, `api_test.go`) |

### Sampling Rate
- **Per task commit:** `go test ./internal/connect/ -count=1` + `go build ./...` + `go vet ./...`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full unit suite green AND `EASYP_GH_TOKEN=... go test ./e2e/ -run TestGenerateWithPinnedBufLock -count=1` green (specifically the `v1.30.1` subtest; the `v1.69.0` subtest has a known unrelated TLS-timeout issue documented in Phase 21's Finding 2 and must NOT block this phase).

### Wave 0 Gaps
- [ ] `internal/connect/blobs_test.go` (or extend `internal/connect/api_test.go`) — covers PR-22-1, PR-22-2, PR-22-3. Reuse the existing `mockProvider` (with `byCommit` / `filesByCommit` maps, already wired in `api_test.go:37-67`) to assert the provider receives the resolved SHA, not the UUID.
- [ ] No framework install needed — Go `testing` + testify already in `go.mod`.

*(If the planner prefers a single file: extend `api_test.go` to keep `mockProvider` reuse local. A new `blobs_test.go` is cleaner but requires the mock helpers to be reachable — they are package-private in `api_test.go`, so a same-package `blobs_test.go` can use them directly.)*

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V5 Input Validation | yes | `isUUID` / `isSHA` 32-/40-/64-char lowercase-hex validation (`commits_helpers.go:67-98`) — already the gate; the fix reuses it unchanged |
| V6 Cryptography | no | The byte-table in `commitUUID` / `commitUUIDInverse` is a deterministic derivation, not a cryptographic primitive; no crypto change in this phase |

### Known Threat Patterns for the Go / Connect / GitHub stack on this path

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Forged 32-char UUID forcing an upstream probe storm | Denial of Service | `maxConcurrentProbes = 4` semaphore (`commits.go:71`) + `probeTimeout` per-source + `missCache` negative caching — already in place from Phase 18; the fix inherits them by routing through `probeCommitID` |
| Wrong-source prefix collision aliasing a UUID to the wrong module | Tampering / Information Disclosure | `strings.HasPrefix(meta.Commit, probeArg)` validation (`commits.go:1067`) — must be preserved (see Pitfall 2) |
| UUID reflecting upstream error text into client response | Information Disclosure | Route errors through `asConnectError` (already the pattern in `blobs.go:26`); do not include raw upstream bodies in the response |

## Sources

### Primary (HIGH confidence)
- `internal/connect/blobs.go:17-56` — the failing handler (read in this session)
- `internal/connect/commits.go:983-1078` — `probeCommitID` UUID handling (read in this session)
- `internal/connect/commits_helpers.go:45-132` — `commitUUID`, `commitUUIDInverse`, `isSHA`, `isUUID` (read in this session)
- `internal/connect/api.go:51-118` — `NewWithConfig` wiring; `*api` / `*commitServiceHandler` relationship (read in this session)
- `internal/connect/commits.go:500-700` — `ServeDownload` decision ladder (the pattern to mirror)
- `internal/providers/github/getfiles.go:33` — the `c.git.GetTree` call site that 404s (read in this session)
- `.planning/phases/21-.../21-01-SUMMARY.md` — Finding 1 (authoritative bug report), Finding 2 (v1.69.0 unrelated TLS issue)
- `e2e/generate_test.go` — the regression gate (read in this session)
- `e2e/testutil/config.go` + `cmd/easyp/internal/config/config.go:103-113` — confirms e2e harness starts with `Probe.Enabled` defaulting to `true`

### Secondary (MEDIUM confidence)
- `git show 4fc6f28` — Phase 18 ship commit (verified `commits_helpers.go` + `probeCommitID` wiring landed)

### Tertiary (LOW confidence)
- None.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new dependencies; all symbols verified by grepping the current source tree.
- Architecture: HIGH — bug location, fix location, and wiring gap are all directly visible in the source.
- Pitfalls: HIGH — all four pitfalls are grounded in code evidence (Phase 21 summary + `runBufGenerate` lock-overwrite pattern + Phase 18 prefix-match validation).
- Verification gate: HIGH — Phase 21 test exists and is the authoritative gate; harness change not required.

**Research date:** 2026-07-08
**Valid until:** 2026-08-07 (30 days — stable internal-refactor phase, no external API surface)
