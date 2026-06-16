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

---
*Roadmap last updated: 2026-06-16*
