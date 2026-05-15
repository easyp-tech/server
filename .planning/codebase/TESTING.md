# Testing Patterns

**Analysis Date:** 2026-05-07

## Test Framework

**Runner:**
- Go standard `testing` package (no custom test runner configuration)
- Config: No `Makefile` test targets, no CI test step in `.github/workflows/easyp_build.yml`

**Assertion Library:**
- `github.com/stretchr/testify v1.8.4` listed as a dependency in `go.mod` (indirect)
- Not currently imported by any source or test file in the project's own code
- Allowed by `depguard` linter config for test files (`.golangci.yml:96-100`)

**Run Commands:**
```bash
go test ./...                     # Run all tests
go test ./internal/...            # Run internal package tests
go test -v ./...                  # Verbose output
go test -race ./...               # With race detector
go test -cover ./...              # With coverage
```

## Current Test Status

**No test files exist in the project's own code.** A search for `*_test.go` files in `internal/`, `cmd/`, and project root returns zero results. The only test files in the repository are under `api/_third_party/buf-v1.69.0/` which is a vendored third-party tool, not part of this project.

**This means:**
- Zero test coverage across all packages
- No test infrastructure, fixtures, or helpers have been established
- No mocking patterns have been established
- The `depguard` linter config explicitly permits `stretchr/testify` in test files, indicating tests are intended but not yet written

## Test File Organization

**Expected Location (based on Go conventions and linter config):**
- Co-located with source files: `internal/connect/api_test.go`, `internal/providers/filter/filter_test.go`
- Test files match the pattern `*_test.go` (recognized by `.golangci.yml:59`)

**Expected Naming:**
- Test files: `<package>_test.go` or `<file>_test.go`
- Test functions: `func TestXxx(t *testing.T)`
- Benchmark functions: `func BenchmarkXxx(b *testing.B)`

**Expected Structure:**
```
internal/
  connect/
    api.go
    api_test.go          # Tests for connect API handlers
  providers/
    filter/
      filter.go
      filter_test.go     # Tests for filter logic
    multisource/
      repo.go
      repo_test.go       # Tests for multisource routing and caching
    github/
      repos.go
      repos_test.go      # Tests for GitHub provider
    bitbucket/
      repos.go
      repos_test.go      # Tests for BitBucket provider
    localgit/
      localgit.go
      localgit_test.go   # Tests for local git provider
    cache/
      file.go
      file_test.go       # Tests for local file cache
      noop.go
      artifactory/
        artifactory.go
        artifactory_test.go  # Tests for Artifactory cache
    content/
      repo.go
  shake256/
    hash.go
    hash_test.go         # Tests for SHAKE256 hashing
  https/
    https.go
    https_test.go        # Tests for HTTPS server
```

## Recommended Test Structure

**Suite Organization (recommended Go pattern):**
```go
package filter

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestCheck_ProtoSuffix(t *testing.T) {
    repo := Repo{Owner: "googleapis", Name: "googleapis"}

    path, ok := repo.Check("google/type/date.proto")
    assert.True(t, ok)
    assert.Equal(t, "google/type/date.proto", path)
}

func TestCheck_NonProtoRejected(t *testing.T) {
    repo := Repo{Owner: "googleapis", Name: "googleapis"}

    _, ok := repo.Check("google/type/readme.md")
    assert.False(t, ok)
}
```

**Patterns:**
- Use `require` for setup assertions that must pass (fail fast): `require.NoError(t, err)`
- Use `assert` for in-test assertions that should report failure: `assert.Equal(t, expected, actual)`
- Table-driven tests for parametric test cases:
  ```go
  func TestFilterEntries(t *testing.T) {
      tests := []struct {
          name     string
          input    string
          expected bool
      }{
          {"proto file", "foo/bar.proto", true},
          {"json file", "foo/bar.json", false},
      }
      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              // test body
          })
      }
  }
  ```

## Mocking

**Framework:** No mocking framework established. `stretchr/testify` is available.

**Recommended approach based on codebase architecture:**

The codebase uses Go interfaces extensively for decoupling, making mocking straightforward:

1. **Interface-based mocking** -- define mock implementations of existing interfaces:

   ```go
   // Mock for source.Source interface (internal/providers/source/source.go)
   type mockSource struct {
       getMetaFunc  func(ctx context.Context, commit string) (content.Meta, error)
       getFilesFunc func(ctx context.Context, commit string) ([]content.File, error)
   }

   func (m *mockSource) GetMeta(ctx context.Context, commit string) (content.Meta, error) {
       return m.getMetaFunc(ctx, commit)
   }

   func (m *mockSource) GetFiles(ctx context.Context, commit string) ([]content.File, error) {
       return m.getFilesFunc(ctx, commit)
   }

   func (m *mockSource) ConfigHash() string { return "mock-hash" }
   func (m *mockSource) Name() string       { return "mock" }
   func (m *mockSource) Owner() string      { return "mock-owner" }
   func (m *mockSource) RepoName() string   { return "mock-repo" }
   func (m *mockSource) Type() string       { return "mock" }
   ```

2. **Key interfaces to mock:**
   - `source.Source` (`internal/providers/source/source.go:9-17`) -- core provider interface
   - `multisource.Provider` (`internal/providers/multisource/repo.go:14-17`) -- provider finder
   - `multisource.Cache` (`internal/providers/multisource/repo.go:19-23`) -- cache operations
   - `github.Repositories` (`internal/providers/github/client.go:17-22`) -- GitHub API
   - `github.Git` (`internal/providers/github/client.go:25-27`) -- GitHub Git API
   - `connect.provider` (`internal/connect/api.go:13-16`) -- internal provider interface

**What to Mock:**
- External HTTP services (GitHub API, BitBucket API, Artifactory)
- File system operations (local git, local cache)
- Time-dependent operations

**What NOT to Mock:**
- `filter.Repo.Check()` -- pure logic, test directly
- `shake256.SHA3Shake256()` -- deterministic hashing, test directly
- `splitRepoName()` -- pure string parsing, test directly
- Config parsing (`config.ReadYaml`) -- use real config files from `testdata/`

## Fixtures and Factories

**Test Data:**
- TLS test fixtures exist in `testdata/`: `cert.pem`, `key.pem`
- Config fixture exists: `local.config.yml` (root directory)
- No test-specific data factories exist yet

**Recommended Location:**
- `testdata/` directory at package level (already exists at repo root)
- For package-specific test data: `internal/providers/github/testdata/`
- For shared test helpers: `internal/testutil/` or `internal/testing/`

**Recommended Factory Pattern:**
```go
// internal/testutil/content.go
package testutil

import "github.com/easyp-tech/server/internal/providers/content"

func MustNewFile(path string, data string) content.File {
    hash, err := shake256.SHA3Shake256([]byte(data))
    if err != nil {
        panic(err)
    }
    return content.File{Path: path, Data: []byte(data), Hash: hash}
}
```

## Coverage

**Requirements:** None enforced. No CI step runs tests.

**View Coverage:**
```bash
go test -cover ./...                         # Per-package coverage
go test -coverprofile=coverage.out ./...     # Generate coverage profile
go tool cover -html=coverage.out             # View in browser
```

## Test Types

**Unit Tests:**
- Primary testing approach for this codebase given the pure-logic functions
- High-priority targets for unit testing:
  - `internal/providers/filter/filter.go` -- `Check()`, `FindRepo()`, `checkPrefix()`, `checkPath()` -- pure functions with no dependencies
  - `internal/shake256/hash.go` -- `SHA3Shake256()`, `Hash.String()`, `MarshalText()`, `UnmarshalText()` -- deterministic, no I/O
  - `internal/connect/bynames.go` -- `splitRepoName()` -- pure string parsing
  - `cmd/easyp/internal/config/cachetype/cachetype.go` -- `UnmarshalText()` -- pure validation
  - `cmd/easyp/internal/config/url.go` -- `UnmarshalText()` -- pure parsing
  - `cmd/easyp/main.go` -- `isSensitiveHeader()`, `maskSensitiveHeaders()`, `getClientIP()` -- pure logic
  - `internal/providers/cache/file.go` -- `Get()`, `Put()` -- file I/O, use temp directories
  - `internal/providers/cache/noop.go` -- `Get()`, `Put()`, `CheckWriteAccess()` -- trivially testable

**Integration Tests:**
- Would test the full HTTP request path through the connect handler
- Require running HTTP server with mock providers
- Use `httptest.NewServer()` for testing the HTTP layer
- Test the connect RPC handlers via generated client stubs

**E2E Tests:**
- Not currently applicable
- Would require a real git repository and network access

## Linter Config for Tests

The `.golangci.yml` config explicitly relaxes these linters for `*_test.go` files:
- `gocyclo` -- test functions may have higher complexity
- `errcheck` -- unchecked errors are acceptable in tests
- `dupl` -- test code duplication is tolerated
- `gosec` -- security checks relaxed for test code
- `gochecknoglobals` -- global test fixtures are acceptable
- `exhaustruct` -- partial struct initialization in tests is fine
- `ireturn` -- returning interfaces from test helpers is fine
- `funlen` -- long test functions are acceptable
- `unparam` -- unused parameters in test helpers are tolerated
- `lll` -- long lines in tests are acceptable

## Recommended Priority Test Cases

**Priority 1 -- Pure logic, no mocks needed:**
| File | Function | Why |
|------|----------|-----|
| `internal/providers/filter/filter.go` | `Check()`, `FindRepo()` | Core filtering logic |
| `internal/shake256/hash.go` | `SHA3Shake256()`, `MarshalText()` | Hash correctness |
| `cmd/easyp/internal/config/cachetype/cachetype.go` | `UnmarshalText()` | Type validation |
| `cmd/easyp/main.go` | `isSensitiveHeader()` | Security-sensitive |

**Priority 2 -- Simple mocking:**
| File | Function | Why |
|------|----------|-----|
| `internal/providers/multisource/repo.go` | `GetMeta()`, `GetFiles()` | Core routing + caching |
| `internal/connect/modulepins.go` | `resolveModulePin()` | RPC response building |
| `internal/connect/bynames.go` | `resolveRepoByFullName()` | RPC response building |

**Priority 3 -- Integration:**
| File | Function | Why |
|------|----------|-----|
| `internal/connect/api.go` | `New()` | Full handler setup |
| `internal/providers/cache/file.go` | `Get()`, `Put()` | File system caching |

---

*Testing analysis: 2026-05-07*
