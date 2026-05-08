# Technology Stack

**Analysis Date:** 2026-05-07

## Languages

**Primary:**
- Go 1.22 - All server implementation code, generated protobuf/gRPC code

**Secondary:**
- Protocol Buffers (proto3) - Buf API schema definitions in `api/proto/` and generated Go types in `gen/proto/`
- Shell (Bash) - Build scripts in `scripts/`
- YAML - Configuration file (`local.config.yml`), CI workflow, buf generation config

## Runtime

**Environment:**
- Go 1.22 runtime (specified in `go.mod`)
- Alpine Linux Docker container (from `Dockerfile`)

**Package Manager:**
- Go Modules
- Lockfile: `go.sum` (present)

## Frameworks

**Core:**
- Connect RPC (connectrpc.com/connect v1.11.1) - HTTP-based RPC framework implementing gRPC-compatible protocol. Used for Buf registry service handlers (`ResolveService`, `RepositoryService`, `DownloadService`)
- net/http (stdlib) - HTTP server, TLS termination, request routing

**Testing:**
- testify v1.8.4 (indirect dependency) - Assertion library (no test files found in codebase currently)

**Build/Dev:**
- Buf CLI - Protocol Buffers code generation (`buf generate` invoked via `api/proto/generate.go`)
- golangci-lint - Linting configured via `.golangci.yml` with enable-all + selective disables
- Docker Buildx - Multi-platform Docker builds

## Key Dependencies

**Critical:**
- `connectrpc.com/connect` v1.11.1 - RPC framework; generates Connect protocol handlers for Buf registry services
- `github.com/google/go-github/v59` v59.0.0 - GitHub API client; used for GitHub repository proxy provider
- `github.com/go-git/go-git/v5` v5.9.0 - Pure Go Git implementation; used for local git repository mirroring
- `google.golang.org/protobuf` v1.34.1 - Protobuf runtime library for generated message types
- `google.golang.org/grpc` v1.59.0 - gRPC framework; generates gRPC service stubs (used alongside Connect)

**Infrastructure:**
- `github.com/ghodss/yaml` v1.0.0 - YAML config file parsing with env var substitution (`cmd/easyp/internal/config/read.go`)
- `golang.org/x/crypto` v0.23.0 - SHA3/SHAKE256 hashing for proto file content digests (`internal/shake256/hash.go`)
- `golang.org/x/exp` v0.0.0-20231006140011 - Structured logging (`slog`) and generic slices utilities

**Proto Code Generation Plugins (configured in `api/proto/buf.gen.yaml`):**
- `go` plugin - Protobuf message types
- `go-grpc` plugin - gRPC service stubs
- `connect-go` plugin - Connect RPC service handlers

## Configuration

**Environment:**
- YAML configuration file via `-cfg` flag (default: `./local.config.yml`)
- Environment variable substitution in config via `os.ExpandEnv` in `cmd/easyp/internal/config/read.go`
- All config values can reference env vars using `${VAR_NAME}` syntax
- Log level configurable: debug, info, warning/warn, error

**Build:**
- `go.mod` / `go.sum` - Go module dependencies
- `Dockerfile` - Multi-stage Docker build (golang:1.22-alpine builder, scratch runtime)
- `.golangci.yml` - Linter configuration (enable-all with selective disables)
- `api/proto/buf.gen.yaml` - Buf code generation configuration
- `api/proto/generate.go` - Go generate directives for protobuf codegen

**TLS:**
- TLS required by `buf` CLI; configured via `tls.cert`, `tls.key`, optional `tls.ca` in config
- Supports mTLS (mutual TLS) when `tls.ca` is provided (client certificate verification)
- Minimum TLS version 1.3 enforced (`internal/https/https.go`)

## Platform Requirements

**Development:**
- Go 1.22+
- Buf CLI (for protobuf code generation)
- Docker (for containerized builds)
- Git (for submodule initialization; `api/_third_party/buf`, `api/_third_party/protobuf`, `api/_third_party/buf-v1.69.0`)

**Production:**
- Docker container deployed to Yandex Container Registry (`cr.yandex/crplga9vcvsvk4uv6541/easyp:stage`)
- TLS certificate required (buf CLI mandates TLS)
- Network access to GitHub API (api.github.com) for GitHub proxy mode
- Network access to BitBucket API for BitBucket proxy mode
- Network access to Artifactory for cache mode
- Local filesystem access for local git mirror and local cache modes

---

*Stack analysis: 2026-05-07*
