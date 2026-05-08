# Phase 8: Go Code Modernization - Context

**Gathered:** 2026-05-08
**Status:** Ready for planning

<domain>
## Phase Boundary

Modernize Go code using `go fix` (apply all recommended analyzer fixes) and replace deprecated `golang.org/x/exp` imports (`slog` and `slices`) with their stdlib equivalents (`log/slog` and `slices`). Verify build and tests pass after all changes.

</domain>

<decisions>
## Implementation Decisions

### go fix scope
- **D-01:** Run `go fix ./...` (all analyzers enabled) to apply modernizations. This covers: `any`, `fmtappendf`, `mapsloop`, `minmax`, `newexpr`, `slicescontains`, `slicessort`, `stditerators`, `stringsbuilder`, `stringscut`, `stringscutprefix`, `stringsseq`, and more.

### exp migration approach
- **D-02:** Custom migration script to handle `golang.org/x/exp` → stdlib migration (go fix does not auto-fix these imports). Script replaces `golang.org/x/exp/slog` with `log/slog` and `golang.org/x/exp/slices` with `slices` across all application code.

### Commit structure
- **D-03:** Multiple sequential commits — one commit per step. Sequence: (1) go fix, (2) exp migration, (3) go mod tidy, (4) verify. Each step tested before moving to next.

### Verification scope
- **D-04:** After all changes: `go build ./...` must pass, E2E tests must pass, `go mod tidy` clean.

### Scope boundaries
- **D-05:** Only application code (`.` package) is modernized. `api/_third_party/` is a git submodule and excluded from changes.

### Claude's Discretion
- Whether go fix and exp migration are combined in a single plan or separated into two plans
- Specific lint suppressions needed after go fix (e.g., if go fix introduces new linter warnings)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 7 context (carried forward)
- `.planning/phases/07-proto-regeneration/07-CONTEXT.md` — Phase 7 decisions: Go 1.26, connect-go v1.19.x

### Project constraints
- `.planning/PROJECT.md` — Go 1.22+ constraint, tech stack
- `.planning/ROADMAP.md` — Phase 8 goal and success criteria
- `go.mod` — current Go version (1.26)

### Codebase insights
- `.planning/codebase/CONCERNS.md` §Tech Debt — exp migration documented as tech debt (deprecated `golang.org/x/exp` imports)
- `.planning/codebase/CONCERNS.md` §Known Bugs — Artifactory Put status code inverted (may surface after exp migration)
- `.planning/codebase/CONVENTIONS.md` — slog usage pattern, import ordering conventions

### Go toolchain
- `go tool fix -help` — full list of available analyzers

### E2E tests (verification target)
- `e2e/smoke_test.go` — Tests buf mod update with both v1.30.1 and v1.69.0
- `e2e/new_proto_test.go` — Tests modern protocol
- `e2e/old_proto_test.go` — Tests old protocol backward compatibility

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `golang.org/x/exp/slog` imports in: `cmd/easyp/main.go:24`, `internal/connect/api.go:7`, `internal/providers/multisource/repo.go:8`, `internal/providers/github/repos.go:8`, `internal/providers/bitbucket/client.go:14`, `internal/providers/localgit/localgit.go:15`, `internal/providers/cache/artifactory/artifactory.go:14` — all need migration to `log/slog`
- `golang.org/x/exp/slices` imports in: `internal/providers/github/getfiles.go:10`, `internal/providers/bitbucket/repos.go:9`, `internal/providers/localgit/localgit.go:18`, `internal/providers/filter/filter.go:8` — all need migration to `slices`

### Established Patterns
- `go generate` via `api/proto/generate.go` — canonical for codegen entry point
- E2E tests with `t.Parallel()` and `testutil.RequireEnvToken` — standard test pattern from Phase 3
- `go fix` runs against `.` only — `api/_third_party/` is a submodule and excluded

### Integration Points
- `log/slog` replaces `golang.org/x/exp/slog` — logger type signature is compatible (both use `*slog.Logger`)
- `slices` stdlib package is API-compatible with `golang.org/x/exp/slices` — drop-in replacement

</code_context>

<specifics>
## Specific Ideas

- go fix may introduce new linter warnings that need `//nolint` suppressions — handle per-case
- `go mod tidy` after go fix and exp migration to remove unused `golang.org/x/exp` dependency
- `api/_third_party/` is a git submodule — must not be modified by the migration script

</specifics>

<deferred>
## Deferred Ideas

### Reviewed (out of scope for this phase)
- Fix Artifactory Put status code inversion (`.planning/codebase/CONCERNS.md` §Known Bugs) — different phase
- Remove unused `internal/logger/` package (`.planning/codebase/CONCERNS.md` §Tech Debt) — separate cleanup

---

*Phase: 08-Go-Code-Modernization*
*Context gathered: 2026-05-08*