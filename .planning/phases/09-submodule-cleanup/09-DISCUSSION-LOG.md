# Phase 9: Submodule Cleanup - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-08
**Phase:** 09-submodule-cleanup
**Areas discussed:** Submodule cleanup approach, code generation adjustment

---

## Submodule Cleanup Approach

| Option | Description | Selected |
|--------|-------------|----------|
| Remove old, then rename | git submodule deinit/rm old → git mv buf-v1.69.0 buf | ✓ |
| Rename to temp, then swap | git mv buf-v1.69.0 buf-temp → git mv buf-temp buf (avoids name conflict) | |
| Single combined operation | Use git filter-branch or subtree merge | |

**User's choice:** Remove old first, then rename
**Notes:** User provided clear direction: remove `api/_third_party/buf` entirely, rename `api/_third_party/buf-v1.69.0` to `api/_third_party/buf`.

---

## Code Generation Adjustment

| Option | Description | Selected |
|--------|-------------|----------|
| Update buf.gen.yaml | Modify `api/proto/buf.gen.yaml` to reference renamed submodule | ✓ |
| Check buf.work.yaml | Also update workspace config if it exists | ✓ |
| Verify paths work | Run `buf generate` to confirm paths are correct | ✓ |

**User's choice:** Update config files, regenerate, and test
**Notes:** User explicitly required: update code generation → regenerate → run E2E tests.

---

## Git History Preservation

| Option | Description | Selected |
|--------|-------------|----------|
| git mv for rename | Preserves commit history for renamed submodule | ✓ |
| Separate commits | Commit submodule changes separately from generated code | |
| Single commit | All changes in one commit | ✓ |

**User's choice:** Use git mv (preserves history), single commit for the phase
**Notes:** Simpler workflow, history preserved via git mv.

---

## Deferred Ideas

None — discussion stayed within phase scope.

---

*Phase: 09-Submodule-Cleanup*
*Discussion completed: 2026-05-08*