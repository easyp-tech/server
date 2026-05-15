# Phase 2: Handler Adaptation - Context

**Gathered:** 2026-05-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Ensure the proxy's handler layer works correctly with the regenerated v1.69.0 proto types. The existing RPC implementations must compile and function against the new generated code. E2E smoke tests confirm both old buf CLI (v1.30.1) and modern buf CLI (v1.69.0+) can communicate with the proxy. New RPCs (GetSDKInfo, GetCargoVersion, etc.) are left as Unimplemented — their requirements will be discovered empirically.

</domain>

<decisions>
## Implementation Decisions

### GetSDKInfo handling
- **D-01:** Leave GetSDKInfo as Unimplemented (returns `CodeUnimplemented`). Do not stub a response. Phase 5 will discover through empirical testing with modern buf CLI whether a real implementation is needed.

### manifest_digest handling
- **D-02:** Leave `manifest_digest` field empty in ModulePin responses. Do not compute or populate it. Phase 5 will discover through empirical testing whether modern buf CLI requires it populated.

### Verification approach
- **D-03:** Include E2E smoke tests for BOTH old and modern buf CLI versions in Phase 2: start the TLS proxy server, run `buf mod update` with buf v1.30.1 AND buf v1.69.0+ against the proxy, verify both succeed. This provides early validation for both protocol variants end-to-end.
- **D-04:** The E2E smoke tests should use minimal test infrastructure — TLS server startup using `~/local-tls/server/` certs, both buf binaries available on PATH, GitHub API token from environment variable. Phase 3 will formalize this into reusable test helpers.

### HAND-01 status
- **D-05:** HAND-01 (handler structs embed new Unimplemented* types) is effectively already complete from Phase 1 — the regenerated code expanded the Unimplemented types, and the existing embedding in `api.go` satisfies all expanded interfaces. No handler struct changes needed.

### Claude's Discretion
- Exact E2E test structure and helper functions — Claude can choose the simplest approach that validates the server works.
- How to structure the test file (table-driven, single test function, etc.).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Handler code (must understand before modifying)
- `internal/connect/api.go` — Connect RPC handler struct definitions with Unimplemented embedding
- `internal/connect/modulepins.go` — ResolveService handler (GetModulePins)
- `internal/connect/blobs.go` — DownloadService handler (DownloadManifestAndBlobs)
- `internal/connect/bynames.go` — RepositoryService handlers (GetRepositoryByFullName, GetRepositoriesByFullName)

### Generated code (target types for handler adaptation)
- `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/resolve.connect.go` — Expanded ResolveServiceHandler interface with GetSDKInfo, GetCargoVersion, etc.
- `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/repository.connect.go` — Expanded RepositoryServiceHandler interface with AddRepositoryGroup, etc.
- `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/download.connect.go` — DownloadServiceHandler interface
- `gen/proto/buf/alpha/module/v1alpha1/module.pb.go` — ModulePin type with manifest_digest field

### Server wiring
- `cmd/easyp/main.go` — Server entry point, wires Connect handlers

### Phase 1 context (carries forward decisions)
- `.planning/phases/01-code-generation/01-CONTEXT.md` — Locked decisions from Phase 1
- `.planning/phases/01-code-generation/01-RESEARCH.md` — Proto diff analysis, connect-go changelog audit

### Project decisions
- `.planning/PROJECT.md` — Key Decisions table (single superset handler, connect-go v1.18.1 ceiling)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/connect/api.go` — The `api` struct already embeds all three Unimplemented types. No struct changes needed.
- `cmd/easyp/main.go` — Server wiring with TLS support already exists. E2E test can reuse the same startup pattern.
- `~/local-tls/server/` — Self-signed TLS certs already set up for local testing.

### Established Patterns
- Connect RPC handler pattern: struct embeds `Unimplemented*Handler`, implements specific methods, returns `*connect.Response[T]`.
- The proxy is read-only — all RPC implementations fetch data from GitHub/VCS APIs and translate to proto types.
- ModulePin is populated with Remote, Owner, Repository, Commit — no digest computation at resolve time.

### Integration Points
- `cmd/easyp/main.go` → `internal/connect/api.go` → `internal/connect/{modulepins,blobs,bynames}.go` — the handler pipeline
- Handler → `provider` interface → GitHub API — data flows from VCS through handlers to proto responses
- E2E test: buf CLI → TLS proxy → handler → GitHub API → handler → buf CLI

</code_context>

<specifics>
## Specific Ideas

- E2E smoke tests should run `buf mod update` with BOTH buf v1.30.1 and buf v1.69.0+ binaries — validates both old and modern protocol compatibility.
- Test needs: GitHub API token (env var), TLS certs (~/local-tls/server/), buf v1.30.1 and buf v1.69.0+ binaries on PATH.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 2-Handler Adaptation*
*Context gathered: 2026-05-07*
