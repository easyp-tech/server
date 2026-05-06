# Project Research Summary

**Project:** easyp-buf-proxy
**Domain:** Buf registry protocol proxy -- adding modern Buf CLI (v1.69.0+) support
**Researched:** 2026-05-07
**Confidence:** HIGH

## Executive Summary

This project is a Buf Schema Registry (BSR) proxy that translates BSR protocol requests into VCS (GitHub, Bitbucket, local Git) operations, allowing `buf mod update` and `buf build` to work against Git repositories instead of the BSR. The core task is adding support for modern buf CLI (v1.69.0+) while maintaining backward compatibility with the currently-supported deprecated version (v1.30.1).

The research reveals a critical simplification: both old and new buf CLI versions use **identical proto package paths** (`buf.alpha.registry.v1alpha1`), **identical service names**, and **identical HTTP procedure paths**. The modern proto is a strict superset of the old -- existing RPCs have unchanged request/response messages, and the only additions are new RPCs that a proxy does not need to implement (SDK version resolution, repository group management). This means the project does NOT need a dual-protocol architecture. A single Connect RPC handler generated from the v1.69.0 proto definitions will serve both old and new buf CLI clients correctly, using protobuf's built-in forward-compatibility.

The primary risks are not protocol incompatibility but rather build chain issues (code generation pointing at the wrong proto submodule, connect-go version mismatches), test infrastructure complexity (TLS certificate trust, buf binary versioning, GitHub API rate limits), and one empirical unknown: whether modern buf CLI requires any of the new RPCs during `buf mod update` and fails on `CodeUnimplemented` responses.

## Key Findings

### Recommended Stack

The stack change is minimal. No new dependencies are required. The core update is upgrading `connectrpc.com/connect` from v1.11.1 to v1.18.1 (the latest version supporting Go 1.22 -- v1.19.x requires Go 1.24). Everything else stays as-is.

**Core technologies:**
- **Go 1.22:** Runtime -- staying on current version; Go 1.24 upgrade is a separate milestone
- **connectrpc.com/connect v1.18.1:** RPC framework -- upgrade from v1.11.1 for compatibility with newly generated Connect stubs; v1.18.1 is the latest supporting Go 1.21+
- **Buf CLI:** Code generation -- generate from v1.69.0 proto submodule instead of the old v1.30.1 submodule; buf.gen.yaml stays on v1 format
- **google.golang.org/protobuf:** Protobuf runtime -- minor security update acceptable but not blocking
- **Existing VCS providers (go-github, go-git):** Unchanged -- no protocol changes affect the provider layer

### Expected Features

**Must have (table stakes -- already implemented, must continue working):**
- `GetRepositoryByFullName` / `GetRepositoriesByFullName` -- module discovery by owner/repo name
- `GetModulePins` -- dependency resolution via `buf mod update`
- `DownloadManifestAndBlobs` -- module content download

**Should verify (new RPCs to handle via Unimplemented):**
- `GetSDKInfo` -- new unified SDK version resolution; HIGH risk if buf CLI requires it during core workflows
- `GetCargoVersion` / `GetNugetVersion` / `GetCmakeVersion` -- niche SDK version RPCs; LOW risk

**Defer (not proxy concerns):**
- Repository group management (`AddRepositoryGroup`, etc.) -- write operations, proxy is read-only
- Full SDK version implementation -- requires BSR plugin registry knowledge the proxy does not have
- Go 1.24 upgrade and connect-go v1.19.x -- separate milestone

### Architecture Approach

The recommended architecture is a **single superset handler**. Generate Go code from the v1.69.0 proto definitions, replacing the old generated code entirely. The generated Connect RPC handlers implement the modern interfaces, with `Unimplemented*Handler` embeddings absorbing new RPCs the proxy does not serve. Old buf clients call the same HTTP paths with the same messages and work without any changes. The provider layer (`internal/providers/`) is completely untouched.

**Major components (in dependency order):**
1. **`api/proto/generate.go`** -- Proto source switch: copy from `buf-v1.69.0` instead of old `buf`
2. **`gen/proto/`** -- Regenerated Go code from v1.69.0 protos (replaces old generated code)
3. **`internal/connect/`** -- RPC handlers: update to embed new `Unimplemented*Handler` types; core handler logic likely unchanged
4. **Provider layer** (`internal/providers/`) -- Completely unchanged; the API layer talks to it via the same `provider` interface

### Critical Pitfalls

1. **Generating from old protos silently** -- `generate.go` has the old submodule path hardcoded. If not updated first, regeneration silently reverts to old definitions. Prevention: update `generate.go` to point at `buf-v1.69.0` before any regeneration.

2. **Over-engineering dual-protocol handlers** -- Creating separate handler packages for old and new protocols is unnecessary and impossible (same Go package paths, same HTTP paths). Prevention: use a single handler generated from v1.69.0 protos.

3. **connect-go version mismatch** -- Newly generated stubs may reference APIs not in v1.11.1. Prevention: upgrade connect-go to v1.18.1 before regeneration.

4. **Wrong buf binary in tests** -- Tests using `$PATH` buf instead of pinned binaries produce misleading results. Prevention: download both versions to explicit test paths, assert version before each test.

5. **TLS certificate trust in tests** -- Self-signed certs may not be trusted by buf CLI's Go TLS stack. Prevention: generate test certs programmatically or use `SSL_CERT_FILE`.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Code Generation and Build Verification
**Rationale:** Everything depends on generated code. This is the foundation -- switch proto source, regenerate, verify the build compiles. No behavioral changes yet.
**Delivers:** Project compiles against v1.69.0 proto definitions; new Unimplemented RPC stubs in generated code; old generated files for removed protos cleaned up.
**Addresses:** Stack updates (connect-go v1.18.1, proto regeneration)
**Avoids:** Pitfall 1 (dual handlers), Pitfall 2 (wrong proto source), Pitfall 7 (version mismatch)

### Phase 2: Handler Adaptation
**Rationale:** With new generated code in place, update handler structs to embed the new `Unimplemented*Handler` types. Existing handler methods (GetModulePins, GetRepositoryByFullName, DownloadManifestAndBlobs) should compile unchanged since their request/response types are identical.
**Delivers:** Server binary that compiles and starts with new proto definitions; all existing RPCs work; new RPCs return `CodeUnimplemented`.
**Uses:** connect-go v1.18.1, regenerated proto code
**Implements:** `internal/connect/api.go` struct updates; potential `manifest_digest` field population in modulepins.go
**Avoids:** Pitfall 4 (implementing unneeded RPCs), Pitfall 15 (missing M mappings)

### Phase 3: Test Infrastructure
**Rationale:** Before running integration tests, build proper test infrastructure: pinned buf binaries, programmatic TLS certs, port-auto-assignment, test helpers. This is foundational for all subsequent validation.
**Delivers:** Reusable test infrastructure for both old and new buf CLI versions.
**Avoids:** Pitfall 3 (wrong binary), Pitfall 4 (TLS trust), Pitfall 9 (port conflicts), Pitfall 11 (race on server startup)

### Phase 4: Validation with Old Protocol (buf v1.30.1)
**Rationale:** Verify backward compatibility. The proto diff says nothing changed for existing RPCs, but this must be empirically confirmed. Run `buf mod update` with old CLI against the server built with new protos.
**Delivers:** Confirmed backward compatibility with buf v1.30.1.
**Avoids:** Pitfall 14 (is_bsr_head field removal)

### Phase 5: Validation with New Protocol (buf v1.69.0+)
**Rationale:** This is the goal. Test modern buf CLI against the proxy. The critical unknown is whether `buf mod update` calls `GetSDKInfo` or other new RPCs and fails on `Unimplemented`. This phase will discover that empirically.
**Delivers:** Confirmed support for modern buf CLI; discovery of any required new RPC implementations.
**Flags:** May require a follow-up phase to implement `GetSDKInfo` or other RPCs if testing shows they are required for core workflows.
**Avoids:** Pitfall 10 (unimplemented RPCs breaking CLI), Pitfall 5 (GitHub API rate limits), Pitfall 6 (wrong buf.yaml registry)

### Phase Ordering Rationale

- **Code generation first** because all handler code depends on generated types. Changing the proto source is a prerequisite for everything else.
- **Handler adaptation before testing** because you need a compiling binary before you can test anything.
- **Test infrastructure before validation** because the project currently has no integration tests with real buf binaries. Building this once avoids manual testing errors.
- **Old protocol before new** because old protocol validation is a smoke test that the regeneration did not break anything. If it fails, stop and fix before testing new protocol.
- **New protocol last** because this is the target state, and it may discover additional work (implementing `GetSDKInfo`).

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 5 (New Protocol Validation):** Needs empirical testing to discover which new RPCs buf v1.69.0 actually calls during `buf mod update` and `buf build`. No amount of document research can answer this -- it requires running the CLI against a server and observing behavior.
- **Phase 2 (Handler Adaptation):** The `manifest_digest` field risk is MEDIUM confidence. Research could not determine whether modern buf CLI requires this field to be populated. May need implementation if validation shows it is required.

Phases with standard patterns (skip additional research):
- **Phase 1 (Code Generation):** Well-documented buf generate workflow, direct proto file diff available.
- **Phase 3 (Test Infrastructure):** Standard Go testing patterns (httptest, exec.Command, t.Cleanup).
- **Phase 4 (Old Protocol Validation):** Should pass trivially since proto messages are identical for implemented RPCs.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Version constraints verified from GitHub releases. Proto diff directly inspected. Go module compatibility verified. |
| Features | HIGH | Direct proto file comparison with line-level diff. No external sources needed -- all facts from codebase analysis. |
| Architecture | HIGH | Proto package identity confirmed. Connect RPC path identity confirmed. Provider interface segregation documented in existing code. |
| Pitfalls | HIGH | Most pitfalls derived from direct codebase analysis. Connect RPC documentation from Context7 confirmed handler patterns. One empirical unknown (GetSDKInfo requirement). |

**Overall confidence:** HIGH

### Gaps to Address

- **GetSDKInfo requirement:** Cannot determine from documentation alone whether buf v1.69.0 calls `GetSDKInfo` during `buf mod update`. Must be tested empirically in Phase 5. If required, will need a stub implementation.
- **manifest_digest field:** The modern `ModulePin` message includes a `manifest_digest` field. Unknown whether modern buf CLI requires it. May need implementation in Phase 2 based on Phase 5 validation results.
- **Connect-go v1.18.1 generated code compatibility:** HIGH confidence that generated stubs work with v1.18.1, but should verify immediately after regeneration with `go build ./...`. The version check constant in generated code may reference a newer version marker.

## Sources

### Primary (HIGH confidence)
- Direct proto file comparison: `api/_third_party/buf/` vs `api/_third_party/buf-v1.69.0/` -- all findings about protocol compatibility
- Existing codebase analysis: `internal/connect/`, `gen/proto/`, `api/proto/` -- architecture and handler patterns
- connect-go Context7 docs: `/connectrpc/connect-go` -- handler registration, Unimplemented pattern, version compatibility
- buf CLI Context7 docs: `/bufbuild/buf` -- code generation, buf.gen.yaml configuration
- connect-go GitHub releases: version constraints and changelog entries verified

### Secondary (MEDIUM confidence)
- BSR on-prem TLS guidance: buf.build/docs -- test infrastructure recommendations
- Buf CLI stability policy: github.com/bufbuild/buf README -- "no breaking changes within v1.x"

---
*Research completed: 2026-05-07*
*Ready for roadmap: yes*
