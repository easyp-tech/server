# Roadmap: EasyP Buf Proxy — Protocol Modernization

## Overview

Modernize the Buf registry proxy to serve both old (v1.30.1) and modern (v1.69.0+) Buf CLI clients. The journey starts with mechanical code generation from updated proto definitions, then adapts handlers to the new generated types, builds test infrastructure for integration testing with real buf binaries, and validates backward compatibility with the old protocol before confirming support for the new one.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Code Generation** - Switch proto source to v1.69.0, upgrade connect-go, regenerate code, verify build *(completed 2026-05-07)*
- [x] **Phase 2: Handler Adaptation** - Update handler structs to embed new Unimplemented types, verify all RPCs compile and serve *(completed 2026-05-07)*
- [x] **Phase 3: Test Infrastructure** - Build reusable test helpers for TLS server, buf binary management, and GitHub API integration *(completed 2026-05-07)*
- [x] **Phase 4: Old Protocol Validation** - Confirm buf v1.30.1 still works against the updated proxy using real binaries and real GitHub API *(completed 2026-05-07)*
- [x] **Phase 5: New Protocol Validation** - Confirm buf v1.69.0+ works against the proxy, discover any required new RPC implementations *(completed 2026-05-07)*

## Phase Details

### Phase 1: Code Generation
**Goal**: Project compiles against v1.69.0 proto definitions with updated dependencies
**Depends on**: Nothing (first phase)
**Requirements**: BCG-01, BCG-02, BCG-03, BCG-04
**Success Criteria** (what must be TRUE):
  1. `generate.go` points at the `buf-v1.69.0` submodule and `go generate ./api/proto/...` completes without errors
  2. `go.mod` lists `connectrpc.com/connect` v1.18.1 and `go mod tidy` shows no version conflicts
  3. `go build ./...` succeeds with newly generated proto code replacing the old generated code
  4. `buf.gen.yaml` no longer includes the go-grpc plugin in the codegen pipeline
**Plans**: 2 plans

Plans:
- [x] 01-01: Switch proto source and upgrade dependencies
- [x] 01-02: Regenerate proto code and verify build

### Phase 2: Handler Adaptation
**Goal**: Server binary compiles, starts, and serves RPCs using new generated types with all new RPCs returning Unimplemented
**Depends on**: Phase 1
**Requirements**: HAND-01, HAND-02, HAND-03, HAND-04
**Success Criteria** (what must be TRUE):
  1. Handler structs in `internal/connect/` embed the new `Unimplemented*Handler` types from regenerated code and the server starts without panics
  2. Existing RPCs (`GetModulePins`, `DownloadManifestAndBlobs`, `GetRepositoryByFullName`, `GetRepositoriesByFullName`) compile and return correct response types for known request patterns
  3. `GetSDKInfo` returns a gRPC `CodeUnimplemented` error (per D-01)
  4. `ModulePin` responses include `manifest_digest` field present but empty (per D-02)
**Plans**: 1 plan

Plans:
- [x] 02-01-PLAN.md — Verify handler adaptation baseline and run E2E smoke tests for both buf CLI versions

### Phase 3: Test Infrastructure
**Goal**: Reusable test helpers exist for starting a TLS proxy server, managing pinned buf binaries, and making authenticated GitHub API calls
**Depends on**: Phase 2
**Requirements**: TINF-01, TINF-02, TINF-03, TINF-04, TINF-05, TINF-06
**Success Criteria** (what must be TRUE):
  1. A test helper can programmatically start the proxy server with TLS using `~/local-tls/server/` certs and stop it cleanly after the test
  2. Both buf v1.30.1 and v1.69.0+ binaries are downloaded (or path-configured) and their versions are asserted before test execution
  3. Tests read GitHub API token and target repository from environment variables and fail fast with a clear message if not configured
  4. Multiple tests can run in parallel without port conflicts or shared state interference
  5. Test configuration supports CI execution via environment variables with no hardcoded paths or secrets
**Plans**: 2 plans

Plans:
- [x] 03-01-PLAN.md — Create testutil package with config generation, server lifecycle, and buf binary management
- [x] 03-02-PLAN.md — Refactor smoke test to use testutil and create internal validation tests

### Phase 4: Old Protocol Validation
**Goal**: Backward compatibility confirmed — buf v1.30.1 commands work against the updated proxy
**Depends on**: Phase 3
**Requirements**: OLD-01, OLD-02
**Success Criteria** (what must be TRUE):
  1. `buf mod update` succeeds against the proxy using buf v1.30.1 binary with a real GitHub provider and produces a valid `buf.lock` file
  2. `buf dep update` (reinterpreted as two-step `buf mod update`) succeeds against the proxy using buf v1.30.1 binary with a real GitHub provider
**Plans**: 1 plan

Plans:
- [x] 04-01-PLAN.md — Expose server output from StartServer and create two-step buf mod update test for OLD-02

### Phase 5: New Protocol Validation
**Goal**: Modern buf CLI support confirmed — buf v1.69.0+ commands work against the proxy, and any required new RPC implementations are identified
**Depends on**: Phase 4
**Requirements**: NEW-01, NEW-02
**Success Criteria** (what must be TRUE):
  1. `buf mod update` succeeds against the proxy using buf v1.69.0+ binary with a real GitHub provider and produces a valid `buf.lock` file
  2. `buf dep update` succeeds against the proxy using buf v1.69.0+ binary with a real GitHub provider
**Plans**: 2 plans

Plans:
- [x] 05-01-PLAN.md — Add RunBufDepUpdate helper and write new protocol tests with debug logging for v1.69.0
- [x] 05-02-PLAN.md — Fix any RPC implementation blockers discovered by Plan 05-01 testing

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Code Generation | 2/2 | Complete | 2026-05-07 |
| 2. Handler Adaptation | 1/1 | Complete | 2026-05-07 |
| 3. Test Infrastructure | 2/2 | Complete | 2026-05-07 |
| 4. Old Protocol Validation | 1/1 | Complete | 2026-05-07 |
| 5. New Protocol Validation | 2/2 | Complete | 2026-05-07 |
