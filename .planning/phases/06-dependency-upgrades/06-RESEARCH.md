# Phase 6: Dependency Upgrades — Research Summary

**Researched:** 2026-05-07
**Status:** Complete — ready for planning

---

## Current Versions vs. Target Versions

| Dependency | Current | Target | Notes |
|------------|---------|--------|-------|
| Go toolchain | 1.22 | 1.26 | Dockerfile + go.mod both update |
| connectrpc.com/connect | v1.18.1 | v1.19.2 | Requires Go 1.24+ — Go upgrade must happen first |
| golangci-lint | (implicit via .golangci.yml) | v2.12.2 | Must support Go 1.26; confirmed v2.12.2 is latest |
| golang.org/x/crypto | v0.23.0 | v0.50.0 | Compatible with Go 1.26 |
| golang.org/x/exp | v0.0.0-20231006140011-7918f672742d | v0.0.0-20260410095643-746e56fc9e2f | Compatible; consider migrating to stdlib `log/slog` later |
| google.golang.org/protobuf | v1.34.2 | v1.36.11 | Compatible with connect-go v1.19.x |
| google.golang.org/grpc | (not in go.mod direct) | (let tidy update) | Only used for code-gen stubs; let go mod resolve |
| github.com/google/go-github/v59 | v59.0.0 | v59.0.0 | No newer v59; v60+ is a separate module |
| github.com/go-git/go-git/v5 | v5.9.0 | v5.19.0 | Compatible with Go 1.26 |
| github.com/stretchr/testify | v1.8.4 | v1.11.1 | Note: testify README says "superseded" — still functional |
| github.com/ghodss/yaml | v1.0.0 | v1.0.0 | Stable, no update needed |

**Resolved via Go Proxy:**
- `connectrpc.com/connect`: v1.19.2 (latest)
- `golangci-lint`: v2.12.2 (latest)
- `golang.org/x/crypto`: v0.50.0
- `golang.org/x/exp`: v0.0.0-20260410095643-746e56fc9e2f
- `google.golang.org/protobuf`: v1.36.11
- `google.golang.org/grpc`: v1.81.0
- `github.com/go-git/go-git/v5`: v5.19.0
- `github.com/stretchr/testify`: v1.11.1

---

## Compatibility Considerations

### connect-go v1.19.x requires Go 1.24+

**Critical ordering constraint:** The go.mod `go` directive must be updated to 1.26 **before** upgrading connect-go, because connect-go v1.19.2's go.mod declares `go 1.24.0`. Go 1.26 satisfies this requirement.

If you upgrade connect-go before updating the `go` directive, `go mod tidy` will fail.

### Generated code API changes in v1.19.x: "simple" flag

The major enhancement in v1.19.0 is the **`simple` flag** for `protoc-gen-connect-go`. This flag produces cleaner generated code — metadata (headers/trailers) pass through context instead of explicit wrapper types. However:

- The flag is **opt-in** per plugin invocation
- Your existing `buf.gen.yaml` does not use `opt: simple`, so generated code will be unchanged
- The handler and client interfaces remain the same when not using `simple`
- **Verdict:** No breaking API changes in generated code for this project

### Bugfixes in v1.19.x vs v1.18.1

- **v1.19.2**: Fix nil pointer deref in duplexHTTPCall under concurrent Send+CloseAndReceive; use 'deadline_exceeded' instead of 'canceled' on HTTP/2 cancelation
- **v1.19.1**: Bugfix release
- **v1.19.0**: "simple" flag, Go 1.24 requirement, drop `golang.org/x/net/http2` dependency, Edition 2024 support

These are all additive or bugfix — no breaking changes to the runtime APIs you use.

### go-github v59 stays at v59.0.0

There is no newer v59 release. `go-github/v60` is a separate module (different import path). The project uses `github.com/google/go-github/v59` and there is no compelling reason to migrate to v60 in this milestone.

### testify v1.11.1 deprecation notice

The testify README indicates the library is "superseded" but it remains functional and widely used. The upgrade to v1.11.1 is safe for this milestone.

---

## Upgrade Strategy Recommendations

### Recommended Order

1. **Update Dockerfile first** — change `FROM golang:1.22-alpine` to `FROM golang:1.26-alpine`
2. **Update go.mod go directive** — change `go 1.22` to `go 1.26`
3. **Upgrade connect-go** — `go get connectrpc.com/connect@v1.19.2`
4. **Upgrade all direct dependencies** — use `go get` for each
5. **Run `go mod tidy`** — let Go resolve all transitive dependencies
6. **Run `go build ./...`** — verify compilation
7. **Run `go test ./...`** — verify tests pass

### Single-pass approach (simpler)

Update go.mod go directive, then run:
```bash
go get connectrpc.com/connect@v1.19.2 \
  google.golang.org/protobuf@v1.36.11 \
  golang.org/x/crypto@v0.50.0 \
  golang.org/x/exp@v0.0.0-20260410095643-746e56fc9e2f \
  google.golang.org/grpc@v1.81.0 \
  github.com/go-git/go-git/v5@v5.19.0 \
  github.com/stretchr/testify@v1.11.1
go mod tidy
```

Both approaches produce the same result. The single-pass is simpler.

---

## Known Pitfalls and Risks

### Pitfall 1: Wrong upgrade order blocks connect-go

If you run `go get connectrpc.com/connect@v1.19.2` with `go 1.22` still declared, `go mod tidy` will fail with:
```
connectrpc.com/connect@v1.19.2 requires go >= 1.24
```
**Fix:** Always update the `go` directive first.

### Pitfall 2: golangci-lint v2.12.2 is a CLI tool, not a Go module

golangci-lint v2.12.2 is installed separately (e.g., `brew install golangci-lint` or `go install`). It is NOT a `go get` dependency. The `.golangci.yml` configures which linters to run, but the golangci-lint binary itself must be updated independently:

```bash
brew upgrade golangci-lint
# or
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.12.2
```

The project does not declare golangci-lint as a go.mod dependency (it uses the local binary approach).

### Pitfall 3: golangci-lint v2.12.2 may flag new issues

Upgrading golangci-lint may surface new lint warnings in existing code (new linter rules, stricter checks). These should be addressed, not suppressed. However, Phase 6 success criteria only require `go build ./...` and `go test ./...` — not `golangci-lint` passing. Phase 7 (proto regeneration) may need lint fixes.

### Pitfall 4: go.sum must stay in sync

Both `go.mod` and `go.sum` must be committed together. Never commit one without the other.

---

## Specific Commands for the Upgrade

### Step 1: Update go.mod go directive

```bash
sed -i '' 's/^go 1\.22$/go 1.26/' go.mod
```

### Step 2: Update all dependencies

```bash
go get connectrpc.com/connect@v1.19.2
go get google.golang.org/protobuf@v1.36.11
go get golang.org/x/crypto@v0.50.0
go get golang.org/x/exp@v0.0.0-20260410095643-746e56fc9e2f
go get google.golang.org/grpc@v1.81.0
go get github.com/go-git/go-git/v5@v5.19.0
go get github.com/stretchr/testify@v1.11.1
```

### Step 3: Tidy and verify

```bash
go mod tidy
go build ./...
go test ./...
```

### Step 4: Update Dockerfile

Change line 1 from:
```dockerfile
FROM golang:1.22-alpine AS builder
```
to:
```dockerfile
FROM golang:1.26-alpine AS builder
```

### Step 5: Update golangci-lint (optional but recommended)

```bash
# macOS
brew upgrade golangci-lint

# Linux/other
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.12.2
```

### Verification commands

```bash
go version                    # Should show go1.26
grep "^go " go.mod            # Should show "go 1.26"
grep "connectrpc.com/connect" go.mod  # Should show v1.19.2
go build ./...
go test ./...
docker build -t easyp-test .  # Verify Dockerfile still builds
```

---

## connect-go v1.18.1 → v1.19.x Breaking Changes Analysis

**Verdict: No breaking runtime API changes.**

The v1.19.0 release notes describe:
1. **`simple` flag** (opt-in, not enabled by default) — your `buf.gen.yaml` does not use it
2. **Go version requirement** — Go 1.24 minimum; Go 1.26 satisfies this
3. **Dropped `golang.org/x/net/http2`** — now uses stdlib `http.Protocol` — this is internal
4. **Edition 2024 support** — proto compiler feature, not runtime
5. **Bugfixes** (nil pointer, HTTP/2 cancelation) — fixes, not breaking changes

The generated connect code (`*_connect.go` files) will reference `connect.IsAtLeastVersion1_20_0` or similar (version bump in the constant), but the runtime APIs (`connect.NewUnaryHandler`, `connect.Request[T]`, `connect.NewClient[T]`) are unchanged.

**Handler struct compatibility:** The handler structs in `internal/connect/` embed `connect.UnimplementedHandler` types. These are compatible between v1.18.1 and v1.19.x — no structural changes to the interface.

---

## golangci-lint Version for Go 1.26

**Target:** v2.12.2 (latest as of 2026-05-07)

golangci-lint uses semantic versioning and releases versions that support newer Go versions. v2.12.2 is the latest release and supports Go 1.26.

**Update command:**
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.12.2
```

The project's `.golangci.yml` enables all linters and has reasonable exclusion rules. No config changes should be needed for Go 1.26 compatibility.

---

## go-github/v59 vs v60 Decision

**Decision: Stay on v59.0.0**

- v60 is a separate module (`github.com/google/go-github/v60`) with a different import path
- Migrating requires changing all import statements from `v59` to `v60`
- The v59 API is functional and covers all GitHub API calls this project needs
- No security or compatibility reason to upgrade in this milestone

---

## Summary

| Question | Answer |
|----------|--------|
| Latest compatible versions? | connect-go v1.19.2, protobuf v1.36.11, grpc v1.81.0, crypto v0.50.0, go-git v5.19.0, testify v1.11.1 |
| connect-go v1.19.x + Go 1.26 compatibility issues? | None. Go 1.26 satisfies v1.19.2's Go 1.24+ requirement. |
| Upgrade order? | (1) Update go.mod go directive to 1.26, (2) upgrade all deps, (3) go mod tidy, (4) build/test |
| Breaking changes in connect-go v1.19.x? | None for this project. `simple` flag is opt-in. |
| golangci-lint version for Go 1.26? | v2.12.2 (separate CLI tool, not go get) |
| API changes in generated code? | No. Generated interfaces unchanged unless `simple` flag is enabled (it is not). |

---

*Sources verified via: Go Proxy (`proxy.golang.org`), GitHub Releases (connectrpc/connect-go, golangci/golangci-lint), project go.mod/go.sum analysis*