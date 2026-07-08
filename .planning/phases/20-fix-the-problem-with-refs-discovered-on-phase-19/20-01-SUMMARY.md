---
phase: 20-fix-the-problem-with-refs-discovered-on-phase-19
plan: 01
subsystem: connect
tags: [proto, buf, name, ref, field-number, regression]

# Dependency graph
requires:
  - phase: 18-respect-buf-yaml-dependency-refs-not-always-head-fix-related
    provides: "ref-honoring code path in providers (GetMeta ref-resolution branch + isSHA gate)"
  - phase: 19-we-need-e2e-tests-for-the-ref-specified-for-dependency-looks
    provides: "e2e tests TestRefRespected_* that exposed the field-3/field-4 misread as a GitHub 422"
provides:
  - "parseResourceRefName now reads Name.ref at proto field 4 (was field 3 = label_name)"
  - "TestParseResourceRefName_ReadsRef now writes ref at field 4 (lockstep with production fix)"
  - "New TestParseResourceRefName_LabelNameIsField3 regression-guard test"
affects: [ref-honoring, buf-v1.30.1, buf-v1.69.0, providers]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "buf BSR Name.child oneof: label_name=3, ref=4 — ref is read from field 4, never field 3"
    - "Regression-guard test that pins a specific wire field's semantics to prevent the same class of bug"

key-files:
  created: []
  modified:
    - internal/connect/commits_helpers.go
    - internal/connect/commits_helpers_test.go

key-decisions:
  - "Read ref from field 4 of the Name proto (the buf v1beta1/v1 spec) rather than field 3"
  - "Add a dedicated regression-guard test (TestParseResourceRefName_LabelNameIsField3) that explicitly pins the field-3-is-label_name case"
  - "Test/production lockstep moved in the same change set: when production's `num == 3` arm became `num == 4`, the matching `AppendTag(name, 3, ...)` in the ReadsRef test became `AppendTag(name, 4, ...)` in the same plan"

patterns-established:
  - "When parsing a buf BSR Name proto: owner=1, module=2, label_name=3 (ignore for ref resolution), ref=4"

requirements-completed: []

# Metrics
duration: 5min
completed: 2026-07-08
---

# Phase 20: Fix refs regressions discovered on Phase 19 — Summary

**parseResourceRefName now reads Name.ref at proto field 4 (the buf v1beta1/v1 spec), eliminating the two Phase 19 regressions: GitHub 422 on v1.30.1 no-ref requests and HEAD-instead-of-SHA on v1.69.0 ref-pinned requests.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-07-08T07:46Z
- **Completed:** 2026-07-08T07:51Z
- **Tasks:** 4 / 4 complete
- **Files modified:** 2

## Accomplishments

- Fixed a one-line bug in `parseResourceRefName` (commits_helpers.go:196) that misread `Name.label_name` (field 3) as `Name.ref`. The buf v1beta1/v1 `Name.child` oneof is `{label_name=3, ref=4}`; the previous code read field 3.
- Updated the matching unit test (`TestParseResourceRefName_ReadsRef`) to write the ref at field 4, in lockstep with the production fix.
- Added a new regression-guard test (`TestParseResourceRefName_LabelNameIsField3`) that explicitly pins the wire contract: field 3 is `label_name`, field 4 is `ref`, and the parser must read the latter (and silently ignore the former). This is the test that would have caught the original bug if it had existed in Phase 18.
- Verified the full surface: `go build ./...`, `go vet ./...`, full connect+provider test sweep, and the Phase 19 e2e tests all compile + list + skip cleanly without `EASYP_GH_TOKEN`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix parseResourceRefName to read field 4 (ref) instead of field 3 (label_name)** — `e7eb788` (fix)
2. **Task 2: Update TestParseResourceRefName_ReadsRef to write the ref at field 4 (lockstep with production fix)** — `88f9083` (test)
3. **Task 3: Add TestParseResourceRefName_LabelNameIsField3 (regression guard for the v1.30.1 case)** — `6dd128d` (test)
4. **Task 4: Verify e2e tests still compile + list + skip cleanly** — no commit (verification-only; all 4 SC-19 e2e tests skip cleanly without `EASYP_GH_TOKEN`; TestSmokeBufModUpdate still lists; testutil passes; full unit+integration sweep clean)

**Plan metadata:** (this file)

_Note: TDD was off (`workflow.tdd_mode=false`); commits follow the fix-then-test lockstep rather than red→green→refactor._

## Files Created/Modified

- `internal/connect/commits_helpers.go` — `parseResourceRefName` field-3 arm (line 196) became field-4 arm; comment updated to name the `Name.child` oneof shape.
- `internal/connect/commits_helpers_test.go` — `TestParseResourceRefName_ReadsRef` writes ref at field 4 (was 3); new `TestParseResourceRefName_LabelNameIsField3` writes `label_name` at field 3 and asserts `ref == ""`.

## Decisions Made

- One-line production fix at the existing `else if` arm. No helper extraction, no new files, no new dependencies. The existing `parseResourceRef` (lines 159-178) is unchanged — the bug is confined to the `Name` parser.
- Test/production lockstep moved in the same change set: the ReadsRef test writes field 4, matching the production code that reads field 4. Future drift fails both Task 2's and Task 3's automated verify.
- A second regression-guard test (`TestParseResourceRefName_LabelNameIsField3`) is added rather than relying solely on the existing tests, because the new test explicitly pins the wire contract that the previous tests did not assert: "field 3 is `label_name`, not `ref`". The test is intentionally redundant on the happy path (no field 3, no field 4 → `ref == ""` is already covered by `TestParseResourceRefName_NoRef`); its value is in pinning the wire contract for the case where field 3 IS set with `label_name="main"`.

## Deviations from Plan

### Plan verification check conflict (non-blocking)

The plan's `<verification>` block at the bottom lists a structural check:

```
test -z "$(grep -n 'AppendTag(name, 3, protowire.BytesType)' internal/connect/commits_helpers_test.go)" && echo OK_no_field3_in_test
```

This check is **now false** by design: `TestParseResourceRefName_LabelNameIsField3` (added in Task 3) legitimately writes `label_name` at field 3 via `protowire.AppendTag(name, 3, protowire.BytesType)` — that is the test's entire purpose (to assert field 3 is `label_name`, not `ref`).

This is a contradiction in the plan: the same plan that adds the new test in Task 3 has a verification check that forbids any `AppendTag(name, 3, ...)` in the test file. The plan was self-inconsistent.

**Resolution:** The semantic check (the new test passes) is the meaningful one — it directly asserts the wire contract. The structural check was written for the lockstep ReadsRef test only, before Task 3 was added; it does not account for the new test. The new test passes, all 3 TestParseResourceRefName_* tests pass, and the production code reads field 4 correctly. No code change is needed; the plan's structural check is a planning defect, not a code defect.

### Other deviation: none

All 17 success criteria from the plan are satisfied except SC-15 (the contradicted `OK_no_field3_in_test` check), which is now obsolete. The two actual functional checks that matter — the production code reads field 4, and the new test pins field 3 as label_name — both pass.

## Verification Evidence

| Check | Result |
|-------|--------|
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test ./internal/connect/... ./internal/providers/... -count=1` | PASS (all packages) |
| `go test ./internal/connect/ -run TestParseResourceRefName -v -count=1` | 3 PASS (ReadsRef, NoRef, LabelNameIsField3) |
| `go test ./e2e/ -list 'TestRefRespected.*'` | 3 tests listed |
| `go test ./e2e/ -run TestRefRespected -count=1` | exit 0, 3 SKIP lines (no `EASYP_GH_TOKEN`) |
| `go test ./e2e/ -list 'TestSmokeBufModUpdate'` | 1 test listed |
| `go test ./e2e/testutil/ -count=1` | PASS |
| `grep -c 'num == 4 && typ == protowire.BytesType' internal/connect/commits_helpers.go` | 1 |
| `grep 'num == 3 && typ == protowire.BytesType' internal/connect/commits_helpers.go` | 0 matches |
| `grep -c 'func TestParseResourceRefName_LabelNameIsField3' internal/connect/commits_helpers_test.go` | 1 |
| New TODO/FIXME/XXX/HACK/PLACEHOLDER markers in modified files | 0 |

## Issues Encountered

None at runtime. The only plan defect is the verification-block self-contradiction noted above, which has no code impact.

## Next Up

This phase closes the loop on the Phase 19 e2e regressions. With the field-3/field-4 misread fixed, the providers' `commit == ""` branch (GitHub `getrepo.go:48-58`, Bitbucket `getrepo.go:43-53`) returns HEAD without an upstream call for v1.30.1 no-ref requests, and the providers' `else` branch resolves via `repos.GetCommit` for v1.69.0 ref-pinned requests. The actual SC-19-1/2/3 success criterion (full e2e run with `EASYP_GH_TOKEN` + cached buf binaries) is now reachable but cannot be exercised in this token-less environment.

Phase 20 is ready for verification. Recommend `/gsd-verify-work 20` to run the verifier agent and produce `20-VERIFICATION.md`.
