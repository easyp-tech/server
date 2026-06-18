# Requirements: EasyP Buf Proxy — Diagnostic Logging

**Defined:** 2026-06-16
**Core Value:** The proxy must correctly serve both old (v1.30.1) and modern (v1.69.0+) Buf CLI clients simultaneously

## v1.3 Requirements

### Logging Foundation

- [ ] **FOUND-01**: Log level can be configured via `EASYP_LOG_LEVEL` env var override (debug, info, warn, error)
- [ ] **FOUND-02**: Sensitive data is centrally redacted via `slog.HandlerOptions.ReplaceAttr` before any new log calls are added
- [ ] **FOUND-03**: Log output format can be toggled between text and JSON via LogConfig
- [ ] **FOUND-04**: Source line information can be optionally included in log entries via LogConfig

### Logging Infrastructure

- [ ] **INFR-01**: Each request carries a correlation ID (`X-Request-Id`) propagated through `context.Context` so concurrent request logs are distinguishable
- [ ] **INFR-02**: A Connect RPC unary interceptor logs procedure, peer, duration, request/response size, and error code for all v1alpha1 handlers (blobs, modulepins, bynames)
- [ ] **INFR-03**: Existing HTTP middleware error logging is demoted to INFO-level timing/status to prevent double-logging with handler-level logs

### Error Path Logging — v1beta1/v1 Raw Handlers

- [ ] **ERR-01**: `ServeHTTP` (CommitService) logs structured error context on failure (owner, repo, error, request_id)
- [ ] **ERR-02**: `ServeGraph` (GraphService) logs structured error context on failure (owner, module, error, request_id)
- [ ] **ERR-03**: `ServeDownload` (DownloadService) logs structured error context on failure (owner, module, commit, error, request_id)
- [ ] **ERR-04**: `ServeGetModules` (ModuleService) logs structured error context on failure (owner, module, error, request_id)
- [ ] **ERR-05**: All handler-level error logs use consistent attribute naming convention (`protocol: "v1beta1"`, structured fields)

### Provider Logging

- [ ] **PROV-01**: GitHub provider API calls log before/after with redacted URL, method, response status, and timing at debug level
- [ ] **PROV-02**: Cache (Artifactory) operations log hit/miss with duration at debug level, distinguishing context cancellation from API errors

### Operational

- [ ] **OPS-01**: Panic recovery middleware wraps the entire ServeMux, logging full stack trace and returning HTTP 500

## v1.4 Requirements (Deferred)

### Logging Infrastructure (v1.4)

- **INFR-04**: Dynamic log level via `slog.LevelVar` + signal handler (SIGUSR1) toggling between INFO and DEBUG

### Provider Logging (v1.4)

- **PROV-03**: BitBucket provider tracing at debug level
- **PROV-04**: Local git provider tracing at debug level

### Advanced Diagnostics (v1.4)

- **ADVN-01**: Request/response body hex dump at debug level with truncation and opt-in header
- **ADVN-02**: Per-provider duration tracking

## Out of Scope

| Feature | Reason |
|---------|--------|
| Explicit v1alpha1 handler error-path logging | Covered by Connect interceptor (INFR-02) — no separate handler-level logging needed |
| OpenTelemetry integration | Overkill for single-process proxy with <10 spans per request |
| Log sampling / rate limiting | Not needed until production load requires it |
| Admin HTTP endpoint for log level | Signal handler is simpler and sufficient — deferred to v1.4 |
| Structured error codes | Add after operational experience reveals which paths need dashboarding |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| FOUND-01 | Phase 11 | Pending |
| FOUND-02 | Phase 11 | Pending |
| FOUND-03 | Phase 11 | Pending |
| FOUND-04 | Phase 11 | Pending |
| INFR-01 | Phase 12 | Pending |
| INFR-02 | Phase 12 | Pending |
| INFR-03 | Phase 12 | Pending |
| ERR-01 | Phase 13 | Pending |
| ERR-02 | Phase 13 | Pending |
| ERR-03 | Phase 13 | Pending |
| ERR-04 | Phase 13 | Pending |
| ERR-05 | Phase 13 | Pending |
| PROV-01 | Phase 14 | Pending |
| PROV-02 | Phase 14 | Pending |
| OPS-01 | Phase 15 | Pending |

**Coverage:**
- v1.3 requirements: 15 total
- Mapped to phases: 15
- Unmapped: 0 ✓

### Phase Requirement Summary

| Phase | Requirements |
|-------|--------------|
| Phase 11: Logging Foundation | FOUND-01, FOUND-02, FOUND-03, FOUND-04 |
| Phase 12: Logging Infrastructure | INFR-01, INFR-02, INFR-03 |
| Phase 13: Error Path Logging | ERR-01, ERR-02, ERR-03, ERR-04, ERR-05 |
| Phase 14: Provider Logging | PROV-01, PROV-02 |
| Phase 15: Operational Logging | OPS-01 |

---
*Requirements defined: 2026-06-16*
*Last updated: 2026-06-16 — roadmap created, all 15 v1.3 requirements mapped to phases 11-15*
