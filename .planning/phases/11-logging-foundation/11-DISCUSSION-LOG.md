# Phase 11: Logging Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-16
**Phase:** 11-Logging Foundation
**Areas discussed:** Env var override && format env, Sensitive data redaction scope, Invalid log level behavior, Text format style

---

## Env var override && format env

| Option | Description | Selected |
|--------|-------------|----------|
| Env var overrides config | EASYP_LOG_LEVEL always wins over config file level | ✓ |
| Config wins, env fallback | Config file value takes precedence; env var is only used when config level is empty/not set | |
| Env is the only source | Drop log level from config entirely. Only use EASYP_LOG_LEVEL env var. | |

**Follow-up — format env:**
- Selected: Config-only, no env var for `EASYP_LOG_FORMAT`. Format is a deployment-time decision, not something operators change ad-hoc.

**Follow-up — env detection:**
- Selected: Explicit `os.Getenv` check after config parsing. The existing `${VAR}` substitution in YAML is a separate mechanism and stays.

**Follow-up — source env:**
- Selected: Config-only — no env var for `AddSource`. Source line info is a debug developer preference set once in config.

**User's choice:** Env var overrides config; no env for format or AddSource; explicit `os.Getenv` check.
**Notes:** The existing `os.ExpandEnv` mechanism (for `${VAR}` in YAML) stays unchanged. The new env var is a top-level override, not a substitution.

---

## Sensitive data redaction scope

| Option | Description | Selected |
|--------|-------------|----------|
| Key-name based: tokens + creds | Redact values for attributes whose key matches patterns like 'token', 'password', 'secret', 'key', 'auth', 'credential' | |
| Same as current headers only | Only redact the same headers already masked in middleware: Authorization, Cookie, X-Api-Key, Token, Password | |
| Also pattern-match values | In addition to key-name matching, scan string values for patterns matching tokens/API keys | ✓ |

**Follow-up — attr types:**
- Selected: All types — also inspect Any values. The redactor attempts string type assertions, fmt.Stringer, and encoding.TextMarshaler before falling back to passthrough.

**Follow-up — value patterns:**
- Selected: Broad — all common API token prefixes. Covers JWT (`eyJ`), GitHub (`ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`), OpenAI (`sk-`), Slack (`xox`), AWS (`AKIA`/`ASIA`), `Bearer` strings.

**Follow-up — key names:**
- Selected: All of the above + env-var-style suffixes. Covers `token`, `password`, `secret`, `key`, `auth`, `credential`, `apikey`, `api_key`, `access_key`, `private_key`, `certificate` (case-insensitive substrings), plus keys ending in `_TOKEN`, `_KEY`, `_SECRET`, `_PASSWORD`.

**User's choice:** Pattern-match values; inspect Any values; broad prefix list; cover env-var-style key suffixes.
**Notes:** `slog.Any` deep inspection is a pragmatic compromise — full reflection would be expensive. The redactor tries type assertions and falls back to passthrough. Most call sites use `slog.String(...)` so the string path covers them.

---

## Invalid log level behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Fail-fast: error + exit(1) | Print clear error listing valid values, then os.Exit(1). Zero ambiguity. | ✓ |
| Warn + fallback to INFO | Log a startup warning about the invalid value, then default to INFO. | |

**User's choice:** (Locked from ROADMAP.md success criteria #5 — "Invalid log level values produce a clear error message at startup and exit gracefully.")
**Notes:** This is a behavior change from the current silent-default-to-INFO logic in `newLogger`. The new code must validate the value and exit with a non-zero status. Both `warn` and `warning` are accepted (case-insensitive) for backward compatibility.

---

## Text format style

| Option | Description | Selected |
|--------|-------------|----------|
| Standard slog.NewTextHandler | Default slog.NewTextHandler with time=level=msg=key=value format. No customization. | ✓ |
| Custom format with shortened keys | Stripped-down custom handler: short keys (t= l= m=), no quotes around values unless needed, fixed-width timestamps. | |
| JSON as default, no text | Text output is just JSON-formatted as human-readable multi-line. | |

**User's choice:** Standard `slog.NewTextHandler` with no customization. Idiomatic Go ecosystem output, no custom styling.
**Notes:** Matches the standard Go slog ecosystem. Custom styling is overkill for a server proxy where logs are mostly piped through `jq` or read via structured log aggregators.

## Claude's Discretion

- Location of the redaction logic (inline in `main.go` vs. new `internal/logging/` package) — pick based on file size; prefer extraction if `newLogger` grows past ~80 lines.
- Test coverage scope: at minimum, unit tests for ReplaceAttr (key patterns, value patterns, case sensitivity, Any values). Optional: integration test of the full logger pipeline.
- Whether to expose a helper `redactValue(s string) string` for reuse in places that build log attributes outside of slog.

## Deferred Ideas

- Removing `internal/logger/logger.go` (the duplicate, unused global logger) — noted in CONCERNS.md. Out of milestone scope; cleanup task for a future phase.
- Dynamic log level via `slog.LevelVar` + signal handler (SIGUSR1) — explicitly in REQUIREMENTS.md v1.4 (INFR-04).
- Log sampling / rate limiting — out of scope per REQUIREMENTS.md.
- Admin HTTP endpoint for log level — out of scope per REQUIREMENTS.md.
