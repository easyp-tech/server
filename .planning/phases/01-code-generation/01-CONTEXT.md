# Phase 1: Code Generation - Context

**Gathered:** 2026-05-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Switch proto source from old `buf` submodule to `buf-v1.69.0`, upgrade `connectrpc.com/connect` from v1.11.1 to v1.18.1, regenerate Go code, remove go-grpc codegen plugin, and verify the project compiles. This is a mechanical code generation phase — handler adaptation is Phase 2.

</domain>

<decisions>
## Implementation Decisions

### buf.gen.yaml M-mapping strategy
- **D-01:** Diff-based approach — remove M entries only for the 3 proto files absent in v1.69.0 (`labels.proto`, `recommendation.proto`, `sync.proto`) from the `go` and `connect-go` plugins. All other M entries stay unchanged.
- **D-02:** Remove the entire `go-grpc` plugin block from `buf.gen.yaml`.

### Old buf submodule disposition
- **D-03:** Keep `api/_third_party/buf` submodule after switching. Needed as reference during Phase 2 handler adaptation (comparing old vs new message types). Remove after Phase 2 completes.

### Compilation error strategy
- **D-04:** Embed new `Unimplemented*Handler` types from regenerated code in handler structs to satisfy Connect interface requirements. Existing RPC method signatures stay as-is — only struct embedding changes. This is the minimal change to get `go build ./...` passing.

### connect-go upgrade impact
- **D-05:** Research the connect-go v1.11.1 → v1.18.1 changelog for breaking changes to generated handler interfaces, middleware signatures, and the Unimplemented pattern BEFORE making changes. The researcher must audit this before implementation.

### go-grpc dependency cleanup
- **D-06:** Let `go mod tidy` handle `google.golang.org/grpc` removal naturally after codegen changes. No manual removal from go.mod.

### Proto diff documentation
- **D-07:** Include a structured proto diff analysis (old vs new registry/v1alpha1 protos) as part of Phase 1 research. Document field additions, message changes, and new RPCs. This gives Phase 2 a clear map of what changed.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Code generation pipeline
- `api/proto/generate.go` — Go generate directives for protobuf codegen (entry point)
- `api/proto/buf.gen.yaml` — Buf code generation configuration with M mappings
- `api/_third_party/buf-v1.69.0/proto/buf/alpha/registry/v1alpha1/` — Modern proto definitions (v1.69.0, target source)
- `api/_third_party/buf/proto/buf/alpha/registry/v1alpha1/` — Old proto definitions (v1.30.1, reference for diff)

### Dependencies
- `go.mod` — Current dependency declarations (connect-go v1.11.1, grpc v1.59.0)
- `api/proto/buf.gen.yaml` lines 104-153 — connect-go plugin config (target of M-mapping removal)

### Handler code (for understanding build impact)
- `internal/connect/api.go` — Connect RPC handler struct definitions (will need Unimplemented embedding)
- `internal/connect/blobs.go` — Download service handler
- `internal/connect/bynames.go` — Repository service handler
- `internal/connect/modulepins.go` — Resolve service handler

### Project decisions
- `.planning/PROJECT.md` — Key Decisions table (connect-go v1.18.1 ceiling, single superset handler)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `api/proto/generate.go` — Already has the `rm -rf` + `cp -r` + `buf generate` pattern. Only the cp source path needs changing from `../_third_party/buf/proto/buf` to `../_third_party/buf-v1.69.0/proto/buf`.
- `buf.gen.yaml` — The M-mapping structure is already established. Diff-based removal is straightforward: delete lines matching the 3 removed proto files.

### Established Patterns
- Generated code is committed to `gen/proto/` — not gitignored. After regeneration, the entire `gen/` directory is replaced.
- The `go-grpc` plugin output is not used at runtime (Connect protocol only). Removing it from codegen has no runtime impact.
- Handler structs currently embed old `Unimplemented*Handler` types — these will need updating to the new generated types.

### Integration Points
- `generate.go` → `buf.gen.yaml` → `gen/proto/` → `internal/connect/*.go` — the codegen pipeline
- `cmd/easyp/main.go` wires Connect handlers — may need import path updates if generated package paths change

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches. The phase is mechanical: switch source, regenerate, verify build.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 1-Code Generation*
*Context gathered: 2026-05-07*
