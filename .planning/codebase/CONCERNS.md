# Codebase Concerns

**Analysis Date:** 2026-05-07

## Tech Debt

**Deprecated `golang.org/x/exp` imports used throughout:**
- Issue: The project imports `golang.org/x/exp/slog` and `golang.org/x/exp/slices` extensively. Since Go 1.21, `log/slog` is in the standard library. Since Go 1.21, `slices` is also in the standard library (`golang.org/x/exp` has no compatibility guarantee).
- Files: `cmd/easyp/main.go:24`, `internal/connect/api.go:7`, `internal/providers/multisource/repo.go:8`, `internal/providers/github/client.go:8`, `internal/providers/github/repos.go:8-9`, `internal/providers/github/getfiles.go:10`, `internal/providers/bitbucket/client.go:14`, `internal/providers/bitbucket/repos.go:9-10`, `internal/providers/bitbucket/getfiles.go:8`, `internal/providers/localgit/localgit.go:15`, `internal/providers/cache/artifactory/artifactory.go:14`
- Impact: Dependency on unstable package; increases module size unnecessarily; `golang.org/x/exp` explicitly warns against production use.
- Fix approach: Replace all `golang.org/x/exp/slog` with `log/slog` and `golang.org/x/exp/slices` with `slices`. The module declares `go 1.22` so both are available in stdlib.

**Duplicate `internal/logger` package is unused:**
- Issue: `internal/logger/logger.go` implements a global logger with `Init()`, `Debug()`, `Info()`, `Warn()`, `Error()`, and `Get()` functions, but `cmd/easyp/main.go` creates its own `slog.Logger` via `newLogger()` and passes it to all providers. The `logger` package is never imported by application code.
- Files: `internal/logger/logger.go` (52 lines)
- Impact: Dead code; creates confusion about which logging approach to use.
- Fix approach: Remove `internal/logger/` entirely. The dependency-injected logger pattern used in `main.go` is the correct approach.

**Duplicate `fileFiltered` type and `getFiles` logic across providers:**
- Issue: `internal/providers/github/getfiles.go` and `internal/providers/bitbucket/getfiles.go` both define identical `fileFiltered` structs and nearly identical `getFiles()` methods that iterate filtered files, download content, hash it, and build `content.File` slices. `localgit` also hashes files individually in `enumerateProto()`.
- Files: `internal/providers/github/getfiles.go:37-80`, `internal/providers/bitbucket/getfiles.go:33-74`, `internal/providers/localgit/localgit.go:176-216`
- Impact: Three copies of the same download-hash-accumulate pattern. Bug fixes must be applied in three places.
- Fix approach: Extract a shared helper in `internal/providers/content/` that handles the hash-and-accumulate step. Each provider only provides the raw download mechanism.

**Duplicate `ConfigHash()` implementation across providers:**
- Issue: All three source providers (github, bitbucket, localgit) implement `ConfigHash()` with the same `crc32.ChecksumIEEE(fmt.Sprintf("%+v", ...))` pattern. `localgit` uses `r.repo` while github/bitbucket use `r.repo.Repo` -- an inconsistency that may cause config hash mismatches.
- Files: `internal/providers/github/repos.go:68-69`, `internal/providers/bitbucket/repos.go:76-77`, `internal/providers/localgit/localgit.go:111-112`, `internal/providers/filter/filter.go:29-31`
- Impact: `filter.Repo` already has a `Hash()` method that is not used by any of these. Potential for cache key collisions if the string format diverges between implementations.
- Fix approach: Use `filter.Repo.Hash()` consistently across all three providers.

**`draft.txt` tracks roadmap items in the repo root:**
- Issue: `draft.txt` describes unfinished work items (test suite, modern buf protocol implementation) but is committed to the repo root and not in any structured issue tracker.
- Files: `draft.txt`
- Impact: Non-actionable; not tracked in CI or project management.
- Fix approach: Move items to GitHub Issues or a proper TODO tracking system. Remove `draft.txt`.

## Known Bugs

**`splitRepoName()` panics on malformed input:**
- Symptoms: Index out-of-range panic when a full name without a `/` separator is passed to `GetRepositoryByFullName` or `GetRepositoriesByFullName`.
- Files: `internal/connect/bynames.go:87-91`
- Trigger: Any request with `fullName` that does not contain a `/` character (e.g., `"googleapis"` instead of `"googleapis/googleapis"`). `strings.Split` produces a single-element slice, and `fields[1]` panics.
- Workaround: None. Server crashes on malformed request.

**`loggingResponseWriter.status` defaults to 0, not 200:**
- Symptoms: Successful responses without explicit `WriteHeader()` call have `status == 0` instead of `200`. The logging middleware checks `status >= 400` which correctly skips zero, so successful responses are only logged at debug level. But if the logic is ever changed to use the numeric value, it would be incorrect.
- Files: `cmd/easyp/main.go:235-250`
- Trigger: Normal HTTP handling where `WriteHeader` is not called (implicit 200).
- Workaround: The current code path only uses `status >= 400` and `status >= 500` checks, so the zero value is effectively treated as success. However, if anyone logs the status or uses it for metrics, it will show `0` instead of `200`.

**`AppendCertsFromPEM` return value ignored:**
- Symptoms: If the CA certificate file is malformed or empty, `AppendCertsFromPEM` returns `false` but the error is silently ignored. The server starts with an empty CA cert pool, causing all mTLS client certificate validation to fail.
- Files: `internal/https/https.go:51`
- Trigger: Provide a valid path to a file that does not contain valid PEM certificates.
- Workaround: None. Silent failure at startup.

**BitBucket file listing ignores pagination:**
- Symptoms: The `listFiles()` method requests up to 1,000,000 files (`filesListUnlimited = "1000000"`) in a single API call to avoid pagination. If the repository has more files than the server-side limit allows, files are silently truncated.
- Files: `internal/providers/bitbucket/getfiles.go:95-125`
- Trigger: BitBucket repository with more files than the API's maximum page size.
- Workaround: None. Some proto files would be silently missing from the response.

**Artifactory Put status code check is inverted:**
- Symptoms: The condition `resp.StatusCode < http.StatusOK && resp.StatusCode >= http.StatusMultipleChoices` (line 120) is always false. The intent is `resp.StatusCode >= http.StatusMultipleChoices` (i.e., 300+), but the `< 200 && >= 300` conjunction can never be true simultaneously.
- Files: `internal/providers/cache/artifactory/artifactory.go:120`
- Trigger: Any Artifactory PUT that returns an error status (e.g., 403, 500) is treated as success.
- Workaround: None. Cache put failures are silently accepted.

**BitBucket `CreatedAt`/`UpdatedAt` use `time.Now()` instead of real values:**
- Symptoms: The BitBucket provider sets `CreatedAt` and `UpdatedAt` to `time.Now()` because the BitBucket API response does not include these timestamps in the branch endpoint response.
- Files: `internal/providers/bitbucket/getrepo.go:41-42`
- Trigger: Every `GetMeta` call for a BitBucket repository.
- Workaround: None. Repository metadata always shows the current time rather than actual creation/update time.

## Security Considerations

**`http.DefaultClient` used for all external HTTP calls:**
- Risk: `http.DefaultClient` has no timeout configured. A slow or unresponsive Artifactory or BitBucket server can block goroutines indefinitely, causing resource exhaustion and denial of service.
- Files: `internal/providers/cache/artifactory/artifactory.go:62,113,144,167`, `internal/providers/bitbucket/client.go:94`
- Current mitigation: Request-level `context.Context` with timeouts may be passed by callers, but the Artifactory client does not enforce any client-level timeout.
- Recommendations: Create dedicated `http.Client` instances with explicit `Timeout` values for Artifactory and BitBucket connections.

**Unbounded `io.ReadAll` on external response bodies:**
- Risk: `io.ReadAll` without size limits on responses from Artifactory (`artifactory.go:77`), GitHub (`getfiles.go:102`), and BitBucket (`client.go:105`) can cause memory exhaustion if an external service returns an unexpectedly large response.
- Files: `internal/providers/cache/artifactory/artifactory.go:77`, `internal/providers/github/getfiles.go:102`, `internal/providers/bitbucket/client.go:105`
- Current mitigation: Only error-path reads in `artifactory.go:151,174` use `LimitReader` (capped at 1KB).
- Recommendations: Use `io.LimitReader` with a reasonable cap (e.g., 50MB) for all external response body reads.

**Config file uses `os.ExpandEnv` enabling env var injection:**
- Risk: `os.ExpandEnv` in config parsing expands `${VAR}` and `$VAR` references in YAML. If config files come from untrusted sources, environment variables could leak into config. More importantly, if a config value like `${UNKNOWN_VAR}` is used for a token, it expands to empty string silently.
- Files: `cmd/easyp/internal/config/read.go:18`
- Current mitigation: Config files are expected to be operator-controlled.
- Recommendations: Document this behavior. Consider validating that required fields (tokens, URLs) are non-empty after expansion.

**Test TLS certificates committed to the repository:**
- Risk: `testdata/cert.pem` and `testdata/key.pem` are committed to version control. While these are test certificates, they could be accidentally used in production or leak information about the test setup.
- Files: `testdata/cert.pem`, `testdata/key.pem`
- Current mitigation: `.gitignore` does not exclude them; they are intentionally committed.
- Recommendations: Add a comment in the files or a README in `testdata/` clarifying these are for local testing only.

**BitBucket credentials in `httpClient` struct stored as plain strings:**
- Risk: User and password are stored as plain `string` fields in the `httpClient` struct and passed around by value. They could appear in log output if the struct is ever logged with `%+v` or `%#v`.
- Files: `internal/providers/bitbucket/client.go:68-72`, `internal/providers/bitbucket/repos.go:18-20`
- Current mitigation: The sensitive header masking in `cmd/easyp/main.go` masks `Authorization` headers in debug logs. However, the struct fields themselves are not protected.
- Recommendations: Be cautious about logging any `Repo` or `httpClient` structs directly.

**No request rate limiting:**
- Risk: The proxy has no rate limiting on incoming requests. A malicious or misconfigured client could flood the proxy, causing excessive GitHub API calls (potentially hitting GitHub rate limits) or exhausting local git resources.
- Files: `cmd/easyp/main.go` (no middleware for rate limiting)
- Current mitigation: None.
- Recommendations: Add rate limiting middleware, at minimum per-IP or per-repository.

**`X-Forwarded-For` header trusted for client IP:**
- Risk: `getClientIP()` trusts `X-Real-Ip` and `X-Forwarded-For` headers from the client, which can be spoofed. This is only used for logging, not for auth, so the risk is limited to log poisoning.
- Files: `cmd/easyp/main.go:208-216`
- Current mitigation: Used only for logging, not for access control.
- Recommendations: Document that client IPs in logs may be spoofed if the proxy is not behind a trusted reverse proxy.

## Performance Bottlenecks

**Sequential file downloads in GitHub and BitBucket providers:**
- Problem: Files are downloaded one at a time in a loop. For large repositories (e.g., `googleapis/googleapis` with hundreds of proto files), this creates significant latency.
- Files: `internal/providers/github/getfiles.go:56-80`, `internal/providers/bitbucket/getfiles.go:52-74`
- Cause: Simple `for` loop with blocking HTTP calls per file.
- Improvement path: Use goroutine pool with bounded concurrency (e.g., 10-20 parallel downloads) to reduce total latency.

**Local git provider checks out working tree on every request:**
- Problem: `getRepoSwitchedCommit()` calls `w.Checkout()` on every `GetMeta()` and `GetFiles()` call. This modifies the on-disk working tree under a named lock, serializing all access to the same repository.
- Files: `internal/providers/localgit/localgit.go:120-173`
- Cause: go-git's `Worktree.Checkout()` writes files to disk. The named lock prevents concurrent access but also prevents parallel reads of different commits.
- Improvement path: Use `go-git` object access without checkout (read blobs directly from the object store) or maintain a separate clone per active commit.

**Named lock map grows unbounded:**
- Problem: `namedlocks` uses a `map[string]*sync.Mutex` that never shrinks. Over time with many unique repository+commit combinations, this map grows indefinitely.
- Files: `internal/providers/localgit/namedlocks/lock.go:15-46`
- Cause: Locks are created on demand but never removed.
- Improvement path: Add cleanup logic or use a bounded cache with eviction.

**No response compression:**
- Problem: The connect RPC responses (manifests and blobs) can be large but no HTTP compression middleware is configured.
- Files: `cmd/easyp/main.go` (no compression middleware), `internal/connect/api.go`
- Cause: Plain `http.ServeMux` with no middleware for gzip/deflate.
- Improvement path: Add `compress/gzip` middleware or use a library like `github.com/NYTimes/gziphandler`.

## Fragile Areas

**`splitRepoName` input validation:**
- Files: `internal/connect/bynames.go:87-91`
- Why fragile: No validation on input. `strings.Split` followed by direct index `[1]` access panics on any string without `/`.
- Safe modification: Add length check after split; return error for malformed names.
- Test coverage: None (no test files in the project).

**BitBucket client template execution panics on error:**
- Files: `internal/providers/bitbucket/client.go:118-124`
- Why fragile: `tmplExec()` calls `panic(err)` if template execution fails. This crashes the entire server if a template variable contains unexpected values.
- Safe modification: Return errors instead of panicking. Template execution should never fail with the current static templates, but the panic path is dangerous.
- Test coverage: None.

**`multisource.Repo.GetFiles()` returns partial results on error:**
- Files: `internal/providers/multisource/repo.go:62-82`
- Why fragile: If `s.GetFiles()` returns `(files, err)` where `files != nil` and `err != nil`, the partial files are returned along with the error. The caller may process partial data.
- Safe modification: Return `nil` on error consistently.
- Test coverage: None.

**`resolveModulePins()` returns partial results on error:**
- Files: `internal/connect/modulepins.go:30-43`
- Why fragile: Returns `out` (partially resolved module pins) along with an error. The gRPC layer will serialize this partial response before returning the error.
- Safe modification: Return `nil, err` on any error.
- Test coverage: None.

**`enumerateProto()` silently swallows directory read errors:**
- Files: `internal/providers/localgit/localgit.go:184-187`
- Why fragile: `WalkDir` callback returns `nil` when `err != nil` (silenced with `//nolint:nilerr`). This means permission errors, I/O errors, etc., during directory traversal are silently ignored.
- Safe modification: Return the error from `WalkDir` to propagate I/O problems.
- Test coverage: None.

## Scaling Limits

**Single-process, no horizontal scaling:**
- Current capacity: Single process handles all requests. No clustering support.
- Limit: Cannot scale beyond a single machine. Cache is local to the process (unless Artifactory is used).
- Scaling path: Add external cache (Artifactory support exists). For horizontal scaling, extract shared state and add load balancing.

**GitHub API rate limiting not handled:**
- Current capacity: GitHub API has rate limits (typically 5000 requests/hour for authenticated users). The proxy makes one tree request plus one request per file per `GetFiles` call.
- Limit: A repository with 500 proto files consumes 501 API calls per cache miss. Approximately 10 cache misses per hour would exhaust the rate limit.
- Scaling path: Implement GitHub API rate limit detection and backoff. Increase cache hit ratio.

**Artifactory HTTP client lacks connection pooling configuration:**
- Current capacity: Uses `http.DefaultClient` which has default transport settings (100 idle connections per host, 2 idle connections per host).
- Limit: Under high concurrency, the default settings may not be optimal for Artifactory communication.
- Scaling path: Configure a custom `http.Transport` with tuned `MaxIdleConns`, `MaxIdleConnsPerHost`, and `IdleConnTimeout`.

## Dependencies at Risk

**`github.com/ghodss/yaml` (YAML parser):**
- Risk: This library is a wrapper around `gopkg.in/yaml.v2` and has not seen significant updates. The `yaml.v2` parser is superseded by `yaml.v3`.
- Impact: Potential unpatched vulnerabilities; missing YAML 1.2 features.
- Migration plan: Switch to `gopkg.in/yaml.v3` directly or use `sigs.k8s.io/yaml` which wraps v3.

**`github.com/go-git/go-git/v5` (git operations):**
- Risk: Pinned at `v5.9.0` which is not the latest. Several CVEs have been addressed in newer versions.
- Impact: Potential security issues in git protocol handling.
- Migration plan: Update to the latest v5 release and verify compatibility.

**`connectrpc.com/connect v1.11.1`:**
- Risk: Not the latest version. Newer versions may contain bug fixes and performance improvements.
- Impact: Missing bug fixes and improvements.
- Migration plan: Update to latest stable version.

**`google.golang.org/grpc v1.59.0`:**
- Risk: This is a transitive dependency but pinned at an older version. May have known vulnerabilities.
- Impact: Potential security issues in gRPC transport layer.
- Migration plan: Update indirect dependency via `go mod tidy` and version bumping.

## Missing Critical Features

**No test suite:**
- Problem: Zero test files exist in the project (excluding third-party submodules). The `draft.txt` explicitly calls out the need for tests.
- Blocks: Safe refactoring, CI quality gates, regression detection.
- Files affected: All source files under `internal/` and `cmd/`.

**No graceful shutdown:**
- Problem: The HTTP server has no signal handling or `Shutdown()` call. When the process receives SIGTERM, in-flight requests are dropped.
- Blocks: Safe deployments in containerized environments (Kubernetes sends SIGTERM).

**No health check endpoint for cache providers:**
- Problem: While `checkCacheAccess()` runs at startup and periodically, there is no HTTP health check endpoint that reflects cache/provider health. Container orchestrators cannot determine if the proxy is healthy.
- Blocks: Kubernetes liveness/readiness probes.

**Modern buf protocol not implemented:**
- Problem: The proxy only implements the deprecated buf protocol (pre-v1.30.1). Modern buf clients cannot use this proxy.
- Blocks: Upgrading to modern buf toolchain. Noted in `draft.txt`.

**Repository description always empty:**
- Problem: `resolveRepoByFullName()` hard-codes `Description: ""` with a `// TODO` comment.
- Files: `internal/connect/bynames.go:81`
- Blocks: Rich repository metadata in API responses.

## Test Coverage Gaps

**Entire project has zero test coverage:**
- What's not tested: All RPC handlers (`GetRepositoriesByFullName`, `GetRepositoryByFullName`, `GetModulePins`, `DownloadManifestAndBlobs`), all provider implementations (GitHub, BitBucket, local git), all cache implementations (Artifactory, local, noop), config parsing, TLS setup, middleware, hashing.
- Files: All files under `internal/` and `cmd/`
- Risk: Any change can introduce regressions without detection. Critical bugs like the `splitRepoName` panic and the Artifactory status code check inversion exist undetected.
- Priority: High

**No integration tests for external providers:**
- What's not tested: GitHub API interaction, BitBucket API interaction, Artifactory API interaction.
- Files: `internal/providers/github/*.go`, `internal/providers/bitbucket/*.go`, `internal/providers/cache/artifactory/*.go`
- Risk: API changes on external services break the proxy silently.
- Priority: High

**No tests for config parsing with `os.ExpandEnv`:**
- What's not tested: Environment variable expansion in config files, handling of missing/invalid env vars, malformed YAML.
- Files: `cmd/easyp/internal/config/read.go`
- Risk: Config misconfiguration goes undetected until runtime.
- Priority: Medium

---

*Concerns audit: 2026-05-07*
