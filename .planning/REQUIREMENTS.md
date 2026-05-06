# Requirements: EasyP Buf Proxy — Protocol Modernization

**Defined:** 2026-05-07
**Core Value:** The proxy must correctly serve both old (v1.30.1) and modern (v1.69.0+) Buf CLI clients simultaneously

## v1 Requirements

### Build & Code Generation

- [ ] **BCG-01**: Proto source switched from old `buf` submodule to `buf-v1.69.0` submodule in `api/proto/generate.go`
- [ ] **BCG-02**: connect-go upgraded to v1.18.1 in `go.mod`
- [ ] **BCG-03**: `gen/proto/` regenerated from v1.69.0 proto definitions and project compiles without errors
- [ ] **BCG-04**: go-grpc plugin removed from `api/proto/buf.gen.yaml` codegen pipeline (unused at runtime)

### Handler Adaptation

- [ ] **HAND-01**: Handler structs updated to embed new `Unimplemented*` types from regenerated code
- [ ] **HAND-02**: Existing RPC logic (`GetModulePins`, `DownloadManifestAndBlobs`, `GetRepositoryByFullName`, `GetRepositoriesByFullName`) works correctly with new generated types
- [ ] **HAND-03**: `manifest_digest` field populated on `ModulePin` responses if modern buf CLI requires it
- [ ] **HAND-04**: `GetSDKInfo` RPC returns appropriate response or `CodeUnimplemented` based on modern buf CLI behavior

### Test Infrastructure

- [ ] **TINF-01**: Test helper programmatically starts and stops the proxy server with TLS using `~/local-tls/server/` certs
- [ ] **TINF-02**: Buf binary v1.30.1 and v1.69.0+ pinned and managed for test execution (downloaded or path-configured)
- [ ] **TINF-03**: Test suite configured with GitHub API token for real API calls
- [ ] **TINF-04**: Test GitHub repository identified/configured for test operations (repo with proto files)
- [ ] **TINF-05**: Tests can run in parallel without port conflicts or state interference
- [ ] **TINF-06**: Test configuration supports CI execution with environment-based setup

### Old Protocol Validation (buf v1.30.1)

- [ ] **OLD-01**: `buf mod update` succeeds against the proxy using buf v1.30.1 binary with real GitHub provider
- [ ] **OLD-02**: `buf dep update` succeeds against the proxy using buf v1.30.1 binary with real GitHub provider

### New Protocol Validation (buf v1.69.0+)

- [ ] **NEW-01**: `buf mod update` succeeds against the proxy using buf v1.69.0+ binary with real GitHub provider
- [ ] **NEW-02**: `buf dep update` succeeds against the proxy using buf v1.69.0+ binary with real GitHub provider

## v2 Requirements

### Additional Protocol Support

- **ADDL-01**: Implement full `GetSDKInfo` response with SDK resolution logic
- **ADDL-02**: Implement repository group management RPCs (`AddRepositoryGroup`, `UpdateRepositoryGroup`, `RemoveRepositoryGroup`)

### Extended Testing

- **ADDL-03**: Test suite covers BitBucket provider
- **ADDL-04**: Test suite covers local git provider
- **ADDL-05**: Test suite covers Artifactory cache

## Out of Scope

| Feature | Reason |
|---------|--------|
| BitBucket provider testing | GitHub provider is sufficient for protocol validation |
| Local git provider testing | Not relevant to protocol correctness — tests GitHub provider only |
| Artifactory cache testing | Not relevant to protocol correctness |
| Removing old v1alpha1 protocol | Both protocols must coexist — old clients must keep working |
| BSR-specific features (labels, recommendations, sync) | These were removed from the modern proto and proxy never implemented them |
| Push functionality | Proxy is read-only; push was never implemented |
| mTLS testing | Basic TLS is sufficient for protocol validation |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| BCG-01 | Phase 1 | Pending |
| BCG-02 | Phase 1 | Pending |
| BCG-03 | Phase 1 | Pending |
| BCG-04 | Phase 1 | Pending |
| HAND-01 | Phase 2 | Pending |
| HAND-02 | Phase 2 | Pending |
| HAND-03 | Phase 2 | Pending |
| HAND-04 | Phase 2 | Pending |
| TINF-01 | Phase 3 | Pending |
| TINF-02 | Phase 3 | Pending |
| TINF-03 | Phase 3 | Pending |
| TINF-04 | Phase 3 | Pending |
| TINF-05 | Phase 3 | Pending |
| TINF-06 | Phase 3 | Pending |
| OLD-01 | Phase 4 | Pending |
| OLD-02 | Phase 4 | Pending |
| NEW-01 | Phase 5 | Pending |
| NEW-02 | Phase 5 | Pending |

**Coverage:**
- v1 requirements: 18 total
- Mapped to phases: 18
- Unmapped: 0 ✓

---
*Requirements defined: 2026-05-07*
*Last updated: 2026-05-07 after initial definition*
