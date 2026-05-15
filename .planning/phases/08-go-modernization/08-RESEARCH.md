# Phase 8: Go Code Modernization — Research

**Research Date:** 2026-05-08
**Status:** Complete

## Research Complete

---

### 1. What analyzers does `go fix` run? Which ones produce changes vs. are read-only?

`go fix ./...` (or `go tool fix ./...`) runs **all registered analyzers by default**. There are 22 analyzers total:

**Apply fixes (modifying):**
- `any` — replace `interface{}` with `any` (purely stylistic, Go 1.18+)
- `fmtappendf` — replace `[]byte(fmt.Sprintf(...))` with `fmt.Appendf(nil, ...)` (avoids allocation)
- `forvar` — remove redundant `x := x` shadowing in range loops (Go 1.22 made re-declaration unnecessary)
- `hostport` — replace `fmt.Sprintf("%s:%d", ...)` with `net.JoinHostPort`
- `inline` — apply fixes based on `//go:fix inline` comment directives (moves calls to inlinable wrappers)
- `mapsloop` — replace map iteration loops with `maps.Copy`/`maps.Insert`/`maps.Clone`/`maps.Collect` (Go 1.23+)
- `minmax` — replace `if a < b { x = a } else { x = b }` with `min(a, b)` / `max(a, b)` (avoids floating point NaN cases)
- `newexpr` — simplify `func varOf(x T) *T { return &x }` to use `new(x)` (Go 1.26, from generated code pattern)
- `omitzero` — replace `json:",omitempty"` with `omitzero` on struct fields (Go 1.24+, **behavior change**)
- `reflecttypefor` — replace `reflect.TypeOf(x)` with `reflect.TypeFor[T]()` (Go 1.22+)
- `slicescontains` — replace element-existence loops with `slices.Contains`/`slices.ContainsFunc` (Go 1.21+)
- `slicessort` — replace `sort.Slice(s, func(i,j int) bool { return s[i] < s[j] })` with `slices.Sort(s)` (Go 1.21+)
- `stditerators` — replace `for i := 0; i < x.Len(); i++ { use(x.At(i)) }` loops with iterator `for elem := range x.All()` (Go 1.23+)
- `stringsbuilder` — replace string `+=` concatenation with `strings.Builder` (Go 1.10+)
- `stringscut` — replace `strings.Index` + slice with `strings.Cut` (Go 1.18+)
- `stringscutprefix` — replace `HasPrefix`+`TrimPrefix` with `CutPrefix`/`CutSuffix` (Go 1.20+)
- `stringsseq` — replace `for range strings.Split(...)` with `strings.SplitSeq(...)` (Go 1.24+)
- `waitgroup` — replace `wg.Add(1)/go/.../wg.Done()` with `wg.Go(...)` (Go 1.25+)

**Read-only / informational:**
- `buildtag` — check `//go:build` and `// +build` directives (reports issues, not an auto-fix modernizer per se)
- `plusbuild` — remove obsolete `//+build` comments (replaced by `//go:build`)

**No code changes expected for this codebase** based on analysis:
- `go build` already passes, so `buildtag` likely finds nothing
- `go mod` already modern at `go 1.26`, so `plusbuild` likely finds nothing
- `newexpr` only applies to auto-generated wrapper functions — unlikely to find any in this codebase
- `waitgroup` requires Go 1.25 patterns — codebase uses `go-goroutine` patterns from go-git, not `wg.Go`
- `rangeint` requires old-style `for i := 0; i < n; i++` — codebase likely already uses modern `for range`
- `stringsseq` requires `strings.Split` iteration — codebase uses `slices` not raw strings
- `omitzero` requires `json:",omitempty"` on struct fields — none found in scan

**Key concern:** `mapsloop` (Go 1.23+) and `stditerators` (Go 1.23+) may produce changes if there are map iteration loops or Len/At-style APIs. The codebase uses `go-git` and no explicit map iteration loops were found, but these analyzers scan broadly.

**Recommendation:** Run `go fix ./...` first (without `-fix`) to see diagnostics, then apply with `-fix`.

---

### 2. What are the migration paths for `golang.org/x/exp/slog` → `log/slog`? Are there API differences?

**Migration path:** Import path replacement only. `golang.org/x/exp/slog` and `log/slog` (Go 1.21+) are **API-identical**.

**Confirmed by code analysis:**
- `cmd/easyp/main.go:24` → `log/slog`
- `internal/connect/api.go:7` → `log/slog`
- `internal/providers/multisource/repo.go:8` → `log/slog`
- `internal/providers/github/repos.go:9` → `log/slog`
- `internal/providers/github/client.go:11` → `log/slog`
- `internal/providers/bitbucket/client.go:14` → `log/slog`
- `internal/providers/bitbucket/repos.go:10` → `log/slog`
- `internal/providers/localgit/localgit.go:15` → `log/slog`
- `internal/providers/cache/artifactory/artifactory.go:14` → `log/slog`

**Functions used in this codebase** (all API-compatible):
- `slog.Logger` — type, identical
- `slog.String(key, value)` → `log/slog.String(...)` ✓
- `slog.Int(key, value)` → `log/slog.Int(...)` ✓
- `slog.Duration(key, value)` → `log/slog.Duration(...)` ✓
- `slog.Any(key, value)` → `log/slog.Any(...)` ✓
- `slog.New(handler)` → `log/slog.New(...)` ✓
- `slog.NewJSONHandler(os.Stdout, opts)` → `log/slog.NewJSONHandler(...)` ✓
- `slog.HandlerOptions` → `log/slog.HandlerOptions` ✓
- `slog.Level` → `log/slog.Level` ✓
- `slog.LevelDebug`, `slog.LevelInfo`, `slog.LevelWarn`, `slog.LevelError` → all identical ✓
- `log.Enabled(ctx, level)` → identical ✓
- `log.Error(...)`, `log.Warn(...)`, `log.Info(...)`, `log.Debug(...)` → all identical ✓

**No behavioral changes.** Type signatures are identical. Only the import path changes.

**Custom migration script:** Simple `sed`/`perl` replacement of import paths, then run `goimports` to reorder imports per gci linter rules.

---

### 3. What are the migration paths for `golang.org/x/exp/slices` → `slices`? Are there API differences?

**Migration path:** Import path replacement only. `golang.org/x/exp/slices` and `slices` (Go 1.21+) are **API-identical**.

**Confirmed by code analysis:**
- `internal/providers/github/getfiles.go:10` → `slices`
- `internal/providers/github/repos.go:8` → `slices`
- `internal/providers/filter/filter.go:8` → `slices`
- `internal/providers/bitbucket/getfiles.go:8` → `slices`
- `internal/providers/bitbucket/repos.go:9` → `slices`
- `internal/providers/localgit/localgit.go:18` → `slices`

**Functions used in this codebase** (all API-compatible):
- `slices.IndexFunc(s, func(T) bool)` → identical in stdlib ✓
- `slices.SortFunc(s, func(T, T) int)` → identical in stdlib ✓

**Both functions have identical signatures.** No function name changes, no parameter changes, no return value changes. The stdlib `slices` package was modeled directly after the exp package.

**Note:** The `go fix` analyzers `slicescontains` and `slicessort` are **related but different**:
- `slicessort` replaces `sort.Slice(s, func(i,j int) bool { return s[i] < s[j] })` with `slices.Sort(s)` — the codebase does NOT use `sort.Slice` directly (it uses `slices.SortFunc`), so this analyzer won't apply
- `slicescontains` replaces element-existence loops with `slices.Contains` — the codebase uses `slices.IndexFunc` which is already idiomatic, so this likely won't apply

**Custom migration script:** Same approach as slog — replace import paths, then `goimports` to fix ordering.

---

### 4. How does `go mod tidy` behave after replacing exp imports? Does it auto-remove the golang.org/x/exp dependency?

**Yes, `go mod tidy` will auto-remove `golang.org/x/exp`** from `go.mod` after the imports are replaced, but only if no other package transitively depends on it.

**Critical finding from `go mod graph`:**
```
github.com/go-git/go-billy/v5@v5.9.0 golang.org/x/exp@v0.0.0-20260410095643-746e56fc9e2f
```

The `go-billy/v5` package (v5.9.0) is a transitive dependency of `go-git/go-git/v5` and **transitively imports `golang.org/x/exp`**. This means `go mod tidy` will NOT remove `golang.org/x/exp` from `go.mod` even after our application code no longer imports it directly.

**Two options:**
1. **Leave `golang.org/x/exp` in go.mod** as an indirect dependency (managed by go-billy). This is the pragmatic approach — the direct dependency is removed, but the transitive dependency remains until `go-billy` updates.
2. **Downgrade `go-billy`** to a version that doesn't use `golang.org/x/exp`. Current `go-billy/v5` is `v5.9.0`. Check if a newer version removes the exp dependency.

**Decision for planner:** After exp migration, run `go mod tidy` and check if `golang.org/x/exp` remains. If it does (due to go-billy), document this as a deferred action — the direct dependency is eliminated (tech debt addressed), but the transitive dependency remains until a go-billy update. The goal from CONCERNS.md "Fix approach" is satisfied: "Replace all `golang.org/x/exp/slog` with `log/slog` and `golang.org/x/exp/slices` with `slices`" — accomplished. The transitive dependency is a separate issue.

**Verification step:** After exp migration, `go mod tidy` should produce clean output. Check `go.mod` for remaining `golang.org/x/exp` line and note if it's `// indirect`.

---

### 5. Are there any other `golang.org/x/exp` packages in use beyond `slog` and `slices`?

**No.** Scanning all `.go` files (excluding `api/_third_party/`) shows only two exp subpackages:

- `golang.org/x/exp/slog` — 8 files (listed above in Q2)
- `golang.org/x/exp/slices` — 6 files (listed above in Q3)

Total: 14 import references across 13 unique files.

**Note about `api/_third_party/` (git submodule — excluded from migration):**
The submodule does use `golang.org/x/exp/slices` and `golang.org/x/exp/constraints`, but per D-05 this directory must not be modified. The submodule is a read-only dependency.

**No `golang.org/x/exp/maps`, `slog`, `constraints`, or other exp packages** are used in application code.

---

### 6. What potential issues could arise during the migration (lint failures, API incompatibilities)?

**slog/slices migration:**
- **No API incompatibilities** — both packages are API-identical per Go 1.21 stdlib promotion
- **No build failures expected** — purely import path changes
- **Import ordering** — after replacement, the imports need re-ordering. The gci linter (`.golangci.yml:73-82`) enforces: (1) stdlib, (2) third-party, (3) project. Both `log/slog` and `slices` are stdlib, so they move from the third-party group to the stdlib group. `goimports` handles this automatically, but the gci linter must pass after the change.

**go fix potential issues:**

| Analyzer | Risk | Details |
|----------|------|---------|
| `omitzero` | **Medium** — behavior change | Replaces `json:",omitempty"` with `omitzero"` on struct fields. This changes encoding behavior (zero-value structs are now omitted instead of encoded). Only applies if there are struct fields with `omitempty`. No such fields found in codebase scan, but `go fix` should be reviewed before auto-apply. |
| `stditerators` | **Low** | Replaces Len/At-style loops with iterators. May produce changes if there are such patterns in provider code. Unlikely. |
| `mapsloop` | **Low** | Replaces map iteration with maps package functions (Go 1.23+). May produce changes for any map-to-map copy patterns. Unlikely. |
| `inline` | **Low** | Looks for `//go:fix inline` directives — none exist in this codebase. Read-only for this project. |
| `newexpr` | **Low** | Only applies to auto-generated wrapper functions. Unlikely to apply. |
| `waitgroup` | **N/A** | Requires `sync.WaitGroup` goroutine patterns — codebase uses go-git which may already use modern patterns internally, but application code doesn't directly use WaitGroup. |

**Linter interaction:**
- After `go fix` and exp migration, `golangci-lint run ./...` must pass (`.golangci.yml` has `enable-all: true`)
- Most likely issue: `gci` (go-imports-order linter) may flag import groups after migration until `goimports` is run
- No new `//nolint` suppressions expected from go fix itself, but `go fix` may introduce linter-triggering patterns that need suppression (e.g., if `omitzero` is applied to a kubebuilder-annotated struct, but this codebase doesn't use kubebuilder)

**Build/test verification:**
- No tests currently exist in the project (per CONCERNS.md "Entire project has zero test coverage")
- E2E tests (`e2e/`) exist and must pass after migration (per D-04)
- Build: `go build ./...` must pass
- `go vet ./...` should be clean

---

### 7. Should `go fix` and exp migration be done in separate commits? What's the recommended order?

**Yes, separate commits are recommended** per D-03: "Multiple sequential commits — one commit per step."

**Recommended commit sequence:**

1. **Commit 1: `go fix`** — Apply all `go fix` modernizations (`go fix ./... -fix`)
   - Review diff first with `go fix ./...` (without `-fix`)
   - Commit message: `"Modernize Go code with go fix"`
   - Test: `go build ./...` and `go vet ./...`

2. **Commit 2: exp migration** — Replace `golang.org/x/exp/slog` → `log/slog` and `golang.org/x/exp/slices` → `slices`
   - Custom script: `find . -name "*.go" ! -path "./api/_third_party/*" -exec sed -i 's|golang.org/x/exp/slog|log/slog|g' {} \;` and same for slices
   - Then run `goimports -w .` to fix import ordering
   - Commit message: `"Replace golang.org/x/exp imports with stdlib equivalents"`
   - Test: `go build ./...`

3. **Commit 3: `go mod tidy`** — Clean module graph
   - `go mod tidy`
   - Commit message: `"Run go mod tidy after exp migration"`
   - Review `go.mod` changes (check if `golang.org/x/exp` is removed or remains as indirect)

4. **Commit 4: verification** — Full verification pass
   - `go build ./...`
   - `golangci-lint run ./...`
   - E2E tests: `go test -v ./e2e/...` or `./e2e.test`
   - Commit message: `"Verify build and tests after modernization"`

**Rationale for separation:**
- Each step is independently verifiable
- If `go fix` introduces an issue, it's isolated to commit 1
- If `go mod tidy` produces unexpected changes, they're isolated to commit 3
- Clean revert path for any step
- Matches the D-03 decision from 08-CONTEXT.md

**Alternative (combined):** If go fix produces no changes (expected), the planner may combine commits 1+2 into a single "modernization" commit. But running `go fix` first in dry-run mode will reveal whether it finds anything.

---

### 8. What files are directly affected by the exp migration (confirmed from CONCERNS.md)?

**13 files confirmed with `golang.org/x/exp` imports** (excluding `api/_third_party/`):

**`slog` (8 files, 8 unique):**
```
cmd/easyp/main.go:24
internal/connect/api.go:7
internal/providers/multisource/repo.go:8
internal/providers/github/repos.go:9
internal/providers/github/client.go:11
internal/providers/bitbucket/client.go:14
internal/providers/bitbucket/repos.go:10
internal/providers/cache/artifactory/artifactory.go:14
```

**`slices` (6 files, 6 unique):**
```
internal/providers/github/getfiles.go:10
internal/providers/github/repos.go:8
internal/providers/filter/filter.go:8
internal/providers/bitbucket/getfiles.go:8
internal/providers/bitbucket/repos.go:9
internal/providers/localgit/localgit.go:18
```

**Note:** `internal/providers/github/repos.go` and `internal/providers/bitbucket/repos.go` have **both** `slog` and `slices` imports.

**Additional confirmed from CONCERNS.md §Tech Debt:**
- `internal/providers/localgit/localgit.go:15` — confirmed ✓ (slog)
- All other files in CONCERNS.md §Tech Debt match the grep findings

**Scope for migration script:** All 13 files, excluding `api/_third_party/` (git submodule, per D-05).

---

### 9. Are there any third-party dependencies that transitively depend on `golang.org/x/exp`?

**Yes.** From `go mod graph`:

```
github.com/go-git/go-billy/v5@v5.9.0 golang.org/x/exp@v0.0.0-20260410095643-746e56fc9e2f
```

`go-billy/v5` (v5.9.0) transitively depends on `golang.org/x/exp`. This is a dependency chain:
```
github.com/easyp-tech/server
  → github.com/go-git/go-git/v5@v5.19.0
    → github.com/go-git/go-billy/v5@v5.9.0
      → golang.org/x/exp
```

**Impact on migration:**
- After replacing all direct exp imports, `go mod tidy` will NOT remove `golang.org/x/exp` from `go.mod` because `go-billy/v5` still depends on it
- The dependency will appear as `golang.org/x/exp v0.0.0-... // indirect` in `go.mod`
- This is acceptable per the tech debt goal (direct imports eliminated), but should be documented

**Options for planner:**
1. **Accept indirect dependency** — Document that `golang.org/x/exp` remains as an indirect dependency via go-billy. Track upgrade of go-billy to a non-exp version in a future phase.
2. **Attempt go-billy upgrade** — Check if a newer version of `go-billy/v5` removes the exp dependency. If so, upgrade `go-billy` alongside the exp migration.
   - Current go-billy: v5.9.0
   - Check `go list -m -versions github.com/go-git/go-billy/v5` for newer versions

**Recommendation:** Check go-billy versions before committing. If a newer version without exp exists, upgrade it in the same phase (add as a mini-step between exp migration and go mod tidy). Otherwise, accept the indirect dependency and document it.

---

### 10. What's the expected state of `go.mod` and `go.sum` after the migration?

**Before migration (current state):**
```
module github.com/easyp-tech/server
go 1.26

require (
    ...
    golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f   # DIRECT dependency
    ...
)
```

**After migration (expected state, option A — no go-billy upgrade):**
```
module github.com/easyp-tech/server
go 1.26

require (
    ...
    golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f   # INDIRECT via go-billy (if no go-billy upgrade)
    ...
)

require (
    ...
    github.com/go-git/go-billy/v5 v5.9.0                  # still brings in exp
    ...
)
```

**After migration (expected state, option B — go-billy upgraded):**
```
module github.com/easyp-tech/server
go 1.26

require (
    ... (golang.org/x/exp REMOVED — no longer needed)
    ...
)
```

`go.sum` will be updated by `go mod tidy` accordingly — entries for `golang.org/x/exp` may remain if it's still indirectly required (option A) or will be pruned (option B).

**Verification checklist:**
- [ ] `golang.org/x/exp` is NOT in the direct `require` block (non-indirect)
- [ ] `go mod tidy` exits with code 0
- [ ] `go build ./...` passes
- [ ] All import paths in 13 files updated from `golang.org/x/exp/slog` → `log/slog` and `golang.org/x/exp/slices` → `slices`
- [ ] No remaining references to `golang.org/x/exp` in application code (excluding `api/_third_party/`)

---

## Summary of Risks and Concerns for Planner

| Risk | Severity | Mitigation |
|------|----------|------------|
| `go-billy` transitively keeps `golang.org/x/exp` in go.mod | Low | Check go-billy versions; accept indirect dep if no upgrade available |
| `go fix` may apply `omitzero` changes that alter JSON encoding behavior | Medium | Run `go fix` without `-fix` first to review; exclude or individually review `omitzero` changes |
| `go fix` may produce many changes across many files | Low | Review diff before applying; separate commits per D-03 |
| `gci` import ordering linter may fail until `goimports` runs | Low | Run `goimports -w .` after exp migration script |
| `api/_third_party/` submodule has its own exp imports but must not be modified | Low | Migration script must explicitly exclude `./api/_third_party/` |
| No unit tests — only E2E tests verify correctness | Medium | Build verification (`go build ./...`) is the primary correctness check; E2E tests provide integration confidence |
| `internal/logger/logger.go` still uses deprecated logging but is unused (per CONCERNS.md) | Out of scope | Deferred — separate cleanup phase |

---

*Research for Phase 8: 08-Go-Code-Modernization*
*Sources: go tool fix -help, go doc, go mod graph, grep analysis of codebase, CONCERNS.md, CONVENTIONS.md, 08-CONTEXT.md, go.mod*
