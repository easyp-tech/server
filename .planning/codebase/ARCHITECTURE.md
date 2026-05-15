<!-- refreshed: 2026-05-07 -->
# Architecture

**Analysis Date:** 2026-05-07

## System Overview

```text
┌──────────────────────────────────────────────────────────────────┐
│                        HTTP Entry Point                          │
│          `cmd/easyp/main.go` (ListenAndServe / TLS)              │
│          Logging middleware wraps Connect RPC handler             │
├──────────────────────────────────────────────────────────────────┤
│                     Connect RPC API Layer                        │
│              `internal/connect/api.go`                            │
│  ┌─────────────────┬──────────────────┬────────────────────────┐ │
│  │ RepositorySvc   │ ResolveService   │ DownloadService        │ │
│  │ `bynames.go`    │ `modulepins.go`  │ `blobs.go`             │ │
│  └────────┬────────┴────────┬─────────┴──────────┬─────────────┘ │
└───────────┼─────────────────┼────────────────────┼───────────────┘
            │                 │                    │
            ▼                 ▼                    ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Multi-Source Router                           │
│              `internal/providers/multisource/repo.go`             │
│    Routes owner/repo queries to the first matching provider      │
│    Implements cache-aside pattern for file retrieval              │
├──────────┬──────────────────┬──────────────────┬─────────────────┤
│  Local   │   BitBucket      │    GitHub        │                 │
│  Git     │   Proxy          │    Proxy         │                 │
│ ──────── │ ──────────────── │ ──────────────── │                 │
│`localgit`│ `bitbucket`      │ `github`         │                 │
│`/local-  │ `/bitbucket/     │ `/github/        │                 │
│ git.go`  │ client.go`       │ client.go`       │                 │
└──────────┴──────────────────┴──────────────────┘
         │              │               │
         ▼              ▼               ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Cache Layer (optional)                        │
│  ┌──────────┬──────────────┬──────────────────────┐              │
│  │  Noop    │    Local     │   Artifactory         │              │
│  │ `noop.go`│ `file.go`   │ `artifactory.go`      │              │
│  └──────────┴──────────────┴──────────────────────┘              │
└──────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| main | CLI entry point, config loading, wiring, HTTP server lifecycle | `cmd/easyp/main.go` |
| config | YAML config parsing with env var expansion, type definitions | `cmd/easyp/internal/config/config.go` |
| config.ReadYaml | Generic YAML reader with `os.ExpandEnv` | `cmd/easyp/internal/config/read.go` |
| config.URL | Custom URL type with `UnmarshalText` | `cmd/easyp/internal/config/url.go` |
| cachetype | Enum type for cache backend selection (none/local/artifactory) | `cmd/easyp/internal/config/cachetype/cachetype.go` |
| connect.api | Connect RPC handler implementing Buf registry services | `internal/connect/api.go` |
| connect.blobs | Download manifest and blob generation | `internal/connect/blobs.go` |
| connect.bynames | Repository lookup by full name (owner/repo) | `internal/connect/bynames.go` |
| connect.modulepins | Module pin resolution (owner/repo/ref to commit) | `internal/connect/modulepins.go` |
| multisource.Repo | Aggregates providers with priority routing + cache-aside | `internal/providers/multisource/repo.go` |
| localgit | Reads .proto files from local git mirrors on disk | `internal/providers/localgit/localgit.go` |
| github | Proxies to GitHub API for repo metadata and file content | `internal/providers/github/` |
| bitbucket | Proxies to BitBucket Server API v1 for repo metadata and files | `internal/providers/bitbucket/` |
| filter | Path/prefix filtering for .proto files, repo lookup by owner/name | `internal/providers/filter/filter.go` |
| content | Shared data types (`Meta`, `File`) for provider results | `internal/providers/content/repo.go` |
| source | Interface definition for a single repository source | `internal/providers/source/source.go` |
| cache.Noop | Pass-through cache (no-op) | `internal/providers/cache/noop.go` |
| cache.Local | Filesystem-based JSON cache | `internal/providers/cache/file.go` |
| artifactory | JFrog Artifactory HTTP-based cache | `internal/providers/cache/artifactory/artifactory.go` |
| https | TLS server with optional mTLS (client cert verification) | `internal/https/https.go` |
| shake256 | SHAKE256 hashing for blob digest generation | `internal/shake256/hash.go` |
| namedlocks | Named mutex map for per-repo git worktree locking | `internal/providers/localgit/namedlocks/lock.go` |
| logger | Global slog wrapper (currently unused in favor of main.go logger) | `internal/logger/logger.go` |

## Pattern Overview

**Overall:** Plugin-style provider architecture with cache-aside

**Key Characteristics:**
- Interface-driven design: `source.Source`, `multisource.Provider`, `multisource.Cache` are the core contracts
- Provider priority: localgit > bitbucket > github (order passed to `multisource.New`)
- Cache-aside: `multisource.Repo.GetFiles` checks cache first, falls through to provider, stores result
- Config-driven: all repository sources and cache settings come from YAML with env var expansion
- Connect RPC protocol: implements Buf's `registry.v1alpha1` gRPC services via Connect protocol (HTTP/JSON or gRPC wire)

## Layers

**Transport Layer (HTTP/gRPC):**
- Purpose: Accept incoming HTTP requests, serve Connect RPC protocol
- Location: `cmd/easyp/main.go` (server), `internal/https/https.go` (TLS)
- Contains: HTTP server setup, TLS configuration, logging middleware
- Depends on: `internal/connect` for request handling
- Used by: external `buf` CLI clients

**API Layer (Connect RPC Handlers):**
- Purpose: Implement Buf registry service endpoints
- Location: `internal/connect/`
- Contains: three service handlers (RepositoryService, ResolveService, DownloadService)
- Depends on: `multisource.Repo` (exposed as `provider` interface inside connect)
- Used by: transport layer via `http.ServeMux`

**Router Layer (Multi-Source):**
- Purpose: Route owner/repo lookups to the correct provider, manage caching
- Location: `internal/providers/multisource/repo.go`
- Contains: `Repo` struct that aggregates `Provider` instances and a `Cache`
- Depends on: `source.Source`, `content.File`, `Cache` interfaces
- Used by: `internal/connect/api.go`

**Provider Layer (VCS Adapters):**
- Purpose: Fetch repository metadata and .proto files from specific VCS backends
- Location: `internal/providers/{localgit,github,bitbucket}/`
- Contains: each provider implements `multisource.Provider` with `Find()` and `Repositories()`
- Depends on: `filter.Repo` for path filtering, `source.Source` interface, external APIs
- Used by: `multisource.Repo`

**Cache Layer:**
- Purpose: Cache resolved .proto file sets keyed by (owner, repo, commit, configHash)
- Location: `internal/providers/cache/`
- Contains: three implementations (Noop, Local filesystem, Artifactory)
- Depends on: `content.File` for serialization
- Used by: `multisource.Repo` via `Cache` interface

**Domain Types:**
- Purpose: Shared data structures used across layers
- Location: `internal/providers/content/repo.go`, `internal/shake256/hash.go`
- Contains: `content.Meta`, `content.File`, `shake256.Hash`
- Depends on: nothing (leaf types)
- Used by: all layers

## Data Flow

### Primary Request Path (buf mod update / DownloadManifestAndBlobs)

1. `buf` CLI sends Connect RPC request to the server (`cmd/easyp/main.go:53-54`)
2. Connect RPC routes to `api.DownloadManifestAndBlobs` (`internal/connect/blobs.go:17`)
3. Handler calls `a.repo.GetFiles(ctx, owner, repo, reference)` which goes to `multisource.Repo.GetFiles` (`internal/providers/multisource/repo.go:62`)
4. `multisource.Repo` finds the matching provider via `findSource()` (`internal/providers/multisource/repo.go:122`), checking localgit first, then bitbucket, then github
5. Cache is checked: `r.cache.Get(ctx, owner, repoName, commit, configHash)` (`internal/providers/multisource/repo.go:70`)
6. On cache miss, provider fetches files: `s.GetFiles(ctx, commit)` (`internal/providers/multisource/repo.go:74`)
7. Files are cached: `r.cache.Put(...)` (`internal/providers/multisource/repo.go:79`)
8. Handler builds manifest (shake256 digest per file) and blob list (`internal/connect/blobs.go:34-36`)
9. Response sent back to `buf` CLI

### Repository Resolution Path (GetRepositoryByFullName)

1. Connect RPC routes to `api.GetRepositoryByFullName` (`internal/connect/bynames.go:32`)
2. Handler splits `owner/name` string and calls `a.repo.GetMeta(ctx, owner, repoName, "")` (`internal/connect/bynames.go:67`)
3. `multisource.Repo.GetMeta` finds provider and delegates (`internal/providers/multisource/repo.go:49`)
4. Provider resolves default branch and latest commit from VCS
5. Response wraps metadata into `registry.Repository` protobuf message

### Module Pin Resolution Path (GetModulePins)

1. Connect RPC routes to `api.GetModulePins` (`internal/connect/modulepins.go:13`)
2. Handler iterates module references, resolving each via `resolveModulePin` (`internal/connect/modulepins.go:45`)
3. Each resolution calls `a.repo.GetMeta(ctx, owner, repo, reference)` to get commit hash
4. Returns `ModulePin` with domain, owner, repo, and resolved commit

**State Management:**
- No persistent state within the server process (stateless)
- Local git repos are on-disk state managed externally (cloned via `scripts/clone_repos.sh`)
- Cache is external (filesystem or Artifactory)
- Named locks (`namedlocks`) coordinate concurrent git worktree checkouts in localgit provider

## Key Abstractions

**`source.Source` interface:**
- Purpose: contract for a single repository source (one owner/repo pair)
- Examples: `internal/providers/localgit/localgit.go:sourceRepo`, `internal/providers/github/repos.go:sourceRepo`, `internal/providers/bitbucket/repos.go:sourceRepo`
- Pattern: interface segregation -- each provider has a private `sourceRepo` type that satisfies `source.Source`

**`multisource.Provider` interface:**
- Purpose: contract for a collection of repositories from one VCS backend
- Examples: `internal/providers/localgit/localgit.go:store`, `internal/providers/github/repos.go:multiRepo`, `internal/providers/bitbucket/repos.go:multiRepo`
- Pattern: factory/registry -- `Find(owner, name)` returns a `source.Source`, `Repositories()` returns all known repos

**`multisource.Cache` interface:**
- Purpose: contract for file-content caching keyed by (owner, repo, commit, configHash)
- Examples: `internal/providers/cache/noop.go:Noop`, `internal/providers/cache/file.go:Local`, `internal/providers/cache/artifactory/artifactory.go:artifactory`
- Pattern: strategy -- cache backend selected at startup based on config

**`connect.api.provider` interface (private):**
- Purpose: narrow interface consumed by the Connect RPC handler layer
- Defined in: `internal/connect/api.go:13-16`
- Pattern: interface segregation -- only exposes `GetMeta` and `GetFiles`

**`filter.Repo` struct:**
- Purpose: encapsulates repository filtering rules (owner, name, path prefixes, path filters)
- Examples: used by all three providers to determine which .proto files to include
- Pattern: value object -- also computes `ConfigHash()` from its contents for cache keys

## Entry Points

**`cmd/easyp/main.go:main()`:**
- Location: `cmd/easyp/main.go:38`
- Triggers: process start (Docker container entrypoint, or `go run`)
- Responsibilities: parse `-cfg` flag, load config, initialize logger, build cache, build providers, wire multisource router, create Connect RPC handler, start HTTP/TLS server, run startup checks

**`api/proto/generate.go`:**
- Location: `api/proto/generate.go`
- Triggers: `go generate ./api/proto/`
- Responsibilities: code generation -- copies Buf proto definitions from `_third_party/buf`, runs `buf generate` to produce Go protobuf and Connect stubs in `gen/proto/`

## Architectural Constraints

- **Threading:** Single-process Go HTTP server. Local git provider uses named mutex locks (`namedlocks`) to prevent concurrent worktree checkout operations on the same repo. Other providers are safe for concurrent use (stateless HTTP clients).
- **Global state:** `internal/logger/logger.go` holds a package-level `globalLogger` (currently unused by main code path; main.go creates its own logger instance).
- **Circular imports:** None detected. Dependency flow is strictly top-down: cmd -> connect -> multisource -> providers -> content/source.
- **Proto generation:** Generated code lives in `gen/proto/` and is committed to the repo. It is regenerated from `api/_third_party/buf` proto definitions via `buf generate` orchestrated by `api/proto/generate.go`.
- **Interface segregation:** The connect layer depends on a private `provider` interface with only `GetMeta` and `GetFiles`. It never sees `source.Source` or `multisource.Provider` directly.

## Anti-Patterns

### Localgit worktree mutation as read operation

**What happens:** `sourceRepo.GetMeta()` and `GetFiles()` in `internal/providers/localgit/localgit.go` perform `git.Worktree.Checkout()` to switch the local clone to a specific commit as a side effect of reading.
**Why it's wrong:** This mutates on-disk state during a read operation. If two requests target different commits of the same repo concurrently, the named lock serializes them, but this is a bottleneck. The local repo must be writeable by the server process.
**Do this instead:** Consider using `git.PlainOpen` with object resolution without checkout, or use a separate clone per request.

### Provider creates new HTTP client per call

**What happens:** In `internal/providers/github/repos.go:79-83`, `connect()` creates a new `github.Client` on every `GetMeta`/`GetFiles` call. Similarly, `internal/providers/bitbucket/repos.go:86-87` calls `connect()` per operation.
**Why it's wrong:** Connection pooling and rate limiting are lost between calls. Each call re-authenticates.
**Do this instead:** Create the HTTP client once at provider construction time and reuse it across calls.

### Startup panics for invalid cache config

**What happens:** `buildCache()` in `cmd/easyp/main.go:346` calls `panic("unreachable reached")` for unrecognized cache types.
**Why it's wrong:** A configuration typo causes an unhandled panic rather than a clear error message.
**Do this instead:** Return an error with the invalid value, and `main()` should exit with a user-friendly message.

## Error Handling

**Strategy:** Error wrapping with `fmt.Errorf("context: %w", err)` at each layer boundary. Errors are not wrapped inside the same layer.

**Patterns:**
- Provider methods wrap errors with context about which operation and which repo: `fmt.Errorf("investigating %q/%q:%q: %w", owner, name, commit, err)`
- `multisource.Repo` does not wrap provider errors (uses `//nolint:wrapcheck`) to avoid double-wrapping
- Cache errors are logged but not propagated: if cache read fails, the provider is called directly; if cache write fails, it is logged and silently skipped
- Startup errors use `panic()` via the `must()` helper in `main.go:252`

## Cross-Cutting Concerns

**Logging:** `log/slog` with JSON handler. Structured logging with key-value pairs. Debug level logs successful requests; error/warn levels log failures. Sensitive HTTP headers (Authorization, Cookie, X-Api-Key, Token) are masked in debug logs. Log level configured via YAML `log.level`.

**Validation:** Input validation is minimal. Repository names are split by `/` with no bounds checking (`splitRepoName` in `internal/connect/bynames.go:87`). Config validation relies on YAML parsing and type constraints. The `cachetype` package validates cache type values during config loading.

**Authentication:** No server-side authentication. The proxy itself does not authenticate incoming `buf` CLI requests. mTLS is supported but optional (when `tls.ca` is configured). Provider-level authentication (GitHub token, BitBucket user/password, Artifactory credentials) is configured in YAML with env var expansion.

---

*Architecture analysis: 2026-05-07*
