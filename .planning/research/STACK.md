# Technology Stack: Buf Protocol Modernization

**Project:** easyp-buf-proxy
**Researched:** 2026-05-07
**Scope:** Adding modern Buf protocol (v1.69.0+) alongside deprecated v1alpha1 protocol

---

## Current Stack (In Production)

| Technology | Version | Purpose | Status |
|------------|---------|---------|--------|
| Go | 1.22 | Runtime + language | Current; Go 1.24+ now available |
| connectrpc.com/connect | v1.11.1 | HTTP-based RPC framework (Connect/gRPC/gRPC-Web) | Outdated; latest is v1.19.1 |
| google.golang.org/protobuf | v1.34.1 | Protobuf runtime for generated message types | Acceptable; latest is v1.36.x |
| google.golang.org/grpc | v1.59.0 | gRPC framework (code generation stubs only) | Outdated; latest is v1.73.0 |
| Buf CLI | (dev tool) | Proto code generation via `buf generate` | Local install |
| golang.org/x/exp | v0.0.0-20231006140011 | Structured logging (slog) | Should migrate to stdlib slog |
| golang.org/x/crypto | v0.23.0 | SHA3/SHAKE256 hashing for content digests | Current enough |
| github.com/google/go-github/v59 | v59.0.0 | GitHub API client | Functional; not a priority to update |
| github.com/go-git/go-git/v5 | v5.9.0 | Pure Go Git implementation | Functional; not a priority to update |
| github.com/ghodss/yaml | v1.0.0 | YAML config parsing with env var substitution | Stable, no changes needed |

---

## Recommended Stack Changes

### Critical Update: connectrpc.com/connect

| Decision | From | To | Rationale |
|----------|------|----|-----------|
| connectrpc.com/connect | v1.11.1 | v1.18.1 (NOT v1.19.x) | v1.19.x requires Go 1.24; we are on Go 1.22. v1.18.1 is the latest that supports Go 1.21+. Has all needed RPC features. |
| google.golang.org/grpc | v1.59.0 | v1.64.0+ (or drop) | Only used for code generation (`go-grpc` plugin). If we drop the gRPC plugin and only use Connect codegen, we can remove this entirely. |
| google.golang.org/protobuf | v1.34.1 | v1.34.2+ | Minor update for security fixes (CVE-2023-45288 mitigation). Not urgent. |

**Confidence: HIGH** -- Version constraints verified from GitHub releases and Go module requirements.

### NOT Changing

| Technology | Why Keep As-Is |
|------------|---------------|
| Go 1.22 | Project constraint. v1.22 is stable and supported. Go 1.24 would require connect-go v1.19.x which changes generated code APIs (new "simple" flag). Not worth the churn for this milestone. |
| golang.org/x/exp/slog | Migrate to stdlib `log/slog` (available since Go 1.21) as a separate cleanup, not part of this protocol work. |
| github.com/ghodss/yaml | Stable, works, no reason to change. |
| github.com/google/go-github/v59 | Functional. Updating brings no protocol benefits. |
| github.com/go-git/go-git/v5 | Functional. Updating brings no protocol benefits. |

---

## Code Generation Configuration

### Current Setup (buf.gen.yaml v1 format)

The existing `api/proto/buf.gen.yaml` uses v1 format with three plugins:
1. `go` -- generates protobuf message types
2. `go-grpc` -- generates gRPC service stubs (UNUSED at runtime, only for code generation)
3. `connect-go` -- generates Connect RPC handlers (ACTIVE at runtime)

The `managed: enabled: true` section + extensive `-M` flags control `go_package` mappings, mapping all `buf/alpha/...` proto paths to `github.com/easyp-tech/server/gen/proto/buf/alpha/...`.

### Recommended Code Generation Strategy

**Do NOT change the buf.gen.yaml format.** Stay on v1 format. The v2 format requires buf CLI v1.28+ and introduces different plugin syntax (`remote:` vs `name:`). The current v1 format works and changing it adds risk with no benefit.

**Do NOT create separate generation configs for old and new protos.** The old and new proto files use the SAME package paths (`buf.alpha.registry.v1alpha1`). They are designed to be drop-in replacements. The new protos are a superset (additive RPCs, reserved fields, no breaking changes to existing messages).

**Strategy: Replace the proto source, regenerate once.**

1. Point `generate.go` at the v1.69.0 proto files instead of the old ones
2. Run `buf generate` to regenerate all Go code from the new protos
3. The generated code will have the same package paths and message types, PLUS new RPC methods on the generated Connect interfaces
4. The `UnimplementedResolveServiceHandler` will now include new methods (`GetSDKInfo`, `GetCargoVersion`, `GetNugetVersion`, `GetCmakeVersion`, `GetPythonVersion`)
5. Implement the new methods (most can return `connect.CodeUnimplemented` initially)

**Confidence: HIGH** -- Verified by diffing all proto files between old and new versions. The differences are:
- `resolve.proto`: New RPCs added (GetSDKInfo, GetCargoVersion, GetNugetVersion, GetCmakeVersion, GetPythonVersion), new `revision` field on `GetRemotePackageVersionPlugin`, `is_bsr_head` field reserved
- 3 proto files removed: `labels.proto`, `recommendation.proto`, `sync.proto`
- All other files: Identical or reserved-field-only changes

### Proto Source Change

Current `api/proto/generate.go`:
```go
//go:generate cp -r ../_third_party/buf/proto/buf ./
```

Change to:
```go
//go:generate cp -r ../_third_party/buf-v1.69.0/proto/buf ./
```

**Confidence: HIGH** -- The new protos are already available as a git submodule at `api/_third_party/buf-v1.69.0/`.

---

## Dependency Impact Analysis

### What breaks with a connect-go upgrade to v1.18.1?

The generated `resolve.connect.go` contains a version compatibility check:
```go
const _ = connect.IsAtLeastVersion1_7_0
```

With connect-go v1.18.1, the generated code will reference `connect.IsAtLeastVersion1_13_0` or similar. This is a source-level change only -- the runtime API (`connect.Request[T]`, `connect.Response[T]`, `connect.NewUnaryHandler`) has been stable since v1.7.0. The upgrade is safe.

The only API concern: between v1.11.1 and v1.18.1, there were error handling improvements and HTTP status code mapping changes (RFC 003 compliance). These affect edge cases in error propagation, not the happy path. The proxy server's error handling is simple and will not be affected.

**Confidence: HIGH**

### What happens to removed proto files?

Three proto files exist in the old version but NOT in the new:
- `labels.proto` -- The generated code (`labels.pb.go`, `labels_grpc.pb.go`, `labels.connect.go`) is NOT imported anywhere in the server codebase
- `recommendation.proto` -- Same, no imports in server code
- `sync.proto` -- Same, no imports in server code

After regenerating from new protos, these generated files will simply not be regenerated. They should be deleted. No server code will break because nothing imports them.

**Confidence: HIGH** -- Verified by searching the codebase for imports of these generated packages.

### What about the `google.golang.org/grpc` dependency?

Currently used ONLY for the `go-grpc` protoc plugin, which generates `_grpc.pb.go` files. These files are not used by the Connect RPC server at runtime -- only the `_connect.go` files are used.

Options:
1. **Keep as-is (recommended)**: The gRPC stubs add ~0 cost and the dependency is already in go.mod. Removing the plugin from buf.gen.yaml means fewer generated files and slightly cleaner builds, but the benefit is marginal.
2. **Remove go-grpc plugin from buf.gen.yaml**: Removes ~40 generated `_grpc.pb.go` files. Can then `go mod tidy` to potentially drop the grpc dependency. Cleaner, but adds risk if anything unexpectedly depends on those stubs.

**Recommendation: Keep as-is for this milestone.** The go-grpc plugin generates stubs that are harmlessly unused. Removing it is a cleanup task, not a protocol change.

**Confidence: MEDIUM** -- Need to verify no transitive dependencies rely on the grpc generated code being present.

---

## New Dependencies Required

**None.** The modern Buf protocol does not introduce any new technology or library requirements. The changes are:
1. New RPC method implementations (pure Go code using existing connectrpc.com/connect APIs)
2. New message types (auto-generated from protobuf definitions)
3. Potentially new `google.golang.org/protobuf/types/known/timestamppb` usage (already used in `bynames.go`)

---

## Build Tooling

| Tool | Current | Recommendation |
|------|---------|---------------|
| Buf CLI | Local install | Keep current version. Code generation config is v1 format which is supported by all buf CLI versions. |
| Dockerfile | `golang:1.22-alpine` builder, `scratch` runtime | No changes needed. The binary builds the same way. |
| golangci-lint | Configured via `.golangci.yml` | May need adjustments if new generated code triggers lint errors. The existing `exhaucomp` and `exhaustruct` nolint directives should cover new message types. |

---

## Installation / Upgrade Commands

```bash
# Update connect-go to v1.18.1
go get connectrpc.com/connect@v1.18.1

# Update protobuf runtime (security fix)
go get google.golang.org/protobuf@latest

# Tidy dependencies
go mod tidy

# Regenerate proto code (after changing generate.go to point at buf-v1.69.0)
cd api/proto && go generate ./...

# Clean up generated files that are no longer produced
rm gen/proto/buf/alpha/registry/v1alpha1/labels*.go
rm gen/proto/buf/alpha/registry/v1alpha1/recommendation*.go
rm gen/proto/buf/alpha/registry/v1alpha1/sync*.go
# Remove connect files for these too
rm gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/labels.connect.go
rm gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/recommendation.connect.go
rm gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/sync.connect.go
```

---

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Connect-go version | v1.18.1 | v1.19.x (latest) | Requires Go 1.24; we are constrained to Go 1.22 |
| Connect-go version | v1.18.1 | Stay on v1.11.1 | New protos may generate code referencing newer connect APIs; also v1.11.1 has known error handling issues fixed in v1.15+ |
| Proto generation | Single source (v1.69.0 only) | Dual source (generate both old and new) | Proto package paths are IDENTICAL -- cannot generate both into the same Go packages. Would need separate Go module paths, massive refactoring, for zero benefit. |
| buf.gen.yaml format | Stay on v1 | Migrate to v2 | v2 syntax is different and requires testing. No benefit for this work. |
| go-grpc plugin | Keep generating | Remove from config | Cleanup task, not protocol work. Risk of breaking something unexpected. Defer. |
| Go version | Stay on 1.22 | Upgrade to 1.24 | Out of scope. Would unlock connect-go v1.19.x but requires Dockerfile changes, CI changes, and testing. Separate milestone. |

---

## Sources

- connect-go releases: https://github.com/connectrpc/connect-go/releases (verified v1.18.1 is latest for Go 1.21+)
- connect-go Context7 docs: `/connectrpc/connect-go` library documentation
- buf CLI Context7 docs: `/bufbuild/buf` library documentation
- Proto file diffs: Direct file comparison between `api/_third_party/buf/` and `api/_third_party/buf-v1.69.0/`
- Generated code inspection: `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/resolve.connect.go`
