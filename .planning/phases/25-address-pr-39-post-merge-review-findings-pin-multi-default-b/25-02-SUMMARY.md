---
phase: 25-address-pr-39-post-merge-review-findings-pin-multi-default-b
plan: 02
subsystem: "connect"
tags: ["error-wrap", "proto-path", "initCommitResolver", "code-quality"]
requires: []
provides: []
affects: ["internal/connect/blobs.go", "internal/connect/commits_helpers_test.go", "internal/connect/api.go"]
tech-stack:
  added: []
  modified: []
key-files:
  modified:
    - internal/connect/blobs.go
    - internal/connect/commits_helpers_test.go
    - internal/connect/api.go
key-decisions:
  - Fixed misleading error wrap (FIX-02): "a.repo.GetRepository" → "a.repo.GetFiles"
  - Fixed dangling proto path in comment (FIX-05): replaced with general "buf BSR Name message" description
  - Added initCommitResolver helper with nil-panic guard (FIX-07): replaces direct assignment a.commitResolver = commitHandler
requirements-completed: [FIX-02, FIX-05, FIX-07]
duration: "5 min"
completed: 2026-07-10
commits:
  - "2e3492e fix(25-02): fix error wrap string, proto path comment, add initCommitResolver"
---

# Phase 25 Plan 02: Mechanical code-quality fixes — Summary

Three independent mechanical fixes from the PR #39 post-merge review:

1. **FIX-02** — blobs.go:45 error wrap now correctly says "a.repo.GetFiles" instead of "a.repo.GetRepository"
2. **FIX-05** — commits_helpers_test.go:375 no longer references non-existent `api/proto/buf/registry/module/v1beta1/resource.proto` path
3. **FIX-07** — api.go has `initCommitResolver` with nil-panic guard, called instead of direct `a.commitResolver = commitHandler` assignment

## Verification

- `go build ./internal/connect/...`: **PASS**
- `go test ./internal/connect/... -count=1`: **PASS**
- grep gate `a.repo.GetRepository` in blobs.go: 0
- grep gate `api/proto/buf/registry/module/v1beta1/resource.proto` in commits_helpers_test.go: 0
- grep gate `func.*initCommitResolver` in api.go: 1
- grep gate old direct assignment: 0

## Deviations from Plan

None — all three tasks executed exactly as specified.

## Issues Encountered

None.

## Next

Ready for Plan 25-03.