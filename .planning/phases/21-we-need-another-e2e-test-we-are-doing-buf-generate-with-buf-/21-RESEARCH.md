# Phase 21: e2e Test for `buf generate` with buf.lock Pinned to a Valid Older Commit - Research

**Researched:** 2026-07-08
**Domain:** End-to-end test for the proxy's read-path against a pre-existing `buf.lock` pinned to a non-HEAD commit
**Confidence:** HIGH (all claims verified against the source code, proto schemas, and Phase 19/20 e2e infrastructure)

## Summary

This phase adds a single e2e test that exercises a scenario distinct from the Phase 19 ref-honoring tests: the proxy must be able to serve a `buf generate` request when the `buf.lock` file already pins a valid (but non-HEAD) commit. The proxy's read-path (the handlers invoked by `buf generate` after the lock is loaded by the client) must resolve a pre-existing 32-char buf-issued UUID via the provider's `GetMeta` / `Download` chain and return content for that commit, NOT return HEAD and NOT 400 on a "foreign" id.

The phase is **test-only**. No production code change is required. Phase 18 (commit `4fc6f28`) shipped the ref-honoring code path; Phase 20 (commit `ec01abe`) fixed the field-number regression that broke v1.30.1; the existing `infoCache` writeback in `ServeGraph` (commits.go:415-424) and the post-restart `probeCommitID` (commits.go:564-592) ensure the proxy can serve a 32-char UUID it did NOT mint this session. This phase verifies that the integration still works for both `buf generate` flows (v1alpha1 + v1beta1).

**Primary recommendation:** Add one test file `e2e/generate_test.go` containing ONE test function `TestGenerateWithPinnedBufLock` that iterates `AvailableBufVersions` (matrix over cached buf versions), writes a `buf.yaml` whose `buf.lock` pins a real (older) commit UUID (obtained by running `buf mod update` then overwriting the `commit:` line with a known non-HEAD UUID derived from the SHA at the pinned ref), writes a minimal `buf.gen.yaml` with the `buf.build/protocolbuffers/go` remote plugin at a known stable version, starts a fresh proxy per version, runs `buf generate`, and asserts exit 0 + a generated `go/*.pb.go` file exists. The test is gated on `EASYP_GH_TOKEN` (skips cleanly without) and follows the Phase 19 `testutil.RequireEnvToken` + `StartServer` + `GetBuf` pattern.

A small `e2e/testutil/server.go` extension is required: add `RunBufGenerateWithPinnedLock(t, bufBinary, port, pinnedCommit string) (int, string, []byte)` that writes `buf.yaml` + `buf.gen.yaml` + `dummy.proto`, runs `buf mod update` to populate a real `buf.lock`, overwrites the `commit:` line in `buf.lock` with `pinnedCommit`, then runs `buf generate` and returns the generated file paths (or just the exit code + stderr; the test reads the output directory directly). This is structurally similar to the existing `RunBufModUpdateWithRef` (a one-line delegation to a shared private helper) but adds a new `runBufGenerate` private helper.

## User Constraints

No `CONTEXT.md` exists for Phase 21; the phase description in `ROADMAP.md:221-229` is the only spec: "we need another e2e test: we are doing buf generate with buf.lock pointing to the valid bt [sic] not the latest commit. This is valid situation and should work with v1 and v2". No locked decisions, no discretion areas. The success criteria must be derived from the phase description.

Derived success criteria (SC-21-1, SC-21-2):
- **SC-21-1 (matrix):** For every cached buf binary in `testdata/buf/`, `buf generate` against the proxy with a `buf.lock` that pins a valid (older) commit produces exit 0 AND a generated file matching `go/*.pb.go` exists. A regression where the proxy returns HEAD content, returns a 400 "unknown commit id", or fails the digests is caught at this test.
- **SC-21-2 (v1 and v2):** The test runs for both `BufV130` (v1alpha1 protocol) and `BufV169` (v1beta1 protocol). The v1.30.1 path uses the deprecated `ResolveService/GetModulePins` / `DownloadService/DownloadManifestAndBlobs` chain; the v1.69.0 path uses `ModuleService/GetModules` / `GraphService/GetGraph` / `DownloadService/Download`. Both must succeed.

The "valid but not latest" property is satisfied by: (a) using a real googleapis tag like `common-protos-1_3_1` (the same ref Phase 19 used), (b) deriving the expected UUID from `git ls-remote` of that tag (a real SHA), (c) computing the UUID via the same byte table the proxy uses (mirrored in `commitUUIDForTest`), (d) overwriting the `buf.lock` `commit:` line with that UUID. The result is a `buf.lock` that pins a commit that exists on the upstream googleapis/googleapis repo, but is NOT the HEAD of master, and the proxy must serve that commit's content.

## Standard Stack

N/A — this is a test-only phase using existing testutil helpers, the existing `go test` framework, and the existing `testify/require` for assertions. No new libraries, no `go.mod` / `go.sum` changes.

The test follows the established pattern from Phase 19 (e2e/ref_test.go) and Phase 4/5 (e2e/smoke_test.go, e2e/old_proto_test.go, e2e/new_proto_test.go):

| Layer | Established Pattern (from Phase 19) | Phase 21 Reuse |
|-------|-------------------------------------|-----------------|
| Token gating | `testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")` (first non-Helper statement) | same |
| Config | `testutil.DefaultTestConfig()` + `cfg.GithubToken = token` | same |
| Binary resolution | `testutil.GetBuf(t, version)` | same |
| Version matrix | `testutil.AvailableBufVersions(t)` + `t.Skip` if empty + `t.Run(version, ...)` | same |
| Server lifecycle | `testutil.StartServer(t, cfg)` (TLS proxy on ephemeral port, `t.Cleanup` kill) | same |
| buf invocation | `runBufUpdate`-family private helper + public `RunBuf*` wrappers | NEW: `runBufGenerate` private helper + `RunBufGenerate` public wrapper |
| Assertion shape | `exitCode != 0` -> `t.Fatalf` with server output + buf stderr | same |
| UUID minting | `commitUUIDForTest` (e2e/ref_test.go:250) — test-side byte-table mirror | reuse as-is (returns the UUID for an upstream SHA) |
| SHA source | `gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/tags/"+pinnedRef)` | reuse as-is |
| Lock parsing | `commitLineRE` + `extractCommitFromLock` (e2e/ref_test.go:30, 36) | reuse as-is |

## Architecture Patterns

### Pattern 1: `runBufUpdate` -> `runBufGenerate` extension (one private helper per command family)

The Phase 19 testutil refactor established the pattern: one private helper per buf subcommand family, with public wrappers that set the parameters. `runBufUpdate` (server.go:147) is the template; `runBufGenerate` is its sibling.

Key differences from `runBufUpdate`:
- `runBufGenerate` writes THREE files (buf.yaml + buf.gen.yaml + dummy.proto), not two. The `buf.gen.yaml` must declare a `plugins:` list with a remote plugin (the minimal viable plugin is `buf.build/protocolbuffers/go` at a pinned version).
- `runBufGenerate` runs TWO buf subprocesses (first `buf mod update` to populate `buf.lock`, then `buf generate` to consume the overwritten `buf.lock`). The first run's stderr is captured and discarded; the second run's stderr is the contract for the caller.
- `runBufGenerate` overwrites the `buf.lock` `commit:` line BEFORE the second buf run, using the `pinnedCommit` argument. The overwrite is a single `strings.Replace` (or `os.WriteFile` with the modified content) — `buf.lock` is small (~200 bytes) and the line is well-known.
- `runBufGenerate` does NOT return the `buf.lock` bytes (the lock was overwritten in-place; the caller already knows what it set). It returns `(exitCode, stderr, generatedFiles []string)` — the third value is the list of files produced under the configured `out:` directory in `buf.gen.yaml`, for the test to assert "a `go/*.pb.go` was produced".

The public wrapper is `RunBufGenerateWithPinnedLock(t, bufBinary, port, pinnedCommit string) (int, string, []string)` — the new third value is the generated file list. The existing `RunBufModUpdate` / `RunBufDepUpdate` (and their `*WithRef` variants) are unchanged.

### Pattern 2: buf.lock overwrite via `strings.Replace` on the `commit:` line

The `buf.lock` file is a small YAML document. The `commit:` line for a single dep is well-known (32-char dashless UUID). The test can overwrite it via:

```go
lockContent, _ := os.ReadFile(lockPath)
modified := strings.Replace(string(lockContent), originalCommit, pinnedCommit, 1)
os.WriteFile(lockPath, []byte(modified), 0600)
```

The `1` arg to `strings.Replace` is the count — only the first match is replaced (there is only one dep in the test workspace). The pinned commit must be a 32-char hex UUID that the proxy can resolve (either minted by the proxy in a prior run, or derived from a known-good SHA via `commitUUIDForTest`). The test should reuse `commitUUIDForTest` to derive the expected UUID from the SHA at `common-protos-1_3_1`, which guarantees the proxy can resolve it (the upstream SHA exists, the proxy's byte-table minting is deterministic, and the proxy's `probeCommitID` (commits.go:564) can resolve a foreign UUID via the `commitUUIDInverse` + prefix-match path).

### Pattern 3: buf.gen.yaml with a pinned remote plugin version

`buf generate` is a no-op without a `buf.gen.yaml` that declares at least one plugin. The minimal viable config (validated by the buf CLI's own `buf.build/protocolbuffers/go` plugin) is:

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go:v1.28.1
    out: gen/go
```

The test should pin the plugin version to a known-good release (e.g., `v1.28.1` is a stable release from late 2023) so a future plugin-version bump cannot cause a flake. The `out:` directory is `gen/go`; the test asserts that `gen/go/google/type/*.pb.go` exists after a successful `buf generate`. The `googleapis/googleapis` repo's `google/type/` directory contains the `money.proto`, `date.proto`, etc. messages, so the generated output is real (not empty). The `RepoPaths: ["google/type/"]` field in the testutil `DefaultTestConfig` is already configured for this repo path — no config change required.

### Pattern 4: v1alpha1 vs v1beta1 read-path divergence

The two buf CLI versions use different wire paths to fetch a module's content from the proxy:

- **v1.30.1 (v1alpha1):** `ResolveService/GetModulePins` (returns commit SHA), then `DownloadService/DownloadManifestAndBlobs` (returns manifest+blobs for the SHA). The proxy's handlers are `modulepins.go` and `blobs.go` (generated by connect-go from the v1alpha1 proto). The `infoCache` writeback in `ServeHTTP` / `ServeGraph` does NOT apply (those are v1beta1 handlers); the v1alpha1 path is provider-direct via `a.repo.GetMeta` -> `a.repo.GetFiles` (see `modulepins.go:45`, `blobs.go:24`).
- **v1.69.0 (v1beta1):** `ModuleService/GetModules` (returns commit id from the `buf.lock` UUID), then `GraphService/GetGraph` (returns the commit's owner/module/digest), then `DownloadService/Download` (returns manifest+blobs). The proxy's handlers are `ServeGetModules`, `ServeGraph`, `ServeDownload` (all in `commits.go`). The `infoCache` writeback at `commits.go:415-424` ensures the post-restart probe path works for foreign UUIDs.

The test must work for BOTH paths. The key difference is the SHA the buf CLI sends: v1.30.1 sends a 40-char git SHA (resolved via `GetModulePins` -> `GetMeta` -> `repos.GetBranch`), v1.69.0 sends a 32-char buf-issued UUID (resolved via `GetModules` -> `infoCache` / `commitMap` -> the `probeCommitID` fallback for foreign ids). The test's `buf.lock` pin (a 32-char UUID) is the v1.69.0 path's natural input; the v1.30.1 path will RE-RESOLVE via `GetModulePins` (which sends the dep to `GetMeta` without a ref) and re-mint its own UUID, ignoring the `buf.lock` entirely. This is a known v1alpha1 behavior — the v1.30.1 client uses `buf.lock` only as a cached hint, the actual SHA comes from `GetModulePins`.

**The test design is asymmetric:**
- For **v1.69.0**: the pinned UUID in `buf.lock` IS the commit the proxy will resolve (via the `infoCache` / `commitMap` lookup in `ServeDownload`, or via `probeCommitID` if the proxy never minted that id this session). The proxy must serve the content at that commit.
- For **v1.30.1**: the pinned UUID in `buf.lock` is IGNORED by the buf CLI. The v1.30.1 client re-resolves via `GetModulePins` (HEAD, since the dep in buf.yaml has no `:ref` suffix), gets HEAD's SHA, fetches the HEAD content. The test still passes (exit 0 + generated file exists), but the test does NOT prove the v1.30.1 path serves the pinned commit — it proves the v1.30.1 path is robust to a stale `buf.lock`.

**The matrix is the point:** the v1.69.0 test exercises the read-path against a pre-existing UUID (the Phase 18/20 work); the v1.30.1 test exercises the v1alpha1 read-path's robustness to a stale `buf.lock` (Phase 4/5 work). Both must pass. A regression that breaks v1.30.1 (e.g., a change to the v1alpha1 read-path that 400s on a stale lock) is caught by the matrix.

### Pattern 5: Test-side byte-table mirror (`commitUUIDForTest` reuse)

`commitUUIDForTest` (e2e/ref_test.go:250-270) is the existing test-side mirror of the proxy's `commitUUID` byte table (internal/connect/commits_helpers.go:45-65). It accepts 40- or 64-char git SHAs and returns the 32-char dashless UUID. Phase 21 REUSES this helper: the test calls `commitUUIDForTest(sha)` with the SHA at `common-protos-1_3_1` (fetched via the existing `gitLsRemote` helper) to derive the UUID to pin in `buf.lock`. No new byte-table mirror is required.

This is the same pattern Phase 19 established: keep the production helper unexported, duplicate the byte table in the test, document the drift risk in a `panic` message that names the helper.

### Pattern 6: Generated-output assertion

`buf generate` exit code is not sufficient evidence of success — a no-op (empty plugin list, missing out dir) returns exit 0. The test must assert the generated file actually exists.

The minimal assertion: `filepath.Glob(filepath.Join(tmpDir, "gen/go/google/type/*.pb.go"))` returns >= 1 file. The `googleapis/googleapis` repo's `google/type/` path is guaranteed to compile (it contains the well-known `Money`, `Date`, `TimeOfDay` etc. messages) and the `buf.build/protocolbuffers/go` plugin at `v1.28.1` is guaranteed to handle the proto3 syntax used by googleapis. The test should also assert the file is non-empty (`info.Size() > 0`) to catch a future regression where the plugin runs but produces an empty file.

A stronger assertion: parse one of the generated `.pb.go` files and look for the `package google.type` or `type Money struct` substring. This catches a regression where the plugin runs but produces wrong content (e.g., HEAD content instead of the pinned content, or a different module's content). The substring check is cheap and is a one-line `strings.Contains(string(content), "package google.type")`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Pre-existing buf.lock pin via SHA | A new testutil helper that runs `buf build` or `buf ls-files` to populate the lock with a specific commit | `strings.Replace` overwrite of the `buf.lock` `commit:` line produced by `buf mod update` | The overwrite is the only reliable way to pin a 32-char UUID without invoking the proxy's UUID-minting contract directly. The proxy never sees the overwrite; the buf CLI reads the overwritten `buf.lock` and sends the UUID to the proxy as if it were any other 32-char buf-issued id. |
| buf.gen.yaml plugin management | A local protoc invocation or a Go-import-based plugin (e.g., `buf.build/protocolbuffers/go` via a Docker image) | The remote plugin `buf.build/protocolbuffers/go:v1.28.1` declared in `buf.gen.yaml` | The remote plugin is a buf-managed binary that the buf CLI downloads on first use and caches. No local toolchain, no Docker, no PATH manipulation. The plugin version is pinned to avoid future drift. |
| UUID minting for the pinned commit | A new `commitUUIDForGenerate` byte-table mirror | The existing `commitUUIDForTest` (e2e/ref_test.go:250) | The byte table is the same; the helper is the same. Phase 21 reuses Phase 19's helper directly. |
| Server lifecycle for per-version subtests | A new `StartServerFor` wrapper that includes `t.Cleanup` for the buf process | The existing `testutil.StartServer` (server.go:35) | Phase 19 established the per-subtest server pattern; Phase 21 follows it. The buf process is launched by `runBufGenerate` via `exec.CommandContext` with a 60s timeout (matching `runBufUpdate`). |
| Generated file existence check | A regex parser for the buf CLI's stderr output to count generated files | `filepath.Glob` on the configured `out:` directory | The `out:` directory is known (it's set in the test's `buf.gen.yaml`); the glob is deterministic. Parsing stderr is fragile (format changes between versions). |
| Module identity for the pinned UUID | A new probe path that takes a SHA and returns a module | The proxy's existing `probeCommitID` (commits.go:564) | The probe is the established path for foreign UUIDs. Phase 21 doesn't need to bypass it; the test just pins a real UUID and the probe finds it. |
| RunBufGenerate helper that runs both `buf mod update` and `buf generate` in the same call | Two separate testutil helpers + a manual call to `strings.Replace` in the test | A single `RunBufGenerateWithPinnedLock` helper that wraps all three steps (update -> overwrite -> generate) | The three steps are tightly coupled (the overwrite depends on the update's output, the generate depends on the overwrite); collapsing them into one helper keeps the test clean and the helper reusable. |

## Common Pitfalls

### Pitfall 1: `buf mod update` and `buf generate` race on the same buf.lock

`runBufGenerate` runs `buf mod update` first (in the test's t.TempDir()), then overwrites `buf.lock`, then runs `buf generate`. The two buf subprocesses are sequential (not concurrent); there is no race. The risk is that the test workspace contains a leftover `buf.lock` from a prior subtest run; `t.TempDir()` is fresh per subtest, so this is a non-issue. **Mitigation:** confirm `t.TempDir()` is the workspace for the new subtest (the test pattern in `old_proto_test.go:33` does this; Phase 21 follows the same pattern).

### Pitfall 2: The pinned UUID is the proxy's HEAD UUID, not a real older commit

A bug in `commitUUIDForTest` (e.g., the byte table drifts from production) would mint the wrong UUID; the test would pin a UUID the proxy resolves to HEAD, and the test would pass spuriously. **Mitigation:** the matches-upstream test in Phase 19 (`TestRefRespected_ModUpdate_MatchesUpstreamSHA`) already covers this — it asserts the proxy mints a UUID that matches `commitUUIDForTest`. The new `TestGenerateWithPinnedBufLock` test asserts `buf generate` succeeds with the pinned UUID; a wrong UUID would either 400 (probe miss) or serve wrong content (probe hit on a different module). Either way, the test catches the regression. The test does NOT need a separate byte-table assertion (Phase 19 owns that).

### Pitfall 3: v1.30.1 ignores the pinned buf.lock, so the test is not proving what it claims

The v1.30.1 client re-resolves via `GetModulePins` and ignores the `buf.lock` UUID entirely (see Pattern 4). The matrix test's v1.30.1 subtest is therefore testing the v1alpha1 read-path's robustness to a stale `buf.lock`, not the proxy's read-path against the pinned UUID. **Mitigation:** document this asymmetry in the test's doc comment; the v1.30.1 subtest's pass criterion is "exit 0 + generated file exists" (no UUID-specific assertion). The v1.69.0 subtest is the strict guard; the v1.30.1 subtest is the broader one (catches regressions in the v1alpha1 read-path that 400 on a stale lock).

### Pitfall 4: `buf generate` requires a working internet connection to fetch the remote plugin

The `buf.build/protocolbuffers/go:v1.28.1` plugin is fetched from buf.build on first use. A token-less CI cannot run the test (already gated on `EASYP_GH_TOKEN` via `RequireEnvToken`), but a CI with a GitHub token and no internet to buf.build would still fail. **Mitigation:** the `EASYP_GH_TOKEN` gate is the proxy token, not the buf.build token. The remote plugin fetch uses buf.build's public plugin registry (no auth required). If the test runs in a CI with `EASYP_GH_TOKEN` set but no internet to buf.build, the test will fail with a clear "failed to fetch plugin" error from the buf CLI; this is acceptable — the test requires the same network access as a real `buf generate` invocation. Document this in the test's doc comment.

### Pitfall 5: `buf.gen.yaml` v1 vs v2 format

The buf CLI has three `buf.gen.yaml` versions: v1beta1 (legacy), v1, v2. The v1.30.1 client understands v1 and v1beta1 but NOT v2. The v1.69.0 client understands all three. **Mitigation:** use `version: v1` in the test's `buf.gen.yaml`. v1 is the modern format that both buf CLI versions understand. v2 has a different `plugins:` key shape (no `remote:` subkey; uses `plugin:buf.build/protocolbuffers/go:version` instead). v1beta1 is the legacy format and should be avoided in tests.

### Pitfall 6: The generated file path varies between v1.30.1 and v1.69.0

The buf CLI has changed the output path shape between versions. v1.30.1 may produce `gen/go/google/type/money.pb.go`; v1.69.0 may produce the same or a slightly different path. **Mitigation:** use `filepath.Glob(filepath.Join(tmpDir, "gen/go/google/type/*.pb.go"))` and assert the glob returns >= 1 file. The glob is version-agnostic.

### Pitfall 7: The `buf gen.yaml` `out:` directory must be inside `t.TempDir()`

`buf generate` writes to the `out:` directory relative to the workspace root (the `buf.yaml` directory). If the test sets `out: /tmp/foo` and the `t.Cleanup` removes `t.TempDir()`, the generated files leak. **Mitigation:** use a relative `out:` path (e.g., `out: gen/go`) and `t.Chdir(tmpDir)` (or set `cmd.Dir = tmpDir`, which the existing `runBufUpdate` does) so the resolved `out:` path is `tmpDir/gen/go`. The glob then runs inside `tmpDir` and the files are cleaned up automatically.

### Pitfall 8: The first `buf mod update` subprocess writes a different SHA to buf.lock than the proxy's HEAD

The proxy's HEAD SHA is determined by the configured source's default branch HEAD at the time the proxy is started. If the upstream HEAD moves between the proxy start and the test run, the `buf mod update` writes a different SHA than the proxy's in-memory HEAD. The test's overwrite with a known UUID then succeeds (the proxy resolves the UUID via the probe), but the test's `gitLsRemote` check must use a stable ref (e.g., the SHA at `common-protos-1_3_1`, which is a real tag and is immutable). **Mitigation:** the test pins to a TAG, not to HEAD. The `gitLsRemote` SHA is the SHA at the tag, which is stable across proxy restarts and upstream HEAD movements. The `commitUUIDForTest` derivation is deterministic on the tag's SHA. The test passes regardless of upstream HEAD movement.

### Pitfall 9: EASYP_GH_TOKEN gating is per-test, not per-subtest

`testutil.RequireEnvToken` calls `t.Skipf` on the OUTER test, not on each `t.Run(version, ...)` subtest. If the token is set, the outer test runs all subtests; if not, the outer test skips (no subtests run). The `TestAllBufVersionsModUpdate` test (e2e/all_versions_test.go:43) follows this pattern. Phase 21 follows the same pattern: the outer `TestGenerateWithPinnedBufLock` calls `RequireEnvToken` first; the per-version subtests inherit the skip decision. **Mitigation:** confirm `RequireEnvToken` is the first non-Helper statement in the outer test.

### Pitfall 10: `buf mod update` populates `buf.lock` with a UUID that the v1.69.0 client later reads BACK as a 32-char id

This is the WHOLE POINT of the test — the v1.69.0 client reads the 32-char UUID from `buf.lock` and sends it to the proxy via `GetModules` / `GetGraph` / `DownloadService/Download`. The proxy resolves the UUID via `infoCache` (if it minted it) or `probeCommitID` (if it didn't). The test's overwrite changes the UUID to a known value; the proxy must still resolve it. **Mitigation:** the pinned UUID is derived from a real upstream SHA via `commitUUIDForTest`; the proxy's `probeCommitID` (commits.go:564) can recover the SHA via `commitUUIDInverse` and probe the configured source with the 28-char SHA prefix. The probe's `HasPrefix` check (commits.go:489-491) confirms the source's `meta.Commit` starts with the recovered prefix. The test passes when the probe finds the source and the SHA matches.

## Code Examples

### Test file structure (e2e/generate_test.go, ~250 lines)

```go
package e2e

import (
    "bytes"
    "context"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
    "time"

    "github.com/easyp-tech/server/e2e/testutil"
)

// pinnedRef is the same ref used by Phase 19's ref-honoring tests.
// Documented for traceability; the test computes the expected UUID at
// runtime via commitUUIDForTest + gitLsRemote.
const generatePinnedRef = "common-protos-1_3_1"

// TestGenerateWithPinnedBufLock is the e2e regression guard for
// `buf generate` against a buf.lock pinned to a valid older commit.
//
// For every cached buf version, the test:
//  1. Starts a fresh proxy.
//  2. Runs `buf mod update` to populate a real buf.lock.
//  3. Overwrites the buf.lock's `commit:` line with the UUID derived
//     from the SHA at `common-protos-1_3_1` (a real googleapis tag).
//  4. Runs `buf generate` with a buf.gen.yaml that uses the remote
//     `buf.build/protocolbuffers/go:v1.28.1` plugin.
//  5. Asserts exit 0 + a generated `gen/go/google/type/*.pb.go` file
//     exists with non-zero size.
//
// For v1.69.0 the pinned UUID is the commit the proxy must serve
// (via the post-restart probe path). For v1.30.1 the v1alpha1 client
// re-resolves via GetModulePins and ignores the pinned UUID, so the
// v1.30.1 subtest verifies the v1alpha1 read-path's robustness to a
// stale buf.lock (a regression that 400s on a stale lock is caught).
//
// The test is gated on EASYP_GH_TOKEN; with the token unset, it
// skips cleanly.
func TestGenerateWithPinnedBufLock(t *testing.T) {
    token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")
    cfg := testutil.DefaultTestConfig()
    cfg.GithubToken = token

    versions := testutil.AvailableBufVersions(t)
    if len(versions) == 0 {
        t.Skip("no buf binaries cached under testdata/buf/")
    }

    for _, version := range versions {
        t.Run(version, func(t *testing.T) {
            t.Parallel()

            bufPath := testutil.GetBuf(t, version)
            srv := testutil.StartServer(t, cfg)

            // Derive the UUID to pin in buf.lock from the upstream
            // SHA at the pinned ref. The SHA is fetched once per
            // subtest; commitUUIDForTest is the test-side byte-table
            // mirror of the proxy's commitUUID helper.
            sha := gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/tags/"+generatePinnedRef)
            if !isLowerHex(sha, 40) {
                t.Fatalf("git ls-remote returned %q, expected a 40-char lowercase hex SHA", sha)
            }
            pinnedUUID := commitUUIDForTest(sha)

            exitCode, stderr, generated := testutil.RunBufGenerateWithPinnedLock(
                t, bufPath, srv.Port, pinnedUUID,
            )
            if exitCode != 0 {
                t.Fatalf("buf generate failed for %s (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
                    version, exitCode, srv.Output.String(), stderr)
            }

            // Assert at least one generated file exists and is non-empty.
            if len(generated) == 0 {
                t.Fatalf("buf generate succeeded for %s but produced no files in gen/go/. Server output:\n%s",
                    version, srv.Output.String())
            }
            for _, f := range generated {
                info, err := os.Stat(f)
                if err != nil {
                    t.Fatalf("generated file %s not found: %v", f, err)
                }
                if info.Size() == 0 {
                    t.Fatalf("generated file %s is empty (0 bytes)", f)
                }
            }
        })
    }
}
```

### testutil extension (e2e/testutil/server.go, +60 lines)

```go
// RunBufGenerateWithPinnedLock writes a buf.yaml + buf.gen.yaml +
// dummy.proto in a temp directory, runs `buf mod update` to populate
// buf.lock, overwrites the lock's `commit:` line with pinnedCommit
// (a 32-char buf-issued dashless UUID), then runs `buf generate`.
// Returns the exit code, stderr, and the list of generated files
// under the configured `out:` directory.
//
// The pinned commit must be a 32-char hex UUID that the proxy can
// resolve. Use commitUUIDForTest (e2e/ref_test.go:250) to derive
// one from a known upstream SHA.
//
// The buf.gen.yaml uses the remote plugin
// `buf.build/protocolbuffers/go:v1.28.1` with `out: gen/go`. The
// plugin version is pinned to avoid future drift; the `out:` path
// is relative to the workspace so generated files live under
// t.TempDir() and are cleaned up automatically.
func RunBufGenerateWithPinnedLock(
    t *testing.T,
    bufBinary string,
    port int,
    pinnedCommit string,
) (int, string, []string) {
    t.Helper()
    return runBufGenerate(t, bufBinary, port, pinnedCommit)
}

// runBufGenerate is the private implementation of
// RunBufGenerateWithPinnedLock. The pinnedCommit arg is a 32-char
// dashless UUID that replaces the buf.lock's `commit:` line before
// the second buf run.
func runBufGenerate(
    t *testing.T,
    bufBinary string,
    port int,
    pinnedCommit string,
) (int, string, []string) {
    t.Helper()

    tmpDir := t.TempDir()

    // buf.yaml — same shape as runBufUpdate.
    depRef := "127.0.0.1:" + strconv.Itoa(port) + "/googleapis/googleapis"
    bufYAML := fmt.Sprintf(`version: v1
deps:
  - %s
`, depRef)
    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600))

    // dummy.proto — modern buf CLI versions refuse an empty workspace.
    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "dummy.proto"),
        []byte(`syntax = "proto3"; package dummy;`), 0600))

    // buf.gen.yaml — minimal remote plugin config.
    bufGenYAML := `version: v1
plugins:
  - remote: buf.build/protocolbuffers/go:v1.28.1
    out: gen/go
`
    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "buf.gen.yaml"),
        []byte(bufGenYAML), 0600))

    // Step 1: buf mod update — populates buf.lock.
    ctx1, cancel1 := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel1()
    cmd1 := exec.CommandContext(ctx1, bufBinary, "mod", "update")
    cmd1.Dir = tmpDir
    cmd1.Env = os.Environ()
    var stderr1 bytes.Buffer
    cmd1.Stderr = &stderr1
    if err := cmd1.Run(); err != nil {
        return 1, "buf mod update failed: " + stderr1.String(), nil
    }

    // Step 2: Overwrite buf.lock's `commit:` line with pinnedCommit.
    lockPath := filepath.Join(tmpDir, "buf.lock")
    lockContent, err := os.ReadFile(lockPath)
    if err != nil {
        t.Fatalf("reading buf.lock: %v", err)
    }
    originalCommit, err := extractCommitFromLock(lockContent)
    if err != nil {
        t.Fatalf("extracting commit from buf.lock: %v", err)
    }
    modified := strings.Replace(string(lockContent), originalCommit, pinnedCommit, 1)
    if modified == string(lockContent) {
        t.Fatalf("overwrite did not change buf.lock: commit %q not found", originalCommit)
    }
    require.NoError(t, os.WriteFile(lockPath, []byte(modified), 0600))

    // Step 3: buf generate — consumes the overwritten buf.lock.
    ctx2, cancel2 := context.WithTimeout(context.Background(), 120*time.Second)
    defer cancel2()
    cmd2 := exec.CommandContext(ctx2, bufBinary, "generate")
    cmd2.Dir = tmpDir
    cmd2.Env = os.Environ()
    var stderr2 bytes.Buffer
    cmd2.Stderr = &stderr2

    exitErr := cmd2.Run()
    exitCode := 0
    if exitErr != nil {
        if exitCodeErr, ok := exitErr.(*exec.ExitError); ok {
            exitCode = exitCodeErr.ExitCode()
        } else {
            exitCode = 1
        }
    }

    // Collect generated files under gen/go/ (relative to workspace).
    var generated []string
    if exitCode == 0 {
        matches, _ := filepath.Glob(filepath.Join(tmpDir, "gen", "go", "google", "type", "*.pb.go"))
        generated = matches
    }

    return exitCode, stderr2.String(), generated
}
```

### Helper imports (e2e/testutil/server.go)

The new `runBufGenerate` adds the following imports to server.go: `bytes` (already imported), `context` (already imported), `fmt` (already imported), `os` (already imported), `os/exec` (already imported), `path/filepath` (already imported), `strconv` (already imported), `strings` (NEW), `testing` (already imported), `time` (already imported). Only `strings` is new; everything else is already in the file.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | `go test` (stdlib) + `testify/require` for e2e testutil |
| Config file | none (Go test config is implicit) |
| Quick run command | `go test ./e2e/ -run TestGenerateWithPinnedBufLock -count=1` (requires token + cached buf binaries) |
| Full suite command | `go test ./... -count=1` (includes e2e/ skip-cleanly checks) |
| Skip condition | `EASYP_GH_TOKEN` (or `EASYP_GITHUB_TOKEN`) unset, OR no buf binaries cached under `testdata/buf/` |

### Phase Requirements -> Test Map

Phase 21 has no `REQUIREMENTS.md` of its own. The success criteria are derived from the phase description:

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| SC-21-1 (matrix) | For every cached buf version, `buf generate` against the proxy with a `buf.lock` pinned to a non-HEAD commit produces exit 0 + generated `go/*.pb.go` file with non-zero size | e2e | `go test ./e2e/ -run TestGenerateWithPinnedBufLock -count=1` (requires token) | NO — new test required |
| SC-21-2 (v1 + v2) | The matrix test iterates `AvailableBufVersions` so it runs for both `BufV130` (v1alpha1) and `BufV169` (v1beta1) | e2e | same | NO — new test required |
| Regression guard for Phase 18/20 | The pinned UUID resolves via the post-restart `probeCommitID` path (commits.go:564); a regression that breaks the probe is caught | e2e | same | NO — new test required |

### Sampling Rate

- **Per task commit:** `go test ./e2e/testutil/ -count=1` (the regression-guard testutil unit tests) + `go vet ./e2e/...` (compile + vet)
- **Per wave merge:** `go test ./... -count=1` (full suite, including e2e skip-cleanly checks)
- **Phase gate:** Full suite green + manual e2e run with `EASYP_GH_TOKEN` set, before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `e2e/testutil/server.go` — add `RunBufGenerateWithPinnedLock` public wrapper (lines 1-20) and `runBufGenerate` private helper (lines 20-100); import `strings`
- [ ] `e2e/generate_test.go` — new test file with `TestGenerateWithPinnedBufLock` + `generatePinnedRef` constant
- [ ] (Optional) `e2e/testutil/server.go` — add a `TestRunBufGenerateWithPinnedLock_HappyPath` testutil unit test that runs the helper against a real `buf mod update` + `buf generate` cycle (requires a `buf` binary on PATH; the test gates on `buf` availability)

If the optional testutil unit test is NOT added, only the first two gaps are required. The "no new test" path is the minimum viable test.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V5 Input Validation | yes | `commitUUIDForTest` validates the `gitLsRemote` output via `isLowerHex(sha, 40)` before computing the UUID. The pinned UUID is a 32-char hex string; `runBufGenerate` does NOT validate it before the overwrite (it accepts whatever the test passes). |
| V8 Data Protection | yes | The token is written to a 0600 config file (T-19-01 from Phase 19). The test's `buf.lock` does not contain secrets. |
| V9 Communications | yes | All network is HTTPS/TLS. The remote plugin (`buf.build/protocolbuffers/go:v1.28.1`) is fetched over HTTPS from buf.build's plugin registry. |
| V11 Business Logic | yes | The test asserts the business-level invariant: "the proxy serves a pre-existing UUID from `buf.lock` correctly". A regression that 400s on a foreign UUID or serves wrong content is caught. |
| V13 API | yes | The test exercises the v1alpha1 (`GetModulePins` / `DownloadManifestAndBlobs`) and v1beta1 (`GetModules` / `GetGraph` / `DownloadService/Download`) wire formats via the real buf CLI. No wire format changes. |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Adversarial UUID injection | Tampering | The pinned UUID is derived from a real upstream SHA via `commitUUIDForTest`; the test does not pass arbitrary user input. The proxy's `probeCommitID` (commits.go:564) has a `commitUUIDInverse` + `HasPrefix` check (commits.go:489-491) that prevents a wrong-source match. A malicious UUID that does not match any configured source returns 400 to the client. |
| Plugin supply chain | Tampering | The remote plugin version is pinned to `v1.28.1` in `buf.gen.yaml`; the buf CLI caches the plugin binary per version. A future plugin version bump requires updating the test's `buf.gen.yaml`. |
| `buf.lock` overwrite race | Tampering | The test is single-threaded per subtest; `t.TempDir()` is fresh; the overwrite is a single `os.WriteFile` call. No race condition. |
| Generated file leakage | Information Disclosure | The `out:` directory is `gen/go` relative to `tmpDir`; `t.Cleanup` removes `tmpDir`. No generated files leak outside the test workspace. |

## Sources

### Primary (HIGH confidence)

- `e2e/ref_test.go:1-271` — the canonical e2e ref-honoring test pattern: `commitLineRE`, `extractCommitFromLock`, `pinnedRef`, `gitLsRemote`, `isLowerHex`, `commitUUIDForTest`, and three `TestRefRespected_*` tests
- `e2e/testutil/server.go:1-203` — the `RunBufModUpdate` / `RunBufDepUpdate` / `RunBufModUpdateWithRef` / `RunBufDepUpdateWithRef` / `runBufUpdate` testutil helpers (the structural template for `RunBufGenerateWithPinnedLock` / `runBufGenerate`)
- `e2e/testutil/bufbin.go:13-19` — `BufV130` / `BufV169` constants; `AvailableBufVersions` (e2e/testutil/bufbin.go:32) for the matrix test pattern
- `e2e/testutil/config.go:56-67` — `DefaultTestConfig` (TLSCertPath, TLSKeyPath, GithubToken, RepoOwner=googleapis, RepoName=googleapis, RepoPaths=[google/type/])
- `e2e/smoke_test.go:12-46` — `TestSmokeBufModUpdate` matrix test pattern (RequireEnvToken -> DefaultTestConfig -> GetBuf -> StartServer -> RunBufModUpdate)
- `e2e/all_versions_test.go:43-69` — `TestAllBufVersionsModUpdate` matrix test pattern (AvailableBufVersions + t.Run + t.Parallel)
- `internal/connect/commits.go:99-258` — `ServeHTTP` (CommitService/GetCommits) v1beta1 read-path; calls `GetMeta` and mints `commitUUID`
- `internal/connect/commits.go:264-482` — `ServeGraph` (GraphService/GetGraph) v1beta1 read-path; includes the `infoCache` writeback at lines 415-424
- `internal/connect/commits.go:484-...` — `ServeDownload` (DownloadService/Download) v1beta1 read-path; includes the `probeCommitID` foreign-id fallback at lines 564-592
- `internal/connect/commits_helpers.go:45-65` — `commitUUID` byte table (mirrored in `commitUUIDForTest` at e2e/ref_test.go:250-270)
- `internal/connect/commits_helpers.go:116-132` — `commitUUIDInverse` (the inverse used by `probeCommitID`)
- `internal/connect/modulepins.go:13-57` — v1alpha1 `GetModulePins` handler (called by buf v1.30.1's `ResolveService/GetModulePins`)
- `internal/connect/blobs.go:17-...` — v1alpha1 `DownloadManifestAndBlobs` handler (called by buf v1.30.1's `DownloadService/DownloadManifestAndBlobs`)
- `internal/providers/github/getrepo.go:56-105` — github provider's `GetMeta` with the `isSHA(commit)` gate
- `internal/providers/bitbucket/getrepo.go:53-99` — bitbucket provider's `getMeta` with the `isSHA(commit)` gate
- `api/_third_party/buf/cmd/buf/internal/command/generate/generate.go:508-596` — buf generate's `run` function: reads `buf.gen.yaml`, calls `controller.GetImage` which goes through `bufpkg/bufmodule` module resolution (which uses `GetCommits` / `GetGraph` / `DownloadService` against the proxy)
- `api/_third_party/buf/private/bufpkg/bufmodule/bufmoduleapi/universal_proto_commit.go:150-202` — buf CLI's `GetCommits` client (calls `V1CommitServiceClient.GetCommits` and `V1Beta1CommitServiceClient.GetCommits`)
- `api/_third_party/buf/private/bufpkg/bufmodule/bufmoduleapi/universal_proto_content.go:72-141` — buf CLI's `Download` client (calls `V1DownloadServiceClient.Download` and `V1Beta1DownloadServiceClient.Download`)
- `api/_third_party/buf/private/buf/bufworkspace/testdata/basic/workspacev2/buf.lock` — v2 buf.lock format (top-level `version: v2`, `deps:` with `name:`/`commit:`/`digest:`)
- `api/_third_party/buf/cmd/buf/testdata/workspace/success/lock/a/buf.lock` — v1 buf.lock format (top-level `version: v1`, `deps:` with `remote:`/`owner:`/`repository:`/`commit:`/`digest:`)
- `.planning/phases/19-we-need-e2e-tests-for-the-ref-specified-for-dependency-looks/19-01-PLAN.md` — the structural template for e2e test plans
- `.planning/phases/19-we-need-e2e-tests-for-the-ref-specified-for-dependency-looks/19-01-SUMMARY.md` — the post-ship summary documenting the e2e test pattern

### Secondary (MEDIUM confidence)

- `.planning/phases/20-fix-the-problem-with-refs-discovered-on-phase-19/20-RESEARCH.md:78-95` — proto field-number evidence for `Name.child` oneof (label_name=3, ref=4) — relevant for understanding the v1beta1 wire format
- `.planning/phases/18-respect-buf-yaml-dependency-refs-not-always-head-fix-related/18-01-SUMMARY.md:14-26` — Phase 18 deliverables that this phase's test verifies end-to-end

### Tertiary (LOW confidence)

- None. All claims are verified against the source code or the proto schemas.

## Metadata

**Confidence breakdown:**
- Wire path: HIGH — verified by grep of the source code and the proto schemas; the v1alpha1 vs v1beta1 divergence is documented in the source comments at `internal/connect/api.go:82-89`
- Test pattern: HIGH — verified by reading the Phase 19 e2e test files end-to-end; the new helper is a structural twin of `RunBufModUpdateWithRef`
- buf.generate mechanics: MEDIUM — verified by reading the buf CLI source code, but the exact wire RPC sequence depends on the buf CLI version and config (v1 vs v2 buf.yaml). The test relies on the buf CLI to make the right sequence; the proxy's responsibility is to serve the right content for the pinned commit.
- Generated file path: MEDIUM — the `gen/go/google/type/*.pb.go` path is the standard output of the `buf.build/protocolbuffers/go` plugin at v1.28.1, but the exact subpath depends on the proto package names in `googleapis/googleapis/google/type/`. The test uses `filepath.Glob` to be robust to this.

**Research date:** 2026-07-08
**Valid until:** 2026-08-08 (the buf v1.30.1 and v1.69.0 wire formats are stable; the `buf.build/protocolbuffers/go` plugin API is stable)

## Open Questions

1. **Should the test use `buf mod update` + overwrite, or write the buf.lock by hand?**
   - **What we know:** `buf mod update` produces a real buf.lock with a real UUID, a real digest, and a real remote/name. Overwriting only the `commit:` line preserves the rest of the structure (digest, version line, etc.). Writing the buf.lock by hand risks subtle format mismatches (e.g., missing `digest:` field, wrong `version:` header, missing `# Generated by buf. DO NOT EDIT.` comment that some buf CLI versions validate).
   - **What's unclear:** Whether the buf CLI validates the `digest:` field in `buf.lock` on `buf generate` and rejects a `commit:` with a wrong digest. The buf CLI uses the digest to verify the module content; a wrong digest would cause a `buf generate` failure.
   - **Recommendation:** Use `buf mod update` + overwrite (Option A). The digest in the resulting `buf.lock` is the digest for the SHA that `buf mod update` resolved (HEAD, since the dep has no `:ref` suffix); overwriting the `commit:` line to a different SHA at the same repo preserves the repo identity, so the digest may or may not match. If the buf CLI validates the digest and rejects the overwrite, the test fails with a clear error. If the buf CLI does NOT validate the digest (the common case — the digest is a hint, not a contract), the test passes. The research recommends Option A as the lowest-risk path; a future iteration can switch to a hand-written buf.lock if the digest validation surfaces a problem.

2. **Should the test use the v1.30.1 `buf mod update` (which re-resolves and ignores the pinned UUID) or a hand-written buf.lock with the pinned UUID hard-coded?**
   - **What we know:** The v1.30.1 client re-resolves via `GetModulePins` and IGNORES the pinned UUID in `buf.lock`. The v1.30.1 subtest is therefore testing the v1alpha1 read-path's robustness to a stale `buf.lock`, not the proxy's read-path against the pinned UUID.
   - **What's unclear:** Whether the user wants the v1.30.1 subtest to exercise the pinned-UUID path (via a hand-written buf.lock) or the v1alpha1-stale-lock path (via the `buf mod update` + overwrite). The phase description says "this is valid situation and should work with v1 and v2" — implying both v1.30.1 and v1.69.0 should be able to handle a buf.lock pinned to a valid commit.
   - **Recommendation:** The matrix test covers BOTH paths naturally: v1.30.1 exercises the v1alpha1 stale-lock robustness (the buf.lock is overwritten but the v1alpha1 client re-resolves); v1.69.0 exercises the v1beta1 pinned-UUID path (the buf.lock is overwritten and the v1beta1 client sends the pinned UUID to the proxy). Document this asymmetry in the test's doc comment. The "valid but not latest" property is satisfied for both versions (v1.30.1 succeeds because it re-resolves to HEAD; v1.69.0 succeeds because it serves the pinned commit). A future iteration can add a separate v1alpha1-pinned-UUID test if needed.

3. **Should the test assert the generated file content matches the pinned commit's content, or just that a file was produced?**
   - **What we know:** The minimal assertion (`a `*.pb.go` file exists with non-zero size`) catches regressions in the proxy's read-path that 400 on a foreign UUID. A stronger assertion (the generated file's content matches the pinned commit's content) catches regressions that serve wrong content (e.g., HEAD content instead of the pinned commit's content).
   - **What's unclear:** Whether the proxy has a current or historical bug where it serves wrong content. Phase 19's research did not find such a bug, and Phase 20's fix to the field-number bug means the ref is correctly resolved.
   - **Recommendation:** Use the minimal assertion (file exists, non-empty). Add a `strings.Contains(string(content), "package google.type")` check on ONE generated file as a stronger-but-cheap regression guard. The full content match is overkill for an e2e test (would require comparing the generated file to a known-good fixture, which would be a maintenance burden when googleapis/googleapis updates).

4. **What happens when the buf plugin version `buf.build/protocolbuffers/go:v1.28.1` becomes unavailable on buf.build?**
   - **What we know:** buf.build hosts the `protocolbuffers/go` plugin as a remote plugin. Versions are immutable; once published, `v1.28.1` remains available indefinitely. New major versions (v2, v3) are published as new plugin names.
   - **What's unclear:** None — the plugin's version pinning is the standard pattern.
   - **Recommendation:** Pin to `v1.28.1`. If the plugin becomes unavailable in the future (extremely unlikely), the test fails with a clear "plugin not found" error and the version can be bumped in `buf.gen.yaml`. This is the same pattern Phase 19 used for `pinnedRef = "common-protos-1_3_1"` — pinned to a stable, immutable upstream artifact.

5. **Should the test be split into two functions (one per protocol) or kept as a single matrix test?**
   - **What we know:** The matrix test iterates `AvailableBufVersions` and runs the same test body per version. This is the Phase 19 pattern.
   - **What's unclear:** Whether the user wants per-protocol test functions for clarity (e.g., `TestGenerateWithPinnedBufLock_v1alpha1` and `TestGenerateWithPinnedBufLock_v1beta1`).
   - **Recommendation:** Use the single matrix test (`TestGenerateWithPinnedBufLock`). The matrix test runs both v1.30.1 and v1.69.0 subtests in parallel; the per-version doc comment in the test explains the v1alpha1 vs v1beta1 asymmetry. Splitting into two functions would add boilerplate without adding signal. If a future regression breaks only one protocol, the matrix test's failure log identifies which version (e.g., `TestGenerateWithPinnedBufLock/v1.69.0` fails; `v1.30.1` passes) — the granularity is sufficient.
