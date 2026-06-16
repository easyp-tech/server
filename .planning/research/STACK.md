# Technology Stack — Diagnostic Logging

**Project:** EasyP Buf Proxy
**Researched:** 2026-06-16
**Mode:** Additive — existing stack (Go 1.26, Connect RPC v1.19.x, `slog.Logger` DI) is validated; this document covers ONLY what is needed for the diagnostic logging improvements.

**Overall confidence: HIGH** — All recommendations verified against current Go 1.26 stdlib, connect-go v1.19.x docs, and existing codebase patterns.

---

## Executive Summary

**Verdict: Zero new dependencies.** Go 1.26 stdlib `log/slog` is sufficient for all diagnostic logging features. No third-party logging library, no OpenTelemetry SDK, no structured logging middleware — the existing `slog.Logger` DI pattern, extended with a Connect RPC unary interceptor and contextual logger propagation, covers the full requirement.

**Key insight:** The missing piece is not a library — it is a **Connect RPC unary interceptor** and **contextual logger propagation** via `slog.Logger.With()`. The existing HTTP-level `loggingMiddleware` catches HTTP status codes but has no access to structured RPC fields (owner, repo, commit, procedure). Adding a Connect interceptor bridges this gap without any new dependency.

---

## Current Logging Architecture (Validated, No Changes Needed)

| Component | Technology | Purpose | Status |
|-----------|------------|---------|--------|
| Logger type | `*slog.Logger` (stdlib) | General structured logging | Correct — Go 1.26 stdlib |
| Log handler | `slog.NewJSONHandler(os.Stdout, opts)` | JSON output with configurable level | Correct — no change needed |
| Log level config | `LogConfig.Level` string in YAML | Config file level setting | Correct — extensible |
| Level parsing | `newLogger()` in `cmd/easyp/main.go` | Maps string to `slog.Level` | Correct — supports debug, info, warn, error |
| DI pattern | Constructor injection (`log *slog.Logger`) | Logger propagation | Correct — used throughout |
| HTTP middleware | `loggingMiddleware()` in `main.go` | HTTP-level request/response timing + masking | Correct — keep as-is |
| Sensitive masking | `maskSensitiveHeaders()` + `isSensitiveHeader()` | Prevents credential leakage in logs | Correct — keep as-is |

## Recommended Additions (Zero New Dependencies)

### 1. Connect RPC Unary Interceptor

**Purpose:** Log structured procedure context for all v1alpha1 Connect RPC handlers. Access typed request fields (owner, repository, reference) that the HTTP middleware cannot see.

**Why this pattern:** Connect RPC v1.19.x provides the `connect.Interceptor` interface with `WrapUnary(next UnaryFunc) UnaryFunc`. The interceptor runs inside the Connect protocol stack, after HTTP-level parsing but before the handler method. This gives access to:
- `req.Spec().Procedure` — fully qualified procedure name
- `req.Msg` — typed protobuf request message (for debug-level structured logging)
- `req.Header()` — HTTP headers (for correlation ID, client info)

**Implementation:** `internal/connect/interceptor.go`

```go
package connect

import (
    "context"
    "log/slog"

    "connectrpc.com/connect"
)

type LoggingInterceptor struct {
    log *slog.Logger
}

func NewLoggingInterceptor(log *slog.Logger) *LoggingInterceptor {
    return &LoggingInterceptor{log: log}
}

func (i *LoggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
    return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
        procedure := req.Spec().Procedure

        if i.log.Enabled(ctx, slog.LevelDebug) {
            i.log.DebugContext(ctx, "rpc started",
                slog.String("procedure", procedure),
                slog.String("peer", req.Peer().String()),
            )
        }

        resp, err := next(ctx, req)

        if err != nil {
            i.log.ErrorContext(ctx, "rpc failed",
                slog.String("procedure", procedure),
                slog.String("error", err.Error()),
                slog.String("error_code", connect.CodeOf(err).String()),
            )
        } else if i.log.Enabled(ctx, slog.LevelDebug) {
            i.log.DebugContext(ctx, "rpc completed",
                slog.String("procedure", procedure),
            )
        }

        return resp, err
    }
}

func (i *LoggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
    return next // no streaming in this server
}

func (i *LoggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
    return next // no streaming in this server
}
```

**Registration** (in `api.go` `New()`):
```go
interceptor := NewLoggingInterceptor(log)
mux.Handle(connect.NewResolveServiceHandler(a, connect.WithInterceptors(interceptor)))
mux.Handle(connect.NewRepositoryServiceHandler(a, connect.WithInterceptors(interceptor)))
mux.Handle(connect.NewDownloadServiceHandler(a, connect.WithInterceptors(interceptor)))
```

**Scope:** Only v1alpha1 Connect RPC handlers. v1beta1 handlers (commitServiceHandler) use raw `http.HandlerFunc` + `protowire` and are not Connect-generated handlers. They already pass through the HTTP `loggingMiddleware` for status/timing, and should log structured context internally using `api.log`.

**Confidence: HIGH** — Verified against connect-go v1.19.x Context7 docs: `Interceptor` interface, `WrapUnary`, `connect.WithInterceptors`.

---

### 2. `slog.Logger.With()` for Contextual Logging

**Purpose:** Reduce repetitive attribute construction. Ensure every log line from a component carries its operation context without per-call allocation.

**Current pattern (keep):**
```go
r.log.Debug("looking for meta", "owner", owner, "repo", repoName)
```

**Recommended extension:**
```go
// In commitServiceHandler where owner+module are known for a request scope:
log := h.api.log.With("procedure", r.URL.Path)
log.DebugContext(ctx, "processing request", "owner", ref.owner, "module", ref.module)
```

**When to apply:**
- `commitServiceHandler` methods — add contextual child loggers with `procedure` (URL path)
- `Repo.GetFiles` / `Repo.GetMeta` in `multisource/repo.go` — already uses explicit attributes; could use `With()` for cache operations
- `client.getRepo` in `github/getrepo.go` — already uses attributes on `c.log`

**Performance note:** `Logger.With()` allocates once per child logger. Subsequent log calls reuse the stored attributes. This is faster than repeating `"owner", owner, "repo", repoName` on every call when the values don't change for the scope.

**Where NOT to apply:** Hot loops with changing attribute values (e.g., iterating module refs where owner/module changes per iteration). For those, explicit attributes per call is correct.

**Confidence: HIGH** — Verified against Go 1.26 stdlib `log/slog` documentation.

---

### 3. Log Level Configuration: Environment Variable Override

**Purpose:** Support 12-factor/container-style configuration.

**Current config (already correct):**
```go
type LogConfig struct {
    Level string `json:"level"`     // debug, info, warn, error
}
```

**Recommended extension:**
```go
type LogConfig struct {
    Level     string `json:"level"`
    Format    string `json:"format"`     // "json" (default) or "text"
    AddSource bool   `json:"addSource"`  // include source file:line
}
```

**Env var fallback** in `newLogger()`:
```go
func newLogger(cfg LogConfig) *slog.Logger {
    level := cfg.Level
    if envLevel := os.Getenv("EASYP_LOG_LEVEL"); envLevel != "" {
        level = envLevel
    }
    // ... parse level, create handler
}
```

**Precedence:** `EASYP_LOG_LEVEL` env var > config file `log.level` > default ("info").

**Format option:** Allow `log/slog` text handler for local development (easier to read) vs JSON handler for production (structured ingestion). Controlled by config file or env var.

**Confidence: HIGH** — Pattern used across Go projects; no library change needed.

---

### 4. `slog.HandlerOptions.ReplaceAttr` for Centralized Redaction

**Purpose:** Catch accidentally leaked sensitive values at log serialization time, as a defense-in-depth layer beneath the existing HTTP header masking.

**Current state (keep):** `maskSensitiveHeaders()` in `main.go` catches known header names at the HTTP middleware layer.

**Recommended addition** in `newLogger()`:
```go
opts := &slog.HandlerOptions{
    Level: logLevel,
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        key := a.Key
        if a.Value.Kind() == slog.KindString {
            lowerKey := strings.ToLower(key)
            if lowerKey == "token" || lowerKey == "password" || lowerKey == "access_token" || lowerKey == "secret" {
                return slog.String(key, "***")
            }
        }
        return a
    },
}
```

**Why both layers:**
1. HTTP header masking catches raw header values before they reach any log call
2. `ReplaceAttr` catches any attribute named "token", "password", etc. that might leak through non-header paths (e.g., error messages that include tokens)

**Important:** `ReplaceAttr` matches by attribute key, not by value. It cannot detect secrets embedded inside error strings. For that, the existing approach of masking at the source is the right mitigation.

**Confidence: HIGH** — Verified against Go 1.26 stdlib `slog.HandlerOptions.ReplaceAttr` documentation.

---

### 5. Diagnostic Context on Error Paths

**Purpose:** Every error log should include enough context to diagnose the failure without source code access.

**Current pattern (good, already established in `multisource/repo.go`):**
```go
r.log.Error("cache get failed",
    "owner", owner,
    "repo", repoName,
    "commit", commit,
    "error", err)
```

**What to extend to other error paths:**

| File | Current Behavior | Recommended |
|------|-----------------|-------------|
| `github/getrepo.go` | Returns `fmt.Errorf(...)` without logging | Log `"owner"`, `"repo"`, `"error"` at Error level before returning |
| `github/getfiles.go` | Returns `fmt.Errorf(...)` without logging | Log `"owner"`, `"repo"`, `"commit"`, `"error"` at Error level before returning |
| `connect/blobs.go` | Returns `fmt.Errorf(...)` without logging | Log `"owner"`, `"repository"`, `"reference"`, `"error"` at Error level before returning |
| `connect/modulepins.go` | Returns `fmt.Errorf(...)` without logging | Log `"owner"`, `"repository"`, `"reference"`, `"error"` at Error level before returning |
| `connect/bynames.go` | Returns `fmt.Errorf(...)` without logging | Log `"name"`, `"error"` at Error level before returning |
| `connect/commits.go` handlers | Writes `http.Error` and returns | Log `"procedure"`, `"error"`, operation context at Error level before writing response |

**Important caveat:** Logging BEFORE returning an error means the HTTP middleware will log again at Warn/Error level with status code. This is fine — the handler-level log has the structured context (owner, repo, commit) while the middleware log has the HTTP-level data (status, duration, client IP). They complement each other.

**Anti-pattern to avoid:** Logging the same detail twice in the same method. When a helper function logs an error and the caller also logs it, the caller should NOT re-log the same structured fields — use error wrapping (`%w`) instead.

**Confidence: HIGH** — Pattern already established in codebase; extension is mechanical.

---

### 6. slog.FuncAttr for Conditionally Expensive Attributes

**Purpose:** For debug-level log attributes that are expensive to compute (e.g., hex-encoding a digest, formatting a protobuf message), defer the computation until the log level is confirmed.

**Pattern:**
```go
log.DebugContext(ctx, "file hashing details",
    slog.Int("file_count", len(files)),
    slog.Func("digests", func() slog.Value {
        // Only evaluated if Debug is enabled
        digests := make([]string, len(files))
        for i, f := range files {
            digests[i] = hex.EncodeToString(f.Hash[:])
        }
        return slog.AnyValue(digests)
    }),
)
```

**When to use:** For attributes that involve iteration, serialization, or allocation. NOT for simple values like strings, ints, or durations.

**When NOT to use:** For simple attribute values where the cost of the Func closure overhead exceeds the computation itself.

**Confidence: HIGH** — Verified against Go 1.26 stdlib `slog.FuncAttr` documentation. The feature is available since Go 1.21.

---

## Technologies Explicitly NOT Recommended

| Technology | Why Not | Better Approach |
|------------|---------|-----------------|
| `rs/zerolog` | Go 1.26 stdlib slog is stable, zero-dependency, and meets all needs. Adding zerolog would introduce a dependency for marginal benefit. | Use `slog.NewJSONHandler` with custom options |
| `uber-go/zap` | Same reasoning. Zap's performance advantage over slog eroded significantly in Go 1.22+ with slog's handler optimizations. For a proxy server doing network I/O, logging overhead is dominated by JSON serialization. | Use `slog.LogAttrs` for zero-alloc hot paths |
| `sirupsen/logrus` | Panic-based API, unstructured by default, no built-in level gates. | Not competitive; slog is superior in every dimension |
| OpenTelemetry logging SDK | Overkill for single-process proxy. Adds dependency surface and configuration complexity without solving "log context on error." OTel is for distributed tracing across services. | Connect interceptor + slog is simpler and more maintainable |
| `connectrpc.com/otelconnect-go` | Only useful with existing OTel infrastructure. Not present here. | Connect's built-in `Interceptor` interface is sufficient |
| Chi/gorilla/logr middleware wrappers | The project already has a hand-rolled HTTP middleware. Adding a framework-specific wrapper gives no benefit. | Extend existing middleware |
| `go.uber.org/zap/zapcore` Hooks | Not needed — slog's `ReplaceAttr` and `Enabled()` provide equivalent capabilities | Use slog-native features |

## Performance Characteristics for This Codebase

| Concern | Mitigation | Priority |
|---------|-----------|----------|
| Level checks on hot path | Use `log.Enabled(ctx, level)` before constructing attributes. Already done in HTTP middleware; extend to handlers. | **High** — `commitServiceHandler` can be called many times per `buf build` |
| Debug-level body allocation | v1alpha1 handlers (blobs, modulepins, bynames) return structured responses. Under debug, use `loggingResponseWriter` (already exists). For v1beta1, body logging is handled by the HTTP middleware at the raw byte level. | **Medium** — body is already logged at HTTP level |
| Logger.With() allocation | Each child logger allocates once. Acceptable for request-scoped loggers. | **Low** — one allocation per RPC is negligible |
| slog.FuncAttr overhead | Closure overhead is microseconds. Only use when the deferred computation is more expensive than the closure. | **Low** — apply only in hot loops |
| JSON serialization | Acceptable overhead. Server makes external HTTP calls (GitHub API, git operations) dominating latency (100ms+). JSON log serialization is microseconds. | **Low** — not a bottleneck |

## Dependencies

### No new production dependencies.

Everything needed is in Go 1.26 stdlib:
- `log/slog` — structured logging, level checks, handler options
- `context` — level checks and propagation
- `net/http` — existing middleware (unchanged)
- `os` — environment variable reading

### Existing dependencies that remain unchanged (no version bumps needed):
- `connectrpc.com/connect v1.19.2` — `Interceptor` interface, `UnaryInterceptorFunc`, `WithInterceptors`
- `google.golang.org/protobuf v1.36.11` — typed message access (already imported)

---

## Sources

- **connect-go v1.19.x interceptor API** — Context7 `/connectrpc/connect-go` library documentation: `Interceptor` interface, `UnaryInterceptorFunc`, `connect.WithInterceptors`
- **Go 1.26 log/slog package** — stdlib documentation: `Enabled()`, `LogAttrs()`, `HandlerOptions.ReplaceAttr`, `Logger.With()`, `slog.FuncAttr`
- **Existing codebase patterns** — `cmd/easyp/main.go` (lines 130-147 for `newLogger`, lines 150-206 for `loggingMiddleware`); `internal/multisource/repo.go` (lines 86-119 for existing error logging pattern); `internal/connect/api.go` (handler registration pattern)
