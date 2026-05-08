# Codebase Structure

**Analysis Date:** 2026-05-07

## Directory Layout

```
easyp-buf-proxy/
├── api/                        # Proto definitions and code generation config
│   ├── proto/                  # Buf generation config and Go generate trigger
│   │   ├── buf.gen.yaml        # Buf code generation plugin config (go, go-grpc, connect-go)
│   │   └── generate.go         # `go generate` entry point (copies protos, runs buf generate)
│   ├── _third_party/           # Vendored Buf proto sources (git submodule)
│   │   ├── buf-v1.69.0/        # Pinned Buf release proto definitions
│   │   ├── buf/                # Symlink or copy of active buf protos
│   │   └── protobuf/           # Google well-known protobuf types
│   └── buf.work.yaml           # Buf workspace config (defines proto directories)
├── cmd/                        # Application entry points
│   └── easyp/                  # Main server binary
│       ├── main.go             # Entry point: config, wiring, HTTP server, middleware
│       └── internal/           # Private config packages for the easyp binary
│           └── config/
│               ├── config.go   # Config struct definitions (TLS, proxy, cache, repos)
│               ├── read.go     # Generic YAML reader with env var expansion
│               ├── url.go      # Custom URL type with UnmarshalText
│               └── cachetype/  # Cache type enum (none/local/artifactory)
│                   └── cachetype.go
├── gen/                        # Generated protobuf + Connect code (committed)
│   └── proto/buf/alpha/
│       ├── module/v1alpha1/    # Module protobuf messages (Blob, Digest, ModuleReference, etc.)
│       ├── registry/v1alpha1/  # Registry protobuf messages (Repository, ModulePin, etc.)
│       │   └── v1alpha1connect/ # Connect RPC service interfaces (generated)
│       └── ...                 # Other Buf proto packages (breaking, lint, image, etc.)
├── internal/                   # Shared internal packages
│   ├── connect/                # Connect RPC handler layer
│   │   ├── api.go              # Service registration, provider interface, health check
│   │   ├── bynames.go          # GetRepositoryByFullName, GetRepositoriesByFullName
│   │   ├── modulepins.go       # GetModulePins, module pin resolution
│   │   └── blobs.go            # DownloadManifestAndBlobs, blob/manifest construction
│   ├── https/                  # TLS server with optional mTLS
│   │   └── https.go            # ListenAndServeTLS with CA cert loading
│   ├── logger/                 # Global logger wrapper (slog)
│   │   └── logger.go           # Init, Debug, Info, Warn, Error, Get
│   ├── shake256/               # SHAKE256 hashing for blob digests
│   │   └── hash.go             # Hash type, SHA3Shake256 function
│   └── providers/              # VCS provider implementations
│       ├── content/            # Shared domain types
│       │   └── repo.go         # Meta and File structs
│       ├── source/             # Source interface definition
│       │   └── source.go       # Source interface (GetMeta, GetFiles, ConfigHash, etc.)
│       ├── filter/             # Path/prefix filtering logic
│       │   └── filter.go       # Repo filter struct, FindRepo, Check, Hash
│       ├── multisource/        # Provider aggregator + cache-aside router
│       │   └── repo.go         # Repo struct, Provider/Cache interfaces, routing logic
│       ├── localgit/           # Local git mirror provider
│       │   ├── localgit.go     # store struct, sourceRepo, GetMeta, GetFiles, enumerateProto
│       │   └── namedlocks/     # Named mutex for per-repo locking
│       │       └── lock.go     # namedLocks, Lock/Unlock
│       ├── github/             # GitHub API provider
│       │   ├── client.go       # client struct, Repositories/Git interfaces, connect()
│       │   ├── repos.go        # multiRepo (Provider), sourceRepo (Source), NewMultiRepo
│       │   ├── getrepo.go      # GetMeta, getRepo (branch/commit resolution)
│       │   └── getfiles.go     # GetFiles, filterEntries, getFile, getFiles
│       ├── bitbucket/          # BitBucket Server API provider
│       │   ├── client.go       # client struct, httpClient, httpGetJSON, URL templates
│       │   ├── repos.go        # multiRepo (Provider), sourceRepo (Source), NewMultiRepo
│       │   ├── getrepo.go      # getMeta, getRepo, searchRepo, repoInfo struct
│       │   └── getfiles.go     # GetFiles, listFiles, getFile, filterEntries
│       └── cache/              # Cache implementations
│           ├── noop.go         # Noop cache (pass-through)
│           ├── file.go         # Local filesystem cache (JSON files)
│           └── artifactory/    # JFrog Artifactory cache
│               └── artifactory.go  # HTTP-based Artifactory cache (PUT/GET/DELETE)
├── scripts/                    # Build and deployment scripts
│   ├── build.sh                # Build all cmd binaries with Dockerfile
│   ├── publish.sh              # Build and push Docker image
│   ├── push_stage_to_docker.sh # Build and push staging Docker image to Yandex Cloud
│   └── clone_repos.sh          # Clone git repos for local mirror setup
├── testdata/                   # Test fixtures
│   ├── cert.pem                # Self-signed TLS certificate for testing
│   └── key.pem                 # TLS private key for testing
├── .github/workflows/          # CI/CD
│   └── easyp_build.yml         # Build and push Docker image on main push
├── .golangci.yml               # Golangci-lint configuration
├── Dockerfile                  # Multi-stage Docker build (Go builder + scratch runtime)
├── local.config.yml            # Local development configuration template
├── draft.txt                   # Project notes / roadmap
├── go.mod                      # Go module definition (github.com/easyp-tech/server, Go 1.22)
├── go.sum                      # Dependency checksums
├── README.md                   # User-facing documentation
└── LICENSE                     # Apache 2.0 license
```

## Directory Purposes

**`cmd/easyp/`:**
- Purpose: Application entry point and configuration
- Contains: `main.go` (server bootstrap), `internal/config/` (config types and YAML reader)
- Key files: `cmd/easyp/main.go`, `cmd/easyp/internal/config/config.go`

**`internal/connect/`:**
- Purpose: Connect RPC service handlers implementing Buf registry protocol
- Contains: API handler struct and three service implementations
- Key files: `internal/connect/api.go`, `internal/connect/blobs.go`, `internal/connect/bynames.go`, `internal/connect/modulepins.go`

**`internal/providers/`:**
- Purpose: VCS backend adapters, routing, caching, and shared types
- Contains: All provider implementations, cache backends, filter logic, domain types
- Key files: `internal/providers/multisource/repo.go`, `internal/providers/content/repo.go`, `internal/providers/source/source.go`

**`internal/providers/localgit/`:**
- Purpose: Read .proto files from local on-disk git mirrors
- Contains: Provider implementation that opens git repos via `go-git`, checks out commits, enumerates .proto files
- Key files: `internal/providers/localgit/localgit.go`

**`internal/providers/github/`:**
- Purpose: Proxy to GitHub API for repository metadata and file content
- Contains: Provider that uses `google/go-github` client
- Key files: `internal/providers/github/repos.go`, `internal/providers/github/getfiles.go`, `internal/providers/github/getrepo.go`

**`internal/providers/bitbucket/`:**
- Purpose: Proxy to BitBucket Server API v1 for repository metadata and files
- Contains: Provider that uses custom HTTP client with basic auth
- Key files: `internal/providers/bitbucket/repos.go`, `internal/providers/bitbucket/getfiles.go`, `internal/providers/bitbucket/getrepo.go`

**`internal/providers/cache/`:**
- Purpose: File-content caching backends
- Contains: Noop, Local filesystem, and Artifactory implementations
- Key files: `internal/providers/cache/file.go`, `internal/providers/cache/artifactory/artifactory.go`

**`gen/`:**
- Purpose: Generated protobuf and Connect RPC Go code
- Contains: `.pb.go` files (messages), `_grpc.pb.go` files (gRPC stubs), `.connect.go` files (Connect handlers)
- Key files: `gen/proto/buf/alpha/registry/v1alpha1/*.pb.go`, `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/*.connect.go`

**`api/`:**
- Purpose: Proto source definitions and code generation configuration
- Contains: Buf workspace config, generation config, vendored Buf proto definitions
- Key files: `api/proto/buf.gen.yaml`, `api/proto/generate.go`, `api/buf.work.yaml`

**`scripts/`:**
- Purpose: Build, deployment, and setup helper scripts
- Contains: Shell scripts for building, Docker pushing, and repo cloning
- Key files: `scripts/build.sh`, `scripts/clone_repos.sh`

## Key File Locations

**Entry Points:**
- `cmd/easyp/main.go`: Server application entry point (flag parsing, config, wiring, HTTP server)

**Configuration:**
- `cmd/easyp/internal/config/config.go`: Config type definitions
- `cmd/easyp/internal/config/read.go`: YAML config reader with env var expansion
- `local.config.yml`: Example/local development configuration
- `Dockerfile`: Container build (defaults to `/local.config.yml`)

**Core Logic:**
- `internal/connect/api.go`: Connect RPC handler registration and provider interface
- `internal/providers/multisource/repo.go`: Provider routing and cache-aside logic
- `internal/providers/filter/filter.go`: .proto file path filtering rules

**Domain Types:**
- `internal/providers/content/repo.go`: `Meta` and `File` structs
- `internal/providers/source/source.go`: `Source` interface
- `internal/shake256/hash.go`: SHAKE256 hash type and function

**Code Generation:**
- `api/proto/generate.go`: `go generate` trigger
- `api/proto/buf.gen.yaml`: Buf code generation configuration
- `api/buf.work.yaml`: Buf workspace definition
- `gen/proto/`: Generated output (committed to repo)

**Testing:**
- `testdata/cert.pem`: Test TLS certificate
- `testdata/key.pem`: Test TLS private key
- No `_test.go` files exist currently

## Naming Conventions

**Files:**
- Go source files use `lowercase.go` naming (standard Go convention)
- Multiple words in filenames use no separator: `getfiles.go`, `getrepo.go`, `localgit.go`
- Generated protobuf files follow proto package structure: `gen/proto/buf/alpha/registry/v1alpha1/`
- Connect handler files are in a `v1alpha1connect/` subdirectory alongside the proto package

**Directories:**
- `internal/` for packages not importable outside this module
- `cmd/<binary-name>/` for each executable (`cmd/easyp/`)
- `cmd/<binary-name>/internal/` for packages private to that binary
- Provider packages use lowercase single-word names: `localgit`, `github`, `bitbucket`, `cache`, `filter`, `content`, `source`, `multisource`
- `namedlocks` is a sub-package of `localgit`

**Types:**
- Provider structs: lowercase unexported (`store`, `multiRepo`, `sourceRepo`, `client`, `artifactory`)
- Exported constructors: `New()`, `NewMultiRepo()`
- Interface types: exported (`Source`, `Provider`, `Cache`, `Repositories`, `Git`)
- Domain types: exported (`Meta`, `File`, `Hash`, `Repo`, `Config`)
- Type aliases for domain clarity: `bitbucket.User = string`, `bitbucket.Password = string`

## Where to Add New Code

**New VCS Provider (e.g., GitLab):**
- Create `internal/providers/gitlab/` directory
- Implement `multisource.Provider` with `Find()` and `Repositories()` methods
- Create a private `sourceRepo` type satisfying `source.Source`
- Add config struct to `cmd/easyp/internal/config/config.go`
- Wire it in `cmd/easyp/main.go` (add constructor call and pass to `multisource.New`)

**New Cache Backend (e.g., Redis):**
- Create `internal/providers/cache/redis/` directory (or `internal/providers/cache/redis.go`)
- Implement `multisource.Cache` interface (`Get`, `Put`, `CheckWriteAccess`)
- Add cache type to `cmd/easyp/internal/config/cachetype/cachetype.go`
- Add config struct to `cmd/easyp/internal/config/config.go` in the `Cache` struct
- Wire it in `buildCache()` in `cmd/easyp/main.go`

**New Connect RPC Service Handler:**
- Add handler methods to `internal/connect/api.go` (the `api` struct already embeds `Unimplemented*` handlers)
- Import generated Connect service interface from `gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/`
- Register handler in `connect.New()` function in `internal/connect/api.go`

**New Proto Service:**
- Add proto definition to `api/_third_party/buf/proto/` or create new proto file
- Regenerate: `cd api/proto && go generate`
- Generated code appears in `gen/proto/`

**Utilities:**
- Shared helpers used by multiple providers: `internal/providers/` (sibling to existing packages)
- Server-wide utilities: `internal/` (sibling to `connect`, `https`, `logger`, `shake256`)

## Special Directories

**`gen/`:**
- Purpose: Generated protobuf and Connect RPC Go code
- Generated: Yes (by `buf generate` via `api/proto/generate.go`)
- Committed: Yes (generated code is committed to git)

**`api/_third_party/`:**
- Purpose: Vendored Buf proto definitions (upstream Buf repository source)
- Generated: No (manually managed, appears to be a git submodule based on `.gitmodules`)
- Committed: Partially (referenced as submodule)

**`testdata/`:**
- Purpose: Test fixtures (TLS certificates for local development/testing)
- Generated: No (manually created self-signed certs)
- Committed: Yes

**`.claude/`:**
- Purpose: Claude Code GSD (Get Shit Done) framework configuration
- Contains: Agents, hooks, commands, and workflow definitions
- Not part of the application code

---

*Structure analysis: 2026-05-07*
