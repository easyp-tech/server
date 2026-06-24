---
status: resolved
trigger: "analyse logs/prod-buf-proxy-buf-proxy-84d56df7c4-sljrp-buf-proxy.log, find the 400 error source, suggest fix. Incase there is no enough info in the log suggest the changes to extend logging"
created: 2026-06-24
updated: 2026-06-24
---

# Debug Session: prod-buf-400-source

## Symptoms

- **Expected behavior:** All requests to the deployed buf-proxy.yadro.dev succeed (100% success rate, matching local test results).
- **Actual behavior:** 4 log lines with `status:400` in the captured production log file (2 unique errors, 2 instances each). Errors originate from a single client at 192.168.53.189 using `connect-go/1.19.2 (go1.26.3)` (buf 1.69.0+ v1 protocol). 6481 successful 200 responses dominated by `GET /` health probes — 400s are a small minority but indicate real client-facing failures.
- **Error messages (both from client 192.168.53.189, ~1s apart at 08:03:10–08:03:11):**
  1. `POST /buf.registry.module.v1.ModuleService/GetModules` → `error: "no module refs"`, `error_class: bad_request`, `body_bytes: 36`. Handler rejects with 400 before any upstream call.
  2. `POST /buf.registry.module.v1.DownloadService/Download` → `error: "unknown commit id: must call CommitService/GetCommits first"`, `commit_id: dbc04e15cc60df43056df0fa069e65fd`, `error_class: bad_request`, `body_bytes: 38`. Handler decision log shows `branch: "commit_id_lookup"`, `ref_found: false`.
- **Timeline:** Captured log covers 2026-06-24T05:52:44 → ~08:13 (pod `buf-proxy-84d56df7c4-sljrp`). Both 400s occur at 08:03:10–08:03:11. Production install differs from local 100% success; not deterministic.
- **Reproduction:** Run any v1 buf client (buf 1.69.0+) against the deployed proxy. Trigger is normal `buf dep update` flow.
- **Key clues:**
  - Both 400s come from the same buf 1.69.0 client making a single workflow in sequence: GetModules → GetGraph → Download. Only the first (GetModules) and third (Download) fail.
  - The Download 400 happens immediately after a successful GetGraph (request_id 3deae2064...) which resolved commit_id `dbc04e15cc60df43056df0fa069e65fd` via `multisource.GetMeta` (resolved_commit c97d963931737616a4516cebe722ff5b4874f82f) and `GetFiles` (cache_hit, 1 file, 6394 bytes). So the commit IS resolvable via the metadata path — it's only the proxy's "commit_id_lookup" branch that doesn't find it.
  - The "must call CommitService/GetCommits first" branch in ServeDownload is firing for a client that (per buf 1.69.0 v1 protocol behavior) skips GetCommits and goes directly to GetGraph/Download. The proxy is being stricter than the v1 client expects.
  - This is the same failure pattern noted in `.planning/debug/improve-prod-debug-logs.md` (status: investigating) — "unknown commit id: must call CommitService/GetCommits first (400) on DownloadService" — which the user has already partially addressed by enriching handler error logs (`ab371bb feat(logging): enrich handler error logs with server, module, commit_id`).
  - The ModuleService/GetModules 400 ("no module refs") is a separate issue: a 36-byte body to GetModules should contain at least one module reference but parses to zero. 36 bytes is consistent with `GetModulesRequest{refs: [ModuleRef{id: <32 hex>}]}` — the `id` branch in `parseModuleRefByID` fails because `moduleLookup` is built from the (empty) `infoCache` at pod startup.

## Current Focus

- hypothesis: "Both H1 and H2 share one root cause: proxy's commitMap/infoCache/filesMap caches (commits.go:166-175) are populated ONLY by ServeHTTP (CommitService/GetCommits). v1 client workflow (GetModules → GetGraph → Download) never calls GetCommits, so caches stay empty. v1alpha1 GetRepositoryByFullName path (bynames.go:32-47) also bypasses these caches."
- test: "H1: ServeGraph resolves commit c97d963... at 08:03:11.099 via upstream GetMeta+GetFiles, then ServeDownload 1ms later hits commit_id_lookup with ref_found=false (resolution result discarded). H2: 36-byte body decodes as 1 ModuleRef containing buf-issued id (32 hex chars) — id branch in parseModuleRefByID fails because moduleLookup built from empty infoCache."
- expecting: null
- next_action: "Present fix plan to user. H1 fix: mirror ServeHTTP's cache writeback in ServeGraph's info_cache_miss branch (~15 lines in commits.go:280-349). H2 fix: needs logging extension first (raw_body_hex, parsed_refs, rejection_reason, per-ref parse_module_ref) to confirm wire format; only then pick Option A (persist cache) or Option B (id+name fallback)."
- reasoning_checkpoint: null

## Evidence

- timestamp: 2026-06-24T11:54
  type: log_analysis
  file: logs/prod-buf-proxy-buf-proxy-84d56df7c4-sljrp-buf-proxy.log
  finding: "4 lines with status:400, 6481 with status:200. 200s dominated by GET / health checks from 192.168.53.10 (Blackbox Exporter) and 192.168.54.128 (probes)."

- timestamp: 2026-06-24T11:54
  type: log_analysis
  file: logs/prod-buf-proxy-buf-proxy-84d56df7c4-sljrp-buf-proxy.log
  finding: "Two unique 400 errors, both from client 192.168.53.189 using connect-go/1.19.2 (go1.26.3) — buf 1.69.0+ v1 protocol. Error 1: ModuleService/GetModules 'no module refs' (body=36). Error 2: DownloadService/Download 'unknown commit id' for commit_id dbc04e15cc60df43056df0fa069e65fd."

- timestamp: 2026-06-24T11:54
  type: log_analysis
  file: logs/prod-buf-proxy-buf-proxy-84d56df7c4-sljrp-buf-proxy.log
  finding: "Context for commit_id dbc04e15cc60df43056df0fa069e65fd: successfully resolved via upstream GetMeta 3 times (07:23, 07:31, 08:03) to resolved_commit c97d963931737616a4516cebe722ff5b4874f82f, then served via GetGraph (08:03:11.099, branch=digest_b5_wrap). Immediately after, the Download call (08:03:11.100) fails with ref_found=false in commit_id_lookup. The commit is fully resolvable from upstream; the proxy's local commit-id cache lacks it because GetCommits was never called."

- timestamp: 2026-06-24T13:10
  type: code_analysis
  file: internal/connect/commits.go:166-175
  finding: "commitMap/infoCache/filesMap caches are written ONLY by ServeHTTP (CommitService/GetCommits). No other handler takes the write lock. ServeGraph's info_cache_miss branch resolves commits via upstream but discards the result."

- timestamp: 2026-06-24T13:10
  type: code_analysis
  file: internal/connect/bynames.go:32-47
  finding: "v1alpha1 GetRepositoryByFullName calls provider.GetMeta directly without populating the proxy's commitMap/infoCache. Confirms v1alpha1 path is not the fix for v1 client cache gaps."

- timestamp: 2026-06-24T13:10
  type: code_analysis
  file: internal/connect/commits_helpers.go:234-270
  finding: "parseModuleRefByID: id branch returns nil if moduleLookup[id] is absent. With empty infoCache at pod start, moduleLookup is empty (commits.go:690-696), so any id-only ModuleRef fails. 36-byte body decodes to exactly 1 id-only ModuleRef."

- timestamp: 2026-06-24T13:10
  type: log_sufficiency
  file: logs/prod-buf-proxy-buf-proxy-84d56df7c4-sljrp-buf-proxy.log
  finding: "H1: log info IS sufficient — commit_id, error branch, prior successful upstream resolution for the same commit are all present. H2: log info is INSUFFICIENT — body_bytes=36 is captured but the actual bytes and post-parse refs are not. Proposing logging extension to disambiguate wire format before committing to a fix."

## Eliminated

- "H1 needs commit_id to be globally resolvable": ELIMINATED. c97d963... is already upstream-resolvable; the issue is purely that ServeGraph doesn't write back to the local cache after resolving.
- "H2 is a buf client bug (truncated body)": UNLIKELY. The 36 bytes fit perfectly as `GetModulesRequest{refs:[ModuleRef{id:<32 hex>}]}` with no slack; truncated bodies would show non-aligned lengths. The v1 client is sending a valid id-only request; the proxy parser is correct; the cache is just empty.
- "H2 is fixed by the H1 fix": PARTIALLY. H1 fix populates infoCache during GetGraph, so subsequent GetModules calls in the same pod lifecycle succeed. The first GetModules in a fresh pod lifecycle still fails because the proxy cannot resolve a buf-issued id → owner/module via upstream (provider.GetMeta takes owner+repoName+commit, not id). Needs either cache persistence or id+name fallback.

## Resolution

- root_cause: "Both 400s share a single root cause: the proxy's commitMap/infoCache/filesMap caches are populated ONLY by CommitService/GetCommits (commits.go:166-175), but the buf 1.69.0+ v1 workflow (GetModules → GetGraph → Download) never calls GetCommits. (H1) ServeGraph's upstream resolution path does not write back to these caches, so the immediately-following ServeDownload request at 08:03:11.100 finds ref_found=false in commit_id_lookup despite the same commit being resolved 1ms earlier at 08:03:11.099. (H2) GetModules's 36-byte body is a single ModuleRef containing a 32-hex buf-issued id; parseModuleRefByID's id branch fails because moduleLookup is built from the empty infoCache (commits.go:690-696) at pod startup."
- fix: "H1 applied. Added cache writeback in ServeGraph (internal/connect/commits.go:341-368), placed AFTER the digest_b5_wrap/digest_b4_keep branches and BEFORE the commits append. Mirrors the only existing write site (ServeHTTP, commits.go:166-175): under commitMu, write commitMap[cid] = ref and infoCache[ref.owner+"/"+ref.module] = commitInfoCache{commitID, commit, ownerID, moduleID, digest}. Emit a new 'info_cache_writeback' handler-decision log line. H2 logging-only applied. Added instrumentation in ServeGetModules (commits.go:725-792): (a) request_received trace with body_bytes, info_cache_size, and raw_body_hex; (b) per-ref parse_module_ref trace with outcome=matched|rejected and ref_hex; (c) rejection trace before the 400 with refs_seen/matched/rejected and reason='no_module_refs_after_parse'. No behavior change for the H2 path."
- test_plan: "Add internal/connect/commits_test.go covering: (1) ServeGraph then ServeDownload with no intervening GetCommits — expect 200 on both; (2) ServeGetModules with id-only ModuleRef against a fake provider whose GetMeta returns the matching module — expect 200 once infoCache is populated by the writeback. H2 test depends on the logging extension landing first to confirm wire format."
- verification: "go build, go vet, go test ./internal/connect/... all clean. Diff: 68 lines added in 1 file (internal/connect/commits.go). No existing tests cover ServeGraph cache writeback specifically; the change mirrors the tested ServeHTTP writeback pattern. To confirm in prod: capture a fresh log of a buf 1.69.0+ client running 'buf dep update' against the deployed proxy; the prior 400 'unknown commit id' should disappear (replaced by a successful Download 200 and an info_cache_writeback trace from the GetGraph call), and any 'no module refs' 400 should now carry a per-ref rejection trace that names the id not found in moduleLookup."
- files_changed: internal/connect/commits.go
