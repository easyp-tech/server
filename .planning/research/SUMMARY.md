# Project Research Summary

**Project:** EasyP Buf Proxy — Diagnostic Logging Improvements
**Domain:** Go-based Connect RPC proxy server with dual-protocol support (v1alpha1 Connect RPC + v1/v1beta1 raw protobuf)
**Researched:** 2026-06-16
**Confidence:** HIGH

## Executive Summary

This research evaluates what is needed to make 400/500 errors in the EasyP Buf Proxy diagnosable without source code access. The project is a stateless Go proxy that translates between buf CLI protocols and multiple VCS backends (GitHub, BitBucket, local git) with a caching layer. The core finding is that **zero new dependencies are required** — Go 1.26 stdlib `log/slog` and the existing connect-go v1.19.x interceptor API provide everything needed for production-grade diagnostic logging.

The recommended approach is a five-phase rollout built on three architectural patterns: (1) a Connect RPC unary interceptor for v1alpha1 handlers, (2) context-based correlation ID propagation linking middleware, handlers, and providers, and (3) structured error logging at the handler level using `slog.Logger` already injected via DI. The HTTP middleware should be demoted from error logging to INFO-only timing/status, with all diagnostic detail pushed to handler-level logs where structured fields (owner, repo, commit) are available.

The key risks are: (1) inadvertently leaking sensitive data (GitHub tokens, BitBucket passwords) through error chains when adding handler-level logging, (2) double-logging errors at both middleware and handler levels creating log noise and alert fatigue, and (3) log volume explosion from debug-level per-file operations without proper level separation (TRACE vs DEBUG). These are all preventable with upfront redaction design, a clear logging-boundary decision, and structured log-level planning.

## Key Findings

### Recommended Stack

**Verdict: Zero new dependencies.** All diagnostic logging features are implementable with Go 1.26 stdlib `log/slog` and the existing connect-go v1.19.x interceptor API. The missing piece is not a library — it is a Connect RPC unary interceptor and contextual logger propagation via `slog.Logger.With()`.

**Core technologies:**
- `log/slog` (Go 1.26 stdlib): Structured logging with level gates, `ReplaceAttr` for redaction, `With()` for contextual loggers, `LogAttrs` for zero-alloc hot paths, `slog.LevelVar` for dynamic level control — all built in, no third-party library needed
- `connectrpc.com/connect v1.19.2` (existing): `Interceptor` interface with `WrapUnary` for request/response logging of all v1alpha1 handlers without modifying handler code
- `crypto/rand` (Go stdlib): Correlation ID generation when `X-Request-Id` header is absent — 8-byte hex string sufficient for single-server tracing

**Explicitly not recommended:** `rs/zerolog`, `uber-go/zap`, `sirupsen/logrus`, OpenTelemetry SDK, `connectrpc.com/otelconnect-go` — all are either redundant with slog's capabilities or overkill for a single-process proxy.

### Expected Features

The MVP (v1.3) focuses on making failures diagnosable. All P1 features require zero new dependencies.

**Must have (table stakes):**
- **Correlation ID propagation** — Generate or pass-through `X-Request-Id`, attach to `context.Context` in logging middleware, extract in all handlers and providers. Without this, logs from concurrent requests are indistinguishable.
- **Connect RPC interceptor logging** — Single unary interceptor covering all 3 v1alpha1 handlers (blobs, modulepins, bynames) with structured procedure, peer, duration, request_size, response_size, error code. Zero changes to handler code.
- **v1beta1 raw handler instrumentation** — Per-handler logging in commits.go ServeHTTP/ServeGraph/ServeDownload/ServeGetModules with full structured context (owner, module, commit).
- **Error-path structured logging** — Every `http.Error()` and `fmt.Errorf()` in handler code gets a matching `log.ErrorContext()` or `log.WarnContext()` with owner, repo, commit, error, request_id.
- **Provider call tracing (debug)** — Before/after logging for GitHub, BitBucket, Artifactory external API calls with timing and response status.
- **Sensitive data masking** — Extend existing header masking to new log paths. Add `slog.HandlerOptions.ReplaceAttr` for key-name-based redaction as defense-in-depth.

**Should have (v1.4):**
- **Runtime log level via signal handler** — `SIGUSR1` toggling between INFO and DEBUG using `slog.LevelVar`. Enables live debugging without restart.
- **Panic recovery middleware** — `recover()` wrapper around entire ServeMux logging stack trace and returning 500.
- **Request/response body hex dump** — At debug level only, truncated to 4KB, with opt-in flag/header.

**Defer (v2+):**
- Log sampling / rate limiting — Not needed until production load requires it.
- OpenTelemetry integration — Overkill for <10 spans per request; structured attributes achieve 90% of the value.
- Admin HTTP endpoint for log level — Signal handler is simpler and sufficient.
- Structured error codes — Add after operational experience reveals which error paths need dashboarding.

### Architecture Approach

The existing structure is sound. Diagnostic logging requires one new file, modifications to six existing files, and no new packages.

**Major components:**
1. **Connect RPC unary interceptor** (`internal/connect/interceptor.go`, NEW) — Wraps all v1alpha1 handlers to log procedure, peer, duration, request/response size, and error code. Generates or extracts correlation ID from request headers. One interceptor covers blobs, modulepins, bynames with zero handler changes.
2. **Correlation ID middleware** (`cmd/easyp/main.go`, MODIFY) — Extends existing `loggingMiddleware` to generate or pass-through `X-Request-Id`, attach to context via `context.WithValue` and `slog.WithContext`. Enables every downstream log line to carry a request ID without explicit parameter passing.
3. **Structured error logging at handler level** (`internal/connect/blobs.go`, `modulepins.go`, `bynames.go`, `commits.go`, MODIFY) — Every error path logs structured attributes (owner, repo, commit, error, request_id) before writing HTTP error response. The HTTP middleware is demoted to INFO-level timing/status only.

**Key architectural decision:** Log at the handler level for errors, not the middleware level. Handlers have access to structured fields (owner, repo, commit) that the HTTP middleware cannot see. This avoids the anti-pattern of "status=500" with no context.

### Critical Pitfalls

1. **Logging sensitive data through error chains** — Error messages wrapped through multiple layers (`fmt.Errorf("...: %w", err)`) can contain full HTTP URLs with credentials, API tokens, or internal paths. Prevention: audit every error path before adding logging, implement URL redaction at the provider layer, never log `err.Error()` as a raw string without checks. Structured attributes (owner, repo, commit) are safe; error strings are not.

2. **Double logging of errors (middleware vs. handler)** — Both the HTTP middleware (current, on 4xx/5xx) and new handler-level logging will fire for the same error, producing duplicate ERROR-level log lines. Prevention: demote middleware logging to INFO-level timing/status only. Push all diagnostic detail to handler-level logs. This is a deliberate architectural decision that must be made before writing any code.

3. **Protobuf binary body logging without truncation** — Bodies up to 50MB logged as raw protobuf bytes at debug level fill disks in minutes, break JSON log parsing (binary encoded as base64), and are unreadable. Prevention: never log raw protobuf bytes — log structured fields extracted during parsing instead. If body hex dump is absolutely needed, truncate to 1024 bytes, encode as hex, and gate behind both DEBUG level AND an explicit opt-in header.

4. **Missing request ID propagation** — Correlation ID captured in logging middleware but never stored in `context.Context`. When concurrent requests interleave (common in buf CLI), log entries are indistinguishable. Prevention: attach request ID to `r.Context()` in the middleware, use `slog.WithContext` so `slog.FromContext(ctx)` automatically picks it up in downstream code.

5. **Log volume explosion from debug-level tracing** — A single `buf mod update` can generate 2000+ debug log lines (per-file download, hash operations). Prevention: use TRACE level (below DEBUG) for per-file operations, DEBUG for per-RPC metadata, INFO for operational messages. Document expected log volume at each level so operators know what to expect before enabling debug in production.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Logging Foundation
**Rationale:** Must come first because all other phases depend on logger configuration, redaction, and env-var override being in place. This phase has no external dependencies.
**Delivers:** Enriched LogConfig (Format, AddSource), `EASYP_LOG_LEVEL` env var override, `slog.HandlerOptions.ReplaceAttr` for centralized redaction, `slog.LevelVar` for future dynamic control.
**Addresses:** Sensitive data masking (table stake, P1 from FEATURES.md)
**Avoids:** Pitfall 1 (sensitive data through error chains — redaction layer must exist before any new log calls), Pitfall 6 (credentials in startup validation — redaction catches startup log leaks)
**Stack elements:** Go 1.26 `log/slog`, `slog.HandlerOptions.ReplaceAttr`, `slog.LevelVar`
**Files modified:** `cmd/easyp/main.go` (newLogger), `cmd/easyp/internal/config/config.go` (LogConfig)

### Phase 2: Logging Infrastructure
**Rationale:** Correlation ID propagation and the Connect interceptor are the infrastructure that all subsequent handler and provider logging will use. Both must be in place before error-path logging is added to handlers, otherwise logs from concurrent requests will be indistinguishable.
**Delivers:** Correlation ID generation/passthrough in all requests, Connect RPC unary interceptor logging procedure/peer/duration/error for v1alpha1 handlers, context-based request ID propagation via `slog.WithContext`.
**Addresses:** Correlation ID propagation (table stake, P1), Connect RPC interceptor logging (P1)
**Avoids:** Pitfall 2 (double logging — by demoting middleware error logging to INFO), Pitfall 5 (missing request ID propagation — design it before writing handler logs), Pitfall 11 (dynamic log level — use `slog.LevelVar` from the start)
**Research flag:** Phase 3-4 depend on this infrastructure being correct.
**Files modified:** `cmd/easyp/main.go` (loggingMiddleware), `internal/connect/interceptor.go` (NEW), `internal/connect/api.go` (pass opts)

### Phase 3: Error Path Logging (Handlers)
**Rationale:** After infrastructure is in place, add structured error logging to all handler error paths. This is the highest-value phase for diagnosability — every 400/500 error will have full structured context in logs. The v1alpha1 and v1beta1 paths must be handled separately with consistent attribute schemas.
**Delivers:** Every `http.Error()` and `return nil, fmt.Errorf(...)` in handler code produces a structured log with owner, repo, commit, error, request_id, protocol. Consistent attribute schema across v1alpha1 (interceptor) and v1beta1/v1 (manual logging) paths.
**Addresses:** Error-path structured logging (P1), v1beta1 raw handler instrumentation (P1)
**Avoids:** Pitfall 4 (error context not captured at source — providers also get logging in Phase 4), Pitfall 9 (logging after context cancellation — add `ctx.Err()` check before ERROR-level logs), Pitfall 10 (inconsistent attributes — establish naming convention before writing log calls), Pitfall 13 (protocol mismatch — use `slog.String("protocol", "v1alpha1"/"v1beta1")` on all error logs)
**Files modified:** `internal/connect/blobs.go`, `modulepins.go`, `bynames.go`, `commits.go`
**Research flag:** None — standard error-logging patterns, well-documented.

### Phase 4: Provider Logging Enhancement
**Rationale:** Provider logging (GitHub API calls, BitBucket API calls, Artifactory cache, local git) gives the diagnostic depth needed when errors originate outside the proxy. This phase depends on Phase 2 (correlation ID propagation) so provider log lines carry request context.
**Delivers:** Before/after debug logging for all external API calls with redacted URL, method, response status, duration. Cache hit/miss logging with duration. Context cancellation distinction in error logs. Provider type attribute on every log line.
**Addresses:** Provider call tracing at debug level (P1)
**Avoids:** Pitfall 1 (sensitive data — redact URLs at provider layer before they enter error chains), Pitfall 4 (error context at source — log at provider boundary, not just handler), Pitfall 9 (context cancellation — distinguish client disconnect from API error)
**Research flag:** BitBucket and localgit providers may need deeper developer attention during implementation, as they have zero logging currently and their provider interfaces differ. Skippable if Phase 1 redaction and Phase 2 infrastructure are solid.
**Files modified:** `internal/providers/github/getrepo.go`, `internal/providers/github/getfiles.go`, `internal/providers/bitbucket/*.go`, `internal/providers/localgit/*.go`, `internal/providers/multisource/repo.go`, `internal/providers/cache/artifactory/artifactory.go`

### Phase 5: Advanced Diagnostics (v1.4)
**Rationale:** Panic recovery, dynamic log level, and body hex dump are valuable but not required for MVP. They depend on the logging infrastructure (Phase 2) being in place. Panic recovery is high-value for production stability and should be prioritized higher within this phase.
**Delivers:** Panic recovery middleware logging full stack trace and returning 500, `SIGUSR1` signal handler toggling log level between INFO and DEBUG, request/response body hex dump at debug level with truncation and opt-in header.
**Addresses:** Runtime log level (P2), Panic recovery (P2), Body hex dump (P3)
**Avoids:** Pitfall 3 (body logging without truncation — enforce truncation from day one if body logging is implemented), Pitfall 7 (panic in logging code — add `recover()` in middleware), Pitfall 12 (flaky test assertions — design test logging infrastructure before writing tests)
**Research flag:** Body hex dump needs careful design for protobuf binary encoding. Can skip `--research-phase` if using simple hex encoding of first 1024 bytes. Dynamic log level via signal handler is a well-documented pattern (`slog.LevelVar` + `os.Signal.Notify`).

### Phase Ordering Rationale

- **Phase 1 before Phase 2:** Redaction and env-var override must be in place before any new log lines are written, otherwise sensitive data could leak from the very first new log call.
- **Phase 2 before Phase 3:** Correlation ID propagation is required for any multi-step request diagnostic. Without it, handler-level logs from concurrent requests are indistinguishable.
- **Phase 3 and Phase 4 are parallelizable:** Error-path logging at the handler level and provider logging enhancement can be done simultaneously by different developers, as they modify different files. However, both depend on Phase 2 infrastructure.
- **Phase 5 last:** Dynamic log level is nice-to-have but not required for MVP. Panic recovery is higher value and should be prioritized if the team has capacity in v1.3.
- **Phase 3 is the highest-ROI phase:** It addresses the core diagnosability gap — 400/500 errors will have full structured context in logs. This alone justifies the milestone.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 4 (Provider Logging):** BitBucket and localgit providers have zero logging currently and their provider interfaces differ from GitHub. The developer implementing this phase will need to understand each provider's internal structure. This is low-risk but may need `--research-phase` if the team is unfamiliar with these providers.
- **Phase 5 (Body Hex Dump):** Protobuf binary encoding into logs requires careful design for readability and performance. Simple hex encoding of first 1024 bytes is well-understood, but if the team wants structured protobuf deserialization for debug logs, that needs more research.

Phases with standard patterns (skip research-phase):
- **Phase 1 (Logging Foundation):** Well-documented Go patterns (`slog.HandlerOptions`, `ReplaceAttr`, `slog.LevelVar`). No research needed.
- **Phase 2 (Logging Infrastructure):** Connect interceptor pattern is documented in connect-go v1.19.x via Context7 docs. Correlation ID via context is a Go standard pattern.
- **Phase 3 (Error Path Logging):** Mechanical changes to existing handler code. Patterns are established in `multisource/repo.go` which already demonstrates the correct approach.
- **Phase 5 (Panic Recovery, Dynamic Log Level):** Both are well-documented Go patterns. `recover()` middleware is a web server standard. `slog.LevelVar` + `os.Signal.Notify` is documented in Go stdlib.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All recommendations verified against Go 1.26 stdlib docs, connect-go v1.19.x Context7 docs, and existing codebase patterns. Zero new dependencies required. |
| Features | HIGH | Derived from direct codebase analysis and buf CLI reference implementation. Feature dependencies and MVP boundary are clear. |
| Architecture | HIGH | Verified against all source files in the codebase. The three architectural patterns (interceptor, correlation ID, handler-level error logging) are well-documented and appropriate for the dual-protocol architecture. |
| Pitfalls | HIGH | Based on direct codebase analysis (all provider error chains audited), official Go/slog documentation, and established structured logging patterns. Recovery strategies and prevention approaches are concrete. |

**Overall confidence:** HIGH

### Gaps to Address

- **Provider-specific error chain audit:** While Pitfall 1 identifies the general risk of sensitive data in error chains, a full audit of every provider's error wrapping (BitBucket `client.go` URL inclusion, Artifactory cache URL exposure) will be needed during Phase 4 implementation. This is developer diligence, not a research gap.
- **Log attribute naming convention:** Pitfall 10 identifies the risk of inconsistent attribute names. The standard names (`request_id`, `owner`, `repo`, `commit`, `procedure`, `protocol`, `provider_type`, `cache_hit`, `duration_ms`) should be documented as project-wide constants before Phase 3 implementation. This is a design decision, not a research gap.
- **Body logging format for protobuf:** If body logging is implemented in Phase 5, the exact format (hex vs base64 vs structured field dump) needs a design decision. Simple hex encoding of first 1024 bytes is recommended to avoid protobuf decoding complexity.

## Sources

### Primary (HIGH confidence)
- **Go 1.26 stdlib `log/slog`** — Official Go documentation for `Enabled()`, `LogAttrs()`, `HandlerOptions.ReplaceAttr`, `Logger.With()`, `slog.FuncAttr`, `slog.LevelVar`, `slog.WithContext`, `slog.FromContext`
- **connect-go v1.19.x** — Official library API for `Interceptor` interface, `WrapUnary`, `connect.WithInterceptors`, `connect.UnaryInterceptorFunc`
- **Existing codebase** — Direct analysis of all Go source files in `cmd/`, `internal/connect/`, `internal/providers/`, `internal/https/`
- **buf CLI reference** — buf's own `NewDebugLoggingInterceptor` pattern in `buf/private/bufpkg/bufconnect/interceptors.go`

### Secondary (MEDIUM confidence)
- **Connect RPC documentation (connectrpc.com/docs)** — Interceptor patterns and handler options (cross-referenced against Context7 library docs)
- **Dave Cheney, "Let's Talk About Logging"** — Structured logging best practices (widely cited in Go community)
- **Go issue #59369** — `slog` performance characteristics and allocation patterns
- **OWASP Log Injection Cheat Sheet** — Log injection attack patterns
- **Protobuf wire format documentation (protobuf.dev)** — Binary encoding format

---
*Research completed: 2026-06-16*
*Ready for roadmap: yes*
