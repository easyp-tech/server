# Pitfalls Research: Diagnostic Logging

**Domain:** Adding diagnostic logging to an existing Go Connect RPC proxy server with sensitive data handling
**Researched:** 2026-06-16
**Confidence:** HIGH

## Critical Pitfalls

### Pitfall 1: Logging Sensitive Data Through Error Chains

**What goes wrong:**
Adding structured logging to error paths inadvertently leaks API tokens, passwords, or internal URLs. The error chain is wrapped through multiple layers (`fmt.Errorf("...: %w", err)`), and by the time it reaches a log statement, it may contain full HTTP URLs with query parameters that include credentials or other secrets.

**Why it happens:**
The existing codebase wraps errors with increasingly detailed context at each layer:

- BitBucket's `httpClient.get()` builds a URL with `req.SetBasicAuth` and then includes `req.URL.String()` in error messages (line 104, 110, 115 of `bitbucket/client.go`).
- Artifactory cache `Get()` and `Put()` methods include the full URL in error messages (e.g., `getting %q: response %d: %w`), and those URLs contain the repo path and commit hash which could encode sensitive information.
- The `multisource.Repo.cacheGet()` method at line 87 calls `log.Error("cache get failed", ...)` which will include these wrapped errors.
- The connect handlers at `commits.go` line 61 call `fmt.Sprintf("resolving %s/%s: %v", ...)` which exposes the full error chain including any secrets from downstream providers.
- If a log statement is added at the connect handler level (e.g., `a.log.Error(...)` with the error from `a.repo.GetMeta()`), it will include any sensitive data from the provider error chain.

**How to avoid:**
1. Audit every error path before adding logging — trace from connect handler through providers to external API calls. For each path, identify what data could end up in the error message.
2. Implement a centralized error redaction layer: wrap errors from external API calls in a custom error type that strips URLs or replaces sensitive segments before they propagate up.
3. When adding `log.Error()` calls at the handler level, explicitly list safe attributes as structured fields rather than logging the full error string as a single attribute.
4. Never log `err.Error()` as a raw string without redaction. Use structured attributes for safe fields: `slog.String("owner", owner)`, `slog.String("repo", repoName)`.
5. For BitBucket and GitHub providers, strip URL query parameters from error messages before they wrap. Replace the full URL with a safe identifier.

**Warning signs:**
- Error messages at the handler level contain URLs, query parameters, or credential-like strings.
- A `log.Error()` call uses `slog.String("error", err.Error())` as the primary error attribute rather than structured fields.
- Error wrapping in providers passes through function arguments that could contain sensitive data into string formatting.

**Phase to address:**
Logging implementation phase — audit and redact before any new log.Error() calls are added.

---

### Pitfall 2: Double Logging of Errors (Middleware vs. Handler)

**What goes wrong:**
Both the HTTP middleware and the Connect RPC handler log the same error, producing duplicate log lines. Operators see two log entries per error and cannot tell which is the authoritative one. This causes log volume inflation, alert fatigue, and confusion during incident response.

**Why it happens:**
The existing middleware already logs all 4xx/5xx responses at `slog.LevelWarn` or `slog.LevelError` (main.go lines 178-204). When you add handler-level logging for the same errors, both fire:

```
// Middleware log:
{"level":"ERROR","msg":"request completed","path":"/buf.registry.module.v1.CommitService/","status":500}

// Handler log (newly added):
{"level":"ERROR","msg":"failed to resolve commit","owner":"myorg","module":"mymod","error":"getting repo: ..."}
```

The middleware sees a 500 status and logs it at ERROR level. The handler sees the same error and also logs at ERROR level. Result: two ERROR log lines for one failure.

The problem is worse for Connect RPC handlers that use manual `http.Error()` (the v1beta1/v1 handlers in commits.go) — they set the status code and write an error response, so the middleware will fire for those. But the Connect RPC handlers (GetModulePins, etc.) return errors to the Connect framework, which handles status code mapping internally — the middleware's view of the status may not match the application-level error detail.

**How to avoid:**
1. Decide on a single logging layer. Either log at the handler level (close to the error source) and suppress middleware-level error logging, or keep middleware logging and suppress handler-level logging.
2. The better approach: log at the handler level only, because that's where you have full context (owner, repo, error details). Keep the middleware logging for INFO-level timing/status data only, not for ERROR-level diagnostic details.
3. Alternatively, use a log attribute to deduplicate: add `slog.Bool("logged_by_handler", true)` in handler logs, and have the middleware check for this attribute to skip its own error log.
4. Or use a context-based flag: set a `errorLogged` context key in the handler, and check it in the middleware before logging.

**Warning signs:**
- Two ERROR-level log lines for a single request with overlapping timestamps.
- Operators asking "which log line is the real error?"
- Log volume jumps 2x after adding handler-level logging without changing middleware.

**Phase to address:**
Logging implementation phase — decide logging boundary before writing any code.

---

### Pitfall 3: Logging Protobuf Binary Bodies Without Truncation or Decoding

**What goes wrong:**
Adding debug-level request/response body logging logs raw protobuf binary data. The output is unreadable binary noise that consumes excessive storage, causes log shipping latency, and can include embedded null bytes that break JSON log parsing. A 50MB protobuf body (the configured body limit) logged at debug level fills disks and overwhelms log aggregators.

**Why it happens:**
The existing code already reads `r.Body` entirely with `io.ReadAll(r.Body)` in the commit and graph handlers (commits.go lines 39, 131, 251). The naive approach to diagnostic logging would be:

```go
body, _ := io.ReadAll(r.Body)
log.Debug("request body", "body", body) // DANGER
```

This triggers four problems:
1. **Size**: Bodies up to 50MB — logging them produces gigabytes of output per request.
2. **Readability**: Protobuf wire format is binary. Logging `\x08\x01\x12\x0agoogleapis` is useless for debugging.
3. **Parsing**: JSON handlers choke on binary data. The `slog.JSONHandler` will encode bytes as base64, making logs even less human-readable.
4. **Double-read**: If the body is read for logging, the handler's own `io.ReadAll` later will get an empty body (since `r.Body` is a stream). Unless `r.Body` is wrapped with `io.NopCloser(bytes.NewReader(body))`.

The handlers in commits.go already read the body and parse it with `protowire`. If you add body logging before the parse attempt, you get the raw body. If you add it after parsing, you get the structured fields from the parse result — which is actually useful.

**How to avoid:**
1. Never log raw protobuf bytes. Log the structured fields extracted during parsing instead.
2. If body logging is absolutely necessary for debugging, add it at TRACE level (a custom log level or Debug-only with explicit opt-in), truncate to a small max (e.g., 1024 bytes), and encode as hex for readability.
3. For commit IDs, owner names, module names that are extracted from protobuf: log those as structured string fields (they are already strings after parsing).
4. Wrap `r.Body` with a tee reader if both logging and handler need the body, rather than reading twice and risking the second read returning empty.
5. Consider logging the body *after* successful parsing, since the parsed fields are more useful than the raw bytes. For parse failures, log the first N bytes as hex with a size indicator.

**Warning signs:**
- Log lines containing base64-encoded binary data that operators cannot read.
- Disk usage spikes when debug logging is enabled.
- Log aggregator errors about malformed JSON when debug logging is on.
- Body data appearing in logs at INFO or ERROR level (should be DEBUG only).

**Phase to address:**
Tracing/logging detail phase — design body logging strategy before implementation.

---

### Pitfall 4: Error Context Not Captured Where the Error Originates

**What goes wrong:**
Log statements added at the handler level only capture "what happened" (status code, error message) but not "why it happened" (which provider failed, what external API call failed, what was the HTTP response from GitHub). The logs are insufficient to diagnose the root cause without adding more logging and reproducing the issue.

**Why it happens:**
The current architecture has a clean separation:
- Connect handlers call `a.repo.GetMeta()` or `a.repo.GetFiles()` 
- `multisource.Repo` delegates to providers
- Providers (GitHub, BitBucket, localgit) make external API calls

If you only add logging at the handler level, you get:
```
ERROR failed to resolve commit owner=myorg module=mymod error="resolving myorg/mymod: investigating myorg/mymod: getting default branch: ..."
```

But you don't know:
- Which provider was used (GitHub, BitBucket, localgit)?
- Was it a cache hit or miss?
- What was the HTTP status code from GitHub API?
- Was this a timeout (30s timeout configured) or a 4xx/5xx from GitHub?

The wrapped error message *might* contain this, but it's unstructured and potentially incomplete.

**How to avoid:**
1. Add logging at the provider layer, not just the handler layer. Each provider should log its own external API calls at DEBUG level: URL (redacted), HTTP method, response status, duration.
2. In `multisource.Repo`, log which provider was selected (GitHub vs BitBucket vs localgit), and whether the result came from cache.
3. Use structured logging fields consistently so logs from different layers can be correlated: always include `owner`, `repo`, `commit` (when available), `request_id` (from context).
4. For each external HTTP call, log: method, path (redacted), response status, response size, duration, and a truncated error if any.
5. Cache operations should log: hit/miss, key components (owner/repo/commit/configHash), duration, and file count.

**Warning signs:**
- Error log lines that contain "investigating" or "resolving" with no further detail about which API call failed.
- Support incidents where the fix is "add more logging and ask the user to reproduce."
- The `owner` and `repo` fields are present in logs but `provider_type` is missing.

**Phase to address:**
Provider logging phase — add structured logging at each provider boundary before or alongside handler logging.

---

### Pitfall 5: Missing Request ID Propagation Across Async and Cache Boundaries

**What goes wrong:**
The request ID from `X-Request-Id` header is captured in the HTTP middleware but never propagated into the context or passed to provider calls. When multiple requests interleave (common in concurrent buf CLI operations), log entries from different requests are mixed together with no way to correlate them — every line looks like it came from the same request.

**Why it happens:**
The current code captures `requestID` in the `loggingMiddleware` (main.go line 154) but it's a local variable in the middleware closure. It is not:
- Added to `r.Context()` for downstream handlers to extract
- Logged in any provider-level logging (multisource, github, bitbucket, artifactory)
- Included in cache hit/miss log entries in `multisource/repo.go`

The providers each have their own `log *slog.Logger` instance, but there is no request-scoped context attached to them. When two concurrent requests reference the same owner/repo, the provider logs are indistinguishable:

```
// Concurrent requests A and B:
DEBUG cache hit              owner=myorg repo=mymod files=12     // A or B?
DEBUG cache get failed       owner=myorg repo=mymod error=...    // A or B?
```

Without a request ID in every log line, operators cannot tell which request caused the cache failure.

The situation is worse for cache operations: `cacheGet` is called from `GetFiles`, but the goroutine in `startPeriodicAccessChecks` (main.go line 92) runs cache checks outside any request context — it uses `context.Background()` with a timeout, so there's no request ID at all for periodic access check logs.

**How to avoid:**
1. Store the request ID in `r.Context()` using a custom context key type: `ctx = context.WithValue(r.Context(), requestIDKey, requestID)`.
2. Use a `slog.Handler` wrapper that automatically extracts the request ID from context and adds it to every log line within that request scope.
3. Alternatively, pass request-scoped metadata explicitly in the provider interface (add a `reqID` parameter or embed it in a request context struct).
4. For periodic background checks (startPeriodicAccessChecks), use a fixed request ID like `"health-check"` so they're identifiable.
5. For cache operations, attach the request ID to the log line even though the cache layer is shared across requests — this requires the cache methods to receive the request ID or context with it.

**Warning signs:**
- Log lines from concurrent requests cannot be distinguished.
- Cache hit/miss logs lack any correlation ID.
- Error log lines during high concurrency look like the same error happening repeatedly (it might be one error logged without request context).
- Test failures that are timing-dependent and involve concurrent goroutines become undebuggable.

**Phase to address:**
Logging infrastructure phase — context propagation must be designed before writing any new log statements.

---

### Pitfall 6: Logging Sensitive Provider Credentials in Startup Validation

**What goes wrong:**
The startup phase logs sensitive provider credentials (GitHub tokens, BitBucket passwords, Artifactory credentials) in error messages or debug output. These credentials appear in log files, CI output, or startup logs that may be stored indefinitely.

**Why it happens:**
The startup code in `main.go` calls `checkRepositoryConnections`, `checkCacheAccess`, and `startPeriodicAccessChecks`. If any of these fail, the error is logged. The provider initialization functions (`githubProxy`, `bbProxy`, `buildCache`) receive tokens and passwords but don't redact them in error paths.

The GitHub client stores the token in a `github.Client` struct. If a GitHub API call fails with a 401, the error may include the token or indicate that authentication failed in a way that leaks context. Similarly, BitBucket credentials are stored as `User` and `Password` types in a `Repo` struct — if these end up in error chains, they're exposed.

The Artifactory cache constructor (artifactory.go line 26-44) receives `user` and `password` as bare strings and stores them. Its error messages include the full URL (e.g., `getting "https://artifactory.example.com/artifactory/..."`), which may contain path elements that encode sensitive information.

**How to avoid:**
1. Add a `slog.String("error", redactURLs(err.Error()))` helper that strips credentials and path segments from error strings before logging.
2. Implement a `fmt.Stringer` or redaction wrapper for provider credentials that masks the token/password in log output.
3. In startup validation, log at INFO level for success and WARN level for failure (not ERROR) — startup failures don't need credential details, they need to know *which* connection failed.
4. For GitHub token validation: log `slog.String("owner", owner)` and `slog.String("repo", repoName)` but never `slog.String("token", token)`.
5. Consider logging "authentication: ok" instead of "token eyJhbGci..." for security-audit use cases.

**Warning signs:**
- Startup logs that contain "token", "password", "secret", "eyJ" (JWT prefix), or "Bearer" strings.
- Error messages from provider initialization that include configuration values.
- Logs from a local development environment that accidentally match production credential patterns.

**Phase to address:**
Logging implementation phase — add credential redaction to error logging before merging.

---

### Pitfall 7: Panic in Logging Code Taking Down the Request

**What goes wrong:**
A logging operation itself panics (nil pointer dereference on request fields, type assertion failure on context values, or attempting to access a response header after the response is written), causing the entire request to fail with a 500 or crashing the server goroutine.

**Why it happens:**
The most common scenarios in this codebase:
1. **Middleware logging after response written**: The existing `loggingResponseWriter` captures `status` and `size`. If a new log statement attempts to read `r.Body` or `r.Header` after the handler has already consumed the body, it may get nil or empty data.
2. **Context value extraction**: If a request ID or trace ID is stored in context with a custom type and extracted with a type assertion (e.g., `ctx.Value(key).(string)` without the `ok` check), a missing value causes a panic.
3. **Body re-read**: Reading `r.Body` twice — once for logging, once for the handler — causes the second read to return `io.EOF`. If the handler doesn't handle this, it errors out. If the logging code tries to re-read after the handler, it gets empty data.
4. **Logging middleware wraps Connect RPC stream handlers**: If Connect streaming is added later, the middleware's attempt to capture the response body could deadlock or panic on streaming responses.
5. **Log attribute value that is itself nil**: Passing a nil pointer as `slog.Any("request", nil)` should be safe, but passing a nil `error` value that has a non-nil type (e.g., `var e *myError = nil; log.Error("msg", e)`) triggers a `slog` bug where it tries to call `.Error()` on a nil receiver, which panics if the method is defined on a pointer receiver.

**How to avoid:**
1. Defensive coding: every context value extraction must use the comma-ok idiom and handle missing values gracefully.
2. Body reading: wrap `r.Body` once at the top of the handler chain (in the middleware), store the bytes, and pass an `io.NopCloser(bytes.NewReader(body))` as the new `r.Body`. The logging code and handler both read from the buffered copy.
3. Add a top-level `recover()` in the logging middleware: if logging panics, catch it, log the panic itself via `slog.Default()`, and let the request continue. A logging failure should never take down a request.
4. Use `slog.Any` with typed nil checks, or always pass concrete values. Prefer `slog.String`, `slog.Int`, `slog.Bool` over `slog.Any` for primitive types.
5. Test the logging code explicitly: write unit tests that verify logging does not panic on edge cases (nil context values, empty bodies, truncated responses, concurrent writes).

**Warning signs:**
- Recover handlers in production catching panics from unexpected places.
- Test failures that only occur when test logging is verbose.
- A nil pointer dereference stack trace that mentions `slog` or a logging function in the trace.
- Requests that fail with 500 but the handler logic completed successfully.

**Phase to address:**
Logging infrastructure phase — defensive patterns and recovery must be implemented before any handler-level logging.

---

### Pitfall 8: Log Volume Explosion from Debug-Level Request/Response Tracing

**What goes wrong:**
Enabling debug-level logging for request/response tracing generates thousands of log lines per request, filling disk, overwhelming log shippers, and making it impossible to find actual errors among the noise. The production impact is subtle: disk fills gradually, log indexes grow, and query performance degrades over hours or days.

**Why it happens:**
Each buf CLI operation triggers multiple internal RPCs (CommitService, DownloadService, GraphService). Each RPC handler, if logged at debug level with full context, generates multiple log lines:

```
DEBUG CommitService/ServeHTTP started     // 1 line
DEBUG body read OK                         // 1 line (if body logging added)
DEBUG parseResourceRefs found 3 refs       // 1 line
  For each ref:
DEBUG resolving ref owner/module           // 3 lines
DEBUG GetMeta called                        // 1 line per ref
DEBUG multisource findSource                // 1 line per ref
DEBUG github client.GetRepo                // 1 line per ref
DEBUG github API response                  // 1 line per ref
DEBUG found repo default branch=main       // 1 line per ref
DEBUG computeB4Digest started              // 1 line per ref
DEBUG GetFiles called                      // 1 line per ref
DEBUG multisource cacheGet                 // 1 line per ref
DEBUG cache hit/miss                       // 1 line per ref
  If cache miss:
DEBUG github client.GetTree                // 1 line per ref
DEBUG github tree entries 500              // 1 line per ref
DEBUG downloading file proto/file.proto   // 500 lines per ref (one per file!)
DEBUG hashing file proto/file.proto        // 500 lines per ref
...
```

A single `buf mod update` with 2 modules in a large repo could generate 2000+ debug log lines. If this runs in CI or during development with debug logging, it creates significant volume.

**How to avoid:**
1. Use structured log levels strategically:
   - INFO: request start, request complete, and errors only.
   - DEBUG: one line per RPC call (CommitService, DownloadService) with key metadata.
   - TRACE: per-file operations (downloading, hashing). Consider using a custom level below DEBUG for this.
2. Implement per-package or per-component log levels: let operators enable debug logging only for the connect handlers or only for providers independently.
3. Add a sampling mechanism: only log every Nth request at debug level, or debug-log only requests that eventually error.
4. Use log rate limiting in the `slog.Handler`: wrap the handler to drop messages after a threshold within a time window.
5. Document expected log volume at each level so operators know what to expect before enabling debug in production.

**Warning signs:**
- Debug log output takes longer to write than the request processing.
- Log shipping CPU usage spikes when debug logging is enabled.
- Log storage costs increase measurably after deploying a debug build.
- Developers or operators say "I turned on debug logging but I can't find anything useful."

**Phase to address:**
Tracing/logging detail phase — design log levels and volume expectations before implementing.

---

### Pitfall 9: Logging After Context Cancellation

**What goes wrong:**
Log statements executed after the request context has been cancelled (e.g., during cleanup, deferred operations, or in error branches after a context deadline exceeded) may produce misleading log output, fail silently, or panic. The `slog.Logger.Enabled()` check returns false for cancelled contexts by default, so debug logs are silently dropped — but error logs may still fire, creating confusion.

**Why it happens:**
The Connect RPC framework and HTTP server cancel the request context when the client disconnects, the timeout fires, or the response is fully written. If a handler encounters an error during processing and tries to log diagnostic details in a defer or cleanup block:

```go
func (h *commitServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    defer func() {
        // r.Context() is already cancelled here
        h.api.log.ErrorContext(r.Context(), "cleanup error", ...) // Context cancelled
    }()
    body, err := io.ReadAll(r.Body)
    // ...
}
```

The `slog.Logger.ErrorContext()` will still fire — `slog` doesn't reject error-level logs even on cancelled contexts. But the log may include incomplete data (partial body reads), and the error message itself may be misleading because it's reporting on an operation that was already doomed.

Additionally, providers that make HTTP calls to GitHub/BitBucket with a cancelled context will fail with `context.Canceled` or `context.DeadlineExceeded`. If a log statement at the provider level reports this as "API call failed" without distinguishing "context cancelled" from "API returned 500", it's misleading.

**How to avoid:**
1. Check `ctx.Err()` before logging in defer/cleanup blocks: if the context was cancelled, log at DEBUG level with a `"cancelled"` field rather than at ERROR level.
2. Distinguish between "operation failed because client disconnected" (log at INFO or DEBUG) vs. "operation failed due to external API error" (log at ERROR).
3. In provider logging, add `slog.Bool("context_cancelled", ctx.Err() != nil)` to every log line so operators can filter out cancellation-related failures.
4. Use `slog.LogAttrs` with `slog.LevelError` but add `slog.Bool("cancelled", true)` so log aggregators can filter these separately.
5. For the periodic health checks (startPeriodicAccessChecks), always use `context.WithoutCancel(ctx)` for logging — the health check log should never be silently dropped.

**Warning signs:**
- Log lines with errors like "getting https://api.github.com/...: context deadline exceeded" that appear alongside healthy request logs.
- A sudden spike in error logs during deployments that matches the timeout window (30s).
- "Read: connection reset by peer" errors logged as application failures.
- Error descriptions that mention "context" but aren't identifiable as client disconnections.

**Phase to address:**
Provider logging phase — context cancellation handling must be part of every new log.Error() call.

---

### Pitfall 10: Inconsistent Log Attributes Making Correlation Impossible

**What goes wrong:**
Different packages and handlers use different attribute names for the same logical concepts. A log line from the middleware uses `"request_id"` while provider logs use `"reqID"`. Handler logs use `"module"` while provider logs use `"repo"` and `"repoName"`. Operators cannot write consistent queries across log sources, and automated log analysis tools fail to correlate related events.

**Why it happens:**
The existing codebase shows this pattern already forming:
- `multisource/repo.go` uses `"owner"`, `"repo"`, `"commit"` as attribute keys
- `github/getrepo.go` uses `"default branch"`, `"created"`, `"updated"` with no `owner`/`repo` prefix
- `main.go` middleware uses `"request_id"`, `"method"`, `"path"`, `"client_ip"`, `"status"`, `"size"`, `"duration"`

When new logging is added, each developer will naturally use their own naming conventions. The result is a fragmented log schema with 5 different names for the same thing.

```
"owner"     // multisource/repo.go
"owner"      // Not found — it's part of the log message text in github/getrepo.go
"request_id" // main.go middleware
"request-id" // hypothetical: different developer's convention
"reqID"      // hypothetical: yet another
```

**How to avoid:**
1. Define a project-wide log attribute naming convention before implementing new logging. Document it in a CONTRIBUTING.md or logging guide.
2. Use `slog` package-level constants or helper functions for common attributes:
   ```go
   // internal/log/attrs.go
   const (
       AttrRequestID = "request_id"
       AttrOwner     = "owner"
       AttrRepo      = "repo"
       AttrCommit    = "commit"
       AttrProvider  = "provider_type"
       AttrCacheHit  = "cache_hit"
       AttrDuration  = "duration_ms"
   )
   ```
3. For existing inconsistent attributes, standardize them during the logging refactor. Rename `"default branch"` to `"default_branch"` and add missing `owner`/`repo` fields.
4. Use a custom `slog.Handler` that enforces attribute name conventions (e.g., rejects snake_case violations or renames automatically).
5. Audit all existing log calls and create a migration plan. Phase 1: add new log calls with standard names. Phase 2: migrate old calls to standard names.

**Warning signs:**
- Log queries that return no results because the attribute name was wrong.
- Observability dashboards with duplicate panels for "module" and "repo" that show the same data.
- Pull request comments asking "should this be `request_id` or `reqID`?"
- A `grep` across the codebase for log attribute names shows 5+ different patterns for the same concept.

**Phase to address:**
Logging infrastructure phase — attribute naming convention must be established before any new log calls are added.

---

### Pitfall 11: Log Level Not Dynamically Configurable in Production

**What goes wrong:**
When a production incident occurs, operators need to increase log verbosity to diagnose the issue, but the server must be restarted to change log levels. The restart may clear the state, change load balancing, or be blocked by deployment policies. By the time debug logging is enabled, the incident conditions have passed.

**Why it happens:**
The current `newLogger` function (main.go lines 130-147) reads the log level from configuration at startup and creates a static `slog.HandlerOptions` with `Level: logLevel`. There is no mechanism to change the level at runtime. The `Level` field in `slog.HandlerOptions` is a `slog.Leveler` interface, which could be a `slog.LevelVar` (an atomic level that can be changed at runtime), but the current code uses a concrete `slog.Level`.

The `AddSource: false` setting is also static — source location can't be enabled at runtime without a restart.

**How to avoid:**
1. Use `slog.LevelVar` instead of `slog.Level` in `HandlerOptions`: `Level: new(slog.LevelVar).Level(logLevel)` — wait, that's not the right API. Actually:
   ```go
   var levelVar slog.LevelVar
   levelVar.Set(logLevel)
   opts := &slog.HandlerOptions{Level: &levelVar}
   ```
2. Expose the `*slog.LevelVar` via an HTTP endpoint (e.g., `GET /debug/log-level?level=debug`) that operators can call without restarting. Keep this behind mTLS or localhost-only access.
3. Implement a `SIGHUP` handler that re-reads the config file and updates the log level.
4. Consider `AddSource` as a config-level toggle that can be changed at startup (low cost) but not at runtime (higher cost due to allocation per log call).
5. Document the mechanism in operational runbooks so operators know how to enable debug logging during incidents.

**Warning signs:**
- Support tickets that start with "please enable debug logging and restart the server."
- The phrase "we'll deploy a debug build" in incident response.
- A `slog.Level` value assigned from config that is never reassigned at runtime.

**Phase to address:**
Logging infrastructure phase — dynamic level control is essential for production diagnostic logging.

---

### Pitfall 12: Test Logging Creates Flaky Assertions

**What goes wrong:**
Unit and integration tests that assert on log output become flaky due to:
1. Log output order is non-deterministic in concurrent tests.
2. Log level changes break tests that expected specific log messages.
3. Log format changes (e.g., adding new attributes) cause string-matching assertions to fail.
4. Race conditions between log emission and test assertion.
5. Logs from unrelated goroutines or health checks pollute the captured output.

**Why it happens:**
The existing test suite has 14 tests across 4 packages. If tests are added that capture log output (e.g., using a `bytes.Buffer` as the `slog.Handler` writer), they will run into these issues. The test infrastructure uses `httptest` servers and real buf CLI binaries — these are not isolated log sinks.

For example, the test in `api_test.go` uses `slog.Default()` and the main server handler. If `api_test.go` starts a server and the periodic health check goroutine fires during the test, its log output will pollute the test's captured logs.

**How to avoid:**
1. When testing logging behavior, capture logs at the handler level using a custom `slog.Handler` that records log records in a thread-safe buffer, rather than capturing process-level stdout.
2. Never assert on exact log output strings. Assert on structured attributes: check that a log record has `"owner" == "myorg"` rather than that the output contains `"owner\":\"myorg\"`.
3. Use a `slog.Handler` that records records in a slice with mutex protection, and provide helper methods for assertions:
   ```go
   func (h *testHandler) AssertContains(t *testing.T, key string, value any) { ... }
   func (h *testHandler) AssertNotLogged(t *testing.T, msg string) { ... }
   ```
4. For integration tests, configure the logger to discard all output (`slog.New(slog.DiscardHandler)`), or capture only the specific log level being tested.
5. Isolate tests from background goroutines by using `slog.New(slog.DiscardHandler)` for the main logger and a separate captured handler for the component under test.
6. Use `t.Cleanup` to restore the global logger if you capture it.

**Warning signs:**
- Tests that pass locally but fail in CI due to log output differences.
- Tests that pass when run alone but fail when run in parallel.
- Log output from health checks appearing in test assertion failures.
- Tests that use `strings.Contains` on captured log output.

**Phase to address:**
Testing phase — test logging infrastructure must be designed before writing test assertions.

---

### Pitfall 13: Protocol Mismatch in Error Logging — v1 vs. v1beta1

**What goes wrong:**
The error logging added for the v1beta1 protocol handlers (manual protowire in commits.go) uses different error paths and data structures than the v1alpha1 protocol handlers (generated Connect RPC in modulepins.go). A log analysis that assumes both paths use the same attributes or error patterns will miss errors from one path.

**Why it happens:**
The two protocol paths handle errors very differently:

- **v1alpha1** (`modulepins.go`, `bynames.go`, `blobs.go`): Uses Connect RPC handler pattern. Returns `*connect.Error` via `fmt.Errorf("...: %w", err)`. The Connect framework maps this to HTTP status codes internally. The middleware sees whatever status the framework assigns.

- **v1beta1/v1** (`commits.go`): Uses manual HTTP handling. Calls `http.Error(w, msg, statusCode)` directly. Every error path has its own hardcoded status code. The middleware sees these status codes directly.

When adding logging:
- For v1alpha1 handlers: the logger has access to the full request context and response builder at the handler level, but not at the middleware level (which only sees the HTTP status).
- For v1beta1/v1 handlers: error details are embedded in the response body string written by `http.Error()` — the middleware sees the status code but the error message is lost unless the handler logs it separately.

If you add logging at the middleware level only, v1alpha1 errors are opaque (you see status=500 but not the error detail). If you add logging at the handler level, you need

If you add logging at the middleware level only, v1alpha1 errors are opaque (you see status=500 but not the error detail). If you add logging at the handler level, you need to handle both paths separately with different attribute schemas.

**How to avoid:**
1. Add logging at the handler level for both protocol paths, NOT at the middleware level for errors. The middleware should only log timing and status at INFO level.
2. Use a common logging function that both protocol paths call, with the same attribute names: `logCommitError(ctx, owner, module, commit, err)`. Both ServeHTTP (v1beta1) and GetModulePins (v1alpha1) call this same function.
3. In the v1beta1/v1 handlers (commits.go), add structured logging alongside every `http.Error()` call. The error log should include the same fields as the v1alpha1 error logs.
4. Include a `slog.String("protocol", "v1alpha1")` or `slog.String("protocol", "v1beta1")` attribute so operators can filter by protocol path.
5. For the v1beta1/v1 handlers, log the structured error details BEFORE calling `http.Error()`, because the response body written by `http.Error()` is not accessible to the middleware.

**Warning signs:**
- Error logs from v1beta1 handlers lack `protocol` attribute.
- Log queries for errors in one protocol path return zero results while the other path shows errors.
- During incident response, operators can see the error in the response body but not in the structured logs.
- The middleware shows "status=500" with no additional context about which protocol or handler produced it.

**Phase to address:**
Logging implementation phase — must handle both protocol paths with consistent attribute schemas.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Using the same `slog.Logger` instance for all handlers without context enrichment | Simple DI, easy to add to constructors | Every log line lacks request-scoped context (request ID, user identity) | Never for production logging — acceptable only during initial development when no concurrent requests |
| Adding `log.Error("msg", "error", err.Error())` as a raw string instead of structured attributes | Quick to write, one less import | Cannot query or filter by error domain; log aggregator treats error as unstructured text | Never — structured attributes are always better |
| Logging at ERROR level for client disconnections (context cancellation) | Simple check: if error, log at ERROR | Alert fatigue — operators get paged for routine client disconnections that are not server failures | Only acceptable if a `client_disconnect` boolean attribute is added so filters can exclude them |
| No body truncation in debug logs | Full fidelity, no data loss | Disk fills in minutes, log shipper crashes, JSON parser errors | Never — always truncate body logging to a reasonable max (1-4KB) |
| Logging `r.URL.String()` directly in middleware | Shows the full request path | Query parameters in URLs may contain tokens or PII | Only for known-safe internal endpoints; never for proxy requests that forward external URLs |

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Logging every file download in GetFiles at DEBUG level | Thousands of log lines per request, high CPU in log serialization, high log write latency | Use TRACE or a custom level below DEBUG for per-file operations; log aggregate counts instead | 10+ files per repo (currently individual file logs would dominate output) |
| Calling `log.Enabled()` check before every log call | Negligible overhead individually, but in tight loops (500 file downloads * 3 log calls each = 1500 checks) adds up | Only check `Enabled()` once per handler invocation, not per-file | 100+ files per repo; high request concurrency |
| Capturing full stack traces via `slog.Source` for all log levels | `slog.HandlerOptions.AddSource = true` adds file:line to every log line, increasing allocation and serialization cost ~10x per log call | Only enable `AddSource` at Debug level; leave it off at Info and Error where it's less useful | Any production load at Info level — unnecessary allocation per log call |
| JSON log serialization on every write | The `slog.JSONHandler` serializes synchronously on every `log.Info()` call. At high throughput, this becomes a CPU bottleneck | Use async log writing or a handler that batches writes; consider a structured output format with lower overhead | 1000+ log lines/second (log shipping becomes visible in CPU profiles) |
| Middleware reading and buffering entire request body for all requests | Logging middleware reads `r.Body` into memory for every request to enable body logging, doubling memory usage per request | Only buffer request body when debug logging is enabled; use `httputil.DumpRequest` carefully | Any production load with body logging accidentally enabled |

## Security Mistakes

Domain-specific security issues beyond general web security.

| Mistake | Risk | Prevention |
|---------|------|------------|
| Logging full GitHub API error messages that include request URLs | API tokens in `Authorization` headers or query params appear in plaintext in logs | Redact URLs in error messages at the provider layer; use structured fields for safe values |
| Logging protobuf request bodies that contain module names or paths | If a module name encodes sensitive internal project information (e.g., `acme-corp-secret-project`), it becomes visible in logs | Treat all module names as potentially sensitive; log owner/ref only, not full module paths unless specifically configured |
| Logging `X-Request-Id` header from client | Malicious clients can inject request IDs with special characters (SQL injection via log injection) | Sanitize the request ID to alphanumeric + hyphens only; truncate to reasonable length (64 chars) |
| Logging response bodies that contain file contents | Downloaded .proto files could contain API keys, internal URLs, or sensitive comments embedded in the proto files | Never log file contents; log only file paths and hashes at debug level |
| Storing logs longer than necessary with sensitive data | Regulatory compliance violation (GDPR, SOC2) if PII or secrets are retained in logs longer than policy allows | Implement log retention policies that align with data classification; add a `redacted` marker attribute for lines that contained sensitive data |
| Logging `RemoteAddr` from HTTP request without checking X-Forwarded-For | Internal IP addresses and network topology exposed in logs | The existing `getClientIP` helper handles this correctly — ensure all IP logging uses this helper, not raw `r.RemoteAddr` |

## UX Pitfalls

Common user experience mistakes in this domain.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Debug logs include too much detail to find the actual problem | Operator spends 10 minutes scrolling through file-level download logs before finding the actual error line | Log errors at ERROR level with full context, and keep per-file operations at TRACE level (not DEBUG) |
| Error log lines don't include the request ID | Operator sees "cache get failed" but cannot correlate it to any specific user request or incident | Always include request ID (or "health-check" for background jobs) in every log line |
| Log messages are cryptic or use internal terminology | Operators unfamiliar with the codebase cannot understand what "module pin resolution failed" means | Use clear, operation-oriented messages: "failed to resolve module version" instead of "module pin resolution failed" |
| Different log formats for startup vs. request logs | Operators must use different query syntax to search startup errors vs. request errors | Use the same structured format (JSON with consistent keys) for all log output |
| No summary log line for cache operations | Operator sees 20 "cache put" lines for a request but no aggregate "cache: X hits, Y misses, Z errors" summary | Add a summary log line at the end of each request showing cache performance metrics |

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **Request ID propagation:** Logger is injected at handler level, but request ID from middleware is NOT in the handler's context. Check: does every new `log.Error()` call in handlers include a `request_id` attribute?
- [ ] **Error redaction:** Provider-level error chains are redacted for sensitive URLs/credentials. Check: does a GitHub API 401 error produce a log with the URL containing the token?
- [ ] **Body logging truncation:** Debug body logging has a size limit (e.g., 1024 bytes). Check: what happens when a 50MB protobuf body arrives and debug logging is enabled?
- [ ] **Context cancellation distinction:** Error logs distinguish "client disconnected" from "API call failed." Check: does a cancelled context produce an ERROR-level or DEBUG-level log?
- [ ] **Double-logging prevention:** Handler and middleware don't both log the same error at ERROR level. Check: count ERROR-level log lines for a single failed request — is it 1 or 2+?
- [ ] **Dynamic log level:** Log level can be changed at runtime without restart. Check: is there an HTTP endpoint or signal handler for changing the log level?
- [ ] **Log attribute naming convention:** All new log calls use the same attribute names. Check: is there a documented convention and are there helper constants?
- [ ] **Per-file logging is TRACE, not DEBUG:** The file-level download and hash operations are logged at TRACE (a custom level below DEBUG), not at DEBUG. Check: does enabling DEBUG flood the logs with per-file entries?
- [ ] **Panic recovery in logging middleware:** A panic in logging code does not crash the server. Check: is there a `recover()` in the middleware that catches logging panics?
- [ ] **Health check logs have identifiable request ID:** Background health check goroutines log with `request_id: "health-check"` or similar. Check: can you distinguish health check logs from request logs?
- [ ] **Connect handler `a.log` field is actually used:** The `api.log` field in `connect/api.go` is no longer dead code. Check: is it used in at least one handler method?

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Sensitive data leaked in logs (credentials, tokens, PII) | HIGH — security incident, possible disclosure | 1) Rotate all exposed credentials immediately 2) Purge log entries containing the sensitive data from log storage 3) Add automated log scanning for credential patterns 4) Fix the error chain redaction 5) Document incident in security post-mortem |
| Log volume explosion fills disk | MEDIUM — service degradation or outage | 1) Change log level to ERROR only via dynamic endpoint or SIGHUP 2) Rotate/archive current logs to free disk 3) Reduce retention period temporarily 4) Add log rate limiting 5) Implement per-component log levels as permanent fix |
| Double logging floods log aggregator | LOW — cost increase, noise | 1) Suppress middleware error logging temporarily via config change 2) Fix handler-level logging to use the single logging layer approach 3) Deploy fix 4) Reduce log retention to clear accumulated duplicate data |
| Panic in logging code causes 500 errors | MEDIUM — service errors | 1) Deploy fix with `recover()` in logging middleware 2) If current deployment cannot be replaced, use a reverse proxy to strip the debug endpoint or disable the logging middleware via config change 3) Post-mortem: identify the root cause (nil pointer, type assertion) |
| Log output corrupted by binary protobuf data | LOW — log aggregator errors, missing data | 1) Disable body logging immediately 2) Purge corrupted log entries 3) Fix body logging to use hex encoding + truncation 4) Add automated log format validation to catch similar issues |

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Pitfall 1: Sensitive data through error chains | Logging implementation phase — audit error chains and add redaction before any new `log.Error()` calls | Inject a known credential value into a provider error path and verify it is redacted in the log output (write a unit test) |
| Pitfall 2: Double logging of errors | Logging infrastructure phase — decide logging boundary (handler-level only, middleware-level only, or deduplication) BEFORE writing any log code | Start the server, trigger a 400 error, count ERROR-level log lines — must be exactly 1 |
| Pitfall 3: Protobuf body logging without truncation | Tracing/logging detail phase — design body logging strategy before implementing | Write a test that sends a 1MB body with debug logging enabled and verify log output is truncated to <4096 bytes |
| Pitfall 4: Error context not captured where it originates | Provider logging phase — add structured logging at each provider boundary BEFORE handler-level logging | Trigger a GitHub API error (invalid token) and verify the log shows provider type, HTTP status, and duration — not just "error: ..." |
| Pitfall 5: Missing request ID propagation | Logging infrastructure phase — context propagation design first | Write a concurrent test with 10 simultaneous requests and verify each request's log lines have a unique request_id |
| Pitfall 6: Credentials in startup validation | Logging implementation phase — credential redaction in startup logs | Run the server with an invalid GitHub token and verify the startup error log does not contain the token string |
| Pitfall 7: Panic in logging code | Logging infrastructure phase — defensive logging patterns | Write a unit test that passes a nil context value through the logging path and verifies no panic occurs |
| Pitfall 8: Log volume explosion | Tracing/logging detail phase — design log levels and volume expectations | Enable debug logging, run a full `buf mod update` command, measure total log line count — must be <100 lines per module ref |
| Pitfall 9: Logging after context cancellation | Provider logging phase — context cancellation handling | Write a test that simulates a slow provider (2s delay) and a client disconnection (500ms timeout), verify the error is logged at DEBUG not ERROR |
| Pitfall 10: Inconsistent log attributes | Logging infrastructure phase — naming convention and helper constants | Write a linter or CI check that scans for hardcoded string attribute names that don't match the convention |
| Pitfall 11: Log level not dynamically configurable | Logging infrastructure phase — dynamic level control | Add a test that changes the log level via HTTP endpoint and verifies debug logs appear/disappear without restart |
| Pitfall 12: Flaky test assertions | Testing phase — test logging infrastructure | Write a test that captures log output with a thread-safe handler and verifies structured attributes, then run it with `-count=10 -race` to confirm it's not flaky |
| Pitfall 13: Protocol mismatch in error logging | Logging implementation phase — consistent attribute schemas across both protocol paths | Run both a v1alpha1 and v1beta1 test case, capture the error log from each, and verify both have the same attribute schema (owner, repo, error, protocol) |

## Sources

- Codebase analysis: direct reading of all Go source files in `cmd/easyp/main.go`, `internal/connect/`, `internal/providers/` (HIGH confidence — first-hand analysis)
- `slog` package documentation: Go 1.21+ standard library `log/slog` (HIGH confidence — official Go docs)
- Connect RPC documentation: connectrpc.com/docs (MEDIUM confidence — web search, not verified via Context7)
- Structured logging best practices: Dave Cheney, "Let's Talk About Logging" (MEDIUM confidence — widely cited in Go community, but no official status)
- Go `context` package cancellation behavior: official Go blog "Pipelines and Cancellation" (HIGH confidence — official Go documentation)
- Log injection attack patterns: OWASP Log Injection Cheat Sheet (MEDIUM confidence — web search, OWASP is authoritative but may not cover Go-specific cases)
- `slog` performance characteristics: Go issue #59369 and related discussions (MEDIUM confidence — GitHub issue discussions, not official documentation)
- Protobuf wire format: protobuf.dev/programming-guides/encoding (HIGH confidence — official protobuf documentation)

---

*Pitfalls research for: Adding diagnostic logging to an existing Go Connect RPC proxy*
*Researched: 2026-06-16*
