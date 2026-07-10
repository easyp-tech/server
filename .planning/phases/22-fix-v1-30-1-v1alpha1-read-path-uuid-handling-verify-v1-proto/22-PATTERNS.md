# Phase 22: Fix v1.30.1 v1alpha1 read-path UUID handling; verify v1 protocol - Pattern Map

**Mapped:** 2026-07-08
**Files analyzed:** 4 (2 modify, 1 modify-add, 1 new test)
**Analogs found:** 4 / 4

Scope reminder (from RESEARCH.md): wire the v1alpha1 `DownloadManifestAndBlobs` handler (`*api`, `blobs.go`) to the Phase 18 UUID-resolution machinery that already lives on `*commitServiceHandler` (`commits.go::probeCommitID` via `commitUUIDInverse`). No re-implementation; thin wrapper + back-pointer.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/connect/blobs.go` (MODIFY — add UUID-resolution branch to `DownloadManifestAndBlobs`) | connect handler (method on `*api`) | request-response (RPC) | `internal/connect/modulepins.go::GetModulePins` (v1alpha1 handler on `*api` calling `a.repo`) + `internal/connect/commits.go::ServeDownload` (the Phase 18 decision ladder being mirrored) | exact (role) + exact (ladder) |
| `internal/connect/api.go` (MODIFY — add back-pointer / `CommitResolver` field on `*api`; assign in `NewWithConfig`) | config / wiring (constructor) | construction-time | `internal/connect/api.go::NewWithConfig` lines 70-104 (the existing `a := &api{...}` → `commitHandler := &commitServiceHandler{api: a, ...}` sequence — the very block the new line is inserted into) | exact |
| `internal/connect/commits.go` (MODIFY — add `(*commitServiceHandler).resolveCommitForRead` wrapper) | service method (resolution wrapper) | request-response (lookup) | `internal/connect/commits.go::ServeDownload` lines 497-601 (the `commitMap` → `resolveForeignCommitID` → `probeCommitID` ladder being extracted) + `(*commitServiceHandler).probeCommitID` lines 983-1094 | exact |
| `internal/connect/blobs_test.go` (NEW — unit tests for PR-22-1/2/3) | test (Go `testing`, same package) | unit test | `internal/connect/api_test.go::TestServeDownload_AfterRestart_ProbeResolvesUUID` lines 1524-1592 (the Phase 18 UUID-probe test — identical shape: build UUID, cold `commitMap`, assert provider sees the resolved SHA) | exact |

Reference-only files (NOT modified, called out by the orchestrator): `internal/connect/commits_helpers.go` (`isUUID`, `commitUUIDInverse`), `internal/connect/api_test.go` (`mockProvider`, `mockSource`, `testMux`, `testMuxWithConfig`, `newTestCommitHandler`, `buildDownloadRequest`).

## Pattern Assignments

### `internal/connect/blobs.go` (MODIFY — Connect handler, request-response)

**Analogs:** `internal/connect/modulepins.go` (same role: v1alpha1 handler on `*api`) and `internal/connect/commits.go::ServeDownload` (the resolution ladder to mirror).

**Current imports — keep, add nothing new if a resolver interface is defined in `api.go`** (`internal/connect/blobs.go:1-13`):
```go
package connect

import (
	"bytes"
	"context"
	"fmt"

	"connectrpc.com/connect"

	module "github.com/easyp-tech/server/gen/proto/buf/alpha/module/v1alpha1"
	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	"github.com/easyp-tech/server/internal/shake256"
)
```

**Handler shape — the method signature MUST stay unchanged** (Connect handler registration relies on it; `internal/connect/blobs.go:17-27`):
```go
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
	// ... manifest + blobs unchanged ...
}
```

**Resolution-ladder pattern to mirror** — `ServeDownload` decision sequence (`internal/connect/commits.go:497-601`). The new branch in `DownloadManifestAndBlobs` should call a wrapper that runs this same ladder; do NOT inline the ladder in `blobs.go`:
```go
// commitMap fast path
h.commitMu.RLock()
if mapped, ok := h.commitMap[commitID]; ok { ref = &mapped }
h.commitMu.RUnlock()

// foreign-id fallback
if ref == nil { ref = h.resolveForeignCommitID(commitID) }

// probe fallback (UUID-aware via commitUUIDInverse + prefix-match)
if ref == nil && h.probeEnabled {
	probed, ok := h.probeCommitID(r.Context(), commitID)
	if ok { ref = probed }
}
```

**Resolution-result lookup pattern** — after a probe hit, the resolved SHA is read from `infoCache[owner/module].commit` (`internal/connect/commits.go:603-607, 671`):
```go
h.commitMu.RLock()
cached, infoOK := h.infoCache[ref.owner+"/"+ref.module]
h.commitMu.RUnlock()
// ...
files, err = h.api.repo.GetFiles(r.Context(), ref.owner, ref.module, meta.Commit) // meta.Commit = resolved 40-char SHA
```
`resolveCommitForRead` must return this `infoCache[...].commit` value (or equivalent) so `blobs.go` can pass it to `a.repo.GetFiles`.

**Error-handling pattern** — every error path through the v1alpha1 handlers funnels through `asConnectError` (`internal/connect/blobs.go:26`, `internal/connect/modulepins.go:22`, `internal/connect/bynames.go:24, 41`). The new resolution-error path MUST follow the same shape:
```go
if err != nil {
    return nil, asConnectError(fmt.Errorf("resolving commit uuid %q: %w", ref, err))
}
```
`asConnectError` (`internal/connect/validate.go:47-55`) maps `*validationError → CodeInvalidArgument (400)`, everything else `→ CodeInternal (500)`. Do not introduce a new error type; wrap with `fmt.Errorf("...: %w", err)`.

**Decision-log pattern** — `ServeDownload` emits a `slog.String("branch", ...)` "handler decision" line at every branch (`internal/connect/commits.go:510-532, 553-562, 573-591`). The new wrapper on `*commitServiceHandler` should emit the same shape so the v1alpha1 path is observable in ops logs. `*api` does not currently hold a structured-decision logger for this handler — log inside `resolveCommitForRead` via `h.hlog(...)`, not inside `blobs.go`.

---

### `internal/connect/api.go` (MODIFY — wiring/constructor, construction-time)

**Analog:** the existing `NewWithConfig` body (`internal/connect/api.go:63-121`) — the back-pointer assignment is a one-line insertion in this exact function.

**`*api` struct — where the new field goes** (`internal/connect/api.go:36-43`):
```go
type api struct {
	log *slog.Logger
	v1alpha1connect.UnimplementedRepositoryServiceHandler
	v1alpha1connect.UnimplementedResolveServiceHandler
	v1alpha1connect.UnimplementedDownloadServiceHandler
	repo   provider
	domain string
}
```
Add `commitResolver CommitResolver` (narrow interface, RESEARCH.md Pattern 1 / A3 recommends interface over concrete `*commitServiceHandler`) or `commitHandler *commitServiceHandler` (concrete, matches existing house style where `commitServiceHandler.api *api` is concrete). Either choice lives here.

**Constructor wiring — insert the assignment between lines 104 and 105** (`internal/connect/api.go:92-107`):
```go
commitHandler := &commitServiceHandler{
	api:             a,
	commitMap:       make(map[string]moduleRef),
	infoCache:       make(map[string]commitInfoCache),
	filesMap:        make(map[string][]content.File),
	knownOwners:     knownOwners,
	singleModule:    singleModule,
	missCache:       make(map[string]time.Time),
	probeEnabled:    cfg.ProbeEnabled,
	probeNegativeTTL: cfg.ProbeNegativeTTL,
	probeTimeout:    cfg.ProbeTimeout,
	probeSem:        make(chan struct{}, maxConcurrentProbes),
}
// INSERT: a.commitResolver = commitHandler   (or   a.commitHandler = commitHandler)
```
Safety rationale (RESEARCH.md A2): `NewWithConfig` runs single-threaded at startup; all handler invocations happen after the function returns, so the post-construction mutation of the `*api` pointer is safe. No additional synchronization needed.

**Back-pointer already exists in the reverse direction** (`internal/connect/api.go:93`) — `commitServiceHandler.api: a` — confirming the wiring idiom; the new field is the symmetric counterpart.

**`New` (test entrypoint) MUST stay compatible** (`internal/connect/api.go:52-59`): it delegates to `NewWithConfig` with `CommitResolution{}`, so the new field is populated in tests too — no test-breakage expected. The `testMux` / `testMuxWithConfig` helpers (`internal/connect/api_test.go:124-139`) call these two constructors directly.

---

### `internal/connect/commits.go` (MODIFY — add `resolveCommitForRead` service method)

**Analogs:** `ServeDownload` ladder (`internal/connect/commits.go:497-601`) and `probeCommitID` (`internal/connect/commits.go:983-1094`).

**Method to add — signature derived from RESEARCH.md sketch + ServeDownload's needs:**
```go
// resolveCommitForRead recovers the 40-char git SHA for a buf-issued id
// (UUID or raw SHA), reusing the commitMap → resolveForeignCommitID →
// probeCommitID ladder ServeDownload uses. Intended for the v1alpha1
// DownloadManifestAndBlobs handler on *api, which has no native access
// to this resolution machinery.
//
// Returns ("", error) when the id cannot be resolved — callers MUST
// surface the error, not fall back to HEAD (RESEARCH.md anti-pattern).
func (h *commitServiceHandler) resolveCommitForRead(
	ctx context.Context,
	owner, module, id string,
) (string, error)
```

**Body = thin wrapper around existing primitives — copy the ladder order verbatim from `ServeDownload` (`internal/connect/commits.go:497-592`):**
1. `commitMap[id]` lookup under `commitMu.RLock` (lines 500-506)
2. `resolveForeignCommitID(id)` (line 550) — already nil-safe, returns `*moduleRef`
3. `probeCommitID(ctx, id)` guarded by `h.probeEnabled` (lines 564-592) — this is where the UUID case is handled (Phase 18).

**UUID-aware probe primitive to reuse** (`internal/connect/commits.go:1001-1014, 1067-1069`) — already handles `isUUID(id)` via `commitUUIDInverse` and enforces the prefix-match validation (RESEARCH.md Pitfall 2). Do NOT re-implement any of this in the wrapper or in `blobs.go`:
```go
probeArg := id
if isUUID(id) {
	prefix, err := commitUUIDInverse(id)
	if err != nil { h.rememberMiss(id); return nil, false }
	probeArg = prefix
} else if !isSHA(id) {
	h.rememberMiss(id); return nil, false
}
// ... fan-out, then:
if isUUID(id) && !strings.HasPrefix(meta.Commit, probeArg) { return } // prefix-match
```

**Returning the SHA** — after a hit, read `infoCache[owner+"/"+module].commit` under `RLock` (`internal/connect/commits.go:603-607`). That value is the resolved 40-char SHA `GetFiles` needs. On miss, return a wrapped error (the caller will wrap again with `%w` and pass to `asConnectError`).

**Error semantics** — when the ladder misses entirely, return a non-validation error so `asConnectError` maps it to `CodeInternal` (500) or, if the planner prefers, a `NewValidationError(...)` for `CodeInvalidArgument` (400). RESEARCH.md Pitfall 4 ("Resolution failure surfaces an error, no silent HEAD fallback") is the only hard constraint; the code choice is the planner's.

**`probeCommitID` full signature for reference** (`internal/connect/commits.go:983`): `func (h *commitServiceHandler) probeCommitID(ctx context.Context, id string) (*moduleRef, bool)`. The wrapper consumes both return values.

**Do NOT refactor `ServeDownload` itself** (RESEARCH.md Open Question 2) — keep this phase's blast radius minimal. A future phase can DRY `ServeDownload` onto the same helper.

---

### `internal/connect/blobs_test.go` (NEW — unit test, same `package connect`)

**Analogs:** `TestServeDownload_AfterRestart_ProbeResolvesUUID` (`internal/connect/api_test.go:1524-1592`) — identical scenario shape (UUID input, cold `commitMap`, expect probe fan-out + resolved SHA). And `TestProbeCommitID_HitResolvesAndCaches` (`internal/connect/api_test.go:1367-1387`) for the direct-wrapper variant.

**Package + helpers to reuse — same package, so all of these are directly callable** (`internal/connect/api_test.go`):
- `mockProvider` with `byCommit` / `filesByCommit` maps (lines 32-67) — lets the test assert the provider received the resolved 40-char SHA, not the UUID.
- `mockSource{owner, repoName, commit, getMetaCalls *atomic.Int32}` (lines 78-122) — counts probe fan-out calls.
- `testMux(provider)` / `testMuxWithConfig(provider, log, CommitResolution)` (lines 124-139) — builds the full mux via `New`/`NewWithConfig`; required for the v1alpha1 test because the new code path lives on `*api` (not on a standalone `commitServiceHandler`).
- `newTestCommitHandler(provider)` (lines 1348-1362) — for a direct `resolveCommitForRead` unit test that bypasses the HTTP layer (mirrors `TestProbeCommitID_*`).
- `buildDownloadRequest(commitID)` (lines 215-233) — only useful for the v1beta1 raw-HTTP path, NOT the v1alpha1 Connect RPC. For `DownloadManifestAndBlobs` use the Connect client (`v1alpha1connect.NewDownloadServiceClient`) — see `TestDownloadServiceV1ReturnsProtobuf` (lines 422-480) for the v1alpha1 client-call pattern.

**v1alpha1 client-call pattern to copy** — `TestDownloadServiceV1ReturnsProtobuf` (`internal/connect/api_test.go:422-480`) shows how to drive a Connect RPC against `testMux` via `httptest.NewServer`. The new test issues `DownloadServiceClient.DownloadManifestAndBlobs` with `Reference = <32-char UUID>` and asserts (a) the response is 200 with non-empty manifest, and (b) `mockProvider` recorded a `GetFiles` call whose commit arg is the resolved 40-char SHA, not the UUID. Use `byCommit`/`filesByCommit` keyed on the SHA to make the UUID-keyed lookup fail loudly if the handler forgets to resolve.

**Required test cases (from RESEARCH.md PR-22-1/2/3):**
1. `TestDownloadManifestAndBlobs_ResolveUUID` — UUID input, warm `commitMap` (pre-seed via `CommitService/GetCommits`, mirroring lines 436-452), assert provider receives SHA.
2. `TestDownloadManifestAndBlobs_ProbeFallback` — UUID input, cold `commitMap` (no pre-seed, post-restart state), `probeEnabled=true`, assert `mockSource.getMetaCalls > 0` and provider receives SHA. Mirrors `TestServeDownload_AfterRestart_ProbeResolvesUUID`.
3. `TestDownloadManifestAndBlobs_ResolveError` — UUID whose 14-byte prefix matches no source, assert RPC returns non-OK (Connect error), no `GetFiles` call. Mirrors `TestServeDownload_AfterRestart_ProbeMissesOnUnknownUUID` (`internal/connect/api_test.go:1601+`).

**Assertions library** — `github.com/stretchr/testify/require` (already in `go.mod`, used elsewhere in the package's tests; RESEARCH.md Validation Architecture).

## Shared Patterns

### Error handling (all Connect handlers on `*api`)
**Source:** `internal/connect/validate.go:39-55` + every handler in `blobs.go`, `modulepins.go`, `bynames.go`.
**Apply to:** `blobs.go::DownloadManifestAndBlobs` (modified branch), any error surfaced from `resolveCommitForRead`.
```go
// At each error return in a Connect handler:
return nil, asConnectError(fmt.Errorf("<context>: %w", err))
```
`asConnectError` is nil-safe and walks the wrap chain for `*validationError`. Never construct `connect.NewError` directly in a handler — go through `asConnectError` so the code mapping stays consistent.

### Mutex discipline (any new code touching `commitMap` / `infoCache` / `missCache`)
**Source:** `internal/connect/commits.go:500-506, 526-530, 604-607, 877-901, 989-991`.
**Apply to:** `(*commitServiceHandler).resolveCommitForRead`.
- All three maps are guarded by `commitMu` (RWMutex).
- Lookups use `RLock` / `RUnlock`; mutations use `Lock` / `Unlock`.
- **Callers must NOT hold `commitMu` when calling `probeCommitID`** (documented at `commits.go:980-982`) — `probeCommitID` acquires `probeSem` and can fan out to slow upstream calls. The wrapper must release the lock before calling `probeCommitID`, exactly as `ServeDownload` does.

### UUID detection + inverse (cross-cutting for the UUID code path)
**Source:** `internal/connect/commits_helpers.go:67-132`.
**Apply to:** only via `probeCommitID` — do NOT call `isUUID` / `commitUUIDInverse` directly from `blobs.go`. The wrapper `resolveCommitForRead` also should not call them directly; it delegates to `probeCommitID` which already does. RESEARCH.md "Don't Hand-Roll" table is explicit on this.
- `isUUID(s)` — 32 lowercase hex chars (`commits_helpers.go:87-97`).
- `isSHA(s)` — 40 or 64 lowercase hex chars (`commits_helpers.go:72-82`).
- `commitUUIDInverse(uuid)` — recovers first 28 hex chars (14 bytes) of the originating SHA (`commits_helpers.go:116-132`).

### Decision-log attributes
**Source:** `ServeDownload` log lines (`internal/connect/commits.go:510-532, 553-591`).
**Apply to:** `resolveCommitForRead` (log there, not in `blobs.go` — `*api` handlers in this package do not emit structured decision logs; the commit handler does, via `h.hlog(r)`). For the wrapper, `r *http.Request` is unavailable (Connect-RPC context only); use `h.api.log.With(slog.String("handler", "DownloadManifestAndBlobs"), ...)` directly with `ctx`-derived request id if needed.

## No Analog Found

None. Every file in this phase has a strong exact-match analog:
- The v1alpha1 handler shape is pinned by `blobs.go` itself and `modulepins.go`.
- The resolution ladder is pinned by `ServeDownload` + `probeCommitID`.
- The test shape is pinned by `TestServeDownload_AfterRestart_ProbeResolvesUUID`.

## Metadata

**Analog search scope:** `internal/connect/` (api.go, api_test.go, blobs.go, commits.go, commits_helpers.go, modulepins.go, validate.go, bynames.go).
**Files scanned:** 8 (all `internal/connect/*.go`; no other packages contain relevant analogs — confirmed via grep for `probeCommitID`, `asConnectError`, `DownloadManifestAndBlobs`).
**Pattern extraction date:** 2026-07-08.
**Cross-references:** RESEARCH.md Pattern 1 (back-pointer) ↔ `api.go:36-43, 92-104`; RESEARCH.md Pattern 2 (reuse probeCommitID) ↔ `commits.go:983-1094`; RESEARCH.md Pitfall 2 (prefix-match) ↔ `commits.go:1067-1069`.
