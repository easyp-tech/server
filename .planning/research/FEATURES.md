# Feature Research: Diagnostic Logging for Connect RPC Proxy

**Domain:** Diagnostic logging for a Go-based Buf registry proxy server
**Researched:** 2026-06-16
**Confidence:** HIGH

## Current State Assessment

The codebase already has:
- A single `loggingMiddleware` in `main.go` that wraps the entire `http.ServeMux` and logs request method, path, duration, status code, and client IP
- Sensitive header masking (Authorization, Cookie, X-Api-Key, Token) for debug-level request header logging
- `slog` with JSON handler, level configurable via config file `log.level` field
- Scattered `log.Debug` calls in `multisource/repo.go` (cache hit/miss, repo lookup)
- Startup health checks logged (cache access, repository connections)

**Critical gaps** that this milestone must address:
- No correlation ID propagation from HTTP middleware into handler-level logs
- Connect RPC handlers (`modulepins.go`, `blobs.go`, `bynames.go`) have ZERO logging
- v1beta1 raw protobuf handlers (`commits.go`) have ZERO logging
- Error paths write HTTP error messages but never log server-side diagnostics
- No request/response body logging at debug level
- Log level is static (set at startup, no runtime adjustment)
- No panic recovery middleware
- No per-endpoint metrics or tracing

## Feature Landscape

### Table Stakes (Users Expect These)

Features that any production-grade diagnostic logging system must have. Missing these means the system cannot be diagnosed without source code access.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Correlation ID propagation** | Must trace a single request across middleware, handlers, and providers. Without this, logs from different components cannot be linked to the same request. | LOW | Add `requestID` to `context.Context` in middleware; extract it in handlers/providers. Requires adding a context helper package or key type. |
| **Error-path structured logging** | When a handler returns 400/500, the server-side log must contain the full diagnostic context (owner, repo, commit, error details) -- not just "request completed 500". | LOW | Every `http.Error()` call and every `return nil, fmt.Errorf(...)` in connect handlers needs a matching `log.ErrorContext()` or `log.WarnContext()` call. |
| **Request-level duration tracking** | Must know how long each handler took, broken down by component (VCS API call, digest computation, cache lookup). | MEDIUM | Current middleware tracks total duration only. Need per-handler timing or finer-grained spans. |
| **Sensitive data preservation in enhanced logging** | Existing header masking must not be bypassed by new debug-level body logging. Must be maintained or extended. | LOW | Already implemented for headers. Body masking is more nuanced (protobuf binary is opaque; no masking needed for binary, but text fields in errors need care). |
| **Runtime-configurable log level** | Must be able to increase log level to debug for a specific request or period without restarting the server. | MEDIUM | Options: signal handler (SIGUSR1), HTTP admin endpoint, or environment variable override. Signal handler is simplest for proxy environment. |

### Differentiators (Competitive Advantage)

Features that go beyond basic diagnostics and make the system significantly more operable.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Connect RPC interceptor for structured handler logging** | All connect-go handlers get automatic logging of procedure, peer, request size, response size, duration, and error code -- without modifying each handler. | LOW | Mirror buf's own `NewDebugLoggingInterceptor`. Can be applied to existing 3 connect handlers (Resolve, Repository, Download services). |
| **v1beta1 raw handler instrumentation** | The manual protobuf handlers (CommitService, GraphService, DownloadService, ModuleService) get the same diagnostic coverage as the connect handlers. | MEDIUM | These are `http.HandlerFunc` -- need per-handler wrappers or a second middleware layer within the mux. |
| **Request/response body hex dump at debug level** | For debugging protobuf encoding issues, being able to see the raw payload bytes (truncated) is invaluable. | LOW | At LevelDebug, log first N bytes of request body and response body as hex. Must be opt-in and clearly flagged as diagnostic. |
| **Panic recovery with full stack trace** | A panic in a handler should produce a structured log with stack trace and return 500, not crash the server. | LOW | `recover()` middleware wrapping all handlers. Use `debug.Stack()` for trace. |
| **Provider call tracing** | Each external API call (GitHub, BitBucket, Artifactory) should be logged with timing, URL, and response status at debug level. | MEDIUM | Currently provider logging is sparse. Add slog.Debug before/after external HTTP calls in `github/getrepo.go`, `github/getfiles.go`, `bitbucket/getrepo.go`, `artifactory/artifactory.go`. |
| **Log sampling / rate limiting** | At high-traffic production, debug logging every request could be overwhelming. A sampling mechanism limits noise. | MEDIUM | Could start simple: only enable debug for requests with a specific header (e.g., `X-Debug-Log: true`). More complex: hash-based sampling ratio. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem useful but create problems if implemented.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Log full response body at INFO level** | "I want to see what the server returned without enabling debug" | Response bodies can be large (megabytes of proto content in bulk download). Logging at INFO floods storage and slows the server. | Log response body only at DEBUG level, truncated to first 4KB, with an explicit `body_truncated` flag. |
| **Sensitive data in error logs** | "The error message says 'token rejected', but I need to see which token" | Tokens, passwords, and auth headers must NEVER appear in logs. | Never log raw tokens. Log a token hash prefix or the owner/repo name that was associated with the token. Already handled for headers -- extend same rule to body logging. |
| **Per-request full protobuf deserialization into logs** | "I want to see the decoded protobuf fields in the log" | Protobuf binary -> JSON conversion is expensive and adds allocation pressure on every request, even when not logged. | Use protobuf's `proto.Size()` for request size (already computed, no deserialization). Only do full deserialization if a dedicated debug-on-demand mode is enabled. |
| **Distributed tracing integration (OpenTelemetry)** | "We should have proper trace propagation" | Adds significant dependency (opentelemetry-go SDK), configuration burden, and infrastructure (collector, backend). Overkill for a stateless proxy with <10 spans per request. | Correlation ID + structured log attributes achieve 90% of the debugging value with zero new dependencies. |

## Feature Dependencies

```
Correlation ID propagation
    ├──requires──> Context key type + context helper package
    │
    ├──enables──> Error-path structured logging (can reference requestID)
    ├──enables──> Connect RPC interceptor logging (can include requestID)
    ├──enables──> Provider call tracing (can include requestID)
    └──enables──> Request/response body logging (can include requestID)

Error-path structured logging
    └──requires──> slog.Logger accessible in all handler contexts (already satisfied)

Runtime-configurable log level
    ├──option──> Signal handler (SIGUSR1) -- simplest, no deps
    └──option──> Admin HTTP endpoint -- needs separate listener for security

Panic recovery middleware
    └──requires──> Wrapping the entire ServeMux (handle order: panic recovery outermost, then logging, then mux)

Log sampling / rate limiting
    └──enhances──> Runtime-configurable log level (header-based debug opt-in)
```

### Dependency Notes

- **Correlation ID propagation must come first** -- all other logging features benefit from having a request ID in context. Without it, logs from different components cannot be correlated.
- **Error-path structured logging can be done incrementally** -- each handler can be instrumented independently, starting with most-failing paths first (Download, Graph are the most complex).
- **Connect RPC interceptor is independent from v1beta1 raw handler instrumentation** -- the 3 connect handlers get logging via interceptors, the 4 v1beta1 handler functions need manual wrapping.
- **Request body logging must be behind a separate guard** -- even at debug level, body logging should check `log.Enabled(ctx, LevelDebug)` AND an explicit body-logging flag or header, because body content can be large.
- **Runtime log level does not need to be dynamic for MVP** -- static level at startup is acceptable for initial release; runtime reload can be added later.
- **Log sampling should be deferred** -- not needed until production load requires it. Start with header-based selective debug.

## MVP Definition

### Launch With (v1.3)

The minimum needed to make 400 errors and failures diagnosable without source code access.

- [x] **Correlation ID propagation** -- Add `requestID` to `context.Context` in `loggingMiddleware`, extract it in all handlers and providers. This is the foundational feature.
- [x] **Connect RPC interceptor for structured logging** -- Add a unary interceptor to the 3 connect-go handlers that logs procedure, duration, request_size, response_size, and error code. Mirror buf's own `NewDebugLoggingInterceptor`.
- [x] **v1beta1 raw handler instrumentation** -- Add per-handler logging to `commits.go` functions (ServeHTTP/GetCommits, ServeGraph, ServeDownload, ServeGetModules). Log owner, module, commit, error, and duration.
- [x] **Error-path structured logging** -- Every `http.Error()` and every `return nil, fmt.Errorf(...)` in handler code must have an accompanying `log.ErrorContext(...)` or `log.WarnContext(...)` with structured fields (owner, repo, commit, error, requestID).
- [x] **Provider call tracing at debug level** -- Add `log.Debug` before/after provider calls (GetMeta, GetFiles, Get/Put from cache) with timing and result info, tagged with requestID.
- [x] **Sensitive data masking for enhanced logging** -- Extend existing header masking to cover any new log paths. For protobuf body dumps, mask text that contains tokens (but protobuf binary is opaque so this is mostly about error string sanitization).

### Add After Validation (v1.4)

Features to add once the core diagnostic logging is proven.

- [ ] **Runtime log level via signal handler** -- Add `SIGUSR1` handler that toggles between INFO and DEBUG, or an admin HTTP endpoint. Enables live debugging without restart.
- [ ] **Panic recovery middleware** -- Wrap the ServeMux in a panic-recovery handler that logs stack trace and returns 500. Low risk, high value for stability.
- [ ] **Request/response body hex dump at debug level** -- Log first 4KB of request body and response body as hex when debug level is enabled AND an opt-in flag/header is present.

### Future Consideration (v2+)

Features to defer until production operational experience proves they're needed.

- [ ] **Log sampling / rate limiting** -- Hash-based sampling or header-based selective debug for production environments with high request volume.
- [ ] **OpenTelemetry integration** -- If the organization adopts distributed tracing, replace correlation IDs with trace/span context. Not needed for milestone v1.3.
- [ ] **Admin HTTP endpoint for log level** -- Separate listener on internal port for live log level adjustment. Signal handler is simpler and sufficient.
- [ ] **Structured error codes in logs** -- Map each error path to a unique error code string (e.g., "CACHE_GET_FAILED", "GIT_REPO_NOT_FOUND") for easier dashboarding.

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority | Phase |
|---------|------------|---------------------|----------|-------|
| Correlation ID propagation | HIGH | LOW | P1 | v1.3 |
| Connect RPC interceptor logging | HIGH | LOW | P1 | v1.3 |
| v1beta1 handler instrumentation | HIGH | MEDIUM | P1 | v1.3 |
| Error-path structured logging | HIGH | LOW | P1 | v1.3 |
| Provider call tracing (debug) | MEDIUM | LOW | P1 | v1.3 |
| Sensitive data masking for logs | HIGH | LOW | P1 | v1.3 |
| Runtime log level via signal | MEDIUM | LOW | P2 | v1.4 |
| Panic recovery middleware | MEDIUM | LOW | P2 | v1.4 |
| Request/response body hex dump | LOW | LOW | P3 | v1.4 |
| Log sampling / rate limiting | LOW | MEDIUM | P3 | v2+ |
| OpenTelemetry integration | LOW | HIGH | P3 | v2+ |
| Admin HTTP endpoint | LOW | MEDIUM | P3 | v2+ |

**Priority key:**
- P1: Must have for v1.3 diagnostic logging milestone -- without these, 400 errors are not diagnosable.
- P2: Should have, add in v1.4 when gaps are identified from operational use.
- P3: Nice to have, production experience may never require these.

## Implementation Notes

### Detailed Feature Breakdown

#### 1. Correlation ID Propagation (P1)

**Current state:** `loggingMiddleware` reads `X-Request-Id` from request headers and sets it on the response, but never stores it in `context.Context`.

**Implementation:**
```go
// In a new package: internal/ctxkeys or similar
package ctxkeys

type contextKey struct{ name string }

var RequestID = &contextKey{name: "requestID"}

func WithRequestID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, RequestID, id)
}

func RequestIDFrom(ctx context.Context) string {
    if id, ok := ctx.Value(RequestID).(string); ok {
        return id
    }
    return ""
}
```

**Touch points:**
- `loggingMiddleware` -- `r = r.WithContext(ctxkeys.WithRequestID(r.Context(), requestID))`
- `multisource/repo.go` -- `slog.String("request_id", ctxkeys.RequestIDFrom(ctx))` in every log call
- `connect/api.go` -- pass requestID context (handlers already receive `r.Context()`)
- `github/client.go`, `bitbucket/client.go` -- add requestID to existing debug logs
- `artifactory/artifactory.go` -- add requestID to debug/error logs

#### 2. Connect RPC Interceptor (P1)

**Implementation:**

```go
// internal/connect/logging.go or similar
func loggingInterceptor(log *slog.Logger) connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
            start := time.Now()
            resp, err := next(ctx, req)
            duration := time.Since(start)

            attrs := []slog.Attr{
                slog.String("procedure", req.Spec().Procedure),
                slog.Duration("duration", duration),
                slog.String("peer", req.Peer().Addr),
                slog.Int("request_size", proto.Size(req.Any())),
            }
            if err != nil {
                attrs = append(attrs, slog.String("error", err.Error()))
                attrs = append(attrs, slog.String("code", connect.CodeOf(err).String()))
            }
            if resp != nil && resp.Any() != nil {
                attrs = append(attrs, slog.Int("response_size", proto.Size(resp.Any())))
            }

            log.LogAttrs(ctx, slog.LevelDebug, "rpc completed", attrs...)
            return resp, err
        }
    }
}
```

**Applied in `api.go` `New()` function:**
```go
interceptor := loggingInterceptor(log)
mux.Handle(connect.NewResolveServiceHandler(a, connect.WithInterceptors(interceptor)))
mux.Handle(connect.NewRepositoryServiceHandler(a, connect.WithInterceptors(interceptor)))
mux.Handle(connect.NewDownloadServiceHandler(a, connect.WithInterceptors(interceptor)))
```

#### 3. v1beta1 Raw Handler Instrumentation (P1)

For each of the 4 handler entry points in `commitServiceHandler`, add logging before the handler logic and in error paths:

- `ServeHTTP` (GetCommits): Log the parsed refs (owner/module), results, and any errors.
- `ServeGraph`: Log the parsed refs, cache hit vs miss, digest computation, and errors.
- `ServeDownload`: Log commit ID lookup, cache hit vs miss, files fetched, and errors.
- `ServeGetModules`: Log module keys resolved and errors.

The handler already has access to `h.api.log`. Key pattern for each:

```go
refs := parseResourceRefs(body)
h.api.log.DebugContext(r.Context(), "GetCommits request",
    slog.Int("refs", len(refs)),
)
```

Error paths:
```go
h.api.log.ErrorContext(r.Context(), "GetCommits failed",
    slog.String("owner", ref.owner),
    slog.String("module", ref.module),
    slog.String("error", err.Error()),
)
http.Error(w, fmt.Sprintf("resolving %s/%s: %v", ref.owner, ref.module, err), http.StatusInternalServerError)
```

#### 4. Error-Path Structured Logging (P1)

**Touch points (exhaustive):**

| File | Function | Error Path |
|------|----------|------------|
| `connect/commits.go` | ServeHTTP | `GetMeta` error, `computeB4Digest` error, `toB5Digest` error |
| `connect/commits.go` | ServeGraph | `GetMeta` error (direct and fallback), `computeB4Digest` error, `toB5Digest` error |
| `connect/commits.go` | ServeDownload | `GetMeta` error, `GetFiles` error |
| `connect/commits.go` | ServeGetModules | All paths already return 400 |
| `connect/modulepins.go` | resolveModulePin | `GetMeta` error |
| `connect/bynames.go` | resolveRepoByFullName | `splitRepoName` validation, `GetMeta` error |
| `connect/blobs.go` | DownloadManifestAndBlobs | `GetFiles` error, `shake256` error |
| `multisource/repo.go` | GetMeta | `findSource` returning nil |
| `multisource/repo.go` | GetFiles | `findSource` returning nil, `GetFiles` error |
| `providers/github/getrepo.go` | getRepo | API errors, empty default branch |
| `providers/github/getfiles.go` | GetFiles | API errors |
| `providers/github/getfiles.go` | getFile | Download errors |
| `providers/bitbucket/getrepo.go` | (similar) | API errors |
| `providers/bitbucket/getfiles.go` | (similar) | API errors |
| `providers/cache/artifactory/artifactory.go` | Get/Put | HTTP errors, body errors |

**Pattern:**
```go
if err != nil {
    log.ErrorContext(ctx, "github getMeta failed",
        slog.String("owner", owner),
        slog.String("repo", repoName),
        slog.String("error", err.Error()),
    )
    return out, fmt.Errorf("resolving default branch: %w", err)
}
```

#### 5. Provider Call Tracing at Debug Level (P1)

Already partially done in `multisource/repo.go`. Extend to:
- `github/getrepo.go` `getRepo` -- log before/after `c.repos.Get()` and `c.repos.GetBranch()` calls
- `github/getfiles.go` `GetFiles` -- log number of entries found, time to download
- `bitbucket/getrepo.go`, `bitbucket/getfiles.go` -- same
- `artifactory/artifactory.go` `Get`/`Put` -- log URL, timing, status

#### 6. Runtime Log Level via Signal Handler (P2)

```go
var logLevel slog.LevelVar
logLevel.Set(slog.LevelInfo)
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &logLevel})
logger := slog.New(handler)

signal.Notify(c, syscall.SIGUSR1)
go func() {
    for range c {
        if logLevel.Level() == slog.LevelDebug {
            logLevel.Set(slog.LevelInfo)
        } else {
            logLevel.Set(slog.LevelDebug)
        }
        logger.Info("log level changed", "level", logLevel.Level())
    }
}()
```

#### 7. Panic Recovery Middleware (P2)

```go
func panicRecoveryMiddleware(log *slog.Logger, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                log.ErrorContext(r.Context(), "handler panic",
                    slog.Any("panic", rec),
                    slog.String("stack", string(debug.Stack())),
                )
                http.Error(w, "internal server error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

Order in `main.go`:
```go
handler := connect.New(log, storage, cfg.Domain)
handler = panicRecoveryMiddleware(log, handler)     // outermost
handler = loggingMiddleware(log, handler)            // middle
```

## Sources

- **buf CLI's own `NewDebugLoggingInterceptor`** (in submodule): Used as reference pattern for connect-go handler-level logging. Source: `api/_third_party/buf/private/bufpkg/bufconnect/interceptors.go:148`.
- **Existing codebase analysis**: All `.go` files in `cmd/`, `internal/connect/`, `internal/providers/`, `internal/https/`.
- **Current logging middleware**: `cmd/easyp/main.go:150-206` -- demonstrates existing request ID, duration, status logging pattern.
- **connect-go v1.19.2**: API for `connect.WithInterceptors()` available in the connect-go library already in `go.mod`.
- **Go 1.26 `log/slog`**: `slog.LevelVar`, `slog.HandlerOptions.Level`, `log.LogAttrs` are all available in stdlib (Go 1.21+).

---
*Feature research for: diagnostic logging for Connect RPC proxy*
*Researched: 2026-06-16*
