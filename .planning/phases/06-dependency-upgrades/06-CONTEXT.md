# Phase 6: Dependency Upgrades - Context

**Gathered:** 2026-05-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Upgrade Go from 1.22 to 1.26, update connect-go to v1.19.x, update all other dependencies to latest compatible versions, update Dockerfile to golang:1.26-alpine, and verify `go build ./...` and `go test ./...` pass cleanly.

</domain>

<decisions>
## Implementation Decisions

### Upgrade strategy
- **D-01:** Incremental upgrades by layer — batch dependencies by type, update Go toolchain first, then Connect, then protobuf and other libs. Structured approach for safer upgrades.
- **D-02:** Update go.mod first, then run `go mod tidy` to resolve transitive deps. Fix any errors as they arise.
- **D-03:** Update Dockerfile to use `golang:1.26-alpine` as the base image. This is required for Go 1.26 support.

### Build verification
- **D-04:** Verify builds with `go build ./...` AND run full test suite `go test ./...`. Need to confirm both compile and tests pass after upgrades.

### Key dependencies to update
- Go toolchain: 1.22 → 1.26
- connect-go: v1.18.1 → v1.19.x (requires Go 1.24+)
- google.golang.org/protobuf: likely needs update for connect-go compatibility
- Other deps: go-git, go-github, yaml, crypto, exp/slog — latest compatible versions

### Docker build
- **D-05:** Dockerfile multi-stage build must be updated alongside go.mod. Both must stay in sync.

### Claude's Discretion
- Specific version numbers for each dependency — let `go mod tidy` resolve to highest compatible
- Order of layer updates within the batch approach
- Whether to run tests after each batch or at the end

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing go.mod (baseline)
- `go.mod` — Current dependency declarations (go 1.22, connect v1.18.1, etc.)

### Project decisions (carry forward)
- `.planning/PROJECT.md` — Tech Stack constraint: Go 1.22, Connect RPC, protobuf
- `.planning/ROADMAP.md` — Phase 6 goal and success criteria
- `.planning/REQUIREMENTS.md` — DEPS-01 through DEPS-04 requirements

### Test infrastructure
- `.planning/phases/03-test-infrastructure/03-CONTEXT.md` — Test patterns and helpers

### Codebase
- `Dockerfile` — Current multi-stage build (golang:1.22-alpine)
- `STACK.md` §Key Dependencies — Current critical dependencies list

</canonical_refs>


## Existing Code Insights

### Reusable Assets
- None specific to dependency upgrades — this is infrastructure work

### Established Patterns
- Dependency management via `go.mod` / `go.sum`
- Multi-stage Docker builds from golang base images
- Table-driven tests with `t.Parallel()`

### Integration Points
- `go.mod` → `go build` → all packages
- Dockerfile → Docker image → production deployment
- connect-go → generated proto code → Connect RPC handlers

</code_context>

<specifics>
## Specific Ideas

- connect-go v1.19.x requires Go 1.24+, so Go 1.26 upgrade must happen first
- The go.sum file is present and must be kept in sync with go.mod
- golangci-lint version may need updating for Go 1.26 compatibility

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

---

*Phase: 6-Dependency Upgrades*
*Context gathered: 2026-05-07*
