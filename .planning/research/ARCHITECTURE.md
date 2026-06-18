# Architecture Research: Diagnostic Logging for Connect RPC Server

**Domain:** Diagnostic logging integration for existing Go-based Connect RPC proxy server
**Researched:** 2026-06-16
**Confidence:** HIGH — all findings verified against existing codebase

## Standard Architecture

### System Overview — Current Drift

The existing file described the protocol modernization architecture. Since then, the project has moved to v1beta1/v1 dual-protocol via raw protobuf handlers and a shared `http.ServeMux`. The current architecture as of v1.3 work is:

```
buf CLI (v1.30.1)          buf CLI (v1.69.0+)
       |                          |
       v                          v
┌───────────────────────────────────────────────────┐
│         HTTP Entry Point (cmd/easyp/main.go)       │
│                                                    │
│  ListenAndServe(cfg.Listen, loggingMiddleware(mux))│
│  -OR- https.ListenAndServe(...) for TLS/mTLS       │
└───────────────────────┬───────────────────────────┘
                        │
                        v
┌───────────────────────────────────────────────────┐
│            loggingMiddleware                       │
│                                                    │
│  Request: X-Request-Id passthrough                 │
│           clientIP extraction                      │
│  DEBUG:  masked headers + method/path              │
│  WARN:   status >= 400                             │
│  ERROR:  status >= 500                             │
│                                                    │
│  NO body logging · NO correlation ID gen           │
└───────────────────────┬───────────────────────────┘
                        │
                        v
┌───────────────────────────────────────────────────┐
│           http.ServeMux (connect.New)              │
│                                                    │
│  /buf.alpha.registry.v1alpha1.ResolveService/      │
│  /buf.alpha.registry.v1alpha1.DownloadService/     │
│  /buf.alpha.registry.v1alpha1.RepositoryService/   │
│                                                    │
│  /buf.registry.module.v1.CommitService/            │
│  /buf.registry.module.v1beta1.CommitService/       │
│  /buf.registry.module.v1.GraphService/             │
│  /buf.registry.module.v1beta1.GraphService/        │
│  /buf.registry.module.v1.DownloadService/          │
│  /buf.registry.module.v1beta1.DownloadService/     │
│  /buf.registry.module.v1.ModuleService/            │
│  /buf.registry.module.v1beta1.ModuleService/       │
│  / (root health check)                              │
└───────────┬───────────────────────┬───────────────┘
            │                       │
     Connect RPC handlers     Raw protobuf handlers
     (blobs, modulepins,      (commits.go raw proto
      bynames)                wire-format parsing)
            │                       │
            └───────┬───────────────┘
                    │
                    v
┌───────────────────────────────────────────────────┐
│           multisource.Repo (providers layer)        │
│                                                    │
│  GetMeta(ctx, owner, repo, commit) {               │
│    log.Debug("looking for meta", ...)              │
│    cacheGet() or Source.GetMeta()                  │
│  }                                                  │
│  GetFiles(ctx, owner, repo, commit) {              │
│    cacheGet() or Source.GetFiles() → cachePut()    │
│  }                                                  │
└───────────────────────┬───────────────────────────┘
                        │
                  ┌─────┴─────┐
                  │           │
            github*     bitbucket*    localgit
            (slog    )  (no logs)   (no logs)
                  │           │
                  └────┬──────┘
                       │
                       v
┌───────────────────────────────────────────────────┐
│              Cache Layer                            │
│  ┌────────┐  ┌────────────┐  ┌──────────────┐     │
│  │  Noop  │  │Local (FS)  │  │ Artifactory   │     │
│  │        │  │(no logs)   │  │(no app logs)  │     │
│  └────────┘  └────────────┘  └──────────────┘     │
└───────────────────────────────────────────────────┘
```

### Component Responsibilities — Current State

| Component | Responsibility | Current Logging | Gap |
|-----------|----------------|-----------------|-----|
| `cmd/easyp/main.go` | Bootstrap, config, logger creation, HTTP middleware | Logger from `cfg.Log.Level`; DEBUG/WARN/ERROR at HTTP level | No correlation ID generation; no body logging |
| `internal/connect/api.go` | Mux registration, handler struct | Stores `*slog.Logger` but never uses it directly | Logger is dead weight in `api` struct |
| `internal/connect/blobs.go` | v1alpha1 DownloadManifestAndBlobs RPC | Zero logging | Errors returned to Connect RPC framework only |
| `internal/connect/modulepins.go` | v1alpha1 GetModulePins RPC | Zero logging | Same — errors wrapped and returned |
| `internal/connect/bynames.go` | v1alpha1 Repository lookup RPCs | Zero logging | Same |
| `internal/connect/commits.go` | v1/v1beta1 raw proto handlers | Zero logging | Errors written to `http.Error()` only |
| `internal/providers/multisource/repo.go` | Provider routing + cache orchestration | DEBUG at entry/exit; ERROR on cache failures | No correlation ID in log context |
| `internal/providers/github/*.go` | GitHub VCS API | DEBUG: "found repo", "found branch" | Sparse; no duration tracking |
| `internal/providers/bitbucket/*.go` | BitBucket VCS API | No logs at all | Logger stored but unused |
| `internal/providers/localgit/*.go` | Local git access | No logs at all | No logger stored at all |
| `internal/providers/cache/artifactory/*.go` | Artifactory cache | No application logs | Logger stored but unused |
| `internal/providers/cache/file.go` / `noop.go` | Local FS / noop cache | No logs | Expected for noop; file could log errors |
| `internal/https/https.go` | TLS server creation | No logs | Expected — caller handles errors |

## Recommended Project Structure — Additions for Diagnostic Logging

The existing structure is sound. Diagnostic logging does not require new directories — it requires targeted additions to existing files and one new file for the interceptor.

```
internal/
├── connect/
│   ├── api.go                  # [MODIFY] Change New() to accept connect.Option
│   ├── blobs.go                # [MODIFY] Add structured error logging before return
│   ├── modulepins.go           # [MODIFY] Add structured error logging before return
│   ├── bynames.go              # [MODIFY] Add structured error logging before return
│   ├── commits.go              # [MODIFY] Add structured error logging before error responses
│   ├── commits_helpers.go      # NO CHANGE
│   └── interceptor.go          # [NEW] Connect unary interceptor for request/response logging
│
├── providers/
│   └── multisource/
│       └── repo.go             # [MODIFY] Pass context from request through to provider calls
│
cmd/easyp/
├── main.go                     # [MODIFY] Wire interceptor; generate correlation ID
└── internal/config/
    └── config.go               # [MODIFY] Extend LogConfig with format and add_source
```

### Structure Rationale

- **`internal/connect/interceptor.go`** (new): Houses Connect RPC unary interceptors for logging. This keeps interceptor logic separate from handler business logic. One interceptor covers all v1alpha1 Connect RPC handlers (blobs, modulepins, bynames) without modifying any of them.

- **`internal/connect/commits.go`** (modification): The raw proto handlers do NOT go through Connect's interceptor framework. They handle HTTP directly. Each error path needs explicit logging added before the `http.Error(w, msg, code)` call. This is a manual, site-by-site change.

- **`internal/connect/api.go`** (modification): `New()` needs an additional parameter or option to accept interceptors. Currently it constructs `connect.New*ServiceHandler(a)` without any `connect.WithInterceptors(...)` option.

- **`cmd/easyp/internal/config/config.go`** (modification): `LogConfig` needs fields for output format (json/text) and optional source location. The existing single-field `Level` config is sufficient for level control.

## Architectural Patterns

### Pattern 1: Connect RPC Unary Interceptor for Request/Response Logging

**What:** A `connect.UnaryInterceptorFunc` that wraps every Connect RPC call to log the incoming request metadata (procedure, peer, headers at DEBUG) and outgoing response (duration, error details at WARN/ERROR).

**When to use:** Applied at handler creation time in `api.go`. Covers all v1alpha1 Connect RPC handlers (blobs, modulepins, bynames) with zero changes to their code.

**Trade-offs:**
- Pro: Zero-code injection into existing handlers — one interceptor covers all
- Pro: Provides consistent structured attributes (procedure, duration, peer, request_id) on every RPC call
- Pro: Can extract `connect.Code` and `connect.Message` from errors for structured error logging
- Con: Does NOT cover the raw v1/v1beta1 handlers in `commits.go` (those bypass Connect RPC entirely)
- Con: Cannot log the request message body contents (Connect RPC deserializes internally before the interceptor runs)
- Con: Performance overhead at DEBUG level — must check `log.Enabled(ctx, slog.LevelDebug)` before allocating

**Additional consideration for request body logging:** To log request body contents for v1alpha1 handlers, the interceptor would need a custom stream interceptor that reads and replaces the request body. This adds complexity and body-size overhead. For v1.3, it is sufficient to log metadata and error details without full body logging for Connect handlers. The raw handlers in commits.go already have the body available via `io.ReadAll` and can log it at DEBUG level.

**Example:**

```go
// internal/connect/interceptor.go
package connect

import (
    "context"
    "errors"
    "log/slog"
    "net/http"
    "time"

    "connectrpc.com/connect"
)

type contextKey string

const reqIDKey contextKey = "request_id"

func NewLoggingInterceptor(log *slog.Logger) connect.Option {
    return connect.WithInterceptors(&loggingInterceptor{log: log})
}

type loggingInterceptor struct {
    log *slog.Logger
}

func (i *loggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
    return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
        start := time.Now()
        procedure := req.Spec().Procedure
        reqID := getOrCreateRequestID(req.Header())

        // Attach request_id context for downstream propagation
        ctx = context.WithValue(ctx, reqIDKey, reqID)
        logger := i.log.With(
            slog.String("procedure", procedure),
            slog.String("request_id", reqID),
        )

        if logger.Enabled(ctx, slog.LevelDebug) {
            logger.DebugContext(ctx, "rpc request start",
                slog.Any("peer", req.Peer()),
            )
        }

        resp, err := next(ctx, req)

        duration := time.Since(start)
        if err != nil {
            var connectErr *connect.Error
            if errors.As(err, &connectErr) {
                logger.WarnContext(ctx, "rpc error",
                    slog.String("code", connectErr.Code().String()),
                    slog.String("message", connectErr.Message()),
                    slog.Duration("duration", duration),
                )
            } else {
                logger.ErrorContext(ctx, "rpc internal error",
                    slog.String("error", err.Error()),
                    slog.Duration("duration", duration),
                )
            }
        } else if logger.Enabled(ctx, slog.LevelDebug) {
            logger.DebugContext(ctx, "rpc response",
                slog.Duration("duration", duration),
            )
        }

        return resp, err
    })
}

// WrapStream is required by connect.Interceptor but not used for unary-only.
// Keeping minimal implementation.
func (i *loggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
    return next
}
func (i *loggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
    return next
}

func getOrCreateRequestID(headers http.Header) string {
    if id := headers.Get("X-Request-Id"); id != "" {
        return id
    }
    // 8-character hex ID is sufficient for tracing within a single server
    return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
}
```

**Wiring change in api.go:**

```go
func New(log *slog.Logger, core provider, domain string, opts ...connect.Option) *http.ServeMux {
    a := &api{log: log, repo: core, domain: domain}
    mux := http.NewServeMux()

    // Pass opts (including interceptor) to all handler constructors
    mux.Handle(connect.NewResolveServiceHandler(a, opts...))
    mux.Handle(connect.NewRepositoryServiceHandler(a, opts...))
    mux.Handle(connect.NewDownloadServiceHandler(a, opts...))
    // ... rest unchanged
}
```

### Pattern 2: Structured Error Logging Before HTTP Error Response (Raw Handlers)

**What:** A helper method on `api` or `commitServiceHandler` that logs structured diagnostics and then writes the HTTP error response. Replaces every bare `http.Error(w, fmt.Sprintf(...), code)` call in commits.go.

**When to use:** In all v1/v1beta1 raw proto handlers where errors are written directly to HTTP response without any logging. Approximately 12 error sites in `commits.go`.

**Trade-offs:**
- Pro: Captures owner/module/commit context at the error point, not just final HTTP status
- Pro: Errors become traceable in structured logs without needing to capture and replay HTTP traffic
- Pro: Consistent format across all error paths (same attributes)
- Con: Requires modifying every error return site — easy to miss new error paths in future
- Con: Only catches errors that go through the helper; bare `http.Error` calls are missed

**Example:**

```go
// In api.go — shared helper
func (a *api) writeError(log *slog.Logger, w http.ResponseWriter, msg string, code int, attrs ...slog.Attr) {
    level := slog.LevelWarn
    if code >= 500 {
        level = slog.LevelError
    }
    log.LogAttrs(context.TODO(), level, "handler error",
        slog.Int("status_code", code),
        slog.String("error_message", msg),
        slog.Any("details", attrs),
    )
    http.Error(w, msg, code)
}

// Usage in commits.go:
// Before:
http.Error(w, fmt.Sprintf("resolving %s/%s: %v", ref.owner, ref.module, err), http.StatusInternalServerError)

// After:
a.writeError(h.api.log, w,
    fmt.Sprintf("resolving %s/%s: %v", ref.owner, ref.module, err),
    http.StatusInternalServerError,
    slog.String("owner", ref.owner),
    slog.String("module", ref.module),
)
```

### Pattern 3: Context-Based Correlation ID Propagation

**What:** A correlation ID flows through `context.Context` from the HTTP middleware layer down to providers. Every log entry along a request's path carries the same ID, enabling cross-component traceability.

**When to use:** Any request that traverses multiple layers (HTTP handler → multisource → provider → cache). Currently, the `X-Request-Id` header is passed through at the HTTP level but never carried in context.

**Trade-offs:**
- Pro: Essential for debugging multi-step requests (cache miss → GitHub fetch → digest computation)
- Pro: Works across both Connect RPC and raw handler paths when the context is propagated
- Pro: If the client sends no `X-Request-Id`, the server generates one automatically
- Con: Requires modifying `loggingMiddleware` AND the interceptor AND every handler that does logging
- Con: Only works if downstream code uses `slog.WithContext()` or receives a logger with the ID already attached
- Con: Adds overhead of `context.WithValue` call (negligible — ~100ns)

**Implementation approach:**

The correlation ID is set when it first arrives at the server. Two entry points need to handle this:

1. **HTTP middleware** (covers all paths, including raw v1/v1beta1 handlers): Generate or extract request ID, attach to context via `slog.WithContext`.
2. **Connect interceptor** (covers v1alpha1 handlers): Extract from headers again (or inherit from context).

**Recommended approach:** Do it in `loggingMiddleware` (main.go) since it wraps ALL traffic:

```go
// In loggingMiddleware — generate/pass-through request ID, attach to context
func loggingMiddleware(log *slog.Logger, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        reqID := r.Header.Get("X-Request-Id")
        if reqID == "" {
            // Generate short ID for internal tracing
            b := make([]byte, 4)
            rand.Read(b)
            reqID = hex.EncodeToString(b)
        }
        w.Header().Set("X-Request-Id", reqID)
        clientIP := getClientIP(r)

        // Attach request-scoped logger to context
        ctx := slog.WithContext(r.Context(),
            log.With(slog.String("request_id", reqID)))
        r = r.WithContext(ctx)
        // ... rest of existing logic
    })
}
```

### Pattern 4: Config-Driven Log Level with Environment Variable Override

**What:** Log level is set from config file but overridable at startup via environment variable. Standard 12-factor app pattern.

**When to use:** Always. Already partially implemented (config file). Adding env override gives operators a zero-config-change escape hatch for debug.

**Trade-offs:**
- Pro: Operator sets `EASYP_LOG_LEVEL=debug` to debug without touching config file
- Pro: Follows existing pattern in the codebase (`os.ExpandEnv` in config reader already supports env vars in YAML)
- Con: Two sources of truth for log level — env var must clearly take precedence over config file

**Implementation:**

```go
func newLogger(cfg config.LogConfig) *slog.Logger {
    levelStr := cfg.Level
    if envLevel := os.Getenv("EASYP_LOG_LEVEL"); envLevel != "" {
        levelStr = envLevel
    }

    var logLevel slog.Level
    switch strings.ToLower(levelStr) {
    case "debug":
        logLevel = slog.LevelDebug
    case "warn", "warning":
        logLevel = slog.LevelWarn
    case "error":
        logLevel = slog.LevelError
    default:
        logLevel = slog.LevelInfo
    }

    opts := &slog.HandlerOptions{
        Level:     logLevel,
        AddSource: cfg.AddSource,
    }

    var handler slog.Handler
    switch cfg.Format {
    case "text":
        handler = slog.NewTextHandler(os.Stdout, opts)
    default:
        handler = slog.NewJSONHandler(os.Stdout, opts)
    }

    return slog.New(handler)
}
```

## Current Data Flow — Logging Gaps

### Request Flow with Logging Gaps Annotated

```
buf CLI ──► HTTP
              │
              ▼
    loggingMiddleware
    ├── Extracts/Passthrough X-Request-Id (but does NOT attach to context)  ← GAP
    ├── DEBUG: logs method/path/masked-headers (no body)                    ← GAP
    ├── WARN: logs 4xx status + duration (no error message from body)       ← GAP
    └── ERROR: logs 5xx status + duration (no error context)                ← GAP
              │
              ▼
    http.ServeMux
              │
    ┌─────────┴─────────┐
    │                   │
    ▼                   ▼
  Connect RPC        Raw proto handler (commits.go)
  handler             │
  (blobs.go, etc.)    ├── io.ReadAll(body)                                    ← LOG: body available
  │                   ├── parseResourceRefs(body)                              ← GAP: no log
  │                   ├── repo.GetMeta()                                       ← GAP: no log
  │                   ├── http.Error(w, "resolving X: err", 500)               ← GAP: ERROR NOT LOGGED
  │                   │       └── Error message in body, NOT in structured log
  │                   └── w.Write(respMsg) isV1 check                          ← GAP: no log
  │
  ├── repo.GetMeta(ctx, owner, repo, ref)          ← LOG: debug "looking for meta"
  ├── error: return nil, fmt.Errorf(...)            ← GAP: error NOT logged at handler level
  │       └── Connect RPC framework returns code=internal, no structured log
  └── success: return &connect.Response{...}       ← GAP: no response size/duration log

              │
              ▼
    multisource.Repo.GetMeta
    ├── DEBUG: "looking for meta" (owner, repo)
    ├── S = findSource(owner, repoName)
    ├── DEBUG: "module found" (source, config)
    └── S.GetMeta(ctx, commit)
              │
              ▼
    github.sourceRepo.GetMeta
    ├── c.repos.Get(ctx, owner, repoName)           ← HTTP call to GitHub API
    ├── DEBUG: "found repo" (default_branch, dates)
    ├── c.repos.GetBranch(...)
    └── DEBUG: "found branch" (SHA)
              │
              ▼
    return content.Meta
```

### Specific Diagnostic Gaps

| Gap | Location | Impact |
|-----|----------|--------|
| No error logging in Connect handlers | `blobs.go:26`, `modulepins.go:22`, `bynames.go:24,39` | v1alpha1 errors are invisible in logs; only HTTP 200 or 5xx visible from middleware |
| No error logging in raw handlers | `commits.go:41,47,61,68,75,136,147,180,186,193,267,286,292,423` | Error messages ("resolving X: 404") are in HTTP body only, never logged |
| Logger stored but unused in `api` struct | `api.go:18` | `a.log` field is dead weight for error paths |
| No correlation ID propagation | `main.go:154` (middleware), all handlers | Cannot correlate log entries across a single request |
| No request body logging at DEBUG | `loggingMiddleware` + all handlers | Cannot replay the exact request that caused an error |
| No error duration per provider call | All provider methods | Total HTTP duration is logged, but not per-provider breakdown |
| BitBucket provider logs nothing | `bitbucket/getfiles.go`, `bitbucket/getrepo.go` | Errors mask as generic "not found" with no diagnostic detail |
| Cache errors only at ERROR level | `multisource/repo.go:87-93` | Cache misses are expected (first request), should be DEBUG or INFO |
| Simple error string in logs | Various provider `fmt.Errorf("...: %w")` | Wrapped errors lose structured context (owner, repo, commit) |

## Scaling and Performance Considerations

| Concern | Current State | With Diagnostic Logging |
|---------|--------------|------------------------|
| Log volume at INFO/WARN | ~2 lines per failing request (method, path, status, duration) | ~3-4 lines per failing request (added: error details from handler) |
| Log volume at DEBUG | ~8-10 lines per request (headers, provider metadata) | ~15-20 lines per request (added: request body, response summary, per-call duration) |
| Per-request allocation at DEBUG | Header clone (~ few KB) | Body read + truncation (~1 KB max if truncated), slog attrs |
| Error traceability | Requires reproducing with code | Full structured error at point of failure |
| Configuration points | 1 (log.level in config) | 2 (config + env var override) |

### Performance Mitigations

1. **All body logging behind `log.Enabled(ctx, slog.LevelDebug)`**: The debug check must happen BEFORE any allocation. Use `log.Enabled()` on the hot path, not `log.Debug()` (which evaluates arguments eagerly before the level check).

2. **Body truncation at 1KB**: When logging request/response bodies, truncate to 1024 bytes to avoid large allocations. The 50MB body limit means a naive `string(body)` call at DEBUG level could allocate 50MB.

3. **One `slog.With()` call per request, not per sub-component**: Creating a scoped logger with `request_id`, `owner`, `module` attributes should happen at the handler entry point and be passed down, not re-created at each provider call.

4. **Use `slog.LogAttrs` over `slog.Log`**: `LogAttrs` avoids allocating a `[]any` slice for key-value pairs, reducing GC pressure on hot paths.

5. **No logging in the noop cache path**: Cache misses to the noop backend should only be logged at DEBUG level since they are expected behavior.

## Anti-Patterns

### Anti-Pattern 1: Logging After `http.Error(w, msg, code)` Without Context

**What people do:** Call `http.Error(w, "resolving X: err", 500)` and then separately log `log.Error("handler error", "status", 500)`. The specific error context (owner, module) is lost.

**Why it's wrong:** The log entry says "handler error status=500" but does not say which module failed or why. The error message is only in the HTTP response body, which is not logged. The operator sees a 500 in logs but cannot identify the failing module without reproducing the request.

**Do this instead:** Log BEFORE writing the error, including the full context. Then write the HTTP response. Both the structured log and the error response contain the same diagnostic information.

### Anti-Pattern 2: `log.With()` Inside a Hot Loop

**What people do:** In `resolveModulePins` which iterates over module references, calling `log.With("owner", owner, "module", module)` inside the loop to create a new logger for each iteration.

**Why it's wrong:** `slog.With()` creates a new logger by copying the handler and adding attributes. In a loop over 128 repos, this creates 128 logger objects per request. This adds allocation overhead proportional to request complexity.

**Do this instead:** Create attributes at the handler level and use `slog.LogAttrs(ctx, level, msg, attrs...)` to pass per-iteration context without allocating a new logger. Or use a single `slog.With("owner", owner)` at the function entry point.

### Anti-Pattern 3: Logging Request Bodies at INFO or Higher

**What people do:** Logging the full protobuf request body at the same level as operational messages.

**Why it's wrong:** Request bodies can be up to 50MB. Logging them at INFO means every request generates massive log output. Additionally, request bodies may contain repository access tokens or other sensitive data that should not appear in operational logs.

**Do this instead:** Body logging must be at `slog.LevelDebug` AND must check `log.Enabled(ctx, slog.LevelDebug)` before reading. Implement truncation at ~1KB. Apply the same sensitive-field masking to body fields as to headers (look for token, password, authorization patterns in protobuf fields).

### Anti-Pattern 4: "Log Everything" Catch-All Logger

**What people do:** Passing a single `*slog.Logger` to the connect interceptor and logging every request/response at INFO level.

**Why it's wrong:** The interceptor fires for EVERY RPC call. At INFO level, it generates log output for every successful request with no diagnostic value. This wastes disk I/O and log aggregation bandwidth.

**Do this instead:** The interceptor should log at WARN/ERROR for failures, DEBUG for full details. Only operational messages (startup, connection checks, configuration changes) should be at INFO. Follow the existing pattern: "no news is good news" for success paths.

### Anti-Pattern 5: Rolling Your Own Correlation ID

**What people do:** Implementing a custom UUID generator or using a third-party library for trace IDs.

**Why it's wrong:** Adds a dependency for something that can be done with `crypto/rand` and hex encoding, or simply passthrough of the existing `X-Request-Id` header. The correlation ID is only meaningful within this server's log stream — it does not need UUID uniqueness guarantees.

**Do this instead:** Use the existing `X-Request-Id` header when present. When absent, generate an 8-byte hex string from `crypto/rand` (16 characters — sufficient for a single-server trace). Attach it to the context via `slog.WithContext` for zero-cost propagation to any code that calls `slog.FromContext(ctx)`.

## Integration Points

### External Services

| Service | Integration Pattern | Logging Gap |
|---------|---------------------|-------------|
| GitHub API | HTTP via go-github v59 | Only DEBUG-level metadata; no duration or error detail logging |
| BitBucket API | HTTP via net/http | No application logs at all |
| Artifactory cache | HTTP via net/http | Logger stored but no application-level cache metrics |
| Local git repos | os/fs + go-git v5 | No logging |
| Local filesystem cache | os/fs operations | No logging |

### Internal Boundaries

| Boundary | Communication | Current Logging | Recommended Change |
|----------|---------------|-----------------|-------------------|
| HTTP middleware → mux | `http.Handler` interface | Headers + status | Add correlation ID; generate if absent |
| Mux → Connect handlers | Connect RPC (deserialized) | None | Add interceptor for procedure, duration, error |
| Mux → raw handlers | `http.HandlerFunc` (raw protobuf) | None | Add entry/exit logging with correlation ID |
| Handlers → multisource | `provider` interface | DEBUG entry/exit | Add correlation ID propagation via context |
| multisource → Provider | `Provider.Find` → `Source.GetMeta` | None | Add per-provider duration tracking |
| multisource → Cache | `Cache.Get/Put` interface | ERROR on failures | Add DEBUG for hits/misses with duration |
| Provider → VCS API | External HTTP call | Sparse/none | Add request duration + response status |

## Configuration Changes Required

### Enriched LogConfig

```go
// cmd/easyp/internal/config/config.go

type LogConfig struct {
    Level     string `json:"level"`               // debug, info, warn, error
    Format    string `json:"format,omitempty"`     // "json" (default) or "text"
    AddSource bool   `json:"add_source,omitempty"` // include source file:line in log entries
}
```

No breaking change. Existing configs with only `level` continue to work (Format defaults to "json", AddSource defaults to false).

### Environment Variable Override

| Variable | Purpose | Precedence | Implementation |
|----------|---------|------------|----------------|
| `EASYP_LOG_LEVEL` | Override log level at startup | Overrides `log.level` from config | Check in `newLogger()` before switch |
| `EASYP_LOG_FORMAT` | Override output format | Overrides `log.format` from config | Check in `newLogger()` before handler creation |

No new variables are needed for correlation ID or interceptor config — those are always-on features that only activate at DEBUG level.

## Dependency Graph for Implementation

```
Phase: Config and Logger
  cmd/easyp/internal/config/config.go
    └── Add Format, AddSource to LogConfig
  cmd/easyp/main.go
    └── newLogger() — read Format, env var overrides

Phase: Connect Interceptor
  internal/connect/interceptor.go (NEW)
    ├── NewLoggingInterceptor(log) connect.Option
    ├── getOrCreateRequestID(headers) string
    └── requestID context key type
  internal/connect/api.go
    └── New() — accept opts ...connect.Option, pass to handler constructors
  cmd/easyp/main.go
    └── connect.New(log, storage, domain, interceptor)

Phase: Error Path Logging (handlers)
  internal/connect/blobs.go
  internal/connect/modulepins.go
  internal/connect/bynames.go
    └── Log structured error before returning
  internal/connect/commits.go
    └── Log structured error before http.Error()
    └── Log request body at DEBUG level (already io.ReadAll'd)

Phase: Correlation ID
  cmd/easyp/main.go
    └── loggingMiddleware — generate/attach request ID to context
  internal/connect/interceptor.go
    └── Extract request ID from context, add to logger
  internal/providers/multisource/repo.go
    └── No change needed — slog.FromContext(ctx) automatically picks up
        request-scoped logger if set via slog.WithContext in middleware

Phase: Provider Logging Enhancement
  internal/providers/github/getrepo.go
  internal/providers/bitbucket/getfiles.go, getrepo.go
  internal/providers/localgit/localgit.go
    └── Add DEBUG for entry/exit, logging from context logger
```

## Sources

- Existing codebase at `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/` (all source files read and confirmed)
- Connect RPC v1.19.2 documentation via Context7: interceptor patterns and handler options
- Go `log/slog` standard library documentation: `WithContext`, `FromContext`, `LogAttrs`

---

*Architecture research for: Diagnostic logging integration into EasyP Buf Proxy*
*Researched: 2026-06-16*
