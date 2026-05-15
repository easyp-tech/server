# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.2 — Dependency Modernization

**Shipped:** 2026-05-10
**Phases:** 5 | **Plans:** 10 | **Commits:** 32

### What Was Built

- Go 1.26 upgrade with all dependencies at latest compatible versions
- Proto code regenerated from buf v1.69.0 with connect-go v1.19.x
- Deprecated `golang.org/x/exp` imports replaced with stdlib
- Old buf submodule (v1.9.0) removed, buf-v1.69.0 promoted to canonical
- 5 critical bugs fixed (panic, inverted checks, partial results)
- HTTP hardening (30s timeout, 50MB body limit) + shared download helper
- Unit test suite with 14 tests across 4 packages

### What Worked

- Wave-based parallel execution for code quality fixes — 4 waves completed efficiently
- Bug fixes first, then refactoring, then tests — logical ordering prevented rework
- Shared download helper extraction eliminated real duplication (GitHub + BitBucket)
- ConfigHash standardization caught potential cache key divergence across providers

### What Was Inefficient

- Artifactory PUT inverted status check was a pre-existing bug masked by success path — should have been caught in v1.1 code review
- Phase 10 scope grew beyond original "dependency modernization" into code quality — but worth it given bugs found

### Patterns Established

- All error paths return `nil` on error, never partial results
- All providers use `r.repo.Hash()` for ConfigHash (single source of truth)
- HTTP clients have configurable timeout and body limit at construction time
- Unit tests focus on bug fix surfaces and critical API boundaries

### Key Lessons

1. After dependency upgrades, run a code quality pass — version bumps expose latent bugs that were masked by older library behavior
2. Submodule cleanup (rename via `git mv`) should happen immediately after validation to prevent confusion about which proto source is canonical
3. Shared helper extraction works well when two providers have identical download-hash-accumulate logic — generics (`FilterEntries[T]`) handle struct differences cleanly
4. Inverted boolean conditions (`< 200 && >= 300` instead of `>= 300`) are invisible to tests unless you test the failure path explicitly

### Cost Observations

- Model mix: 100% opus
- Sessions: 2 (2026-05-07/08 for phases 6-9, 2026-05-09/10 for phase 10 + UAT)
- Notable: Phase 10 (code quality) was unplanned in original v1.2 scope but delivered 5 bug fixes and 14 tests — high ROI for a single phase

---

## Milestone: v1.1 — Protocol Modernization

**Shipped:** 2026-05-07
**Phases:** 5 | **Plans:** 8

### What Was Built

- v1beta1 API handlers for modern buf CLI (GetCommits, GetGraph, Download, GetModules)
- B4 digest computation (SHAKE256)
- In-memory caching across RPC chain
- IPv4-only GitHub transport for macOS compatibility
- E2E tests for both buf v1.30.1 and v1.69.0+

### What Worked

- Protocol-first approach: analyze proto diff before implementation
- Real buf binary testing caught real protocol issues
- Dual-protocol architecture (v1alpha1 + v1beta1) proved backward compatible

### What Was Inefficient

- E2E tests requiring live GitHub token limited validation coverage
- Test infrastructure race conditions with shared API quota caused flaky v1.69.0 smoke test

### Patterns Established

- Real buf binary + real TLS server + real GitHub API for E2E tests
- Dependency-injected `*slog.Logger` as the logging pattern (not global logger)

### Key Lessons

1. Proto diff analysis before coding saves implementation time
2. Test infrastructure needs its own GitHub API quota to avoid parallel test interference

### Cost Observations

- Model mix: 100% opus
- Sessions: 3

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.1 | 3 | 5 | Established protocol-first development |
| v1.2 | 2 | 5 | Added post-upgrade code quality pass |

### Cumulative Quality

| Milestone | Tests | UAT Passed | Bug Fixes |
|-----------|-------|------------|-----------|
| v1.1 | 5 E2E | N/A | 0 |
| v1.2 | 14 unit + 5 E2E | 9/9 | 5 |

### Top Lessons (Verified Across Milestones)

1. Test the failure paths, not just the happy paths — inverted conditions and partial results are invisible to success-only tests
2. After major infrastructure changes (dependency upgrades, proto regeneration), schedule a code quality audit
3. Shared helper extraction should happen when duplication appears in two places — three is too late
