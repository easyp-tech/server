---
status: investigating
trigger: "we've tested the proxy locally and it is working fine: 100% success rate. we've tested proxy in prod install and we see the problems (logs are in the ./logs dir). but logs are not good enough to debug the problem. we have to make the logs better!"
created: 2026-06-18
updated: 2026-06-18
---

# Debug Session: improve-prod-debug-logs

## Symptoms

- **Expected behavior:** `buf dep update` against the deployed proxy succeeds with 100% reliability, matching local test results.
- **Actual behavior (prod):** Some requests fail in production deployments while the same code path works perfectly locally. Specific observed errors: `unknown commit id: must call CommitService/GetCommits first` (400) on DownloadService; intermittent failures on RepositoryService calls. Failures are not deterministic — same client can succeed on retry.
- **Error messages:**
  - `handler error protocol=v1 request_id=... error="unknown commit id: must call CommitService/GetCommits first" status=400 error_class=bad_request body_bytes=38` on `POST /buf.registry.module.v1.DownloadService/Download`
  - `request completed ... status=200` followed by no clear indication of why a downstream client perceives a failure
  - No correlation token in incoming request log lines (the request_id only appears in the handler error line, missing from the matching "request completed" line)
- **Timeline:** Works in local dev (100% success). Breaks in prod install. No known prior state where it worked in prod.
- **Reproduction:** Run any proto module's `buf dep update` against the deployed proxy. Failures are intermittent and pod-dependent (two pods in cluster, need cross-pod correlation).
- **Key clues:**
  - Two log files (one per pod) of ~5,700 lines each, both timestamped 2026-06-18T15:11–15:16
  - Logs dominated by `GET /` health checks every ~2 seconds (huge noise)
  - Handler errors logged with `error` string but no preceding log of which request triggered the error, what its body/headers were, or what state the handler was in
  - "repository connected" log lines at startup reference bitbucket/github owners, but no logs of cache or artifactory calls during request handling
  - No upstream (cache, git fetch, artifactory) call trace at all in current logs
  - No per-request lifecycle markers (entry, decision branches, upstream calls, exit)

## Current Focus

- hypothesis: null
- test: null
- expecting: null
- next_action: "Phase A complete (build clean, vet clean, all tests pass). Phase C — surface CHECKPOINT to orchestrator: ship these log changes, deploy to prod, re-run `buf dep update`, capture new logs, and feed them back to the next continuation agent for diagnosis. Do not guess at root cause from old logs."
- reasoning_checkpoint: null

## Missing logging (root ask)

- **Upstream calls** — every cache lookup, git fetch, artifactory call should produce an INFO line with: target, latency, outcome (hit/miss/error), bytes returned.
- **Per-request lifecycle / correlation** — every log line for a request must carry the same `request_id`. Today request_id appears on handler errors but is missing from the matching "request completed" line, breaking correlation.
- **Handler decision trace** — when a handler picks a code branch (e.g. "must call CommitService/GetCommits first"), log which branch was taken and what state variable drove the decision.

## Evidence

- timestamp: 2026-06-18T20:30
  type: code_analysis
  file: logs/prod-buf-proxy-buf-proxy-5f87dc676b-5j74x-buf-proxy.log (5734 lines, 1.03 MB)
  finding: "Logs dominated by `GET /` health checks every ~2s (probable liveness/readiness probe). Bursts of 5–10 health checks in a row per probe source IP (192.168.53.10, 192.168.54.128) — pollutes signal. Real requests (DownloadService, RepositoryService, GraphService) are drowned out."

- timestamp: 2026-06-18T20:30
  type: code_analysis
  file: logs/prod-buf-proxy-buf-proxy-5f87dc676b-5j74x-buf-proxy.log
  finding: "Handler error line carries `request_id` but the matching `request completed` line immediately after does NOT carry the same `request_id`. No way to join the two lines for the same HTTP request. The 400 'unknown commit id' error is logged but we cannot tell which client request, what its body was, or what state the cache was in."

- timestamp: 2026-06-18T20:30
  type: code_analysis
  file: logs/prod-buf-proxy-buf-proxy-5f87dc676b-5j74x-buf-proxy.log
  finding: "Zero log lines for cache, git fetch, or artifactory calls during request handling. The 400 'unknown commit id' is thrown but the upstream lookup that should have populated the cache is invisible. This is the core gap: the bug is likely between handler and upstream, but no logs exist there."

- timestamp: 2026-06-18T20:30
  type: code_analysis
  file: logs/prod-buf-proxy-buf-proxy-5f87dc676b-5j74x-buf-proxy.log
  finding: "RepositoryService calls (`GetRepositoriesByFullName`, `GetRepositoryByFullName`) have very long durations (e.g. 1.49s, 0.94s, 0.63s) but no breakdown of which sub-step took the time. Could be git clone, clone over slow link, or cache rebuild — no way to tell."

- timestamp: 2026-06-18T20:30
  type: code_analysis
  file: logs/prod-buf-proxy-buf-proxy-5f87dc676b-nkmzs-buf-proxy.log (5793 lines, 1.04 MB)
  finding: "Second pod log has same shape. Need cross-pod correlation: same client request can land on either pod. Without a request_id present on entry, the two pods' logs cannot be joined."

- timestamp: 2026-06-18T20:35
  type: code_analysis
  file: cmd/easyp/main.go:179-226
  finding: "loggingMiddleware already generates request_id and stores it in context (line 184-194). It is NOT attached to the `request completed` line at line 224, even though it would be straightforward to add via attrs. This is the per-request correlation gap that the user is hitting."

- timestamp: 2026-06-18T20:35
  type: code_analysis
  file: internal/connect/interceptor.go:38-89
  finding: "loggingInterceptor attaches procedure and request_id to its logger (line 44-47) and only logs at DEBUG level (line 49, 73). At INFO level, no request lifecycle is emitted. Bumping to INFO and adding 'rpc started'/'rpc completed' lines would surface per-RPC trace in prod."

- timestamp: 2026-06-18T20:35
  type: code_analysis
  file: internal/providers/multisource/repo.go:50-119
  finding: "GetMeta / GetFiles / cacheGet / cachePut all log at DEBUG level. In prod (log level INFO) this trace is invisible. The 400 'unknown commit id' on DownloadService requires the prior cache miss / GetMeta / GetFiles call sequence to be visible to be diagnosable."

- timestamp: 2026-06-18T20:35
  type: code_analysis
  file: internal/connect/commits.go:259-356
  finding: "ServeDownload has three decision branches: (1) ref lookup miss → 400 'unknown commit id', (2) infoCache hit and filesMap populated → reuse, (3) cache miss → call GetMeta+GetFiles. None of these branches are logged. The 'ref' branch only logs when it fails. The 'cache hit' vs 'cache miss' branch is invisible. The `isV1` branch affecting digest format is not logged."

- timestamp: 2026-06-18T20:35
  type: code_analysis
  file: internal/providers/localgit/localgit.go:120-147
  finding: "GetMeta and GetFiles do named-lock + git checkout. Slow operations but no log. Together with multisource, this is the upstream trace gap."

## Eliminated

- hypothesis: "Root cause is in the existing log lines, we just need to read them more carefully."
  evidence: "Old logs have no per-request correlation across access log + handler error line, no upstream call trace, no handler decision trace. The 400 'unknown commit id' cannot be diagnosed from existing lines because the upstream path that would have populated commitMap is invisible."
  timestamp: 2026-06-18T20:35

- hypothesis: "Add `request_id` to the access log only, no other changes needed."
  evidence: "Even with the access log correlation, the upstream call sequence (which source was selected, cache hit vs miss, digest wrap vs not) is not in the logs. The bug lives somewhere in that invisible path."
  timestamp: 2026-06-18T20:36

- hypothesis: "Add per-request correlation via context.Value from multisource importing connect."
  evidence: "Importing internal/connect from internal/providers/multisource would create a cycle (connect -> multisource via the provider interface). Solved by extracting the request_id key into a new internal/reqid package that both can import."
  timestamp: 2026-06-18T20:37

## Resolution

- root_cause: null  (will be determined in next continuation after new prod logs are captured)
- fix: "Phase A observability changes applied (see Files Changed)."
- verification: "Build clean (`go build ./...`), `go vet` clean, all existing tests pass. Local verification of log emission requires a real request against the proxy; that verification is the user's job after they deploy these changes to prod and re-run the failing `buf dep update` workload."
- files_changed:
  - internal/reqid/reqid.go (new) — shared request_id context key
  - internal/reqid/reqid_test.go (new) — round-trip + empty tests
  - cmd/easyp/main.go — loggingMiddleware now emits 'request received' (entry) and 'request completed' (exit), both carrying request_id; the per-request logger is reused by all handler-level log lines
  - internal/connect/interceptor.go — interceptor now delegates to internal/reqid; rpc started / rpc completed lines emit at INFO (was DEBUG), with request_id, peer, request/response size, duration
  - internal/connect/commits.go — commitServiceHandler now emits 'handler decision' lines for: parsed (refs, body_bytes, protocol), commit_id_lookup (commit_id, ref_found), files_cache_lookup (info_cache_hit, cached_files), files_cache_hit/files_cache_miss, digest_b5_wrap/digest_b4_keep, resolve_meta, compute_digest (with resolved commit + is_v1 flag). All carry request_id.
  - internal/providers/multisource/repo.go — GetMeta / GetFiles / cacheGet / cachePut now log at INFO with target, owner/repo/commit, source selected, outcome (ok/cache_hit/cache_miss/not_found/error), cache_latency, source_latency, duration, files count, bytes. All carry request_id.
  - internal/providers/localgit/localgit.go — GetMeta / GetFiles now log 'upstream call' / 'upstream result' at INFO with target, owner/repo, commit, outcome, enum_latency, duration, files count, bytes. request_id propagation via internal/reqid.
