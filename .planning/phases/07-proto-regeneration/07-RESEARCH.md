# Phase 7: Proto Regeneration & Verification — Research

**Researched:** 2026-05-08
**Phase:** 07-Proto-Regeneration

---

## What I Needed to Know to Plan This Phase

### 1. What does connect-go v1.19.x change in generated code?

**Key finding: No structural breaking changes in v1.19.x for this codebase.**

connect-go v1.19.2 is a minor version with internal improvements and compatibility with Go 1.24+. The critical change relevant to this codebase is:

- **v1.19 changed `require_unimplemented_servers` default from `false` to `true`**
  - This affects what the `protoc-gen-connect-go` plugin generates when the option is NOT set
  - **Your `buf.gen.yaml` explicitly sets `require_unimplemented_servers=false`** for both go and connect-go plugins (line 8, 59)
  - So the generated `Unimplemented*` handler types will remain stub implementations (empty struct + `CodeUnimplemented` returns), not forced to require full method implementations

**What stays the same:**
- `Unimplemented{SvcName}Handler` type names are stable and have been consistent since connect-go v1.7+
- Handler interface signatures (`func(context.Context, *connect.Request[Req]) (*connect.Response[Resp], error)`) are stable across v1.18→v1.19
- Generated file header and package structure is stable
- Compile-time version assertion: `const _ = connect.IsAtLeastVersion1_7_0` stays in generated code

**What might change (proto-level, not connect-go level):**
- buf v1.69.0 proto definitions may add new RPC methods to existing services
- New methods would appear in generated handler interfaces but are handled by `require_unimplemented_servers=false`
- The three services this codebase uses (RepositoryService, ResolveService, DownloadService) may have additional methods in v1.69.0 proto vs older versions

### 2. Current generated code state

**Examined:** `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/`

All 31 connect files follow the same pattern:
```
Unimplemented{SvcName}Handler struct{}
// Returns connect.NewError(connect.CodeUnimplemented, errors.New("..."))
```

**Three Unimplemented types used by `internal/connect/api.go`:**
- `UnimplementedRepositoryServiceHandler` — repository.connect.go:681
- `UnimplementedResolveServiceHandler` — resolve.connect.go:402
- `UnimplementedDownloadServiceHandler` — download.connect.go:150

**Additional Unimplemented type found (not used by api.go but in same package):**
- `UnimplementedLocalResolveServiceHandler` — resolve.connect.go:533 (new service, also in handlers)

The current code already compiles cleanly — `go build ./...` passes with Go 1.26.3 and connect-go v1.19.2. This means the current generated code is compatible with the upgraded dependencies. The regeneration is needed to ensure generated code is freshly produced by the same tooling version as the runtime.

### 3. How regeneration works in this codebase

**Generation entry point:** `api/proto/generate.go`

```go
//go:generate rm -rf ./buf
//go:generate cp -r ../_third_party/buf-v1.69.0/proto/buf ./
//go:generate rm -rf ../../gen
//go:generate buf generate
```

**Pipeline:**
1. Clear and copy proto sources from `api/_third_party/buf-v1.69.0/proto/buf/` → `api/proto/buf/`
2. Clear `gen/proto/` entirely
3. Run `buf generate` with `buf.gen.yaml` (in `api/proto/`)

**buf.gen.yaml plugins:**
- `go` plugin → `gen/proto/` with `paths=source_relative` + ~50 `M=` overrides
- `connect-go` plugin → `gen/proto/` with `paths=source_relative` + `require_unimplemented_servers=false` + same `M=` overrides

**Critical:** The `M=` overrides must cover all proto files to avoid package path mismatches. The current config has ~50 `M=` entries for v1alpha1 registry protos. New proto files added in buf v1.69.0 that aren't covered would cause compilation errors.

### 4. What might break during regeneration

**Risk 1: New proto files in v1.69.0 not covered by M= overrides**
- The `buf.gen.yaml` currently covers ~50 specific proto files
- If buf v1.69.0 introduced new proto files in the registry package, they would be generated without M= overrides → package path mismatch → compilation failure
- **Mitigation:** After regeneration, `go build ./...` will immediately reveal any such gaps. Fix by adding M= entries.

**Risk 2: New RPC methods on existing services**
- New methods appear in generated handler interfaces
- With `require_unimplemented_servers=false`, the Unimplemented* types provide `CodeUnimplemented` stubs for new methods
- The `api.go` struct embeds these → continues to compile (embed provides stubs)
- No action needed unless new methods have special signatures

**Risk 3: Generated file API changes from connect-go version bump**
- connect-go v1.19.2 uses `connectrpc.com/connect` import path
- Generated files already use this import path (verified in current code)
- No change expected in generated API surface

**Risk 4: Dependency mismatches after regeneration**
- `go mod tidy` may be needed if regenerated code pulls in different protobuf versions
- D-05 addresses this: "Run `go mod tidy` if any dependency mismatches appear after regeneration"

### 5. E2E test strategy and what they verify

**Existing E2E tests (`e2e/`):**
- `smoke_test.go` — runs `buf mod update` against proxy with both v1.30.1 and v1.69.0 buf binaries
- `new_proto_test.go` — modern protocol with v1.69.0 buf + `buf mod update` + `buf dep update`
- `old_proto_test.go` — backward compatibility with v1.30.1

**What they test:**
- Proxy correctly serves both old and modern Buf CLI clients
- `buf mod update` produces `buf.lock` (exit code 0)
- No protocol-level regressions

**E2E test infrastructure:**
- `testutil.GetBuf(t, version)` — downloads/fetches pinned buf binaries from `testdata/buf/{version}/buf`
- `testutil.StartServer(t, cfg)` — starts `go run ./cmd/easyp` as subprocess on random port, waits for TCP readiness
- `testutil.RunBufModUpdate(t, bufPath, port)` — creates temp dir, writes `buf.yaml` with proxy as dep, runs `buf mod update`
- `testutil.RequireEnvToken(t, envVar)` — skips test if `EASYP_GITHUB_TOKEN` not set

**Critical for Phase 7:** After regeneration, all three E2E tests should pass without modification (D-04). If they fail, the regenerated code has a protocol compatibility problem.

### 6. What the plan needs to sequence

1. **Regenerate proto code** — `cd api/proto && go generate`
   - This copies proto sources, clears gen/, runs buf generate
   - Expected: ~30 new/overwritten `.connect.go` files, ~30 new/overwritten `.pb.go` files

2. **Check for compilation errors** — `go build ./...`
   - Compiler errors indicate: missing M= overrides, new handler interface methods not implemented
   - D-03 says: compile and fix iteratively

3. **Update `internal/connect/api.go` embed lines** (if needed)
   - Regenerated code may have same or new Unimplemented* type names
   - D-01 says: regenerate first, then update embed lines
   - The three embeds are: `connect.UnimplementedRepositoryServiceHandler`, `connect.UnimplementedResolveServiceHandler`, `connect.UnimplementedDownloadServiceHandler`
   - Most likely: names stay the same, no changes needed

4. **Run `go mod tidy`** — if dependency mismatches appear
   - D-05: conditional, run only if needed

5. **Verify clean build** — `go build ./...`
   - Confirms all generated code + handler code compiles together

6. **Run E2E tests** — `go test ./e2e/... -v`
   - Smoke test with both buf versions
   - Exit 0 = DEPS-05, DEPS-06, DEPS-07 all satisfied

### 7. Key decisions already made (locked)

| Decision | Content | Impact on plan |
|----------|---------|----------------|
| D-01 | Regenerate first, then update embed lines | Step 1 before step 3 |
| D-02 | Full regeneration via `go generate` in api/proto | Single command, not per-service |
| D-03 | Compile and fix iteratively | Don't pre-empt errors, let compiler guide |
| D-04 | Run existing E2E tests unchanged | Don't modify tests, just run them |
| D-05 | Run `go mod tidy` if needed | Conditional, not automatic |

### 8. What remains deferred to planner's judgment

- **Commit strategy:** Commit generated code separately or together with handler fixes
- **golangci-lint compatibility:** Whether lint passes with regenerated code on Go 1.26 (not in phase scope but relevant)
- **Specific error fixes:** D-03 defers to compiler to guide sequence — each error gets its own fix

---

## Knowledge Gaps (for planner awareness)

1. **buf v1.69.0 proto additions**: I could not verify if new proto files were added to the registry package that aren't covered by the 50 M= overrides. The first compilation attempt will reveal this. If new files appear, the fix is mechanical: add `M=<new_proto>=github.com/easyp-tech/server/gen/proto/<path>` entries to `buf.gen.yaml`.

2. **connect-go v1.19.x changelog**: Could not fetch GitHub releases page. Based on semantic versioning, v1.19.2 should be backward-compatible with v1.18.x for generated code API surface. The `require_unimplemented_servers` default change is the only behavioral difference, and it's explicitly set to `false` in the project.

3. **Whether Unimplemented* type names change**: Verified that current generated code uses `Unimplemented{SvcName}Handler` naming, which is stable across connect-go versions. The v1.19.2 generator produces the same naming convention.

---

## Summary for Planning

**Phase 7 is low-risk** given:
- Build already compiles cleanly with Go 1.26 + connect-go v1.19.2
- `require_unimplemented_servers=false` in buf.gen.yaml prevents stub requirement explosions
- E2E tests already cover both protocol versions
- Regeneration is a single `go generate` command
- Any compilation errors are expected to be mechanical (M= overrides for new proto files, if any)

**The plan should sequence:**
1. Run `cd api/proto && go generate`
2. Run `go build ./...` — capture any errors
3. Add any missing M= entries to `buf.gen.yaml` if compilation fails on new proto files
4. Update `internal/connect/api.go` embed lines if new Unimplemented* names appear (unlikely)
5. Run `go mod tidy` conditionally
6. Verify `go build ./...` passes
7. Run `go test ./e2e/...` with both buf versions
8. Update STATE.md and mark phase complete

**Acceptance criteria:**
- DEPS-05: `go build ./...` passes
- DEPS-06: E2E tests pass with both buf v1.30.1 and v1.69.0+
- DEPS-07: `internal/connect/api.go` handler struct compiles with regenerated Unimplemented* types

---

*Researcher: Claude Code*
*Phase: 07-Proto-Regeneration*