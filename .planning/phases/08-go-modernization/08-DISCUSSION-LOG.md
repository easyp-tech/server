# Phase 8: Go Code Modernization - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-08
**Phase:** 08-go-modernization
**Areas discussed:** Phase definition, exp migration approach, commit structure

---

## Phase Definition

| Option | Description | Selected |
|--------|-------------|----------|
| Add to roadmap first | Define Phase 8 in the roadmap before discussing it | ✓ |
| Start new milestone | v1.2 is complete — new work needed | |
| Different phase number | Check if it should be part of an existing milestone | |

**User's choice:** Add to roadmap first
**Notes:** User wants Phase 8 to modernize Go code using `go fix` and replace deprecated exp imports. No new milestone — continue with v1.2.

---

## Milestone Placement

| Option | Description | Selected |
|--------|-------------|----------|
| v1.2 (in progress) | Continue v1.2 milestone with Phase 8 | ✓ |
| v1.3 | v1.2 is complete, start new milestone | |

**User's choice:** v1.2 (in progress)
**Notes:** Phase 8 is part of v1.2 Dependency Modernization.

---

## exp Migration Approach

| Option | Description | Selected |
|--------|-------------|----------|
| Custom migration script | Add a dedicated script/tool to handle exp→stdlib migration (recommended per CONCERNS.md) | ✓ |
| Manual sed commands | Write a shell one-liner with sed/find to replace imports | |
| go fix only | Use go fix only, skip the exp migration for later | |

**User's choice:** Custom migration script
**Notes:** go fix doesn't auto-fix these imports — needs a separate approach. User prefers a script over ad-hoc sed commands.

---

## Commit Structure

| Option | Description | Selected |
|--------|-------------|----------|
| Single commit | All changes in one commit: go fix + import script + build fixes | |
| Multiple commits | Separate commits: one for go fix, one for import migration, one for build fixes | ✓ |
| One per concern | One commit per concern (go fix, exp→stdlib, lint fixes) | |

**User's choice:** Multiple commits
**Notes:** Separate commits for each concern for better traceability.

---

## Commit Order

| Option | Description | Selected |
|--------|-------------|----------|
| Sequential: each step, then test | go fix → exp migration → go mod tidy → verify | ✓ |
| Batch: all changes, then fix build | go fix + exp migration together, then test together, then individual fix commits | |

**User's choice:** Sequential: each step, then test
**Notes:** Commit one step, verify it works, then move to the next.

---

## Discussion Skip

**Decision:** Phase is mostly mechanical, but discussed commit structure in detail.

---

## Deferred Ideas

- Fix Artifactory Put status code inversion (CONCERNS.md §Known Bugs) — separate phase
- Remove unused `internal/logger/` package (CONCERNS.md §Tech Debt) — separate cleanup
- Address other CONCERNS.md items (sequential file downloads, no graceful shutdown, etc.) — future phases