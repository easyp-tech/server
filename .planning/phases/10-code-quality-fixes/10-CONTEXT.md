# Phase 10: Code Quality Fixes - Context

**Gathered:** 2026-05-09
**Status:** Ready for planning

<domain>
## Phase Boundary

Fix critical bugs, clean up code duplication, harden security, and address performance/operational issues in the codebase. This phase focuses on improving code quality without adding new features.
</domain>

<decisions>
## Implementation Decisions

### Critical Bug Fixes

- **D-01:** When functions like `GetFiles()` or `resolveModulePins()` encounter errors, they MUST return `nil` (not partial results). This applies to all providers and the multisource layer.
- **D-02:** `splitRepoName()` will check array length before index access but keep the `(string, string)` signature (no error return). Callers handle malformed input gracefully.
- **D-03:** BitBucket template execution keeps `panic()` behavior. Static templates mean this is low risk. Not a priority fix.

### Code Duplication Cleanup

- **D-04:** Extract shared download helper to `internal/providers/content/download.go` containing the `fileFiltered` type and hash-accumulate logic. All three providers (github, bitbucket, localgit) will use this.
- **D-05:** All providers MUST use `filter.Repo.Hash()` consistently for ConfigHash(). Remove duplicate implementations from github, bitbucket, and localgit providers.
- **D-06:** Remove `internal/logger/` package entirely. The dependency-injected logger pattern in `main.go` is the correct approach.

### Security Hardening

- **D-07:** Add configurable per-provider HTTP client timeout via YAML configuration. Default 30 seconds.
- **D-08:** Add configurable response body size limits using `io.LimitReader`. Default 50MB, configurable per provider.

### Performance & Operations

- **D-09:** Keep sequential file downloads for now. Parallelism will be considered in a future phase if performance issues arise.
- **D-10:** Skip response compression for now. Not a priority.
- **D-11:** Skip graceful shutdown for now. Will be addressed when needed for production deployment.
- **D-12:** Implement a full unit test suite for all public APIs. This is a priority to establish confidence and prevent regressions.

### Claude's Discretion

- Response body size limits: Claude has flexibility to set sensible defaults (50MB) while making limits configurable
- HTTP client implementation details: Claude can choose appropriate `http.Client` setup per provider
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Code Quality
- `.planning/codebase/REVIEW.md` — Full code review with 43 issues identified across the codebase
- `.planning/codebase/CONCERNS.md` — Prior concerns analysis (some items now resolved)

### Architecture
- `.planning/codebase/ARCHITECTURE.md` — System architecture and component relationships
- `.planning/codebase/STRUCTURE.md` — Project structure and package organization

### Codebase Patterns
- `.planning/codebase/CONVENTIONS.md` — Coding conventions and style guidelines

### Configuration
- `cmd/easyp/internal/config/config.go` — Config structure that needs timeout/body-size fields
- `local.config.yml` — Example configuration for reference
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `filter.Repo.Hash()` — Already implemented, should be used consistently
- `shake256.SHA3Shake256()` — Used for content hashing, ready for shared download helper
- `content.File` struct — Standard file representation across all providers

### Established Patterns
- Dependency injection via `*slog.Logger` — Standard pattern in main.go for all providers
- Provider interface pattern — `source.Source` and `multisource.Provider` interfaces established
- Config-driven setup — All providers initialized via config structs

### Integration Points
- New download helper in `internal/providers/content/` needs to integrate with github, bitbucket, localgit providers
- Config changes need to extend `cmd/easyp/internal/config/config.go` and read.go
- Test suite needs to cover all public API surfaces

### Files Affected by Bug Fixes
- `internal/providers/cache/artifactory/artifactory.go:121` — Fix inverted status code check
- `internal/connect/bynames.go:87-91` — Fix splitRepoName panic
- `internal/providers/multisource/repo.go:74-77` — Return nil on error
- `internal/connect/modulepins.go:30-43` — Return nil on error
- `internal/providers/github/getfiles.go:37-80` — Use shared helper
- `internal/providers/bitbucket/getfiles.go:33-74` — Use shared helper
- `internal/providers/localgit/localgit.go:176-216` — Use shared helper
- `internal/providers/github/repos.go:68-69` — Use filter.Repo.Hash()
- `internal/providers/bitbucket/repos.go:76-77` — Use filter.Repo.Hash()
- `internal/providers/localgit/localgit.go:111-112` — Use filter.Repo.Hash()
- `internal/logger/logger.go` — Delete entire package
</code_context>

<specifics>
## Specific Ideas

- **Artifactory Put Fix:** Change `resp.StatusCode < http.StatusOK && resp.StatusCode >= http.StatusMultipleChoices` to `resp.StatusCode >= http.StatusMultipleChoices`
- **Config Hash Consistency:** GitHub/BitBucket currently use `r.repo.Repo` (nested), localgit uses `r.repo` (direct) — all should use `r.repo.Hash()` directly
- **Test Coverage Priority:** Focus on testing the critical bugs that were fixed
</specifics>

<deferred>
## Deferred Ideas

### Ideas Deferred to Future Phases

- **Parallel file downloads** — User chose to keep sequential for now. Consider goroutine pool (10-20 concurrent) in a performance optimization phase.
- **Response compression** — Skip for now. Add gzip middleware if bandwidth becomes an issue.
- **Graceful shutdown** — Skip for now. Add signal handling when deploying to production environments.
- **BitBucket template panic fix** — Low risk due to static templates. Not a priority.
- **Modern buf protocol implementation** — Already tracked in draft.txt, belongs in its own phase.

### Already Resolved

- `golang.org/x/exp` imports — **FIXED** in prior work. Codebase now uses stdlib `log/slog` and `slices`.

---

*Phase: 10-Code Quality Fixes*
*Context gathered: 2026-05-09*
