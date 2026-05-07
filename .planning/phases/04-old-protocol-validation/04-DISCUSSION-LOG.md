# Phase 4: Old Protocol Validation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-07
**Phase:** 4-Old Protocol Validation
**Areas discussed:** OLD-01 depth, buf dep update setup, Failure diagnostics

---

## OLD-01 depth

| Option | Description | Selected |
|--------|-------------|----------|
| Smoke test is sufficient | The smoke test already validates OLD-01. Phase 4 focuses on OLD-02. | ✓ |
| Add deeper assertions | Create dedicated test with buf.lock content validation. | |
| Dedicated test, same depth | Separate Phase 4 test file mirroring smoke test assertions. | |

**User's choice:** Smoke test is sufficient
**Notes:** OLD-01 is verified by `e2e/smoke_test.go` `TestSmokeBufModUpdate` with `buf_v1.30.1` subtest. No additional test needed.

---

## buf dep update setup

| Option | Description | Selected |
|--------|-------------|----------|
| Two-step: mod update then dep update | Test calls buf mod update first to create buf.lock, then buf dep update. | ✓ |
| Standalone: just dep update | Only calls buf dep update with a buf.yaml. | |
| Let research decide | Researcher investigates buf dep update behavior in v1.30.1. | |

**User's choice:** Two-step: mod update then dep update

| Option | Description | Selected |
|--------|-------------|----------|
| Exit code only | Test verifies exit code 0 and no stderr output. Simple and sufficient. | ✓ |
| Exit code + buf.lock integrity | Also verify buf.lock still exists after dep update. | |
| Full validation: exit + lock content | Verify exit code, buf.lock exists, and lock file content unchanged. | |

**User's choice:** Exit code only
**Notes:** Keep it simple — exit code 0 is sufficient for backward compatibility confirmation.

---

## Failure diagnostics

| Option | Description | Selected |
|--------|-------------|----------|
| Server logs on failure | Capture server subprocess output and include in test failure message. | ✓ |
| Buf stderr only | Only capture buf CLI stderr (already done by RunBufModUpdate). | |
| Debug logging + all output | Server logs, buf stderr, and debug-level proxy logging. | |

**User's choice:** Server logs on failure
**Notes:** The `StartServer` helper already captures subprocess output — tests need to surface it on failure.

---

## Claude's Discretion

- Test file location and naming
- Whether to add RunBufDepUpdate helper to testutil or implement inline
- Whether OLD-02 test should share the smoke test structure or be standalone

## Deferred Ideas

None — discussion stayed within phase scope.
