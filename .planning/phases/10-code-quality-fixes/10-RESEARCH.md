# Phase 10: Code Quality Fixes — Research

**Research Date:** 2026-05-09
**Status:** Ready for planning
**Scope:** All changes needed to implement Phase 10 per 10-CONTEXT.md

---

## 1. Executive Summary

Phase 10 fixes 5 critical bugs, eliminates code duplication across three providers, removes dead code, adds security hardening, and establishes a unit test foundation. The phase touches **14 files** across 6 packages, with zero new external dependencies.

**Breakdown:**

| Category | Items | Priority |
|----------|-------|----------|
| Critical Bug Fixes | 5 bugs (panic, inverted check, partial results) | P0 |
| Code Deduplication | 1 shared helper, 3 ConfigHash fixes, 1 delete | P0 |
| Security Hardening | HTTP timeout + body limits per provider | P0 |
| Dead Code Removal | Delete `internal/logger/` package | P1 |
| Test Suite | Unit tests for bug fixes + critical paths | P1 |
| Config Extensions | Timeout/body limits in config structs | P1 |

---

## 2. Critical Bug Fixes

### 2.1 Artifactory Put Status Code — Inverted Condition

**File:** `internal/providers/cache/artifactory/artifactory.go:121`

**Current code:**
```go
if resp.StatusCode < http.StatusOK && resp.StatusCode >= http.StatusMultipleChoices {
    return fmt.Errorf("putting %q: response %d: %w", req.URL.String(), resp.StatusCode, ErrUnexpected)
}
```

**Problem:** The condition `< 200 && >= 300` is always `false`. The programmer clearly intended to check for error status codes (300+) but the `< 200` makes it impossible to satisfy. Error PUTs are silently treated as success.

**Fix:**
```go
if resp.StatusCode >= http.StatusMultipleChoices {
    return fmt.Errorf("putting %q: response %d: %w", req.URL.String(), resp.StatusCode, ErrUnexpected)
}
```

**Impact:** Bug has existed since the Artifactory cache was written. Cache PUT failures are silently ignored — files never reach Artifactory and callers never find out.

---

### 2.2 `splitRepoName()` Panic on Malformed Input

**File:** `internal/connect/bynames.go:87-91`

**Current code:**
```go
func splitRepoName(name string) (string, string) {
    fields := strings.Split(name, "/")
    return fields[0], fields[1]
}
```

**Problem:** Direct array access `fields[1]` panics with `index out of range` if the input doesn't contain `/`. Any request like `"googleapis"` instead of `"googleapis/repo"` crashes the server.

**Decision (D-02 from discuss-phase):** Keep `(string, string)` signature. Check array length but do not change to `(string, string, error)`.

**Fix:**
```go
func splitRepoName(name string) (string, string) {
    fields := strings.Split(name, "/")
    if len(fields) != 2 {
        return "", ""
    }
    return fields[0], fields[1]
}
```

**Callers** (`resolveRepoByFullName`, `resolveReposByFullNames`): Need to handle the empty-string case. If `repositoryName == ""`, return an error:
```go
func (a *api) resolveRepoByFullName(ctx context.Context, name string) (*registry.Repository, error) {
    owner, repositoryName := splitRepoName(name)
    if repositoryName == "" {
        return nil, fmt.Errorf("invalid repository name %q: expected owner/repo format", name)
    }
    // ... rest unchanged
}
```

**Impact:** Server crashes on malformed requests. Easy to trigger — no authentication required.

---

### 2.3 Return `nil` on Errors (Not Partial Results)

Three locations return partial data along with errors:

**A. `multisource/repo.go:74-77`** — `GetFiles` returns partial files on error:
```go
files, err := s.GetFiles(ctx, commit)
if err != nil {
    return files, fmt.Errorf("getting files: %w", err)
}
```
**Fix:** Change to `return nil, fmt.Errorf(...)`.

**B. `internal/connect/modulepins.go:30-43`** — `resolveModulePins` returns partial `out` on error:
```go
for i, m := range in {
    v, err := a.resolveModulePin(ctx, m)
    if err != nil {
        return out, fmt.Errorf("iterating %d of %d: %w", i, len(in), err)
    }
    out = append(out, v)
}
```
**Fix:** Change to `return nil, fmt.Errorf(...)`.

**C. `internal/connect/bynames.go:52-58`** — `resolveReposByFullNames` returns partial results on error:
```go
for i, name := range in {
    v, err := a.resolveRepoByFullName(ctx, name)
    if err != nil {
        return out, fmt.Errorf("iterating %d of %d: %w", i, len(in), err)
    }
    out = append(out, v)
}
```
**Fix:** Change to `return nil, fmt.Errorf(...)`.

**Impact:** gRPC serializes partial responses before returning errors. Clients may process incomplete data.

---

## 3. Code Deduplication

### 3.1 Shared Download Helper

**Decision (D-04):** Create `internal/providers/content/download.go` with `fileFiltered` type and hash-accumulate logic. All three providers will use this.

**What to extract:**

All three providers have this identical/near-identical code:

```go
// fileFiltered struct — identical across all three
type fileFiltered struct {
    orig string
    name string
}

// filterEntries — nearly identical, varies only by entry type
// GitHub: []*github.TreeEntry
// BitBucket: []string
// localgit: not used (uses WalkDir)

// getFiles loop — identical pattern in github, bitbucket
for _, file := range files {
    data, err := downloadFile(ctx, ...)
    if err != nil {
        return nil, fmt.Errorf(...)
    }
    hash, err := shake256.SHA3Shake256(data)
    if err != nil {
        return nil, fmt.Errorf(...)
    }
    out = append(out, content.File{Path: file.name, Data: data, Hash: hash})
}
```

**Proposed new file:** `internal/providers/content/download.go`

```go
package content

import (
    "context"
    "fmt"
    "strings"

    "slices"

    "github.com/easyp-tech/server/internal/shake256"
)

type FileEntry struct {
    Orig string // original path (for the provider's API call)
    Name string // filtered/rewritten path (for content.File.Path)
}

// GetFiles downloads multiple files, hashes them, and returns content.File slices.
// downloadFn is called once per entry and should return raw file bytes.
func GetFiles(
    ctx context.Context,
    entries []FileEntry,
    downloadFn func(ctx context.Context, orig string) ([]byte, error),
) ([]File, error) {
    out := make([]File, 0, len(entries))

    for _, entry := range entries {
        data, err := downloadFn(ctx, entry.Orig)
        if err != nil {
            return nil, fmt.Errorf("downloading %q: %w", entry.Orig, err)
        }

        hash, err := shake256.SHA3Shake256(data)
        if err != nil {
            return nil, fmt.Errorf("hashing %q: %w", entry.Orig, err)
        }

        out = append(out, File{Path: entry.Name, Data: data, Hash: hash})
    }

    return out, nil
}

// FilterEntries takes raw file entries and applies a filter.Repo check,
// returning sorted FileEntry slices for use with GetFiles.
func FilterEntries[T any](
    entries []T,
    getPath func(T) string,
    repo filter.Repo,
) []FileEntry {
    out := make([]FileEntry, 0, len(entries))

    for _, entry := range entries {
        if name, ok := repo.Check(getPath(entry)); ok {
            out = append(out, FileEntry{Orig: getPath(entry), Name: name})
        }
    }

    slices.SortFunc(out, func(a, b FileEntry) int { return strings.Compare(a.Name, b.Name) })
    return out
}
```

**Refactoring GitHub** (`internal/providers/github/getfiles.go`):
- Remove `fileFiltered` struct and `filterEntries` function
- Keep only provider-specific logic: `listFiles` (GitHub tree API), `getFile` (contents API)
- Use `content.GetFiles` for the hash-accumulate loop
- Use `content.FilterEntries` for filtering

**Refactoring BitBucket** (`internal/providers/bitbucket/getfiles.go`):
- Similar pattern
- `listFiles` returns `[]string` from API
- Use `content.FilterEntries` with string accessor
- `getFile` uses HTTP client

**Refactoring localgit** (`internal/providers/localgit/localgit.go:176-216`):
- `enumerateProto` is already structured differently (WalkDir), so it's harder to refactor
- The fileFiltered pattern doesn't apply; localgit reads files directly from the filesystem
- **Decision:** Skip refactoring localgit for now. The duplication exists primarily in the two HTTP-based providers. Localgit uses a fundamentally different access pattern (WalkDir + os.ReadFile) that doesn't benefit from the shared helper.
- Update **10-PLAN.md** accordingly: only refactor GitHub and BitBucket providers.

---

### 3.2 Use `filter.Repo.Hash()` Consistently

**Decision (D-05):** All three providers use `filter.Repo.Hash()` for `ConfigHash()`.

**Current implementations:**

| Provider | Current Code |
|----------|-------------|
| `github/repos.go:68-69` | `r.repo.Repo` (nested) |
| `bitbucket/repos.go:76-77` | `r.repo.Repo` (nested) |
| `localgit/localgit.go:111-113` | `r.repo` (direct) |

All use `crc32.ChecksumIEEE(fmt.Sprintf("%+v", ...))` — but `filter.Repo` already has a `Hash()` method (filter.go:29-31) that does exactly this. The providers are duplicating the logic and using inconsistent receivers (`r.repo` vs `r.repo.Repo`).

**Fix (all three):**
```go
func (r sourceRepo) ConfigHash() string {
    return r.repo.Hash()
}
```

For GitHub/BitBucket where `repo` embeds `filter.Repo`, this means `r.repo.Hash()` — the embedded `Hash()` method is promoted and called directly on the `Repo` struct.

**Note:** `filter.Repo.Hash()` produces `fmt.Sprintf("%X", crc32.ChecksumIEEE([]byte(fmt.Sprintf("%+v", r))))`. This is semantically identical to the current implementations. No cache key invalidation.

---

### 3.3 Remove `internal/logger/` Package

**Decision (D-06):** Remove the entire `internal/logger/` package. The dependency-injected `*slog.Logger` pattern in `main.go` is the correct approach.

**Action:**
1. Delete `internal/logger/logger.go`
2. Check for any remaining imports — based on codebase analysis, the logger package is **never imported** by any application code. `cmd/easyp/main.go` creates its own `slog.Logger` via `newLogger()` and passes it down via dependency injection.
3. Verify no references exist: run `grep -r "internal/logger" --include="*.go" .`

**Current state:** Dead code. 52 lines of unused functionality.

---

## 4. Security Hardening

### 4.1 Per-Provider HTTP Timeouts

**Decision (D-07):** Add configurable HTTP client timeouts. Default 30 seconds.

**Changes needed:**

**Config structs** (`cmd/easyp/internal/config/config.go`):
```go
// Add to GithubRepo, BitBucketRepo, and Artifactory structs:
type GithubRepo struct {
    Repo        Repo   `json:"repo"`
    AccessToken string `json:"token"`
    Timeout    int    `json:"timeout"` // seconds; 0 means default
}

type BitBucketRepo struct {
    Repo        Repo   `json:"repo"`
    User        string `json:"user"`
    AccessToken string `json:"token"`
    BaseURL     URL    `json:"url"`
    Timeout     int    `json:"timeout"` // seconds; 0 means default
}

// Artifactory is already in the config but needs the same treatment:
type Artifactory struct {
    User        string `json:"user"`
    AccessToken string `json:"token"`
    BaseURL     URL    `json:"url"`
    Timeout     int    `json:"timeout"` // seconds; 0 means default
}
```

**HTTP client creation** — providers need a way to construct `http.Client` with timeout. Options:
1. Pass a `*http.Client` to provider constructors
2. Pass timeout duration to constructors; provider creates client internally
3. Add a shared helper function in a new package

**Decision:** Option 2 — provider constructors accept `timeout time.Duration`. Each provider constructs its own `http.Client` with `Timeout: timeout`. Default `30 * time.Second` when `timeout <= 0`.

**GitHub** (`internal/providers/github/client.go`): Currently uses `google/go-github/v59/github.NewClient(token)`. The go-github client has `http.Client` embedded. Need to replace with `httpClient` setup:
```go
func connect(token string, timeout time.Duration) *github.Client {
    httpClient := &http.Client{Timeout: timeout}
    return github.NewClient(httpClient).WithAuthToken(token)
}
```

**BitBucket** (`internal/providers/bitbucket/client.go`): Already has a custom `httpClient` struct with `Do` method. Add `Timeout` field:
```go
type httpClient struct {
    client  http.Client
    user    string
    pass    string
    baseURL *url.URL
    timeout time.Duration
}
```

**Artifactory** (`internal/providers/cache/artifactory/artifactory.go`): Uses `http.DefaultClient` in several places (lines 63, 114, 145, 168). Replace with a `http.Client` field on the struct:
```go
type artifactory struct {
    log      *slog.Logger
    baseURL  string
    user     string
    password string
    client   http.Client // with configured timeout
}
```

---

### 4.2 Response Body Size Limits

**Decision (D-08):** Add configurable response body limits. Default 50MB. Use `io.LimitReader`.

**Files needing limits:**

| File | Line | Operation | Current |
|------|------|-----------|---------|
| `artifactory.go` | 78 | `io.ReadAll(resp.Body)` | Unbounded |
| `github/getfiles.go` | 103 | `io.ReadAll(r)` | Unbounded |
| `bitbucket/client.go` | 105 | `httpGetJSON` body read | Unbounded |

**Implementation:**
```go
const defaultBodyLimit = 50 * 1 << 20 // 50MB

// LimitReadCloser wraps a ReadCloser and limits total bytes read.
type LimitReadCloser struct {
    io.Reader
    closeFn func() error
}

func (l *LimitReadCloser) Close() error { return l.closeFn() }

// In provider code, wrap resp.Body:
data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
if err != nil {
    return nil, fmt.Errorf("reading response: %w", err)
}
```

**Config addition** — add `BodyLimit int64` (bytes; 0 means default) to `GithubRepo`, `BitBucketRepo`, and `Artifactory` config structs. Pass to providers.

**Special case:** Error body reads in `artifactory.go:151,174` already use `io.LimitReader(..., 1024)` — these can stay with the 1KB limit.

---

## 5. Test Suite

**Decision (D-12):** Implement unit tests for all public APIs. Priority on testing the critical bugs that were fixed.

**Strategy:** Since there's zero test coverage currently, start with a focused test suite covering the bugs and critical paths:

### 5.1 Test Files to Create

| Test File | What's Tested | Priority |
|-----------|-------------|----------|
| `internal/connect/bynames_test.go` | `splitRepoName` edge cases, `resolveRepoByFullName` with empty owner | P0 |
| `internal/connect/modulepins_test.go` | `resolveModulePins` returns nil on error | P0 |
| `internal/providers/multisource/repo_test.go` | `GetFiles` returns nil on error | P0 |
| `internal/providers/cache/artifactory/artifactory_test.go` | Artifactory PUT status check | P0 |
| `internal/providers/filter/filter_test.go` | `filter.Repo.Hash()` consistency | P1 |
| `internal/connect/blobs_test.go` | Manifest/blob generation | P1 |

### 5.2 Testing Patterns

**Mock interfaces for provider tests:**

```go
// Mock Source that can be configured to return errors
type mockSource struct {
    metaFn  func(ctx context.Context, commit string) (content.Meta, error)
    filesFn func(ctx context.Context, commit string) ([]content.File, error)
}

func (m *mockSource) GetMeta(ctx context.Context, commit string) (content.Meta, error) {
    return m.metaFn(ctx, commit)
}

func (m *mockSource) GetFiles(ctx context.Context, commit string) ([]content.File, error) {
    return m.filesFn(ctx, commit)
}

func (m *mockSource) ConfigHash() string { return "mock-hash" }
func (m *mockSource) Name() string       { return "mock" }
func (m *mockSource) Owner() string     { return "mock-owner" }
func (m *mockSource) RepoName() string  { return "mock-repo" }
func (m *mockSource) Type() string      { return "mock" }
```

**Key test cases for bugs:**

```go
// splitRepoName tests
func TestSplitRepoName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantOwner string
        wantRepo  string
    }{
        {"normal", "googleapis/googleapis", "googleapis", "googleapis"},
        {"no_slash", "googleapis", "", ""},  // Should NOT panic
        {"empty", "", "", ""},
        {"too_many_parts", "a/b/c", "a", "b"},
    }
    // ...
}
```

### 5.3 Test Infrastructure

**No external test framework needed.** Use Go's stdlib `testing` package.

**Mock implementations needed:**
- `mockSource` (implements `source.Source`)
- `mockProvider` (implements `multisource.Provider`)
- `mockCache` (implements `multisource.Cache`)

**Test utilities location:** `internal/` — tests alongside source files (`*_test.go`).

---

## 6. Config Changes

### 6.1 Extended Config Structs

**File:** `cmd/easyp/internal/config/config.go`

Add to `Config` struct:
```go
type Config struct {
    Listen netip.AddrPort `json:"listen"`
    Domain string         `json:"domain"`
    TLS    TLSConfig      `json:"tls"`
    Cache  Cache          `json:"cache"`
    Proxy  Proxy          `json:"proxy"`
    Local  LocalGit       `json:"local"`
    Log    LogConfig      `json:"log"`
    HTTP   HTTPConfig     `json:"http"`  // new
}
```

New structs:
```go
type HTTPConfig struct {
    Timeout   int   `json:"timeout"`   // seconds; 0 = 30s default
    BodyLimit int64 `json:"bodyLimit"`  // bytes; 0 = 50MB default
}
```

Update provider config structs:
```go
type GithubRepo struct {
    Repo        Repo   `json:"repo"`
    AccessToken string `json:"token"`
    Timeout    int    `json:"timeout"`
    BodyLimit  int64  `json:"bodyLimit"`
}

type BitBucketRepo struct {
    Repo        Repo   `json:"repo"`
    User        string `json:"user"`
    AccessToken string `json:"token"`
    BaseURL     URL    `json:"url"`
    Timeout     int    `json:"timeout"`
    BodyLimit   int64  `json:"bodyLimit"`
}

type Artifactory struct {
    User        string `json:"user"`
    AccessToken string `json:"token"`
    BaseURL     URL    `json:"url"`
    Timeout     int    `json:"timeout"`
    BodyLimit   int64  `json:"bodyLimit"`
}
```

**Provider wiring** (`cmd/easyp/main.go`): Pass timeout and body limit when constructing providers.

---

## 7. Implementation Order

**Recommended order (dependency-aware):**

1. **Fix critical bugs first** (no deps):
   - Artifactory status check inversion
   - splitRepoName panic fix
   - Return nil on errors (3 locations)

2. **Remove dead code** (no deps):
   - Delete `internal/logger/logger.go`

3. **Create shared download helper**:
   - Create `internal/providers/content/download.go`
   - Refactor GitHub provider
   - Refactor BitBucket provider
   - Note: localgit skipped (different access pattern)

4. **Fix ConfigHash consistency** (no deps):
   - Update github, bitbucket, localgit to use `filter.Repo.Hash()`

5. **Config extensions**:
   - Update `config.go` structs
   - Update `main.go` wiring
   - Add HTTP client timeout in providers

6. **Body size limits**:
   - Add `io.LimitReader` to artifactory, github, bitbucket

7. **Unit tests**:
   - Test critical bugs
   - Test critical paths
   - Run `go test ./...` to verify

8. **Final verification**:
   - `go build ./...`
   - `golangci-lint run`
   - `go test ./...`

---

## 8. Files Summary

| Action | File | Change Type |
|--------|------|-------------|
| Fix bug | `internal/providers/cache/artifactory/artifactory.go:121` | Single line fix |
| Fix bug | `internal/connect/bynames.go:87-91` | Guard + error handling |
| Fix bug | `internal/providers/multisource/repo.go:74-77` | Return nil fix |
| Fix bug | `internal/connect/modulepins.go:30-43` | Return nil fix |
| Fix bug | `internal/connect/bynames.go:52-58` | Return nil fix |
| Delete | `internal/logger/logger.go` | File deletion |
| Create | `internal/providers/content/download.go` | New file (~80 lines) |
| Refactor | `internal/providers/github/getfiles.go` | Use shared helper |
| Refactor | `internal/providers/bitbucket/getfiles.go` | Use shared helper |
| Fix | `internal/providers/github/repos.go:68-69` | Use `r.repo.Hash()` |
| Fix | `internal/providers/bitbucket/repos.go:76-77` | Use `r.repo.Hash()` |
| Fix | `internal/providers/localgit/localgit.go:111-113` | Use `r.repo.Hash()` |
| Update | `cmd/easyp/internal/config/config.go` | Add timeout/bodyLimit fields |
| Update | `cmd/easyp/main.go` | Wire new config fields |
| Update | `internal/providers/github/client.go` | Add HTTP timeout |
| Update | `internal/providers/bitbucket/client.go` | Add HTTP timeout |
| Update | `internal/providers/cache/artifactory/artifactory.go` | Add http.Client + LimitReader |
| Create | `internal/connect/bynames_test.go` | Test splitRepoName |
| Create | `internal/connect/modulepins_test.go` | Test resolveModulePins |
| Create | `internal/providers/multisource/repo_test.go` | Test GetFiles |
| Create | `internal/providers/cache/artifactory/artifactory_test.go` | Test PUT status |
| Create | `internal/providers/filter/filter_test.go` | Test Hash() |

**Total: 7 modified, 1 deleted, 5 new, 5 test files created**

---

## 9. Verification Plan

After implementation:

```bash
# 1. Build — all packages must compile
go build ./...

# 2. Lint — no new warnings
golangci-lint run

# 3. Tests — all must pass
go test ./... -v

# 4. Specific bug verifications:
# - Artifactory PUT: status >= 300 returns error (write a specific test)
# - splitRepoName("no-slash"): no panic, returns ("", "")
# - GetFiles error: returns nil, not partial slice
# - resolveModulePins error: returns nil, not partial slice

# 5. Manual smoke test:
# - Server starts with default config
# - Server starts with custom timeout/bodyLimit config
# - Server handles bad repository names gracefully (no crash)
```

---

## 10. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking cache keys when changing ConfigHash | `filter.Repo.Hash()` is semantically identical to the old implementations. No key change. |
| Refactoring GitHub/BitBucket breaks file downloads | Keep the original `getFiles` logic as a reference. Test incrementally. |
| Test mocking is too simplistic | Start with basic mock implementations; expand as needed. |
| Config field additions break existing configs | All new fields are optional; zero values mean default behavior. |
| Removing logger package breaks something unexpected | Verify with `grep -r "logger" --include="*.go" .` before deletion — package is confirmed unused. |

---

## 11. Not in Scope (Deferred)

Per decisions from discuss-phase:

| Item | Reason | Future Phase |
|------|--------|--------------|
| Parallel file downloads | Too risky for v1.2; sequential is acceptable | Performance phase |
| Response compression | Low priority | Future phase |
| Graceful shutdown | Not needed for dev; production later | Production phase |
| BitBucket template panic | Low risk (static templates) | Future phase |
| Modern buf protocol | Tracked separately in draft.txt | Buf v2 phase |

---

## 12. Dependencies and External Packages

**No new external dependencies required.** All changes use existing imports:

- `io`, `io/fs`, `context`, `fmt`, `strings` — stdlib
- `log/slog` — stdlib (already used)
- `hash/crc32` — stdlib (already used)
- `slices` — stdlib (already used)
- `sync` — stdlib (if needed for concurrent tests)
- `net/http` — stdlib (already used)
- `connectrpc.com/connect` — existing dependency
- `google.golang.org/protobuf` — existing dependency

**Go version:** Project uses Go 1.26+ (from v1.2 milestone). All proposed changes are compatible.

---

*Research completed: 2026-05-09*
