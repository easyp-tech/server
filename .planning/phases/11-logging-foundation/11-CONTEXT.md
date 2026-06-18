# Phase 11: Logging Foundation - Context

**Gathered:** 2026-06-16
**Status:** Ready for planning

## Phase Boundary

Operators can configure log level, format, and source info, with centralized sensitive-data redaction applied to all log output via `slog.HandlerOptions.ReplaceAttr`. This phase locks down the logger setup that phases 12–15 build on (correlation IDs, Connect RPC interceptor, handler error logs, provider tracing, panic recovery). The redaction layer is the contract every downstream log call inherits.

## Requirements (locked via REQUIREMENTS.md)

**4 requirements are locked.** See `.planning/REQUIREMENTS.md` for full v1.3 requirements.

This phase implements:
- **FOUND-01:** `EASYP_LOG_LEVEL` env var override (debug, info, warn, error)
- **FOUND-02:** Central redaction via `slog.HandlerOptions.ReplaceAttr` before any new log calls are added
- **FOUND-03:** Log output format togglable between text and JSON via LogConfig
- **FOUND-04:** Optional `AddSource` via LogConfig

Downstream agents MUST read `.planning/REQUIREMENTS.md` and `.planning/ROADMAP.md` (Phase 11 section) for success criteria before planning. Requirements are not duplicated here.

**In scope:** `newLogger()` refactor, `LogConfig` field additions, env var override, ReplaceAttr implementation, startup validation, replacing the existing `loggingMiddleware`-local `maskSensitiveHeaders` with the global handler-level redaction.

**Out of scope:** Per-call request_id propagation (Phase 12), Connect RPC interceptor (Phase 12), handler error logs (Phase 13), provider debug tracing (Phase 14), panic recovery (Phase 15), `internal/logger/logger.go` removal (out of milestone scope, see CONCERNS.md).

## Implementation Decisions

### Environment variable override

- **D-01:** `EASYP_LOG_LEVEL` always wins over the config file value. Check via `os.Getenv("EASYP_LOG_LEVEL")` immediately after `config.ReadYaml` and before constructing the logger. The config file's `log.level` is used as fallback when the env var is unset.
- **D-02:** No env var for `EASYP_LOG_FORMAT` — format is config-only via `LogConfig.Format`. Format is a deployment-time decision; an env override is unnecessary noise. (User explicit choice.)
- **D-03:** `AddSource` is config-only. No env var. Source info is a developer debug preference set once in config. (User explicit choice.)

### Sensitive data redaction (ReplaceAttr)

- **D-04:** `ReplaceAttr` is implemented as a top-level function in `cmd/easyp/main.go` (or extracted to `internal/logging/redact.go` if it grows — Claude's discretion). It runs unconditionally on every log line produced by the configured handler, including the HTTP middleware, the request correlation logs in Phase 12, the handler error logs in Phase 13, and the provider traces in Phase 14.
- **D-05:** Key-name redaction (case-insensitive) covers these substrings: `token`, `password`, `secret`, `key`, `auth`, `credential`, `apikey`, `api_key`, `access_key`, `private_key`, `certificate`. Plus any key ending in `_TOKEN`, `_KEY`, `_SECRET`, `_PASSWORD` (catches env-var-style names like `GITHUB_TOKEN`, `ARTIFACTORY_KEY`).
- **D-06:** Value-pattern redaction covers: JWT `eyJ...` prefix, GitHub `ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`, OpenAI `sk-`, Slack `xox`, AWS `AKIA`/`ASIA`, and any string value containing `Bearer ` followed by non-whitespace.
- **D-07:** ReplaceAttr inspects both string and `slog.Any` attribute values. For Any values, the redactor attempts to extract string fields via reflection (or by type-asserting to `string`/`fmt.Stringer`/`encoding.TextMarshaler`). Non-string Any values are left as-is to avoid expensive deep inspection. (User explicit choice over strings-only.)
- **D-08:** Redaction string is `[REDACTED]`. Single short form keeps JSON/text output scannable.
- **D-09:** The existing `maskSensitiveHeaders` function in `main.go:220` is REMOVED. With global ReplaceAttr in place, the middleware-level masking becomes redundant. The middleware continues to call `r.Header.Clone()` for the `headers` debug attribute, but the redactor catches the sensitive values.

### Invalid log level behavior

- **D-10:** Invalid `EASYP_LOG_LEVEL` or config-file `log.level` values cause fail-fast startup. The error message lists valid values: `debug`, `info`, `warn`, `error`. Process exits with `os.Exit(1)` and a non-zero status. (Per ROADMAP success criteria #5.) This is a behavior change from the current silent-default-to-INFO.
- **D-11:** `warn` and `warning` are both accepted (matching current behavior). Case-insensitive matching.

### Log format (text vs JSON)

- **D-12:** Text format uses the standard `slog.NewTextHandler` with no customization. Output is `time=2026-... level=INFO msg="..." key=value`. (User explicit choice — idiomatic Go, no custom styling needed.)
- **D-13:** Default format is `text` (per ROADMAP success criteria #3 — "default is human-readable text"). JSON requires `log.format: json` in config.
- **D-14:** `LogConfig` adds two new fields: `Format string` (values: `text`, `json`, default `text`) and `AddSource bool` (default `false`).

### LogConfig structure

- **D-15:** All new config lives in the existing `LogConfig` struct in `cmd/easyp/internal/config/config.go`. New fields: `Format string \`json:"format"\`` and `AddSource bool \`json:"addSource"\``. The `Level` field stays as-is.
- **D-16:** Env var overrides are applied in `main.go` after config parsing, not via the existing `${VAR}` mechanism. This is a separate code path that explicitly checks `os.Getenv("EASYP_LOG_LEVEL")` and only overrides `log.level` if non-empty.

### Claude's Discretion

- Exact location of the redaction logic (inline in `main.go` vs. new `internal/logging/` package) — pick based on file size, prefer extraction if `newLogger` grows past ~80 lines.
- Test coverage scope: at minimum, unit tests for ReplaceAttr (key patterns, value patterns, case sensitivity, Any values). Optional: integration test that exercises the full logger pipeline.
- Whether to expose a helper `redactValue(s string) string` for reuse in places that build log attributes outside of slog (e.g., when building an error string).

## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` — v1.3 Logging Foundation requirements (FOUND-01 through FOUND-04)
- `.planning/ROADMAP.md` — Phase 11 success criteria
- `.planning/PROJECT.md` — Project context and prior decisions

### Existing Code Touched
- `cmd/easyp/main.go` lines 12, 39–70, 130–146 — `newLogger()` is the refactor target; `loggingMiddleware` at lines 150–206 has the per-handler `maskSensitiveHeaders` call that becomes redundant
- `cmd/easyp/main.go` line 220 — `maskSensitiveHeaders` function to be removed
- `cmd/easyp/internal/config/config.go` — `LogConfig` struct (line 21) to receive `Format` and `AddSource` fields
- `cmd/easyp/internal/config/read.go` — env var expansion via `os.ExpandEnv` (DO NOT use this for `EASYP_LOG_LEVEL` — use `os.Getenv` directly per D-16)

### Go Standard Library
- `log/slog` package — `slog.NewTextHandler`, `slog.NewJSONHandler`, `slog.HandlerOptions.ReplaceAttr` (Go 1.21+)

## Existing Code Insights

### Reusable Assets
- `newLogger(level string)` at `cmd/easyp/main.go:130` — to be refactored into `newLogger(cfg LogConfig, envLevel string)`. The level-parsing switch (debug/info/warn/error) is reusable.
- `maskSensitiveHeaders` at `cmd/easyp/main.go:220` — the existing list of header names (Authorization, Cookie, X-Api-Key, Token) becomes part of the broader key-name redaction set in D-05.

### Established Patterns
- **Logger is dependency-injected:** `main.go:43` creates the logger and passes it through to all providers, cache, and handlers. The new `newLogger` signature must return `*slog.Logger` so injection sites don't change.
- **Config struct pattern:** All config is one `Config` struct in `cmd/easyp/internal/config/config.go`. Adding fields to `LogConfig` follows the same pattern used for `Cache` and `TLS`.
- **`os.ExpandEnv` for `${VAR}` in config files** is a separate mechanism from direct env override. Both can coexist; env override happens after ExpandEnv runs.

### Integration Points
- `main.go:43` — `log = newLogger(cfg.Log.Level)` is the call site that changes. Becomes `log = newLogger(cfg.Log, os.Getenv("EASYP_LOG_LEVEL"))`.
- `main.go:43` — `cfg.Log.Level` reads from the parsed config. Env var override must happen between config read and logger construction.
- Phase 12 (correlation ID) will add a `slog.Logger.With("request_id", ...)` call. ReplaceAttr must preserve this attribute.
- Phase 13 (handler error logs) will use `log.ErrorContext(ctx, ...)` and `slog.String("owner", ...)` etc. ReplaceAttr must not redact `owner`, `repo`, `name`, `error`, `protocol`, `request_id`, `commit` — these are non-sensitive identifiers.

## Specific Ideas

- The `EASYP_LOG_LEVEL` env var should be documented in the `local.config.yml` example or a comment near the `log.level` field, so operators know it overrides the config value.
- The standard `slog.NewTextHandler` output reads as: `time=2026-06-16T10:00:00.000-04:00 level=INFO msg="cache access verified" type=*cache.Noop`. This matches the Go ecosystem convention.
- Redaction tests should include a known-bad string like `token=ghp_abc123def456` and assert the output is `token=[REDACTED]`.
- The `slog.Any` handling in D-07 is a pragmatic compromise: full reflection is expensive, so the redactor attempts a few common type assertions and falls back to passthrough. Most call sites pass `slog.String(...)` so the string path covers them.

## Deferred Ideas

- Removing `internal/logger/logger.go` (the duplicate, unused global logger) — noted in CONCERNS.md. Out of milestone scope; cleanup task for a future phase.
- Dynamic log level via `slog.LevelVar` + signal handler (SIGUSR1) — explicitly in REQUIREMENTS.md v1.4 (INFR-04).
- Log sampling / rate limiting — out of scope per REQUIREMENTS.md.
- Admin HTTP endpoint for log level — out of scope per REQUIREMENTS.md.

---

*Phase: 11-Logging Foundation*
*Context gathered: 2026-06-16*
