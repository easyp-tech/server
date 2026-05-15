---
phase: 01-code-generation
plan: 02
subsystem: build-toolchain
tags: [protobuf, buf, connect-go, codegen, go-mod]

# Dependency graph
requires:
  - phase: 01-code-generation (Plan 01)
    provides: generate.go pointing to buf-v1.69.0, buf.gen.yaml with go+connect-go plugins only, connect-go v1.18.1
provides:
  - gen/proto/ regenerated from v1.69.0 proto definitions (no grpc artifacts)
  - go.mod cleaned of google.golang.org/grpc and transitive deps
  - Project compiles cleanly with go build and go vet
affects: [02-handler-adaptation]

# Tech tracking
tech-stack:
  added: []
  patterns: [two-plugin codegen output verified (go + connect-go only)]

key-files:
  created: []
  modified:
    - gen/proto/ (113 files changed -- full regeneration)
    - go.mod (grpc + transitive deps removed)
    - go.sum (updated checksums)

key-decisions:
  - "No handler code changes needed -- existing Unimplemented*Handler embedding in api.go satisfies expanded interfaces from regenerated code"
  - "go mod tidy removed google.golang.org/grpc and 3 transitive dependencies (golang/protobuf, genproto, x/text)"

patterns-established: []

requirements-completed: [BCG-03]

# Metrics
duration: 2min
completed: 2026-05-07
---

# Phase 1 Plan 02: Code Regeneration Summary

**Regenerated all proto code from v1.69.0 definitions with two-plugin pipeline, removed grpc dependency, clean build verified**

## Performance

- **Duration:** 2 min
- **Started:** 2026-05-07T08:39:48Z
- **Completed:** 2026-05-07T08:42:32Z
- **Tasks:** 2
- **Files modified:** 115 (113 in gen/proto/, go.mod, go.sum)

## Accomplishments
- Full proto code regeneration from buf-v1.69.0 definitions (113 files changed, 17253 insertions, 33644 deletions)
- All _grpc.pb.go files eliminated (39 deleted) -- only .pb.go and .connect.go remain
- labels.pb.go, recommendation.pb.go, sync.pb.go removed (3 absent proto files from v1.69.0)
- google.golang.org/grpc removed from go.mod along with 3 transitive deps (golang/protobuf, genproto/googleapis/rpc, x/text)
- go build ./... and go vet ./... pass cleanly with zero errors

## Task Commits

Each task was committed atomically:

1. **Task 1: Regenerate proto code from v1.69.0 definitions** - `23f37ed` (feat)
2. **Task 2: Fix compilation errors and clean dependencies** - `9625113` (chore)

## Files Created/Modified
- `gen/proto/` - Full regeneration: 113 files changed (39 _grpc.pb.go deleted, labels/recommendation/sync generated code deleted, all remaining .pb.go and .connect.go regenerated from v1.69.0 definitions)
- `go.mod` - Removed google.golang.org/grpc, github.com/golang/protobuf, google.golang.org/genproto/googleapis/rpc, golang.org/x/text
- `go.sum` - Updated checksums reflecting dependency removal

## Decisions Made
- No handler code changes needed -- the existing Unimplemented*Handler embedding in api.go automatically satisfies the expanded interfaces (new RPCs: GetSDKInfo, GetCargoVersion, GetNugetVersion, GetCmakeVersion, AddRepositoryGroup, UpdateRepositoryGroup, RemoveRepositoryGroup)
- go mod tidy was sufficient to remove all grpc-related dependencies -- no manual go.mod edits required

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None - the research prediction was accurate: connect-go v1.11.1 to v1.18.1 has zero breaking changes to handler interfaces, and the build passed on the first attempt.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 1 is complete: proto source switched, code regenerated, dependencies cleaned, project builds cleanly
- Phase 2 (Handler Adaptation) can now proceed: handler methods need adaptation for any changed message types (e.g., IsBsrHead field removed from LocalModuleResolveResult)
- The old buf submodule at api/_third_party/buf is still available for Phase 2 diff reference (per decision D-03)

## Self-Check: PASSED

- Task 1 commit 23f37ed verified in git log
- Task 2 commit 9625113 verified in git log
- gen/proto/buf/alpha/registry/v1alpha1/resolve.pb.go exists
- gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/resolve.connect.go exists
- gen/proto/buf/alpha/registry/v1alpha1/repository.pb.go exists
- gen/proto/buf/alpha/registry/v1alpha1/download.pb.go exists

---
*Phase: 01-code-generation*
*Completed: 2026-05-07*
