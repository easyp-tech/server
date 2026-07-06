---
phase: 16-commit-id-resolution-improvements
plan: 03
subsystem: docs
tags: [changelog, release-notes, commit-id, buf]

# Dependency graph
requires:
  - phase: 16-commit-id-resolution-improvements
    plan: 01
    provides: commitUUID rewrite to 14-SHA-bytes + UUID version/variant bits format
  - phase: 16-commit-id-resolution-improvements
    plan: 02
    provides: ServeDownload 400 message updated to point operators at buf mod update / buf dep update
provides:
  - User-visible CHANGELOG.md documenting the commit-id format cutover (D-11)
  - Durable, human-readable record that buf.lock entries are invalidated and the recovery action is `buf mod update` or `buf dep update`
affects:
  - operators upgrading the proxy (read CHANGELOG before upgrading)
  - downstream consumers correlating proxy logs to client-side unknown-commit-id errors

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Keep a Changelog style: [Unreleased] heading with ### Changed subsection"

key-files:
  created:
    - CHANGELOG.md
  modified: []

key-decisions:
  - "Followed the file's existing format rule from the plan: CHANGELOG.md did not exist, so used the new-file structure (# Changelog / ## [Unreleased] / ### Changed) per the plan template"
  - "Used the exact D-11 sentence verbatim, including the two backtick-quoted commands and the trailing period — no paraphrasing"

patterns-established:
  - "Single bullet under ### Changed for the Phase 16 entry; no version bump, no date stamp, no placeholder/TODO noise"

requirements-completed: [SC-1]

# Metrics
duration: 1min
completed: 2026-07-06
---

# Phase 16 Plan 03: CHANGELOG Entry for Commit-ID Format Cutover Summary

**Created root-level CHANGELOG.md with the exact D-11 release note announcing the commit-id format cutover and the buf mod update / buf dep update recovery action**

## Performance

- **Duration:** ~1 min
- **Started:** 2026-07-06T13:08:45Z
- **Completed:** 2026-07-06T13:09:30Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments
- Created `CHANGELOG.md` at the repo root with a `## [Unreleased]` section and a `### Changed` subsection
- Added the exact D-11 release note: announces the 14-SHA-bytes + UUID version/variant bits format, invalidation of existing `buf.lock` entries, and `buf mod update` / `buf dep update` as the recovery action
- Reinforced SC-1: operators now have a durable, pre-upgrade reference describing the cutover and the recovery steps

## Task Commits

Each task was committed atomically:

1. **Task 1: Create or update CHANGELOG.md with the D-11 release note** - `1744d98` (docs)

_Note: this is a single-task documentation plan; no plan-metadata commit is required (per the parallel-execution context, the orchestrator owns STATE.md/ROADMAP.md updates centrally after the wave merges)._

## Files Created/Modified
- `CHANGELOG.md` - New file. Root-level changelog in Keep a Changelog style. Single unreleased entry under `### Changed` containing the verbatim D-11 sentence.

## Decisions Made
- Followed the plan's new-file structure exactly (no version bump, no date stamp, no placeholder text) since `CHANGELOG.md` did not previously exist in the repo.
- Wrote the D-11 sentence character-for-character (including the backtick-quoted `buf mod update` and `buf dep update` commands and the trailing period) to satisfy the plan's verbatim requirement.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- SC-1 is reinforced: the user-visible release note exists, in the exact wording approved by D-11, and points operators at the recovery action.
- The Phase 16 work (commit-id format change + probe + 400 message) is now fully covered: code (16-01/16-02) and documentation (16-03, this plan).
- No further plans in this phase.

---

*Phase: 16-commit-id-resolution-improvements*
*Completed: 2026-07-06*
