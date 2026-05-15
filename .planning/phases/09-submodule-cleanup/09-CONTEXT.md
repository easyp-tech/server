# Phase 9: Submodule Cleanup - Context

**Gathered:** 2026-05-08
**Status:** Ready for planning

<domain>
## Phase Boundary

Clean up the API submodule structure:
1. Remove `api/_third_party/buf` submodule (old protocol, deprecated)
2. Rename `api/_third_party/buf-v1.69.0` to `api/_third_party/buf` (promote modern as canonical)
3. Adjust code generation configuration to reference the renamed submodule
4. Regenerate proto code and verify build/tests pass

</domain>

<decisions>
## Implementation Decisions

### Submodule Cleanup
- **D-01:** Remove `api/_third_party/buf` submodule entirely — contains deprecated protocol files, no longer needed after Phase 7/8 proto regeneration with connect-go v1.19.2
- **D-02:** Rename `api/_third_party/buf-v1.69.0` to `api/_third_party/buf` — the v1.69.0 proto is now the canonical/production version
- **D-03:** Use `git submodule deinit` + `git rm` for removal; use `git mv` for renaming (preserves git history)

### Code Generation
- **D-04:** Update `api/proto/buf.gen.yaml` to reference the renamed `api/_third_party/buf` path — should require minimal changes since buf.gen.yaml uses relative paths starting from `api/proto/`
- **D-05:** Update `api/proto/buf.work.yaml` if it exists and references the old submodule path

### Verification
- **D-06:** Run `cd api/proto && go generate` after cleanup
- **D-07:** Verify `go build ./...` passes with regenerated code
- **D-08:** Run E2E tests (`go test -v ./e2e/...`) to ensure everything still works

### Git History Preservation
- **D-09:** Use `git mv api/_third_party/buf-v1.69.0 api/_third_party/buf` to rename — preserves commit history for the submodule
- **D-10:** Update `.gitmodules` file entry from `buf-v1.69.0` to `buf`

### Claude's Discretion
- Submodule cleanup order (remove old first vs rename new first) — either works, standard approach is remove old, then rename new
- Timing of `go generate` relative to submodule changes — can be combined in one plan task
- Whether to commit submodule changes separately or together — single commit is fine for this phase

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Configuration
- `api/proto/buf.gen.yaml` — Code generation config (must be updated after submodule rename)
- `api/proto/buf.work.yaml` — Workspace config (check for submodule references)

### Prior Phase Context
- `.planning/phases/07-proto-regeneration/07-SUMMARY.md` — Proto regeneration completed with connect-go v1.19.2, confirms old submodule no longer primary
- `.planning/phases/08-go-modernization/08-CONTEXT.md` — Go modernization context

### Existing Documentation
- `.planning/ROADMAP.md` §Phase 8 — Go Code Modernization completed

</canonical_refs>

<codebase_context>
## Existing Code Insights

### Integration Points
- Code generation output: `gen/proto/buf/alpha/registry/v1alpha1/` — generated files that will be regenerated after submodule cleanup
- Handler code: `internal/connect/api.go` — imports from generated proto packages

### Established Patterns
- Submodule management via `.gitmodules` — standard git submodule workflow
- Code generation via `buf generate` — configured in `buf.gen.yaml`

</codebase_context>

<specifics>
## Specific Ideas

- User explicitly requested: remove old buf → rename buf-v1.69.0 to buf → update gen config → regenerate → test
- E2E tests must pass as final verification step

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 9-Submodule-Cleanup*
*Context gathered: 2026-05-08*