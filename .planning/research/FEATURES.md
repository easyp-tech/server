# Feature Landscape

**Domain:** Buf registry protocol proxy -- modern protocol addition
**Researched:** 2026-05-07
**Confidence:** HIGH (direct proto file comparison, no external sources needed)

## Protocol Diff Summary

The proto package path remains `buf.alpha.registry.v1alpha1` in both old (v1.30.1-compatible) and new (v1.69.0) versions. This is significant: Buf has NOT introduced a `v1` or `v1beta1` package. Both old and new buf CLI clients speak the same protobuf package and service names. The changes are additive within the same package -- new RPCs and messages added to existing services, some RPCs/messages removed from the old that are not in the new.

### Critical Finding: Same Service Names, Same gRPC Procedure Paths

Both old and new protos define the same three services the proxy implements:
- `buf.alpha.registry.v1alpha1.RepositoryService`
- `buf.alpha.registry.v1alpha1.ResolveService`
- `buf.alpha.registry.v1alpha1.DownloadService`

This means the HTTP procedure paths are identical between versions (e.g., `/buf.alpha.registry.v1alpha1.DownloadService/DownloadManifestAndBlobs`). The new buf CLI will call the same endpoints. The server does NOT need separate route registrations for old vs new -- it needs to handle the expanded set of RPCs that the new client may call.

### Files Removed in v1.69.0 (present in old, absent in new)

| Proto File | Had Services | Impact on Proxy |
|------------|-------------|-----------------|
| `labels.proto` | Yes: `LabelService` | LOW -- Proxy never implemented this |
| `recommendation.proto` | No (messages only) | LOW -- Proxy never used these types |
| `sync.proto` | Yes: `SyncService` | LOW -- Proxy never implemented this |

These removals are BSR-specific features (BSR labels, recommendations, git sync). A proxy that serves VCS-backed modules never implemented them and does not need to.

### Files Changed Between Old and New

Only the changes relevant to the three services the proxy implements are listed below. Full diff details are in the analysis sections that follow.

## Table Stakes (Must Implement)

These are the RPCs the existing proxy ALREADY implements. They MUST continue to work for both old and new buf CLI clients. No changes needed -- the request/response message structures for these RPCs are identical between old and new protos.

### RepositoryService -- Currently Implemented RPCs

| Feature (RPC) | Why Expected | Complexity | Notes |
|---------------|-------------|------------|-------|
| `GetRepositoryByFullName` | Core discovery: buf CLI discovers modules by owner/repo name | LOW (existing) | No message changes between old/new |
| `GetRepositoriesByFullName` | Batch version of above, used by `buf mod update` | LOW (existing) | No message changes between old/new |

### ResolveService -- Currently Implemented RPCs

| Feature (RPC) | Why Expected | Complexity | Notes |
|---------------|-------------|------------|-------|
| `GetModulePins` | Core dependency resolution: `buf mod update` resolves module references to pinned commits | LOW (existing) | Request/response messages unchanged |

### DownloadService -- Currently Implemented RPCs

| Feature (RPC) | Why Expected | Complexity | Notes |
|---------------|-------------|------------|-------|
| `DownloadManifestAndBlobs` | Core module download: `buf mod update` fetches module content | LOW (existing) | Request/response messages unchanged |

### Currently Implemented RPCs That Remain Unimplemented (No Change Needed)

These RPCs are defined in the old proto, remain in the new proto, and the proxy returns `CodeUnimplemented` for all of them. The new buf CLI will handle `CodeUnimplemented` gracefully for these.

| Feature (RPC) | Why Safe to Leave Unimplemented | Complexity |
|---------------|-------------------------------|------------|
| `GetRepository` | Uses ID-based lookup; proxy only supports name-based | N/A |
| `ListRepositories` | BSR listing; proxy has no use case | N/A |
| `ListUserRepositories` | BSR user-scoped listing | N/A |
| `ListRepositoriesUserCanAccess` | BSR auth-scoped listing | N/A |
| `ListOrganizationRepositories` | BSR org-scoped listing | N/A |
| `CreateRepositoryByFullName` | Write operation; proxy is read-only | N/A |
| `DeleteRepository` | Write operation; proxy is read-only | N/A |
| `DeleteRepositoryByFullName` | Write operation; proxy is read-only | N/A |
| `DeprecateRepositoryByName` | Write operation; proxy is read-only | N/A |
| `UndeprecateRepositoryByName` | Write operation; proxy is read-only | N/A |
| `SetRepositoryContributor` | Write operation; proxy is read-only | N/A |
| `ListRepositoryContributors` | BSR contributor management | N/A |
| `GetRepositorySettings` | BSR settings management | N/A |
| `UpdateRepositorySettingsByName` | Write operation; proxy is read-only | N/A |
| `GetRepositoriesMetadata` | BSR metadata; proxy returns minimal data | N/A |
| `GetRepositoryDependencyDOTString` | BSR dependency graph; proxy has no deps graph | N/A |
| `Download` (legacy) | Deprecated in old proto too; `DownloadManifestAndBlobs` is the modern path | N/A |
| `GetGoVersion` | SDK version resolution; proxy is not a BSR | N/A |
| `GetSwiftVersion` | SDK version resolution; proxy is not a BSR | N/A |
| `GetMavenVersion` | SDK version resolution; proxy is not a BSR | N/A |
| `GetNPMVersion` | SDK version resolution; proxy is not a BSR | N/A |
| `GetPythonVersion` | SDK version resolution; proxy is not a BSR | N/A |

## Differentiators (New RPCs in v1.69.0 -- Must Handle)

These RPCs are NEW in the v1.69.0 proto. The new buf CLI MAY call them. The proxy does NOT need to implement them functionally, but MUST return proper `CodeUnimplemented` responses (which the generated `Unimplemented*` handlers already do). The key risk is: if the new buf CLI requires any of these RPCs to complete its core workflow (`buf mod update`, `buf build`), the proxy must implement them or the CLI will fail.

| Feature (RPC) | Service | Value Proposition | Complexity | Risk Assessment |
|---------------|---------|-------------------|------------|-----------------|
| `GetSDKInfo` | ResolveService | Unified SDK version resolution replacing all GetXxxVersion RPCs | MEDIUM | HIGH -- likely called by `buf generate` with `--managed` mode; must test if `buf mod update` calls it |
| `GetCargoVersion` | ResolveService | Rust Cargo SDK version | LOW | LOW -- niche, only called for Rust plugins |
| `GetNugetVersion` | ResolveService | .NET NuGet SDK version | LOW | LOW -- niche, only called for .NET plugins |
| `GetCmakeVersion` | ResolveService | C++ CMake SDK version | LOW | LOW -- niche, only called for C++ plugins |
| `AddRepositoryGroup` | RepositoryService | IdP group management | LOW | LOW -- write operation, proxy is read-only |
| `UpdateRepositoryGroup` | RepositoryService | IdP group management | LOW | LOW -- write operation, proxy is read-only |
| `RemoveRepositoryGroup` | RepositoryService | IdP group management | LOW | LOW -- write operation, proxy is read-only |

### RPCs REMOVED in v1.69.0 (existed in old, not in new)

| Feature (RPC) | Service | Impact | Notes |
|---------------|---------|--------|-------|
| `GetRepositoryContributor` | RepositoryService | LOW | Was never implemented by proxy; old clients may call it, new clients will not |
| `GetReviewFlowGracePeriodPolicy` | AdminService | NONE | Proxy never implemented AdminService |
| `UpdateReviewFlowGracePeriodPolicy` | AdminService | NONE | Proxy never implemented AdminService |
| `SetOrganizationMember` | OrganizationService | NONE | Proxy never implemented OrganizationService |
| `GetUserPluginPreferences` | UserService | NONE | Proxy never implemented UserService |
| `UpdateUserPluginPreferences` | UserService | NONE | Proxy never implemented UserService |

### Message-Level Changes in Existing RPCs

These changes affect message types but NOT the request/response of the RPCs the proxy implements.

| Change | File | Impact on Proxy |
|--------|------|-----------------|
| `LocalModuleResolveResult.is_bsr_head` field removed (reserved) | resolve.proto | NONE -- proxy does not use `LocalResolveService` |
| `GetRemotePackageVersionPlugin` gained `revision` field (field 4) | resolve.proto | LOW -- only affects GetXxxVersion RPCs which proxy returns Unimplemented |
| `Organization.idp_groups` changed from `repeated string` to `repeated IdPGroup` | organization.proto | NONE -- proxy does not use OrganizationService |
| Search results gained new fields (latest_spdx_license_id, owner_verification_status, url, latest_commit_time) | search.proto | NONE -- proxy does not implement SearchService |
| Plugin messages gained `doc`, `collections`, `deprecated` fields | plugin_curation.proto | NONE -- proxy does not serve plugins |
| New config types: `CargoConfig`, `NugetConfig`, `CmakeConfig` | plugin_curation.proto | NONE -- proxy does not serve plugins |
| `DisplayService` gained `DisplayPluginElements` RPC | display.proto | NONE -- proxy does not implement DisplayService |

## Anti-Features (Things to Explicitly NOT Implement)

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Separate service route paths for old vs new protocol | Both versions use identical gRPC procedure paths; no dual registration needed | Use single Connect handler that serves all RPCs |
| Implementing any GetXxxVersion RPCs | These require BSR plugin registry knowledge the proxy does not have | Return `CodeUnimplemented`; buf CLI falls back gracefully |
| Implementing `GetSDKInfo` fully | Requires BSR plugin/module metadata the proxy cannot provide | Return `CodeUnimplemented` UNLESS testing proves `buf mod update` requires it |
| Implementing `SyncService`, `LabelService`, `RecommendationService` | These are BSR-only features removed in v1.69.0; proxy never had them | Not applicable |
| Implementing write operations (Create/Delete/Update) | Proxy is read-only by design | Return `CodeUnimplemented` |
| Generating code from the NEW proto into the same Go package as old proto | The old generated code is in `gen/proto/`; generating new code into the same paths will overwrite and may break imports | Generate new proto code into a separate directory (e.g., `gen/proto-v2/`) or replace the old generation entirely if only new protocol is needed |

## Feature Dependencies

```
RepositoryService.GetRepositoryByFullName (existing)
    -> provider.GetMeta()
    -> No dependencies on other RPCs

ResolveService.GetModulePins (existing)
    -> provider.GetMeta() for each module reference
    -> No dependencies on other RPCs

DownloadService.DownloadManifestAndBlobs (existing)
    -> provider.GetFiles()
    -> No dependencies on other RPCs
    -> NOTE: buf CLI calls GetModulePins BEFORE this to get commit hash

Core workflow path:
    buf mod update ->
        ResolveService.GetModulePins ->
        DownloadService.DownloadManifestAndBlobs

Repository discovery path:
    buf mod update ->
        RepositoryService.GetRepositoryByFullName OR GetRepositoriesByFullName ->
        ResolveService.GetModulePins ->
        DownloadService.DownloadManifestAndBlobs
```

No new RPC dependencies exist between the old and new protocol. The new RPCs in ResolveService (`GetSDKInfo`, `GetCargoVersion`, etc.) are independent of the core workflow.

## MVP Recommendation

### Phase 1: Validate existing implementation with new buf CLI
Priority: Test that the existing three RPCs work with buf v1.69.0+

1. `GetRepositoryByFullName` (existing, unchanged)
2. `GetModulePins` (existing, unchanged)
3. `DownloadManifestAndBlobs` (existing, unchanged)

**Rationale:** The proto message structures for these three RPCs are identical between old and new. The new buf CLI calls the same HTTP procedure paths. The most likely outcome is: it just works.

### Phase 2: Test and handle edge cases
Priority: Determine if new buf CLI requires any new RPCs for core workflows

1. Test `buf mod update` with v1.69.0 against proxy -- if it fails, check which RPC is missing
2. Most likely candidate: `GetSDKInfo` in ResolveService (new unified SDK resolution)
3. If `GetSDKInfo` is required, implement a minimal version that returns `CodeUnimplemented` with a clear message

### Phase 3: Code generation update
Priority: Replace old generated code with new proto-generated code

1. Generate Go code from the v1.69.0 proto definitions
2. Regenerate Connect RPC stubs
3. Verify all existing handler implementations compile against new generated types
4. Run full test suite with both buf v1.30.1 and v1.69.0

**Defer:** Full implementation of `GetSDKInfo`, `GetCargoVersion`, `GetNugetVersion`, `GetCmakeVersion` -- these are only needed if testing proves they are called during `buf mod update` or `buf build` workflows.

## Proto File Inventory

### Old (v1.30.1 compatible) -- 36 files
Located at: `api/_third_party/buf/proto/buf/alpha/registry/v1alpha1/`

### New (v1.69.0) -- 33 files
Located at: `api/_third_party/buf-v1.69.0/proto/buf/alpha/registry/v1alpha1/`

### Removed (old only)
- `labels.proto` -- `LabelService` + label types
- `recommendation.proto` -- recommendation message types
- `sync.proto` -- `SyncService` + git sync types

### Changed (substantive, not just copyright year)
- `admin.proto` -- removed ReviewFlowGracePeriodPolicy RPCs, added fields to GetClusterUsageRequest
- `display.proto` -- added `DisplayPluginElements` RPC, added `limited_write` field
- `organization.proto` -- removed `SetOrganizationMember`, added `UpdateOrganizationGroup`, changed `idp_groups` type
- `plugin_curation.proto` -- added CargoConfig, NugetConfig, CmakeConfig, DotnetTargetFramework enum, PluginCollection, new fields on Plugin
- `resolve.proto` -- added `GetSDKInfo`, `GetCargoVersion`, `GetNugetVersion`, `GetCmakeVersion` RPCs; added `revision` to `GetRemotePackageVersionPlugin`; removed `is_bsr_head` from `LocalModuleResolveResult`
- `repository.proto` -- removed `GetRepositoryContributor` RPC; added `AddRepositoryGroup`, `UpdateRepositoryGroup`, `RemoveRepositoryGroup` RPCs
- `role.proto` -- added `RepositoryRoleSource` enum
- `search.proto` -- added fields to search result types
- `user.proto` -- removed `UserPluginPreference` message and related RPCs

### Unchanged (only copyright year)
- `authn.proto`, `authz.proto`, `convert.proto`, `doc.proto`, `download.proto`, `git_metadata.proto`, `github.proto`, `image.proto`, `jsonschema.proto`, `module.proto`, `owner.proto`, `push.proto`, `reference.proto`, `repository_branch.proto`, `repository_commit.proto`, `repository_tag.proto`, `resource.proto`, `scim_token.proto`, `studio.proto`, `studio_request.proto`, `token.proto`, `verification_status.proto`, `webhook.proto`

## Key Insight: Minimal Implementation Effort

The proto diff reveals that the core protocol (the 3 RPCs the proxy implements) has NOT changed between v1.30.1 and v1.69.0. The `download.proto` is unchanged (copyright year only). The `GetModulePins` request/response messages in `resolve.proto` are unchanged. The `GetRepositoryByFullName` request/response messages in `repository.proto` are unchanged.

The primary implementation work is:

1. **Code generation**: Generate Go/Connect code from the v1.69.0 protos (replacing or alongside the old generated code)
2. **Recompile**: Verify existing handler code compiles against new generated types
3. **Test**: Run buf v1.69.0 against the proxy with real TLS + GitHub API
4. **Handle new Unimplemented RPCs**: The new generated code will include `GetSDKInfo`, `GetCargoVersion`, `GetNugetVersion`, `GetCmakeVersion` in the `ResolveServiceHandler` interface, plus group management RPCs in `RepositoryServiceHandler`. The `Unimplemented*` handlers will return `CodeUnimplemented` automatically.

The main risk is that `buf v1.69.0` may call `GetSDKInfo` as part of `buf mod update` and may not gracefully handle `CodeUnimplemented`. This must be tested empirically.

## Sources

- Direct proto file comparison: `api/_third_party/buf/proto/buf/alpha/registry/v1alpha1/` vs `api/_third_party/buf-v1.69.0/proto/buf/alpha/registry/v1alpha1/`
- Existing generated Connect code: `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/`
- Existing handler implementation: `internal/connect/{api.go,bynames.go,modulepins.go,blobs.go}`
- Module proto: `api/_third_party/buf/proto/buf/alpha/module/v1alpha1/module.proto` (identical to v1.69.0)
