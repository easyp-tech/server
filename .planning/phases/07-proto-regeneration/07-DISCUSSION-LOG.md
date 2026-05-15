# Phase 7: Proto Regeneration & Verification - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-08
**Phase:** 07-Proto-Regeneration
**Areas discussed:** Handler struct migration, Full vs selective regeneration, Breaking change audit, E2E test adjustments

---

## Handler struct migration

| Option | Description | Selected |
|--------|-------------|----------|
| Regenerate then update embed lines | Run `go generate`, update the embedded types to match the new generated names, done. Fast and low-risk. | ✓ |
| Let build errors reveal gaps | Let the compiler guide us — run the generator, see what breaks (likely just the embed types), fix them one by one. | |
| Diff the generated types first | First check what changed in the generated Unimplemented types — new methods? Changed signatures? Audit the diff before touching handler code. | |

**User's choice:** Regenerate then update embed lines
**Notes:** Preference for mechanical, straightforward approach. Generate first, update embed lines second.

---

## Full vs selective regeneration

| Option | Description | Selected |
|--------|-------------|----------|
| Full regeneration (recommended) | Regenerate all proto files at once (current behavior via `buf generate`). Ensures consistency across all services, catches cross-service issues. | ✓ |
| Selective by usage | Regenerate only the services the proxy actually uses (ResolveService, RepositoryService, DownloadService, plus v1beta1 module services). | |

**User's choice:** Full regeneration (recommended)
**Notes:** Prefer consistency across all services. No thinking required — just run `go generate`.

---

## Breaking change audit

| Option | Description | Selected |
|--------|-------------|----------|
| Compile and fix iteratively | Compile first, let type errors reveal breaking changes. Fastest path to a working build. | ✓ |
| Diff generated code upfront | Before compiling, review the newly generated code for API differences. | |
| Trust tests to catch issues | Compile and run E2E tests — if tests pass, any hidden API changes didn't break existing functionality. | |

**User's choice:** Compile and fix iteratively
**Notes:** Pragmatic approach — let the compiler drive the fixes. Fastest path to a working build.

---

## E2E test adjustments

| Option | Description | Selected |
|--------|-------------|----------|
| Run existing tests unchanged | Run existing E2E tests as-is after regeneration. If they pass, the code changes are solid. | ✓ |
| Check test compilation | After regeneration, verify E2E tests still compile and pass without modification. | |
| Enhance test coverage | Update E2E tests to cover edge cases that might have changed with the new generated code. | |

**User's choice:** Run existing tests unchanged
**Notes:** Tests already cover both buf versions. Trust them to validate the regeneration.

---

## Claude's Discretion

- Specific order of fixing compilation errors after regeneration — let the compiler guide the sequence
- Whether to commit generated code separately or together with handler fixes
- golangci-lint version compatibility with Go 1.26 after regeneration

## Deferred Ideas

None — discussion stayed within phase scope.