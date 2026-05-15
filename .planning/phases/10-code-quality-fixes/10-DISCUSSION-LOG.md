# Phase 10: Code Quality Fixes - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-09
**Phase:** 10-code-quality-fixes
**Areas discussed:** Critical bugs, Code duplication cleanup, Security hardening, Performance & operations

---

## Critical Bugs

| Option | Description | Selected |
|--------|-------------|----------|
| Return nil on errors (Recommended) | Safe defaults — return empty on error instead of partial data | ✓ |
| Keep partial results with logging | Log and continue — return partial data so operators can see what failed | |
| Both error + data (with warning) | Return both so error propagation is clear but data is available | |

**User's choice:** Return nil on errors (Recommended)
**Notes:** Applies to GetFiles(), resolveModulePins(), and all provider methods

---

## splitRepoName() Fix

| Option | Description | Selected |
|--------|-------------|----------|
| Return (owner, repo, error) | Cleaner error propagation — caller decides response code | |
| Check len but return (string, string) | No signature change — fix with length check but callers stay simple | ✓ |

**User's choice:** Check len but return (string, string)
**Notes:** Simpler change, keep existing API signature

---

## BitBucket Template Panic

| Option | Description | Selected |
|--------|-------------|----------|
| Return (string, error) (Recommended) | Proper error handling — tmplExec returns (string, error) | |
| Keep panic + add middleware | Safer — add panic recovery middleware (defensive) | |
| Leave as-is (risky) | Keep current panic behavior — these are static templates | ✓ |

**User's choice:** Leave as-is (risky)
**Notes:** Static templates mean low risk. Not a priority fix.

---

## Code Duplication: fileFiltered

| Option | Description | Selected |
|--------|-------------|----------|
| Create shared download helper (Recommended) | Extract to internal/providers/content/download.go | ✓ |
| Leave duplicated (keep it simple) | Add comments and accept duplication — simple enough to maintain | |
| Full abstraction with interfaces | Use interface + strategy pattern — most flexible | |

**User's choice:** Create shared download helper
**Notes:** Extract to internal/providers/content/download.go

---

## Code Duplication: ConfigHash

| Option | Description | Selected |
|--------|-------------|----------|
| Use filter.Repo.Hash() everywhere (Recommended) | Use filter.Repo.Hash() everywhere — consistent, single definition | ✓ |
| Leave providers with their own implementation | Each provider has its own hash method — may differ intentionally | |
| Extract to content package | Single shared function in content package — most explicit | |

**User's choice:** Use filter.Repo.Hash() everywhere
**Notes:** Consolidate to filter.Repo.Hash() method

---

## Unused Logger Package

| Option | Description | Selected |
|--------|-------------|----------|
| Remove logger package (Recommended) | Remove internal/logger/ entirely — main.go already has proper DI pattern | ✓ |
| Keep with deprecation notice | Keep but deprecate it — might be useful for simple tools later | |
| Keep and maintain properly | Add a test that verifies it works so it becomes supported | |

**User's choice:** Remove logger package
**Notes:** Dead code, creates confusion about logging approach

---

## HTTP Client Timeouts

| Option | Description | Selected |
|--------|-------------|----------|
| Per-request timeout (Recommended) | 10-30s per request — covers most slow responses without hanging | |
| Global client timeout | Global client timeout only — simpler but less flexible | |
| Configurable per provider | Per-provider timeout config via YAML — most flexible | ✓ |

**User's choice:** Configurable per provider
**Notes:** Via YAML configuration. Default 30 seconds.

---

## Response Body Size Limits

| Option | Description | Selected |
|--------|-------------|----------|
| 50MB limit with LimitReader (Recommended) | Cap at 50MB — handles large proto repos without memory issues | |
| 10MB limit | Conservative 10MB — safe for proto files | |
| Different limits per usage | Tiered limits — 1MB for errors, 50MB for content | |
| configurable limit | User selected "configurable limit" | ✓ |

**User's choice:** configurable limit
**Notes:** Make it configurable via YAML

---

## Parallel File Downloads

| Option | Description | Selected |
|--------|-------------|----------|
| Bounded goroutine pool (Recommended) | 10-20 concurrent downloads — good balance for typical repos | |
| Configurable pool size | Configurable max concurrent (10-100) via YAML | |
| Keep sequential for now | Start with sequential, add parallelism only if needed | ✓ |

**User's choice:** Keep sequential for now
**Notes:** Simplicity over complexity for now

---

## Response Compression

| Option | Description | Selected |
|--------|-------------|----------|
| Add gzip compression (Recommended) | Compress large responses (50KB+) — reduces bandwidth but adds latency | |
| Skip compression for now | Keep uncompressed — simpler, sufficient for small responses | ✓ |

**User's choice:** Skip compression for now
**Notes:** Not a priority

---

## Test Coverage

| Option | Description | Selected |
|--------|-------------|----------|
| Test critical bugs only (Recommended) | Add tests for critical bugs only — fastest path to confidence | |
| Full unit test suite | Comprehensive coverage — all public APIs tested | ✓ |
| Happy path tests only | Test happy paths only — verify it works, not edge cases | |

**User's choice:** Full unit test suite
**Notes:** Establish confidence and prevent regressions

---

## Graceful Shutdown

| Option | Description | Selected |
|--------|-------------|----------|
| Add graceful shutdown (Recommended) | Handle SIGTERM gracefully — drain in-flight requests | |
| Skip for now | Keep current behavior — simpler | ✓ |

**User's choice:** Skip for now
**Notes:** Will address when needed for production

---

## Claude's Discretion

- Response body size limits: Claude has flexibility to set sensible defaults (50MB) while making limits configurable
- HTTP client implementation details: Claude can choose appropriate `http.Client` setup per provider
- Artifactory Put status check fix: The fix is straightforward — no discretion needed

## Deferred Ideas

- **Parallel file downloads** — Keep sequential for now, consider goroutine pool in future performance phase
- **Response compression** — Skip for now
- **Graceful shutdown** — Skip for now
- **BitBucket template panic fix** — Low risk, not a priority
- **Modern buf protocol** — Already tracked in draft.txt
