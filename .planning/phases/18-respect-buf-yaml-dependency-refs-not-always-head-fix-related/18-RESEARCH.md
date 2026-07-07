# Phase 18: Respect buf.yaml dep refs, fix related bug, drop prewarm - Research

**Researched:** 2026-07-07
**Domain:** Bugfix + small refactor in connect/ — three independent corrections bundled into one phase
**Confidence:** HIGH (all claims verifiable against the source files; no external library research needed)

## Summary

Phase 18 is a tightly-scoped bugfix that lands three corrections to how the proxy resolves a `buf.yaml` dependency:

1. **Make the proxy return the ref the user wrote in `buf.yaml`, not always HEAD.** Today the proxy ignores the `ref` field on the `Name` message (commits_helpers.go:108-136 parses only `owner` and `module`) and the bitbucket/github providers treat any non-`""`/non-`"main"` input as a raw SHA (bitbucket/getrepo.go:18-20, github/getrepo.go:21-23). A dependency like `buf-proxy.yadro.dev/cyp/cyp-net-listeners:main/v2` therefore either collapses to HEAD (ref ignored) or 500s (ref treated as raw SHA, fails hex decode / BitBucket API).
2. **Fix the related bug.** The same `commit != "" && commit != "main"` short-circuit in both providers means that ANY non-`""`/non-`"main"` input — including a properly-resolved ref — gets stamped into `meta.Commit` as a literal string. The fix is a single check: if the input is not 40 or 64 lowercase hex, treat it as a ref and resolve it to a SHA via the provider's commit-fetch API (Bitbucket `/commits/{id}`, GitHub `GET /repos/{o}/{r}/commits/{sha}` — both endpoints accept ref names and short SHAs in addition to full SHAs).
3. **Remove the prewarm logic.** With the new inverse function (`commitUUIDInverse` — recovers the 28-char SHA prefix from a 32-char buf UUID), `probeCommitID` can use the UUID → SHA-prefix derivation to identify the right source on a Download cache-miss without the startup fan-out. The prewarm at [commits.go:968-999](internal/connect/commits.go#L968-L999) and its 3 supporting helpers (the goroutine launch in api.go:111-113, the `prewarmEnabled` / `prewarmTimeout` / `prewarmOnce` fields, the `PrewarmConfig` block in config.go) all go away.

**Primary recommendation:** One plan with three tasks. The three tasks share the `commitUUID` module (task 1 needs the inverse for ref-resolution sanity checks; task 2 adds the inverse; task 3 uses the inverse to replace prewarm in the probe path). Splitting them into separate plans would require shipping task 1 with a hand-rolled inverse or duplicating the SHA-prefix logic across files.

## Key Code Facts (verified against source)

### 1. The ref-parsing gap (commits_helpers.go:108-136)

```go
func parseResourceRefName(msg []byte) *moduleRef {
    var owner, module string
    for len(msg) > 0 {
        num, typ, n := protowire.ConsumeTag(msg)
        ...
        if num == 1 && typ == protowire.BytesType { owner = string(v) }
        else if num == 2 && typ == protowire.BytesType { module = string(v) }
        else { n = protowire.ConsumeFieldValue(num, typ, msg) ... }
    }
    ...
}
```

The `Name` proto in buf v1/v1beta1 has `owner=1, module=2, ref=3`. The proxy throws `ref` away. To preserve byte-for-byte the current parsing for `owner`+`module`, add a third `else if num == 3` arm that captures the ref. The `moduleRef` struct (commits_helpers.go:11-14) gains a `ref string` field.

### 2. The provider short-circuit (bitbucket/getrepo.go:18-20, github/getrepo.go:21-23)

```go
if commit != "" && commit != "main" {
    meta.Commit = commit
}
```

The `commit != "main"` carve-out is the load-bearing bug. The intent was "main branch → HEAD", but the carve-out accidentally also excludes every other ref. A correct check is `isSHA(commit)` (40 or 64 lowercase hex). If the input is not a SHA and not empty, route to a ref-resolution API call.

### 3. `commitUUID` and its inverse (commits_helpers.go:40-59)

```go
func commitUUID(gitSHA string) (string, error) {
    if len(gitSHA) != 40 && len(gitSHA) != 64 { ... }
    sha, _ := hex.DecodeString(gitSHA)
    var result [16]byte
    copy(result[0:6], sha[0:6])
    result[6] = 0x40                       // version-4 nibble (overwritten, drop)
    result[7] = sha[6]                     // 7th input byte
    result[8] = 0x80                       // variant (overwritten, drop)
    copy(result[9:16], sha[7:14])          // bytes 7..13 of input
    return hex.EncodeToString(result[:]), nil
}
```

The inverse is straightforward: decode the 32-char UUID as 16 bytes, drop bytes 6 and 8 (they are version/variant), concatenate the remaining 14 bytes as the SHA prefix. The inverse function is lossy by design (review.md "14-of-20-byte collision surface" note at the end). The function does NOT need to know the input length — the function only emits the first 28 hex chars of the SHA.

```go
func commitUUIDInverse(uuid string) (string, error) {
    if len(uuid) != 32 { return "", errors.New("commitUUIDInverse: input is not 32 lowercase hex characters") }
    u, _ := hex.DecodeString(uuid)
    var sha [20]byte
    copy(sha[0:6], u[0:6])
    sha[6] = u[7]
    copy(sha[7:14], u[9:16])
    return hex.EncodeToString(sha[:14]), nil   // 28 chars, the recoverable SHA prefix
}
```

### 4. The prewarm logic (commits.go:961-1000, 919-959, 1101-1103, 919-959)

Three pieces:

- `prewarmHeads` ([commits.go:968-999](internal/connect/commits.go#L968-L999)) — iterates all configured sources at startup, calls `s.GetMeta(ctx, "")` (HEAD), and registers the result.
- `registerResolved` ([commits.go:919-959](internal/connect/commits.go#L919-L959)) — writes the prewarm result into `commitMap` and `infoCache` under both the raw SHA and the UUID.
- The launch in [api.go:111-113](internal/connect/api.go#L111-L113) — `go commitHandler.prewarmHeads()`.

Plus the related fields on `commitServiceHandler` (commits.go:52-69): `prewarmOnce`, `prewarmEnabled`, `prewarmTimeout`, the `CommitResolution` struct fields in [api.go:30-36](internal/connect/api.go#L30-L36), the `PrewarmConfig` in [config.go:96-101](cmd/easyp/internal/config/config.go#L96-L101), and the `WithDefaults` block at config.go:118-128.

The prewarm's only practical effect is to make the FIRST Download of a fresh buf client (one whose `buf.lock` pins the HEAD UUID) hit `commitMap` after a process restart. With the inverse function, the existing `probeCommitID` ([commits.go:1056-1137](internal/connect/commits.go#L1056-L1137)) can recover the same behavior on first request by deriving the SHA prefix from the UUID and probing each source with the prefix. The probe already handles the "first request after restart" case for raw-SHA requests; extending it to handle UUID-requests is a one-function-call change.

### 5. The probeCommitID bug (commits.go:1106, review.md finding #6)

```go
meta, err := s.GetMeta(pctx, sha)  // sha is a 32-char UUID here
if err != nil { ... }
if meta.Commit == "" { return }
results <- probeResult{...}        // <-- reports a hit regardless of what meta.Commit is
```

If `sha` is a 32-char UUID, the provider (with the new `isSHA(commit)` gate from task 1) returns 500 (input is neither a SHA, nor empty, nor "main"; the ref-resolution path can't resolve a UUID as a ref). The probe today calls the OLD short-circuit, which overwrites `meta.Commit` with the UUID literal, then `meta.Commit != ""` is true, and the probe claims a hit. With task 1's `isSHA` check, the provider returns an error, the probe correctly misses, and the next refactor (task 3) can route the UUID through the inverse function.

This means task 1 ALSO fixes finding #6 from review.md ("probeCommitID reports a hit while registerResolved silently no-ops") as a side effect — task 3 then reworks the probe to use the inverse for legitimate UUID inputs.

## Scope Fence

In scope:

- `parseResourceRefName` learns to read `ref` (commits_helpers.go)
- `moduleRef` struct gets a `ref` field
- `parseResourceRefs` / `parseGetGraphResourceRefs` / `parseGetGraphResourceRefsV1` plumb the ref through
- `ServeHTTP` and `ServeGraph` pass `ref.ref` (or empty for HEAD) into `GetMeta`
- Bitbucket provider: new `isSHA(commit)` gate; if not SHA, call `/commits/{commit}` to resolve to SHA
- GitHub provider: same `isSHA` gate; if not SHA, call `repos.GetCommit(ctx, owner, repo, commit, ...)` to resolve
- New `commitUUIDInverse(uuid) (string, error)` helper in commits_helpers.go
- New `TestCommitUUIDInverse` table-driven test
- Update `probeCommitID` to use the inverse when the input is a 32-char UUID
- Delete `prewarmHeads`, `registerResolved`, the related fields, the launch, the `PrewarmConfig` block
- Update `main.go` (cmd/easyp/main.go:48-65) to drop the prewarm args to `connect.NewWithConfig`
- Update `CommitResolution` struct (api.go:30-36) to remove `PrewarmEnabled` / `PrewarmTimeout`

Out of scope:

- Changing the `commitUUID` byte-table (Phase 17 already finalized it)
- Changing `commitMap` keys (still the buf-issued UUID for client-facing lookups, the raw SHA for upstream lookups)
- Adding new public RPCs
- Renaming any existing function

## Risks

- **R1:** The ref-resolution API call (Bitbucket `/commits/{ref}`) is one extra round-trip per ref-resolved request. For HEAD-only requests (the common case), the new path is a no-op: `commit == ""` still skips resolution. The cost is only paid for non-empty, non-SHA inputs.
- **R2:** The `commitUUIDInverse` is lossy: 2^112 collision space. A probe that recovers the wrong source for a UUID that collides under the first 14 bytes will register a wrong moduleRef. The `meta.Commit` SHA returned by the source MUST be validated to start with the recovered prefix before `registerResolved` is called. If the prefix doesn't match, the probe falls through to the "all-fail" branch and the request 400s.
- **R3:** Removing prewarm means a process restart is observable to the client. Specifically: a `buf build` immediately after a proxy restart will incur one extra upstream round-trip per module (the probe fan-out). The `probeSem` cap ([commits.go:75](internal/connect/commits.go#L75)) and the per-source `probeTimeout` ([commits.go:75](internal/connect/commits.go#L75)) bound the worst case at `maxConcurrentProbes × probeTimeout` per fan-out. For a 5-source deployment at 8s per source, that's 40s in the absolute worst case, but in practice the fan-out is parallel and completes in < 8s.
- **R4:** The Bitbucket provider does not currently have a `getCommit` helper. Task 1 has to add one. It should mirror the `getRepo` shape (HTTP GET, JSON decode into a struct, return a typed `content.Meta`-like value). The struct needs only one field: `id` (the resolved SHA).
- **R5:** The GitHub provider's `repos.GetCommit` returns a `*github.RepositoryCommit` whose `GetSHA()` is the resolved commit. The mock provider in tests (`internal/connect/api_test.go`) will need to be extended to simulate a ref-resolution response (returning a `*github.RepositoryCommit` with the resolved SHA).

## Success Criteria (from user)

The 3 user-supplied items are the success criteria. The 5 ROADMAP success criteria are TBD (the phase was just created); this plan will fill in:

- **SC-1 (refs respected):** `GetCommits` and `GetGraph` for a `Name` with `ref="main/v2"` returns a UUID derived from the SHA at `main/v2`, not HEAD. Verified by an integration test that registers a bitbucket mock source, sets `meta.Commit` for a ref, and asserts the returned UUID matches `commitUUID(meta.Commit)`.
- **SC-2 (related bug fixed):** The `commit != "" && commit != "main"` short-circuit in both providers is replaced with `isSHA(commit) && (len(commit) == 40 || len(commit) == 64)`. A ref input goes through the resolution path. A SHA input goes through the existing fast path. Verified by per-provider unit tests in `getrepo_test.go` (new) for the bitbucket provider, and an extension to the github mock tests.
- **SC-3 (prewarm removed):** `prewarmHeads`, `registerResolved`, the `prewarmEnabled` / `prewarmTimeout` / `prewarmOnce` fields, the goroutine launch in api.go:111-113, the `PrewarmConfig` block in config.go:96-101, the `PrewarmEnabled` / `PrewarmTimeout` fields in `CommitResolution`, and the corresponding `WithDefaults` entries are all gone. `go build ./...` is clean. A new test `TestServeDownload_AfterRestart_ProbeResolvesUUID` simulates a proxy restart (empty commitMap) and a Download with a UUID, asserts the probe-derived SHA prefix recovers the right module.
- **SC-4 (inverse helper):** `commitUUIDInverse(uuid)` exists in `commits_helpers.go`, returns the 28-char SHA prefix, errors on non-32-char or non-hex input. `TestCommitUUIDInverse` covers 4 cases (round-trip with a known 40-char SHA, all-zero, all-ones, error on 31-char and 33-char inputs).
- **SC-5 (no regression):** All existing tests pass, including the SHA-256 path from Phase 17, the probe path from Phase 16, and the buf v1.69.0 UUID format from Phase 16.

## Recommended Plan

One plan, three tasks, two waves:

- **Wave 1:** Tasks 1 + 2 (independent — ref support and inverse helper)
- **Wave 2:** Task 3 (prewarm removal — uses inverse from task 2)

Each task is a separate `<task>` block in the single plan. The plan is committed in one go; the executor can run tasks in the wave order.
