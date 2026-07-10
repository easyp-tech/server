---
phase: 25-address-pr-39-post-merge-review-findings-pin-multi-default-b
plan: 01
subsystem: "providers"
tags: ["carve-out", "isSHA", "isConventionalDefaultName", "extraction", "helpers", "cutover"]
requires: []
provides: ["content.IsSHA", "content.IsConventionalDefaultName", "carve-out-fix"]
affects: ["internal/providers/github/getrepo.go", "internal/providers/bitbucket/getrepo.go", "internal/providers/content"]
tech-stack:
  added: []
  modified: []
key-files:
  created:
    - internal/providers/content/helpers.go
    - internal/providers/content/helpers_test.go
  modified:
    - internal/providers/github/getrepo.go
    - internal/providers/github/getrepo_test.go
    - internal/providers/bitbucket/getrepo.go
    - internal/providers/bitbucket/getrepo_test.go
key-decisions:
  - RETAINED the isConventionalDefaultName carve-out in both providers via shared content.IsConventionalDefaultName helper. The v1alpha1 ResolveService handler (modulepins.go GetModulePins) is NOT gated by parseResourceRefName and still passes label_name="main" (proto field 3) as commit="main" to GetMeta. Only the v1beta1 CommitService path is guarded by parseResourceRefName (Phase 18). If the v1alpha1 ResolveService path is also updated to ignore label_name, the carve-out can be removed in a future phase.
  - Extracted isSHA and isConventionalDefaultName to internal/providers/content/helpers.go as exported functions. The connect-layer isSHA in internal/connect/commits_helpers.go is NOT consolidated (different domain: UUID-derivation gating vs. provider-layer commit-vs-ref gating).
requirements-completed: [FIX-01, FIX-04]
duration: "22 min"
completed: 2026-07-10
commits:
  - "b917971 fix(25-01): remove isConventionalDefaultName carve-out, extract isSHA to content package"
  - "920441b fix(25-01): restore isConventionalDefaultName carve-out via shared content helper"
---

# Phase 25 Plan 01: Remove carve-out, extract helpers — Summary

Extracted duplicated `isSHA` and `isConventionalDefaultName` helpers to shared `internal/providers/content/helpers.go`. Initially removed the carve-out from GetMeta/getMeta, then restored it when e2e testing revealed the v1alpha1 ResolveService path still passes `"main"` as commit string. The carve-out now uses the shared `content.IsConventionalDefaultName` helper.

## Key Finding

The e2e test proved the carve-out IS still reachable — the research assumption that "the v1.30.1 label_name=3 case is no longer reachable" was incorrect for the v1alpha1 ResolveService handler. Only the v1beta1 CommitService path is gated by `parseResourceRefName`. The v1alpha1 `GetModulePins` handler passes `label_name="main"` (proto field 3) directly as `commit="main"` to `GetMeta`.

## Verification

- `go test ./internal/providers/content/... -count=1`: **PASS** — 8 IsSHA + 5 IsConventionalDefaultName subtests
- `go test ./internal/providers/github/... -count=1`: **PASS** — 5 GetMeta tests + retry transport tests
- `go test ./internal/providers/bitbucket/... -count=1`: **PASS** — 5 GetMeta tests
- `go test ./internal/... -count=1`: **PASS** — full unit suite
- e2e `TestRefRespected_*` (6 subtests): **ALL PASS** against real GitHub
- Structural grep gates: all `func isSHA`, `func isConventionalDefaultName` removed from provider packages
- `content.IsSHA(commit)` used in both providers
- `content.IsConventionalDefaultName(commit)` condition retained in both providers

## Deviations from Plan

**Rule 4 - Architectural Change** — The carve-out was initially removed per the plan's research recommendation, but e2e testing proved it's still reachable via the v1alpha1 ResolveService path. The carve-out was restored using the shared `content.IsConventionalDefaultName` helper, which is strictly better than the original (single source of truth for the helper definition). This is a correction of the plan's research assumption, not a code error.

## Issues Encountered

None — all restored tests pass with the shared helper.

## Next

Ready for Plan 25-02.