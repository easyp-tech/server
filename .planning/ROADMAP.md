# Roadmap: EasyP Buf Proxy — Dependency Modernization

## Overview

Upgrade Go from 1.22 to 1.26, update all dependencies to their latest compatible versions, regenerate proto code with the new connect-go, and verify that all build and E2E tests pass.

## Phases

**Phase Numbering:**
Integer phases continue sequentially from v1.1 (which completed at Phase 5).

- [ ] **Phase 6: Dependency Upgrades** — Update Go to 1.26, connect-go to v1.19.x, and all other dependencies to latest; verify `go build ./...` passes

- [ ] **Phase 7: Proto Regeneration & Verification** — Regenerate proto code with new connect-go, update handler structs for new Unimplemented* types, verify E2E tests pass with both buf versions

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

- [ ] 06-01: Update go.mod to Go 1.26 and upgrade all dependencies to latest
- [ ] 06-02: Run `go mod tidy` and verify `go build ./...` passes

### Phase 7: Proto Regeneration & Verification

**Goal**: Proto code regenerated with new connect-go; handlers compile and E2E tests pass

**Depends on**: Phase 6

**Requirements**: DEPS-05, DEPS-06, DEPS-07

**Success Criteria** (observable outcomes):

1. `go generate` produces new proto code from existing buf submodule that compiles against connect-go v1.19.x
2. Handler structs in `internal/connect/` embed the new `Unimplemented*Handler` types and compile without errors
3. E2E tests pass with both buf v1.30.1 and v1.69.0+ after the dependency and code generation updates

**Plans**: 2 plans

- [ ] 07-01: Regenerate proto code and verify compilation with updated connect-go
- [ ] 07-02: Run full E2E test suite with both buf v1.30.1 and v1.69.0+ to confirm everything works

## Progress

**Execution Order:**

Phases execute in numeric order: 6 → 7

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 6. Dependency Upgrades | 0/2 | In Progress | — |
| 7. Proto Regeneration & Verification | 0/2 | Pending | — |