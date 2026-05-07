---
phase: 01-code-generation
plan: 01
subsystem: build-toolchain
tags: [protobuf, buf, connect-go, go-mod, codegen]

# Dependency graph
requires:
  - phase: initialization
    provides: buf-v1.69.0 submodule, project structure, existing codegen pipeline
provides:
  - generate.go pointing to buf-v1.69.0 proto source
  - buf.gen.yaml with go and connect-go plugins only (no go-grpc)
  - go.mod with connectrpc.com/connect v1.18.1
affects: [01-02, 02-handler-adaptation]

# Tech tracking
tech-stack:
  added: [connectrpc.com/connect v1.18.1]
  patterns: [two-plugin codegen (go + connect-go only)]

key-files:
  created: []
  modified:
    - api/proto/generate.go
    - api/proto/buf.gen.yaml
    - go.mod
    - go.sum

key-decisions:
  - "Proto source switched from old buf submodule to buf-v1.69.0 for code generation"
  - "go-grpc plugin block removed entirely from buf.gen.yaml (unused at runtime)"
  - "M-mappings for labels.proto, recommendation.proto, sync.proto removed from both go and connect-go plugins"
  - "connect-go upgraded to v1.18.1 (Go 1.22 compatible ceiling)"

patterns-established:
  - "Two-plugin codegen: go + connect-go only (no go-grpc)"

requirements-completed: [BCG-01, BCG-02, BCG-04]

# Metrics
duration: 5min
completed: 2026-05-07
---

# Phase 1 Plan 01: Proto Source Switch Summary

**Switched proto source to buf-v1.69.0 submodule, removed unused go-grpc codegen plugin, and upgraded connect-go to v1.18.1**

## Performance

- **Duration:** 5 min
- **Started:** 2026-05-07T08:32:35Z
- **Completed:** 2026-05-07T08:37:09Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- generate.go now copies protos from buf-v1.69.0 submodule (ready for code regeneration)
- go-grpc plugin block entirely removed from buf.gen.yaml (was 50 lines of unused config)
- M-mappings for 3 absent proto files (labels, recommendation, sync) removed from both go and connect-go plugins
- connectrpc.com/connect upgraded from v1.11.1 to v1.18.1 in go.mod

## Task Commits

Each task was committed atomically:

1. **Task 1: Update generate.go proto source path and clean buf.gen.yaml** - `d766bd5` (chore)
2. **Task 2: Upgrade connect-go to v1.18.1 in go.mod** - `fba4d35` (chore)

## Files Created/Modified
- `api/proto/generate.go` - Changed cp source path from old buf to buf-v1.69.0 submodule
- `api/proto/buf.gen.yaml` - Removed go-grpc block, removed labels/recommendation/sync M-mappings (57 lines removed)
- `go.mod` - Upgraded connectrpc.com/connect v1.11.1 to v1.18.1, protobuf v1.34.1 to v1.34.2
- `go.sum` - Updated checksums for upgraded dependencies

## Decisions Made
- Proto source switched to buf-v1.69.0 -- prerequisite for modern protocol support
- go-grpc plugin removed entirely -- output was never used at runtime (Connect protocol only)
- grpc dependency left in go.mod intentionally -- cleaned in Plan 02 via `go mod tidy` after codegen

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- generate.go, buf.gen.yaml, and go.mod are ready for Plan 02 code regeneration
- Plan 02 will run `go generate ./api/proto/...` to produce new gen/proto/ code from v1.69.0 protos
- Plan 02 will then update handler struct embedding and run `go mod tidy` to clean grpc dependency

## Self-Check: PASSED

- All 3 modified files verified present: generate.go, buf.gen.yaml, go.mod
- Both task commits verified in git log: d766bd5, fba4d35

---
*Phase: 01-code-generation*
*Completed: 2026-05-07*
