# Architecture Patterns: Dual-Protocol Buf Proxy

**Domain:** Buf registry proxy serving both deprecated (v1.30.1) and modern (v1.69.0+) protocols simultaneously
**Researched:** 2026-05-07

## Recommended Architecture

```text
                          buf CLI (v1.30.1)         buf CLI (v1.69.0+)
                                |                          |
                                v                          v
                    ┌───────────────────────────────────────────────┐
                    │         HTTP Entry Point (TLS)                │
                    │   cmd/easyp/main.go (ListenAndServe)          │
                    │   loggingMiddleware wraps composite handler   │
                    └───────────────────┬───────────────────────────┘
                                        |
                                        v
                    ┌───────────────────────────────────────────────┐
                    │           http.ServeMux (router)              │
                    │   Routes by URL path prefix:                  │
                    │   /buf.alpha.registry.v1alpha1.*              │
                    ├──────────────────────┬────────────────────────┤
                    │                      │                        │
          OLD (existing)          NEW (to be built)
                    │                      │                        │
                    v                      v                        │
   ┌────────────────────────┐  ┌────────────────────────┐          │
   │ Old Connect API Layer  │  │ New Connect API Layer  │          │
   │ internal/connectold/   │  │ internal/connectnew/   │          │
   │ (renamed from          │  │                        │          │
   │  internal/connect/)    │  │                        │          │
   │                        │  │                        │          │
   │ Handlers implement:    │  │ Handlers implement:    │          │
   │ - old RepositorySvc   │  │ - new RepositorySvc    │          │
   │ - old ResolveSvc      │  │ - new ResolveSvc       │          │
   │ - old DownloadSvc     │  │ - new DownloadSvc      │          │
   └──────────┬─────────────┘  └──────────┬─────────────┘          │
              |                            |                        │
              |     Both depend on the     |                        │
              |     SAME provider iface    |                        │
              └────────────┬───────────────┘                        │
                           v                                        │
              ┌─────────────────────────┐                          │
              │  Shared provider        │                          │
              │  interface              │                          │
              │  GetMeta / GetFiles     │                          │
              └────────────┬────────────┘                          │
                           v                                        │
              ┌──────────────────────────────────────────────────┐ │
              │           Multi-Source Router                     │ │
              │   internal/providers/multisource/                 │ │
              ├──────────┬───────────────┬────────────────────────┤ │
              │ localgit │  bitbucket    │   github               │ │
              └──────────┴───────────────┴────────────────────────┘ │
                           |                                        │
                           v                                        │
              ┌──────────────────────────────────────────────────┐ │
              │           Cache Layer                             │ │
              └──────────────────────────────────────────────────┘ │
```

## Critical Discovery: Same Proto Package Path

**HIGH confidence.** Both the old (v1.30.1) and modern (v1.69.0) proto definitions use the **identical proto package path**: `buf.alpha.registry.v1alpha1`. This is not a versioned package like `v1` vs `v2`. Both live under the same `v1alpha1` namespace.

This means:
1. The generated Connect RPC handler paths are **identical** -- `/buf.alpha.registry.v1alpha1.ResolveService/`, `/buf.alpha.registry.v1alpha1.DownloadService/`, `/buf.alpha.registry.v1alpha1.RepositoryService/`
2. Both old and new buf CLI clients hit the **same URL paths** on the server
3. There is NO automatic way for the server to distinguish old vs new clients by path alone
4. The server must distinguish based on the **specific RPC method called** or the **request/response message structure**

### Proto Diff Summary

The differences between old and modern protos are:

**download.proto:** IDENTICAL -- no structural changes.

**resolve.proto:** New version adds these RPCs to ResolveService:
- `GetSDKInfo` -- SDK info resolution
- `GetCargoVersion` -- Cargo registry version resolution
- `GetNugetVersion` -- Nuget registry version resolution
- `GetCmakeVersion` -- CMake registry version resolution
- `GetRemotePackageVersionPlugin` gains a `revision` field (field 4, uint32)
- `LocalModuleResolveResult` loses the `is_bsr_head` field (now reserved)

**repository.proto:** New version adds:
- `AddRepositoryGroup` RPC
- `UpdateRepositoryGroup` RPC
- `RemoveRepositoryGroup` RPC
- `GetRepositoryContributor` RPC (was missing in old)
- Removes some messages from old (labels, recommendation, sync)

**module.proto (module/v1alpha1):** IDENTICAL -- only copyright year changed.

**registry module.proto:** IDENTICAL between old and new.

**Key takeaway:** The modern protocol is a **superset** of the old protocol. The old RPCs (`GetModulePins`, `GetRepositoryByFullName`, `GetRepositoriesByFullName`, `DownloadManifestAndBlobs`) have **identical request/response messages**. The new protocol adds new RPCs and new fields but does not change existing ones in breaking ways.

### What This Means for Architecture

Because old and new buf CLI clients use the same service paths and the same core RPC methods with identical message shapes, the **dual-protocol problem is actually simpler than expected**:

1. A single Connect RPC handler implementing the **modern** (superset) proto definitions will serve **both** old and new clients correctly
2. Old clients will only call the RPCs they know about (the unchanged ones) and will simply ignore any new fields they do not recognize (protobuf forwards-compatibility)
3. New clients will call the same core RPCs plus the new ones

## Recommended Approach: Single Superset Handler

```text
                          buf CLI (v1.30.1)         buf CLI (v1.69.0+)
                                |                          |
                                v                          v
                    ┌───────────────────────────────────────────────┐
                    │         HTTP Entry Point (TLS)                │
                    └───────────────────┬───────────────────────────┘
                                        v
                    ┌───────────────────────────────────────────────┐
                    │           http.ServeMux                       │
                    │                                               │
                    │  /buf.alpha.registry.v1alpha1.ResolveService/ │
                    │  /buf.alpha.registry.v1alpha1.DownloadService/│
                    │  /buf.alpha.registry.v1alpha1.RepositorySvc/ │
                    │                                               │
                    │   SINGLE handler implementing NEW protos     │
                    │   (superset of old)                           │
                    └───────────────────┬───────────────────────────┘
                                        v
                    ┌───────────────────────────────────────────────┐
                    │   New Connect API Layer                       │
                    │   internal/connect/ (updated in place)        │
                    │                                               │
                    │   Implements modern handler interfaces:       │
                    │   - RepositoryService (with new RPCs)         │
                    │   - ResolveService (with new RPCs)            │
                    │   - DownloadService (unchanged)               │
                    │                                               │
                    │   New RPCs return Unimplemented errors        │
                    │   until explicitly implemented                │
                    └───────────────────┬───────────────────────────┘
                                        v
                    ┌───────────────────────────────────────────────┐
                    │   Shared provider / multisource layer         │
                    │   (completely unchanged)                      │
                    └───────────────────────────────────────────────┘
```

### Why This Works

1. **Protobuf is forwards-compatible.** Old clients send the old message shape. The server generates from the new proto (which has the same fields plus new ones). Old messages decode correctly because unknown fields are simply not present.

2. **Connect RPC paths are identical.** Both old and new buf CLI versions hit `/buf.alpha.registry.v1alpha1.ResolveService/GetModulePins` (and the same for Download and Repository services). There is only one path, one handler.

3. **The Unimplemented pattern is safe.** The new ResolveService handler interface requires implementing `GetSDKInfo`, `GetCargoVersion`, etc. By embedding `UnimplementedResolveServiceHandler`, the server returns `CodeUnimplemented` for these new RPCs. Old buf clients never call these. New buf clients may call them and will receive a clear error -- which is acceptable for a proxy that does not provide SDK generation.

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| `cmd/easyp/main.go` | Server wiring, config, startup | Creates connect handler, multisource router, cache |
| `internal/connect/` | Connect RPC handlers implementing modern proto interfaces | Depends on provider interface for GetMeta/GetFiles |
| `internal/connect/api.go` | Handler struct, New() constructor, mux registration | Creates mux, registers all service handlers |
| `internal/connect/bynames.go` | Repository lookup RPCs | Uses provider.GetMeta |
| `internal/connect/modulepins.go` | Module pin resolution RPCs | Uses provider.GetMeta |
| `internal/connect/blobs.go` | Download manifest+blobs RPCs | Uses provider.GetFiles |
| `internal/connect/sdkinfo.go` (new) | GetSDKInfo RPC (returns Unimplemented) | None initially |
| `internal/connect/versions.go` (new) | GetCargoVersion, GetNugetVersion, GetCmakeVersion RPCs | None initially (Unimplemented) |
| `internal/providers/multisource/` | Multi-source router with cache-aside | Aggregates providers, manages cache |
| `internal/providers/{localgit,github,bitbucket}/` | VCS-specific file fetching | External VCS APIs |
| `internal/providers/cache/` | File content caching | Local filesystem or Artifactory |
| `api/proto/buf.gen.yaml` | Code generation config | Drives `buf generate` output |
| `api/proto/generate.go` | Code generation entry point | Copies protos, runs `buf generate` |
| `gen/proto/` (regenerated) | Generated Go code from modern protos | Consumed by `internal/connect/` |

### Data Flow (Modern Client Path)

```text
1. buf CLI v1.69.0 sends GetModulePins request
   POST /buf.alpha.registry.v1alpha1.ResolveService/GetModulePins
   Body: {module_references: [{remote, owner, repository, reference}]}

2. Connect RPC routes to resolveServiceHandler
   -> api.GetModulePins() in internal/connect/modulepins.go

3. Handler iterates module_references:
   a. calls provider.GetMeta(ctx, owner, repo, reference)
   b. provider finds correct VCS source via multisource router
   c. VCS source resolves reference to commit hash
   d. returns content.Meta with commit, default_branch, timestamps

4. Handler builds ModulePin responses with remote, owner, repository, commit
   (manifest_digest can be computed or left empty -- old code does not set it)

5. Response sent back to buf CLI
```

### Code Generation Strategy

The key change is regenerating `gen/proto/` from the modern proto files instead of the old ones. The `api/proto/generate.go` currently copies from `api/_third_party/buf/proto/buf/`:

```go
//go:generate cp -r ../_third_party/buf/proto/buf ./
//go:generate buf generate
```

This must be changed to copy from the modern proto source:

```go
//go:generate cp -r ../_third_party/buf-v1.69.0/proto/buf ./
//go:generate buf generate
```

The `buf.gen.yaml` file requires updating the `M` (mapping) directives to account for the three removed proto files (`labels.proto`, `recommendation.proto`, `sync.proto`) and to add mappings for any new message types.

## Patterns to Follow

### Pattern 1: Superset Handler with Unimplemented Embedding

**What:** Implement the modern (superset) proto interfaces. Embed `Unimplemented*Handler` structs so new RPCs you do not want to implement yet automatically return `CodeUnimplemented`.

**When:** Any service with new RPCs that are not relevant to a proxy (SDK version resolution, repository groups, etc.)

**Example:**
```go
type api struct {
    log *slog.Logger
    // Embed modern Unimplemented handlers -- new RPCs return Unimplemented
    v1alpha1connect.UnimplementedRepositoryServiceHandler
    v1alpha1connect.UnimplementedResolveServiceHandler
    v1alpha1connect.UnimplementedDownloadServiceHandler
    repo   provider
    domain string
}
```

### Pattern 2: Provider Interface Segregation

**What:** The API layer depends on a narrow private `provider` interface with only `GetMeta` and `GetFiles`. It never sees VCS-specific types.

**When:** Always -- this is already established and must not change.

**The existing interface (unchanged):**
```go
type provider interface {
    GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error)
    GetFiles(ctx context.Context, owner, repoName, commit string) ([]content.File, error)
}
```

### Pattern 3: Single Mux with Multiple Service Handlers

**What:** The `connect.New()` function creates a single `http.ServeMux` and registers multiple Connect RPC service handlers on it. Each service gets its own path prefix.

**When:** This is the standard Connect RPC pattern for composing multiple services.

**Existing pattern (keep as-is):**
```go
func New(log *slog.Logger, core provider, domain string) *http.ServeMux {
    a := &api{log: log, repo: core, domain: domain}
    mux := http.NewServeMux()
    mux.Handle(v1alpha1connect.NewResolveServiceHandler(a))
    mux.Handle(v1alpha1connect.NewRepositoryServiceHandler(a))
    mux.Handle(v1alpha1connect.NewDownloadServiceHandler(a))
    mux.HandleFunc("/", rootHandler)
    return mux
}
```

After regeneration, this same code will register handlers with the modern interfaces (which include the new RPCs).

## Anti-Patterns to Avoid

### Anti-Pattern 1: Separate Handler Packages for Old and New

**What:** Creating `internal/connectold/` and `internal/connectnew/` with separate handler implementations.
**Why bad:** Both old and new clients use the **same URL paths**. You cannot route between them by path. You would need request-body inspection or custom middleware to distinguish clients, which is fragile and unnecessary.
**Instead:** Use a single handler implementing the superset (modern) proto. Old clients are naturally compatible because protobuf is forwards-compatible.

### Anti-Pattern 2: Conditionally Switching Proto Sources at Runtime

**What:** Detecting the client version and loading different generated code paths.
**Why bad:** Go does not support runtime code loading. The generated proto types are compile-time. Trying to dynamically switch would require reflection-heavy adapter code.
**Instead:** Always use the modern generated code. It is a superset.

### Anti-Pattern 3: Maintaining Two Sets of Generated Code

**What:** Generating both old and new proto code and keeping both in the repo.
**Why bad:** The old and new protos share the same Go import paths (`github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1`). You cannot have two versions of the same Go package. This would require a custom Go import path mapping for one set, which creates maintenance burden.
**Instead:** Regenerate from modern protos. The generated code replaces the old entirely.

### Anti-Pattern 4: Implementing New RPCs That Are Not Needed

**What:** Implementing `GetSDKInfo`, `GetCargoVersion`, etc. with real logic.
**Why bad:** This proxy is a BSR-to-VCS adapter. It does not have plugin registries or SDK version information. Implementing stub logic for these would be misleading and could cause incorrect behavior.
**Instead:** Leave these as `Unimplemented`. The buf CLI handles `Unimplemented` errors gracefully for optional operations.

## Build Order (Component Dependencies)

```text
Phase 1: Code Generation (no behavioral changes)
  1. Update api/proto/generate.go to copy from buf-v1.69.0
  2. Update api/proto/buf.gen.yaml (remove mappings for deleted protos)
  3. Run code generation -> regenerates gen/proto/
  4. Update internal/connect/api.go to embed new Unimplemented types
  5. Compile and verify old tests still pass

Phase 2: Handler Updates (behavioral changes for new compatibility)
  1. Update GetModulePins response to populate manifest_digest field
  2. Verify DownloadManifestAndBlobs is unchanged (it is)
  3. Verify GetRepositoryByFullName is unchanged (it is)
  4. Add any new Unimplemented method stubs if the Go compiler requires them

Phase 3: Testing
  1. Integration test with buf v1.30.1 (old protocol unchanged)
  2. Integration test with buf v1.69.0+ (modern protocol)
```

### Dependency Graph

```text
api/proto/generate.go change
  |
  v
gen/proto/ regeneration
  |
  +---> internal/connect/api.go (embed new handler interfaces)
  |       |
  |       +---> internal/connect/modulepins.go (may need manifest_digest)
  |       +---> internal/connect/bynames.go (likely unchanged)
  |       +---> internal/connect/blobs.go (likely unchanged)
  |
  +---> main.go wiring (likely unchanged -- same New() signature)

No changes needed below this line:
  - internal/providers/multisource/ (unchanged)
  - internal/providers/{localgit,github,bitbucket}/ (unchanged)
  - internal/providers/cache/ (unchanged)
  - internal/providers/content/ (unchanged)
  - internal/https/ (unchanged)
```

## Scalability Considerations

| Concern | At 100 users | At 10K users | At 1M users |
|---------|--------------|--------------|-------------|
| Dual-protocol routing overhead | None -- single handler | None -- single handler | None -- single handler |
| Proto compatibility risk | Zero -- identical core messages | Zero -- protobuf forwards-compat | Zero -- protobuf forwards-compat |
| New RPC load (GetSDKInfo etc.) | Unimplemented = instant return | Unimplemented = instant return | Unimplemented = instant return |
| Cache effectiveness | Same as before | Same as before | Same as before |

## Key Risk: manifest_digest Field

**MEDIUM confidence.** The modern `ModulePin` message includes a `manifest_digest` field (field 8) that the old code does not populate. Modern buf CLI (v1.69.0+) may require or use this field for content verification. The current handler in `modulepins.go` creates `ModulePin` with `nolint:exhaustruct`, meaning the field is currently unset.

**Impact:** If modern buf CLI requires `manifest_digest` to be non-empty, module resolution will fail or produce warnings.

**Mitigation:** During Phase 2, compute the manifest digest during `GetModulePins` by calling `GetFiles` for each module and building the manifest (same logic as `blobs.go`). This is a performance consideration since `GetModulePins` currently only calls `GetMeta` (which does not fetch files).

## Sources

- Code analysis: `api/_third_party/buf/proto/` vs `api/_third_party/buf-v1.69.0/proto/` (direct proto file diff)
- Code analysis: `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/` (generated Connect RPC handlers)
- Code analysis: `internal/connect/api.go`, `internal/connect/modulepins.go`, `internal/connect/blobs.go`, `internal/connect/bynames.go`
- Connect RPC documentation via Context7: handler registration and mux patterns
- Connect RPC v1.11.1 as used in `go.mod`
