# Roadmap: EasyP Buf Proxy

## Milestones

- ✅ **v1.1 Protocol Modernization** — Phases 1-5 (shipped 2026-05-07)
- ✅ **v1.2 Dependency Modernization** — Phases 6-10 (shipped 2026-05-10)
- 📋 **v1.3 Diagnostic Logging** — Phases 11-15

## Phases

<details>
<summary>✅ v1.1 Protocol Modernization (Phases 1-5) — SHIPPED 2026-05-07</summary>

- [x] Phase 1: Code Generation (2/2 plans) — completed 2026-05-06
- [x] Phase 2: Handler Adaptation (1/1 plan) — completed 2026-05-06
- [x] Phase 3: Test Infrastructure (2/2 plans) — completed 2026-05-07
- [x] Phase 4: Old Protocol Validation (1/1 plan) — completed 2026-05-07
- [x] Phase 5: New Protocol Validation (2/2 plans) — completed 2026-05-07

</details>

<details>
<summary>✅ v1.2 Dependency Modernization (Phases 6-10) — SHIPPED 2026-05-10</summary>

- [x] Phase 6: Dependency Upgrades (2/2 plans) — completed 2026-05-08
- [x] Phase 7: Proto Regeneration & Verification (2/2 plans) — completed 2026-05-08
- [x] Phase 8: Go Code Modernization (1/1 plan) — completed 2026-05-08
- [x] Phase 9: Submodule Cleanup (1/1 plan) — completed 2026-05-09
- [x] Phase 10: Code Quality Fixes (4/4 plans) — completed 2026-05-09

</details>

### 📋 v1.3 Diagnostic Logging — In Progress

**Milestone Goal:** Improve logging across all request/response paths so that 400 errors and other failures are diagnosable without requiring source code access

- [ ] **Phase 11: Logging Foundation** — Logger config, env-var override, centralized redaction, format/source options
- [ ] **Phase 12: Logging Infrastructure** — Correlation ID propagation, Connect RPC interceptor, middleware demotion
- [ ] **Phase 13: Error Path Logging** — Structured error context on all v1beta1/v1 handler failures
- [ ] **Phase 14: Provider Logging** — Debug-level tracing for GitHub provider and Artifactory cache operations
- [ ] **Phase 15: Operational Logging** — Panic recovery middleware with full stack trace
- [x] **Phase 16: Commit ID Resolution Improvements** — Use first 16 bytes of git SHA as commit id (incl. short-sha support), probe all configured repos on cache miss, clearer not-found error response and log (completed 2026-07-06)
- [x] **Phase 17: Fix PR #37 review findings** — Address pre-merge review findings from PR #37 (Phase 16) so the commit-id format cutover lands without undoing the v1.3 logging-quality work or breaking Bitbucket SHA-256 repositories (completed 2026-07-07)
- [x] **Phase 18: Respect buf.yaml dependency refs** — Honor `Name.ref` end-to-end + isSHA-gated provider ref-resolution + prewarm removal with `commitUUIDInverse`-based probe (completed 2026-07-07)
- [x] **Phase 19: e2e tests for ref-honoring in buf.yaml deps** — Three `TestRefRespected_*` tests that exercise the proxy's ref-honoring behavior end-to-end via a real buf CLI + real GitHub API; tests pass with `EASYP_GH_TOKEN` set, skip cleanly otherwise (completed 2026-07-07)
- [ ] **Phase 20: Fix ref-honoring regressions discovered in Phase 19** — Restore the no-ref (HEAD) path that Phase 18 broke for buf v1.30.1, and fix the v1beta1 path where the ref is being ignored on `buf dep update`

## Phase Details

### Phase 11: Logging Foundation

**Goal**: Operators can configure log level, format, and source info, with centralized sensitive-data redaction applied to all log output
**Depends on**: Nothing (foundation phase)
**Requirements**: FOUND-01, FOUND-02, FOUND-03, FOUND-04
**Success Criteria** (what must be TRUE):

  1. Setting `EASYP_LOG_LEVEL=debug` produces debug-level log lines; default (no env var) logs at info level
  2. Sensitive fields (tokens, passwords) are automatically redacted from every log entry via `slog.HandlerOptions.ReplaceAttr` — no sensitive data appears in any output
  3. Setting `EASYP_LOG_FORMAT=json` produces JSON-formatted log output; default is human-readable text
  4. Enabling `AddSource` in config includes source file and line number in log entries
  5. Invalid log level values produce a clear error message at startup and exit gracefully

**Plans**: TBD

### Phase 12: Logging Infrastructure

**Goal**: Every request is traceable via correlation ID, and v1alpha1 handlers are instrumented via a single Connect RPC unary interceptor
**Depends on**: Phase 11
**Requirements**: INFR-01, INFR-02, INFR-03
**Success Criteria** (what must be TRUE):

  1. Every log line in the request lifecycle includes a `request_id` (either from `X-Request-Id` header or auto-generated 8-byte hex)
  2. Logs from concurrent requests are distinguishable by their unique `request_id`
  3. v1alpha1 handler procedures (blobs, modulepins, bynames) produce structured log entries with procedure, peer, duration, request/response size, and error code via a single unary interceptor — zero handler code changes
  4. HTTP middleware logs timing and status at INFO level only — error-level logging is removed from middleware to prevent double-logging with handler-level logs
  5. Error logs from handler code include the `request_id` linking them to the originating request via context propagation

**Plans**: TBD

### Phase 13: Error Path Logging — v1beta1/v1 Handlers

**Goal**: Every failure in v1beta1/v1 raw handlers produces a structured log entry with full request context and consistent attribute naming
**Depends on**: Phase 12
**Requirements**: ERR-01, ERR-02, ERR-03, ERR-04, ERR-05
**Success Criteria** (what must be TRUE):

  1. `ServeHTTP` (CommitService) failure logs include owner, repo, error, and request_id
  2. `ServeGraph` (GraphService) failure logs include owner, module, error, and request_id
  3. `ServeDownload` (DownloadService) failure logs include owner, module, commit, error, and request_id
  4. `ServeGetModules` (ModuleService) failure logs include owner, module, error, and request_id
  5. All handler-level error logs use consistent attribute names (`protocol`, `owner`, `repo`, `commit`, `request_id`, `error`) and include `protocol: "v1beta1"` — no naming inconsistencies across handlers

**Plans**: TBD

### Phase 14: Provider Logging

**Goal**: Provider API calls and cache operations are traceable at debug level with timing, status, and provider-type context
**Depends on**: Phase 12
**Requirements**: PROV-01, PROV-02
**Success Criteria** (what must be TRUE):

  1. GitHub provider HTTP requests log before and after each API call with redacted URL, method, response status, and duration at debug level
  2. Artifactory cache operations log hit/miss with duration at debug level
  3. Cache error logs distinguish context cancellation (client disconnected) from API errors (upstream failure)
  4. Provider log lines include a `provider_type` attribute (e.g., `github`, `artifactory`) for filtering

**Plans**: TBD

### Phase 15: Operational Logging — Panic Recovery

**Goal**: Unhandled panics are caught, logged with full stack trace, and return HTTP 500 instead of crashing the process
**Depends on**: Phase 11
**Requirements**: OPS-01
**Success Criteria** (what must be TRUE):

  1. A panic anywhere in the request handling chain is caught by recovery middleware wrapping the entire ServeMux
  2. The panic is logged with full stack trace including goroutine information and request context
  3. The client receives an HTTP 500 response instead of a connection reset or process termination
  4. Other concurrent requests continue unaffected when one request panics

**Plans**: TBD

### Phase 16: Commit ID Resolution Improvements

**Goal**: Make commit-id resolution more robust (accept short git SHAs, fall back to upstream probe on cache miss) and the not-found failure mode diagnosable (clear error response and structured log line)
**Depends on**: Phase 15
**Requirements**: TBD
**Success Criteria** (what must be TRUE):

  1. The minted commit id is the first 16 bytes of the git SHA (no SHA-256 derivation), and a unit test verifies that both full 40-char and short (7-char) git SHAs round-trip through `commitUUID` deterministically
  2. When a `DownloadService/Download` request carries a commit id that is not in `commitMap` and the proxy serves multiple modules, the handler probes every configured source for the sha and uses the first match — single-source deployments keep the existing `resolveForeignCommitID` fast path
  3. The 400 response returned for an unresolvable commit id names the id itself in both the wire body and the structured log line, so an operator can correlate a client-side "unknown commit id" with a prior `GetCommits` log entry without re-reading the request

**Plans**: TBD

### Phase 17: Fix PR #37 review findings

**Goal:** Address pre-merge review findings from PR #37 (Phase 16) so the commit-id format cutover lands without undoing the v1.3 logging-quality work or breaking Bitbucket SHA-256 repositories
**Depends on:** Phase 16
**Requirements**: TBD
**Success Criteria** (what must be TRUE):

  1. A `computeB4Digest` failure from `GetFiles`, `computeB4DigestFromFiles`, or `commitUUID` is logged via `logHandlerError`/`upstreamError` (not `internalError`) with full context (owner, module, repo, commit, request_id, server, protocol, status) and returns 502 for upstream failures — restores the `ERR-05` contract that the new `internalError` helper bypassed
  2. The new `internalError` helper is removed; all handler-level 500s flow through the existing `logHandlerError` (commits.go:777) so the structured 5xx log line is joinable on `request_id` and `error_class=internal` is set automatically
  3. `commitUUID` accepts git SHAs of 40 chars (SHA-1) and 64 chars (SHA-256) — Bitbucket Server on a SHA-256-enabled repo returns 64-char commits and currently 500s on the strict `len != 40` check (commits_helpers.go:39); a unit test covers both lengths
  4. A new test in `commits_helpers_test.go` (not `_test.go` production file) exercises the SHA-256 path; `preResolveForTest` (commits_helpers.go:60-70) is moved out of the production source so it cannot be reached by future code
  5. The 400 not-found response message and structured log line for an unresolvable commit id remain generic enough to apply to both stale-lockfile misses and genuine foreign-id misses from other registries

**Plans:** 1 plan
Plans:

- [x] 17-01-PLAN.md — Atomic 4-task fix: routing `computeB4Digest` errors through the right helpers, removing `internalError`, accepting 64-char SHA-256, softening the 400 message, moving `preResolveForTest` to the test file (covers SC-1 through SC-5)

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Code Generation            | v1.1 | 2/2 | Complete | 2026-05-06 |
| 2. Handler Adaptation         | v1.1 | 1/1 | Complete | 2026-05-06 |
| 3. Test Infrastructure        | v1.1 | 2/2 | Complete | 2026-05-07 |
| 4. Old Protocol Validation    | v1.1 | 1/1 | Complete | 2026-05-07 |
| 5. New Protocol Validation    | v1.1 | 2/2 | Complete | 2026-05-07 |
| 6. Dependency Upgrades        | v1.2 | 2/2 | Complete | 2026-05-08 |
| 7. Proto Regeneration         | v1.2 | 2/2 | Complete | 2026-05-08 |
| 8. Go Code Modernization      | v1.2 | 1/1 | Complete | 2026-05-08 |
| 9. Submodule Cleanup          | v1.2 | 1/1 | Complete | 2026-05-09 |
| 10. Code Quality Fixes        | v1.2 | 4/4 | Complete | 2026-05-09 |
| 11. Logging Foundation        | v1.3 | 0/0 | Not started | - |
| 12. Logging Infrastructure    | v1.3 | 0/0 | Not started | - |
| 13. Error Path Logging        | v1.3 | 0/0 | Not started | - |
| 14. Provider Logging          | v1.3 | 0/0 | Not started | - |
| 15. Operational Logging       | v1.3 | 0/0 | Not started | - |
| 16. Commit ID Resolution Improvements | v1.3 | 3/3 | Complete    | 2026-07-06 |
| 17. Fix PR #37 review findings | v1.3 | 1/1 | Complete | 2026-07-07 |
| 18. Respect buf.yaml dependency refs (not always HEAD); fix related bug; remove prewarm logic | v1.3 | 1/1 | Complete | 2026-07-07 |

### Phase 18: Respect buf.yaml dependency refs (not always HEAD); fix related bug; remove prewarm logic now that buf id → git id is derivable

**Goal:** Three corrections to dependency resolution, shipped as one atomic pass: (1) honor the `ref` field the buf CLI sends in the `Name` message so a dep like `cyp/cyp-net-listeners:main/v2` returns the SHA at `main/v2`, not the HEAD; (2) replace the `commit != "" && commit != "main"` short-circuit in both bitbucket and github providers with an `isSHA(commit)` gate so non-SHA inputs route through the providers' commit-fetch APIs to resolve; (3) drop the `prewarmHeads` startup fan-out and replace it with a UUID→SHA-prefix inverse function (`commitUUIDInverse`) that lets `probeCommitID` recover the right module on a Download cache-miss without the prewarm.
**Depends on:** Phase 17
**Requirements**: SC-1, SC-2, SC-3, SC-4, SC-5
**Success Criteria** (what must be TRUE):

  1. A buf client pinning `buf-proxy.yadro.dev/cyp/cyp-net-listeners:main/v2` in `buf.yaml` gets a UUID in `buf.lock` derived from the SHA at `main/v2` (not from HEAD) — verified end-to-end by the providers' `TestGetMeta_ResolvesRef` and the connect-package `TestParseResourceRefName_ReadsRef`
  2. The `commit != "" && commit != "main"` short-circuit is gone from both `bitbucket/getrepo.go` and `github/getrepo.go`; both providers now gate on `isSHA(commit)` — verified by structural grep + `go test ./internal/providers/{bitbucket,github}/ -count=1`
  3. `prewarmHeads` and `registerResolved` are deleted from `commits.go`; the goroutine launch in `api.go:111-113` is gone; the `PrewarmConfig` block in `config.go:96-101` is gone; `prewarmEnabled`/`prewarmTimeout`/`prewarmOnce` fields on `commitServiceHandler` and `PrewarmEnabled`/`PrewarmTimeout` on `CommitResolution` are gone — verified by 5 structural grep checks returning 0 hits
  4. `commitUUIDInverse(uuid string) (string, error)` exists in `commits_helpers.go`, returns the 28-char SHA prefix, errors on non-32-char or non-hex input; `TestCommitUUIDInverse` covers 8 cases (round-trip with a known 40-char SHA, all-zero, all-ones, 64-char SHA round-trip, empty, 31-char, 33-char, 32-char non-hex)
  5. `probeCommitID` derives the 28-char SHA prefix from a 32-char UUID input via `commitUUIDInverse`, probes each source with the prefix, and only registers a hit when the returned `meta.Commit` actually starts with the recovered prefix (collision-safe); `TestServeDownload_AfterRestart_ProbeResolvesUUID` and `TestServeDownload_AfterRestart_ProbeMissesOnUnknownUUID` exercise the post-restart path

**Plans:** 1 plan
Plans:
Plans:

- [x] [18-01](./phases/18-respect-buf-yaml-dependency-refs-not-always-head-fix-related/18-01-PLAN.md) — Honor `Name.ref` end-to-end + isSHA-gated provider ref-resolution + prewarm removal with `commitUUIDInverse`-based probe

### Phase 19: we need e2e tests for the ref specified for dependency - looks like it does not work

**Goal:** End-to-end proof that the proxy honors the `ref` field in a `buf.yaml` dependency: a `buf mod update` (and `buf dep update` on v1.32+) run with a `name:ref` dep in buf.yaml must produce a `buf.lock` whose `commit:` line is the UUID derived from the SHA at that ref, not from HEAD. Phase 18 (commit 4fc6f28) shipped the ref-honoring code path (`parseResourceRefName` reads proto field 3, `ServeHTTP`/`ServeGraph` pass `ref.ref` to providers, providers gate on `isSHA(commit)` and route refs through their commit-fetch APIs). This phase adds the missing real-server e2e tests that exercise the integration with a real buf CLI + real GitHub API.
**Depends on:** Phase 18
**Requirements**: SC-19-1, SC-19-2, SC-19-3 (all derived from this plan; see 19-01-PLAN.md)
**Success Criteria** (what must be TRUE):

  1. For every cached buf binary in `testdata/buf/`, `buf mod update` against the proxy with a `name:ref` dep in buf.yaml pins `buf.lock` to a different commit than the no-ref run; a regression where the proxy returns HEAD for the ref-pinned run fails the test with both lock files shown side-by-side (`TestRefRespected_ModUpdate_DiffersFromHead`)
  2. For v1.69.0, `buf mod update` with the pinned googleapis tag (`common-protos-1_3_1`) pins `buf.lock` to the UUID derived from the SHA at that tag — the SHA is fetched via `git ls-remote https://github.com/googleapis/googleapis refs/tags/common-protos-1_3_1` and the UUID is computed via the same `commitUUID` byte table the proxy uses (`TestRefRespected_ModUpdate_MatchesUpstreamSHA`)
  3. For v1.69.0, `buf dep update` with the pinned ref pins `buf.lock` to a different commit than the no-ref run; the modern subcommand exercises the same ref-honoring path the deprecated `buf mod update` does, on the v1beta1 protocol (`TestRefRespected_DepUpdate_DiffersFromHead`)

**Plans:** 1 planPlans:

- [ ] [19-01](./phases/19-we-need-e2e-tests-for-the-ref-specified-for-dependency-looks/19-01-PLAN.md) — Adopt in-progress e2e drafts (ref_test.go + testutil/server.go additions); 2 tasks: (1) finalize testutil additions and verify testutil unit tests still pass; (2) finalize e2e tests, verify they compile + list + skip cleanly without EASYP_GH_TOKEN, and commit both files

### Phase 20: fix the problem with refs discovered on phase 19

**Goal:** [To be planned]
**Requirements**: TBD
**Depends on:** Phase 19
**Plans:** 0 plans
Plans:

- [ ] TBD (run /gsd-plan-phase 20 to break down)

### Phase 21: we need another e2e test: we are doing buf generate with buf.lock pointing to the valid bt not the latest commit. This is valid situation and should work with v1 and v2

**Goal:** Add a single e2e test (`TestGenerateWithPinnedBufLock`) that proves the proxy can serve a `buf generate` request when `buf.lock` pins a valid (but non-HEAD) commit. The test runs for every cached buf version (v1.30.1 v1alpha1 + v1.69.0 v1beta1), derives the pinned UUID from a real googleapis tag via `git ls-remote` + the test-side `commitUUIDForTest` byte-table mirror, overwrites `buf.lock`'s `commit:` line with the pinned UUID, and asserts `buf generate` exits 0 + generates at least one non-empty `gen/go/google/type/*.pb.go` file whose content contains `package google.type`. This is the regression guard for the Phase 18/20 read-path (infoCache writeback + post-restart `probeCommitID`): a future regression that 400s the proxy on a pre-existing UUID or serves wrong content is caught at CI time.
**Requirements**: SC-21-1, SC-21-2 (derived from this plan; see 21-01-PLAN.md)
**Depends on:** Phase 20
**Plans:** 1/1 plans complete
Plans:

- [x] [21-01](./phases/21-we-need-another-e2e-test-we-are-doing-buf-generate-with-buf-/21-01-PLAN.md) — Adopt new e2e drafts (generate_test.go + testutil/server.go additions); 2 tasks: (1) add `RunBufGenerateWithPinnedLock` public wrapper + `runBufGenerate` private helper in testutil/server.go (writes buf.yaml + buf.gen.yaml + dummy.proto, runs `buf mod update`, overwrites the lock's `commit:` line via `strings.Replace(..., 1)`, then runs `buf generate`); (2) add `e2e/generate_test.go` with `TestGenerateWithPinnedBufLock` matrix test, verify it compiles + lists + skips cleanly without `EASYP_GH_TOKEN`, commit both files

### Phase 22: Fix v1.30.1 v1alpha1 read-path UUID handling; verify v1 protocol works

**Goal:** Wire the v1alpha1 `DownloadManifestAndBlobs` handler on `*api` to the existing Phase 18 UUID-resolution machinery on `*commitServiceHandler` (commitMap → resolveForeignCommitID → probeCommitID via commitUUIDInverse), so that a 32-char buf-issued UUID carried in the request reference is resolved to its 40-char git SHA BEFORE being passed to `GetFiles`/`GetTree`; then re-run the Phase 21 `TestGenerateWithPinnedBufLock/v1.30.1` e2e regression gate to confirm the v1 protocol works end-to-end.
**Requirements**: PR-22-1, PR-22-2, PR-22-3, PR-22-4, PR-22-5 (derived from RESEARCH.md Test Map; see 22-01-PLAN.md)
**Depends on:** Phase 21
**Plans:** 1/1 plans complete
Plans:

- [ ] [22-01](./phases/22-fix-v1-30-1-v1alpha1-read-path-uuid-handling-verify-v1-proto/22-01-PLAN.md) — Wire v1alpha1 DownloadManifestAndBlobs to Phase 18 UUID-resolution (CommitResolver interface + resolveCommitForRead wrapper + isUUID branch in blobs.go) with TDD unit cover for PR-22-1/2/3 and the Phase 21 e2e gate for PR-22-4

---

*Roadmap last updated: 2026-07-08*
