# Coding Conventions

**Analysis Date:** 2026-05-07

## Naming Patterns

**Packages:**
- All lowercase single-word names, no underscores: `connect`, `cache`, `filter`, `source`, `content`, `multisource`, `localgit`, `namedlocks`, `shake256`, `cachetype`
- Sub-packages under `internal/providers/` group by provider type: `github`, `bitbucket`, `localgit`, `cache`, `filter`, `content`, `multisource`, `source`
- Config types live under `cmd/easyp/internal/config/` and `cmd/easyp/internal/config/cachetype/`

**Files:**
- One primary type/concern per file, named after the concern: `api.go`, `blobs.go`, `bynames.go`, `modulepins.go`, `repos.go`, `client.go`, `getfiles.go`, `getrepo.go`
- File names are all lowercase, no underscores (except `_test.go` suffix which is not currently used)
- A `filter.go` file in each provider package holds the `Repo` filtering struct and logic

**Functions:**
- Exported constructor functions use `New` or `NewMultiRepo` pattern: `connect.New()`, `multisource.New()`, `github.NewMultiRepo()`, `bitbucket.NewMultiRepo()`, `localgit.New()`, `namedlocks.New()`, `artifactory.New()`
- Unexported helper functions use camelCase: `getRepo`, `getFiles`, `getFile`, `splitRepoName`, `filterEntries`, `buildQuery`, `tmplBuild`, `tmplExec`
- Exported functions are PascalCase: `Find`, `Check`, `Get`, `Put`, `Hash`, `New`
- One-liner methods on a single line: `func (r sourceRepo) Name() string { return "github proxy" }` (see `internal/providers/github/repos.go:72-75`)

**Variables:**
- Private struct fields are camelCase: `log`, `repo`, `domain`, `basePath`, `byName`
- Sentinel errors use `Err` prefix: `ErrNotFound`, `ErrEmpty`, `ErrUnexpected`, `ErrInvalidType`
- Constants are PascalCase (exported) or camelCase (unexported): `ProtoSuffix`, `digestFormat`, `MaxRedirects`, `minNumberOfRepos`, `connectionTimeout`, `accessCheckPeriod`, `filesListUnlimited`

**Types:**
- Struct types are PascalCase: `Repo`, `Meta`, `File`, `Hash`, `Config`, `Cache`
- Unexported implementation structs are camelCase: `api`, `client`, `store`, `multiRepo`, `sourceRepo`, `namedLocks`, `artifactory`
- Type aliases for domain concepts: `type User string`, `type Password string`, `type Hash [64]byte`, `type Type string`
- Interface names are typically one-word or short: `Source`, `Provider`, `Cache`, `Git`, `Repositories`
- Unexported interfaces for internal decoupling: `provider` in `internal/connect/api.go`, `namedLocks` in `internal/providers/localgit/localgit.go`

## Code Style

**Formatting:**
- Tool: `gofmt` / `goimports` enforced via `golangci-lint`
- Import ordering enforced by `gci` linter (see `.golangci.yml:73-82`):
  1. Standard library (`standard`)
  2. Third-party / default imports (`default`)
  3. Project imports (`prefix(github.com/easyp-tech)`)
- Blank lines separate import groups (standard, third-party, project)

**Linting:**
- Tool: `golangci-lint` with `enable-all: true` (`.golangci.yml`)
- Disabled deprecated linters: `exhaustivestruct`, `ifshort`, `maligned`, `interfacer`, `deadcode`, `golint`, `varcheck`, `structcheck`, `nosnakecase`, `scopelint`, `varnamelen`
- Timeout: 5 minutes
- Tests included in linting (`tests: true`)
- Relaxed rules for test files (`.golangci.yml:59-70`): excludes `gocyclo`, `errcheck`, `dupl`, `gosec`, `gochecknoglobals`, `exhaustruct`, `ireturn`, `funlen`, `unparam`, `lll`
- `depguard` restricts imports: production code allows only `$gostd`, test code additionally allows `github.com/stretchr/testify`, cmd code allows only `$gostd`

**Key lint suppressions (nolint directives used deliberately):**
- `//nolint:exhaustruct` -- widely used for protobuf-generated types and structs with default-zero fields. See `internal/connect/api.go:38`, `internal/providers/localgit/localgit.go:126`, `internal/https/https.go:22`
- `//nolint:ireturn` -- used on functions returning interface types. See `cmd/easyp/main.go:260`, `internal/providers/multisource/repo.go:122`
- `//nolint:wrapcheck` -- used for transparent error passthrough from dependencies. See `internal/https/https.go:41`, `internal/providers/multisource/repo.go:59`
- `//nolint:gochecknoglobals` -- used for legitimate module-level vars (flags, template caches). See `cmd/easyp/main.go:27`, `internal/providers/bitbucket/client.go:61`
- `//nolint:musttag` -- used for JSON encode/decode of internal `[]content.File` types. See `internal/providers/cache/file.go:37`, `internal/providers/cache/artifactory/artifactory.go:84`
- `//nolint:gomnd` -- used for octal permission `0o750`. See `internal/providers/cache/file.go:51`
- `//nolint:lll` -- used for long interface method signatures. See `internal/providers/github/client.go:16`
- `//nolint:nilerr` -- used intentionally in `fs.WalkDir` callback to skip errors on directories. See `internal/providers/localgit/localgit.go:186`

## Import Organization

**Order (enforced by gci):**
1. Standard library: `"context"`, `"fmt"`, `"net/http"`, `"os"`
2. Third-party: `"connectrpc.com/connect"`, `"golang.org/x/exp/slog"`, `"github.com/google/go-github/v59/github"`
3. Project: `"github.com/easyp-tech/server/internal/..."`

**Example from `internal/connect/blobs.go`:**
```go
import (
    "bytes"
    "context"
    "fmt"

    "connectrpc.com/connect"

    module "github.com/easyp-tech/server/gen/proto/buf/alpha/module/v1alpha1"
    registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
    "github.com/easyp-tech/server/internal/shake256"
)
```

**Path Aliases:**
- Protobuf-generated packages use meaningful aliases: `module "github.com/easyp-tech/server/gen/proto/buf/alpha/module/v1alpha1"`, `registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"`
- Connect handler import uses alias: `connect "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect"`

## Error Handling

**Patterns:**
- All errors are wrapped with `fmt.Errorf("context: %w", err)` preserving the error chain
- Error messages are lowercase, descriptive, and include operation context:
  ```go
  return nil, fmt.Errorf("getting repository: %w", err)                          // internal/connect/modulepins.go:23
  return nil, fmt.Errorf("resolving %q/%q:%q: %w", owner, repo, ref, err)        // internal/connect/modulepins.go:48
  return nil, fmt.Errorf("iterating %d of %d: %w", i, len(in), err)              // internal/connect/modulepins.go:36
  ```
- Sentinel errors defined as package-level `var ErrXxx = errors.New("xxx")`:
  - `ErrNotFound` in `internal/providers/multisource/repo.go:39`
  - `ErrEmpty` in `internal/providers/github/getrepo.go:25` and `internal/providers/bitbucket/getrepo.go:25`
  - `ErrUnexpected` in `internal/providers/bitbucket/client.go:33`
  - `ErrInvalidType` in `cmd/easyp/internal/config/cachetype/cachetype.go:16`

**Error passthrough (do not wrap):**
- Functions that delegate to a single dependency use `//nolint:wrapcheck` and return errors directly:
  ```go
  return s.GetMeta(ctx, commit) //nolint:wrapcheck    // internal/providers/multisource/repo.go:59
  return server.ListenAndServeTLS(certFileName, keyFileName) //nolint:wrapcheck  // internal/https/https.go:41
  ```

**Panic usage:**
- `must[T any](v T, err error) T` generic helper panics on error -- used only at startup in `cmd/easyp/main.go:41`
- `panic("unreachable reached")` in default switch case in `cmd/easyp/main.go:346`
- Template execution panics in `internal/providers/bitbucket/client.go:122`

## Logging

**Framework:** `golang.org/x/exp/slog` (structured logging with JSON handler)

**Patterns:**
- Logger is passed as `*slog.Logger` to constructors, not stored globally (except `internal/logger/logger.go` which has a global pattern but is not the primary approach)
- JSON output to stdout: `slog.New(slog.NewJSONHandler(os.Stdout, opts))`
- Structured key-value pairs using `slog.String()`, `slog.Int()`, `slog.Duration()`, `slog.Any()`
- Log level configured via config file, resolved at startup:
  ```go
  opts := &slog.HandlerOptions{Level: logLevel, AddSource: false}
  ```
- Context-aware logging with `log.DebugContext(ctx, ...)` for request tracing
- Sensitive data masking for HTTP headers in debug mode (`cmd/easyp/main.go:218-233`)

**Log level usage:**
- `Debug`: detailed operation info, cache hits, request details, periodic check success
- `Info`: successful connections, cache access check success
- `Warn`: HTTP 4xx responses
- `Error`: failed connections, cache failures, HTTP 5xx responses, shutdown

## Comments

**When to Comment:**
- Function documentation comments only on exported constructors: `// New creates and returns gRPC server.`
- Section separator comments in large files: `// Provider initialization`, `// Cache initialization with connection check`, `// Helper functions`
- Inline comments for security notes: `// Security: Mask sensitive headers`
- `// TODO` comments for unfinished work with `//nolint:godox` suppression

**Doc comments:**
- Minimal; most exported types and functions lack doc comments
- Not a heavily documented codebase; code is expected to be self-documenting

## Function Design

**Size:** Most functions are 5-20 lines. The largest is `loggingMiddleware` in `cmd/easyp/main.go` at ~50 lines. The linter enforces `funlen` for non-test code.

**Parameters:**
- `context.Context` is always the first parameter when needed
- Follow standard Go conventions: `(ctx context.Context, owner, repoName, commit string)`
- Related string parameters are grouped together without individual type repetition

**Return Values:**
- Functions return `(result, error)` consistently
- Named return values are NOT used
- Zero-value initialization of output before error paths:
  ```go
  var out content.Meta
  // ... populate out
  return out, nil
  ```

## Module Design

**Exports:**
- Each package exports a small surface: typically one constructor (`New`), one primary type, and interfaces
- Unexported struct types with exported methods: `api`, `client`, `store`, `multiRepo`, `sourceRepo`
- Packages return concrete types from constructors, not interfaces -- callers consume via interface

**Interface satisfaction assertions:**
- Compile-time checks using blank `var` assignments:
  ```go
  var _ source.Source = sourceRepo{} //nolint:exhaustruct
  ```
  Found in `internal/providers/github/repos.go:61`, `internal/providers/bitbucket/repos.go:69`, `internal/providers/localgit/localgit.go:103`

**Barrel Files:** Not used. Each file is a self-contained unit within its package.

**Generic functions:**
- `must[T any](v T, err error) T` -- generic panic-on-error helper in `cmd/easyp/main.go:252`
- `ReadYaml[T any](fileName string) (T, error)` -- generic YAML config reader in `cmd/easyp/internal/config/read.go:10`
- `httpGetJSON[T any](...)` -- generic JSON HTTP client in `internal/providers/bitbucket/client.go:40`

## Struct Initialization

**Pattern:**
- Use `make()` for slices and maps with capacity hints:
  ```go
  out := make([]*module.ModulePin, 0, len(in))
  repos := make([]source.Source, 0, len(m.repos))
  ```
- Inline struct literals with field names (never positional)
- `//nolint:exhaustruct` when zero-value fields are acceptable (common with protobuf types)

## Configuration

**Pattern:**
- YAML config file parsed into typed Go struct via `ghodss/yaml`
- Environment variable substitution with `os.ExpandEnv()` during config reading (see `cmd/easyp/internal/config/read.go:18`)
- Custom `UnmarshalText` for complex types: `config.URL`, `cachetype.Type`
- Config file path passed via `-cfg` CLI flag with sensible default (`./local.config.yml`)

---

*Convention analysis: 2026-05-07*
