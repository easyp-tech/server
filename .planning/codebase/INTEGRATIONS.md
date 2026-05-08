# External Integrations

**Analysis Date:** 2026-05-07

## APIs & External Services

**GitHub API (REST v3):**
- Purpose: Proxy mode - fetches proto files from GitHub repositories on behalf of `buf` CLI clients
- SDK/Client: `github.com/google/go-github/v59` (`internal/providers/github/client.go`)
- Auth: Personal access token per repository config (`token` field in config)
- API calls used:
  - `Repositories.Get` - Get repository metadata (default branch, created/updated timestamps)
  - `Repositories.GetBranch` - Resolve branch to commit SHA
  - `Git.GetTree` - List all files in repository tree (recursive)
  - `Repositories.DownloadContents` - Download individual file content
- Auth pattern: Token set via `c.WithAuthToken(token)` on GitHub client; supports unauthenticated mode (rate limited)

**BitBucket API (REST v1):**
- Purpose: Proxy mode - fetches proto files from BitBucket Server repositories
- SDK/Client: Custom HTTP client using `net/http` (`internal/providers/bitbucket/client.go`)
- Auth: HTTP Basic Auth (username + access token per repository config)
- API calls used:
  - `/branches/default` - Get default branch info
  - `/files` - List files at commit (with `at` query param)
  - `/raw/{name}` - Download raw file content (with `at` query param)
- Configurable base URL (supports self-hosted BitBucket Server instances)

**Buf Registry Protocol (Connect/gRPC):**
- Purpose: Implements the Buf registry service API for `buf` CLI compatibility
- Protocol: Connect RPC (gRPC-compatible over HTTP)
- Services implemented (`internal/connect/api.go`):
  - `ResolveService` - Resolves module references to pinned commits
  - `RepositoryService` - Repository metadata lookup by full name
  - `DownloadService` - Download manifest and blob data for proto files
- Proto definitions sourced from `bufbuild/buf` repository (git submodule at `api/_third_party/buf`)

## Data Storage

**Databases:**
- None. No database dependencies. All data is fetched from upstream git sources and optionally cached.

**File Storage:**
- Local filesystem for git mirrors (`internal/providers/localgit/localgit.go`)
  - Path: configurable via `local.storage` in config
  - Structure: `{storage}/{owner}/{repoName}/` (bare git repositories)
  - Requires write access (service checks out commits via `go-git`)
- Local filesystem cache (`internal/providers/cache/file.go`)
  - Path: configurable via `cache.local.directory` in config
  - Structure: `{dir}/{owner}/{repoName}/{configHash}/{commit}.json`
  - Stores serialized `[]content.File` as JSON
  - Uses atomic write (write to `.tmp`, then `os.Rename`)

**Caching:**
- Three cache strategies (`internal/providers/cache/`):
  - `Noop` - No caching (`cache.type: none`)
  - `Local` - Filesystem-based JSON cache (`cache.type: local`)
  - `Artifactory` - Remote HTTP cache via JFrog Artifactory (`cache.type: artifactory`)
- Cache key: composite of `owner/repoName/configHash/commit`
- `configHash` is CRC32 of the repo filter configuration (changes when path/prefix filters change)
- Cache is read-through: check cache first, fetch from source on miss, then populate cache

**Artifactory Cache (`internal/providers/cache/artifactory/artifactory.go`):**
- JFrog Artifactory HTTP API
- Auth: HTTP Basic Auth (username + token from config)
- Operations: GET (read cache), PUT (write cache), DELETE (connection test cleanup)
- URL pattern: `{baseURL}/{owner}/{repoName}/{configHash}/{commit}.json`
- Periodic write-access check (1-hour interval, configurable via `accessCheckPeriod`)
- Startup connection verification

## Authentication & Identity

**Auth Provider:**
- No user authentication system. The server acts as a transparent proxy.
- TLS client certificate authentication optional (mTLS via `tls.ca` config)

**Upstream Service Auth:**
- GitHub: Per-repository personal access tokens
- BitBucket: Per-repository username + access token (HTTP Basic Auth)
- Artifactory: Username + access token (HTTP Basic Auth)
- All credentials configured in `local.config.yml` with env var substitution support

## Monitoring & Observability

**Error Tracking:**
- None. No external error tracking service.

**Logs:**
- Structured JSON logging via Go `log/slog` (`slog.NewJSONHandler`)
- Log levels: debug, info, warn/warning, error (configurable via `log.level`)
- Request logging middleware with:
  - Request ID propagation (`X-Request-Id` header)
  - Client IP extraction (`X-Real-Ip`, `X-Forwarded-For` headers)
  - Sensitive header masking (authorization, cookie, x-api-key, token)
  - Duration tracking
  - Error-level differentiation (4xx = warn, 5xx = error)
- Startup diagnostics: repository connection checks, cache access verification
- Periodic health checks: cache write-access verification every 1 hour

**Health Check:**
- Root HTTP handler (`/`) returns 200 with plain text health message
- No formal health check protocol or readiness/liveness endpoints

## CI/CD & Deployment

**Hosting:**
- Docker container deployed to Yandex Container Registry
- Image: `cr.yandex/crplga9vcvsvk4uv6541/easyp:stage`

**CI Pipeline:**
- GitHub Actions (`.github/workflows/easyp_build.yml`)
- Trigger: Push to `main` branch
- Steps:
  1. Set up QEMU (multi-arch support)
  2. Set up Docker Buildx
  3. Login to Yandex Container Registry (secrets: `DOCKER_USERNAME`, `DOCKER_TOKEN`)
  4. Build and push Docker image tagged as `stage`

**Build Scripts (`scripts/`):**
- `build.sh` - Local build + Docker image creation and push
- `clone_repos.sh` - Clone git repos for local mirror setup
- `publish.sh` - Build and publish all commands with Dockerfiles
- `push_stage_to_docker.sh` - Build and push stage images to Yandex CR

## Environment Configuration

**Required env vars (when referenced in config via `${VAR_NAME}`):**
- GitHub tokens: per-repository `token` fields
- BitBucket credentials: per-repository `token` and `user` fields
- Artifactory credentials: `token` and `user` fields
- Listen address: `listen` field
- Domain: `domain` field
- TLS certificate paths: `tls.cert`, `tls.key`
- Any config value can use `${ENV_VAR}` syntax for substitution

**Secrets location:**
- Configuration file (`local.config.yml`) contains inline tokens
- Environment variables can be substituted via `${VAR_NAME}` syntax
- GitHub Actions secrets: `DOCKER_USERNAME`, `DOCKER_TOKEN`

## Webhooks & Callbacks

**Incoming:**
- None. The server only responds to Buf Connect/gRPC protocol requests.

**Outgoing:**
- None. The server only makes requests to GitHub/BitBucket APIs and Artifactory on demand.

## Git Submodules

**Third-party Proto Sources:**
- `api/_third_party/buf` - Buf BSR proto definitions (from `github.com/bufbuild/buf.git`)
- `api/_third_party/buf-v1.69.0` - Buf v1.69.0 proto definitions (pinned version)
- `api/_third_party/protobuf` - Google Protocol Buffers well-known types (from `github.com/protocolbuffers/protobuf.git`)

These submodules provide the `.proto` source files that are compiled into Go code via `buf generate` and stored in `gen/proto/`.

---

*Integration audit: 2026-05-07*
