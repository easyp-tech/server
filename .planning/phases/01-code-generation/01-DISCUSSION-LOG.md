# Phase 1: Code Generation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-07
**Phase:** 1-code-generation
**Areas discussed:** buf.gen.yaml M-mapping strategy, old buf submodule disposition, compilation error strategy, connect-go upgrade impact, go-grpc dependency cleanup, proto diff documentation

---

## buf.gen.yaml M-mapping strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Regenerate from scratch | Delete all M options, run buf generate with defaults. Generated Go package paths may change. | |
| Diff-based: remove only deleted entries | Remove M entries for the 3 deleted proto files only. Preserves existing import paths exactly. | ✓ |
| Regenerate + pin to current paths | Regenerate mappings but explicitly set to current import paths. Clean config + compatible imports. | |

**User's choice:** Diff-based — remove only deleted entries
**Notes:** User confirmed: remove M entries for labels.proto, recommendation.proto, sync.proto from go and connect-go plugins, remove entire go-grpc plugin block.

---

## Old buf submodule disposition

| Option | Description | Selected |
|--------|-------------|----------|
| Keep for Phase 2 reference | Old proto files useful when adapting handlers in Phase 2. Remove after Phase 2. | ✓ |
| Remove immediately | No longer used in codegen. Clean removal, rely on git history. | |

**User's choice:** Keep for Phase 2 reference

---

## Compilation error strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Embed Unimplemented stubs | Embed new Unimplemented*Handler types in handler structs. Minimal change to get build passing. | ✓ |
| Stub all new RPCs too | Add stub implementations for new RPCs returning Unimplemented errors. More complete but bleeds into Phase 2. | |
| Comment out broken code | Temporarily remove broken handler code. Quick and dirty. | |

**User's choice:** Embed Unimplemented stubs

---

## connect-go upgrade impact

| Option | Description | Selected |
|--------|-------------|----------|
| Research changelog first | Audit v1.11.1→v1.18.1 changelog for breaking changes before implementation. | ✓ |
| Upgrade and fix on the fly | Upgrade, regenerate, fix what breaks. Faster but may hit surprises. | |
| Claude decides | Let Claude pick the approach based on research findings. | |

**User's choice:** Research changelog first

---

## go-grpc dependency cleanup

| Option | Description | Selected |
|--------|-------------|----------|
| Let go mod tidy handle it | Run go mod tidy after codegen changes. If grpc is unused, it drops out naturally. | ✓ |
| Manually remove from go.mod | Explicitly remove grpc dependency. More control but risk removing something still needed. | |

**User's choice:** Let go mod tidy handle it

---

## Proto diff documentation

| Option | Description | Selected |
|--------|-------------|----------|
| Include diff analysis in Phase 1 | Diff old vs new protos for registry/v1alpha1/ as part of Phase 1 research. Inform Phase 2. | ✓ |
| Defer to Phase 2 research | Phase 2 researcher can do this. Phase 1 is purely mechanical. | |

**User's choice:** Include diff analysis in Phase 1

---

## Claude's Discretion

None — user made all decisions explicitly.

## Deferred Ideas

None — discussion stayed within phase scope.
