# Roadmap: EasyP Buf Proxy — Dependency Modernization

## Overview

Upgrade Go from 1.22 to 1.26, update all dependencies to their latest compatible versions, regenerate proto code with the new connect-go, and verify that all build and E2E tests pass.

## Phases

**Phase Numbering:**
Integer phases continue sequentially from v1.1 (which completed at Phase 5).

- [x] **Phase 6: Dependency Upgrades** — Update Go to 1.26, connect-go to v1.19.x, and all other dependencies to latest; verify `go build ./...` passes (2026-05-08)

- [x] **Phase 7: Proto Regeneration & Verification** — Regenerate proto code with new connect-go, update handler structs for new Unimplemented* types, verify E2E tests pass with both buf versions (2026-05-08)

## Phase Details

### Phase 6: Dependency Upgrades

**Goal**: Go 1.26, connect-go v1.19.x, and all dependencies updated to latest compatible versions with a clean `go mod tidy`

**Depends on**: Nothing (first phase of v1.2)

**Requirements**: DEPS-01, DEPS-02, DEPS-03, DEPS-04

**Success Criteria** (observable outcomes):

1. `go.mod` declares `go 1.26` and `go build ./...` completes without errors
2. `connectrpc.com/connect` is upgraded to v1.19.x and `go mod tidy` produces no conflicts
3. All other dependencies (go-git, go-github, yaml, crypto, exp/slog, protobuf) are updated to their latest compatible versions
4. `go mod tidy` run completes cleanly with no unused or missing dependencies

**Plans**: 2 plans

- [x] 06-01: Update go.mod to Go 1.26 and upgrade all dependencies to latest
- [x] 06-02: Run `go mod tidy` and verify `go build ./...` passes

### Phase 7: Proto Regeneration & Verification

**Goal**: Proto code regenerated with new connect-go; handlers compile and E2E tests pass

**Depends on**: Phase 6

**Requirements**: DEPS-05, DEPS-06, DEPS-07

**Success Criteria** (observable outcomes):

1. `go generate` produces new proto code from existing buf submodule that compiles against connect-go v1.19.x
2. Handler structs in `internal/connect/` embed the `Unimplemented*Handler` types and compile without errors
3. E2E tests pass with both buf v1.30.1 and v1.69.0+ after the dependency and code generation updates

**Plans**: 2 plans

- [x] 07-01: Regenerate proto code and verify compilation with updated connect-go
- [x] 07-02: Run full E2E test suite with both buf v1.30.1 and v1.69.0+ to confirm everything works

### Phase 8: Go Code Modernization

**Goal**: Modernize Go code using `go fix`, replace deprecated `golang.org/x/exp` imports with stdlib equivalents

**Depends on**: Phase 7

**Success Criteria** (observable outcomes):

1. `go fix ./...` runs cleanly and applies all recommended modernizations
2. All `golang.org/x/exp/slog` imports replaced with `log/slog`
3. All `golang.org/x/exp/slices` imports replaced with `slices`
4. `go build ./...` passes without errors
5. E2E tests pass after modernization

**Plans**: 1 plan

- [x] 08-01: Apply go fix and replace deprecated exp imports

### Phase 9: Submodule Cleanup

**Goal**: Clean up the API submodule structure — remove old `buf` submodule, promote `buf-v1.69.0` to canonical `buf`, update code generation, regenerate and verify

**Depends on**: Phase 8

**Success Criteria** (observable outcomes):

1. `api/_third_party/buf` submodule removed (old protocol deprecated)
2. `api/_third_party/buf-v1.69.0` renamed to `api/_third_party/buf` via `git mv` (preserves history)
3. `.gitmodules` updated to single buf entry at `api/_third_party/buf`
4. `api/proto/generate.go` updated to copy from `../_third_party/buf/proto/buf`
5. `go generate` produces fresh proto code without errors
6. `go build ./...` passes with regenerated code
7. E2E tests pass

**Plans**: 1 plan

- [ ] 09-01: Submodule cleanup — remove old buf, rename buf-v1.69.0, update config, regenerate, test

## Progress

**Execution Order:**

Phases execute in numeric order: 6 → 7

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 6. Dependency Upgrades | 2/2 | Complete | 2026-05-08 |
| 7. Proto Regeneration & Verification | 2/2 | Complete | 2026-05-08 |

**Milestone v1.2: In Progress**

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 6. Dependency Upgrades | 2/2 | Complete | 2026-05-08 |
| 7. Proto Regeneration & Verification | 2/2 | Complete | 2026-05-08 |
| 8. Go Code Modernization | 1/1 | Complete | 2026-05-08 |
| 9. Submodule Cleanup | 1/1 | Ready to execute | — |

All 7 v1 requirements satisfied:
- DEPS-01 through DEPS-04: Phase 6 (Go 1.26, connect-go v1.19.x, all deps updated)
- DEPS-05 through DEPS-07: Phase 7 (proto regenerated, handlers compile, E2E tests pass)

---

*Roadmap last updated: 2026-05-08*
