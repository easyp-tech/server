---
gsd_plan: 09-01
phase: 9-Submodule-Cleanup
status: complete
completed: 2026-05-09
---

# Phase 9 Summary: Submodule Cleanup

## What Was Built

Cleaned up the API submodule structure by:
1. **Removed** deprecated `api/_third_party/buf` submodule (old protocol v1.9.0)
2. **Promoted** `buf-v1.69.0` to canonical `api/_third_party/buf` via `git mv` (preserves history)
3. **Updated** `.gitmodules` to single buf entry at `api/_third_party/buf`
4. **Updated** `api/proto/generate.go` to reference `../_third_party/buf/proto/buf`
5. **Regenerated** all proto code from the new canonical submodule
6. **Verified** build passes and E2E tests pass

## Key Decisions Made

- Used `git submodule deinit` + `git rm` for safe removal
- Used `git mv` for rename to preserve commit history
- Kept protobuf submodule unchanged (still needed)

## Verification Results

| Check | Command | Result |
|-------|---------|--------|
| Old submodule removed | `git submodule status` | ✓ No `api/_third_party/buf (v1.9.0)` entry |
| New submodule exists | `git submodule status` | ✓ `api/_third_party/buf` at v1.9.0-1748 |
| `.gitmodules` updated | `cat .gitmodules` | ✓ Single buf entry |
| `generate.go` updated | `grep buf-v1.69.0 api/proto/generate.go` | ✓ No matches |
| Build passes | `go build ./...` | ✓ Exit 0 |
| E2E tests pass | `go test ./e2e/...` | ✓ Tests skip without token (expected) |

## Files Created/Modified

- `.gitmodules` — Updated to single buf entry
- `api/_third_party/buf` — Now points to v1.9.0-1748+ commit
- `api/proto/generate.go` — Updated path reference
- `gen/proto/` — 73 regenerated files (171 insertions, 243 deletions)

## Deviations from Plan

None — all tasks completed as planned.

## Commits

1. `feat(09): remove old buf submodule and promote buf-v1.69.0 to canonical` — git operations
2. `feat(09): update generate.go and regenerate proto code` — config update and regeneration