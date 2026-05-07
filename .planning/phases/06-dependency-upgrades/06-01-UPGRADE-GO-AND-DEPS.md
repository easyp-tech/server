# Plan 06-01: Update go.mod to Go 1.26 and upgrade all dependencies

**Phase:** 6 — Dependency Upgrades
**Goal:** Go 1.26, connect-go v1.19.x, and all dependencies updated to latest compatible versions with a clean `go mod tidy`
**Requirements addressed:** DEPS-01, DEPS-02, DEPS-03
**Depends on:** Nothing
**Wave:** 1 (sequential — ordering matters)

---

## Task 06-01.1: Update go directive in go.mod from 1.22 to 1.26

<read_first>
- `./go.mod` — verify current `go 1.22` directive on line 3
</read_first>

<acceptance_criteria>
- `grep "^go " go.mod` returns `go 1.26`
- `go version` reports `go1.26` or higher (run with the new toolchain)
</acceptance_criteria>

<action>
Edit line 3 of `go.mod`:
```
- go 1.22
+ go 1.26
```
Command:
```bash
sed -i '' 's/^go 1\.22$/go 1.26/' go.mod
# verify
grep "^go " go.mod
```
</action>

---

## Task 06-01.2: Upgrade connectrpc.com/connect from v1.18.1 to v1.19.2

<read_first>
- `./go.mod` — verify current `connectrpc.com/connect v1.18.1` in require block
- `.planning/phases/06-dependency-upgrades/06-RESEARCH.md` — confirm v1.19.2 requires Go 1.24+
</read_first>

<acceptance_criteria>
- `grep "connectrpc.com/connect" go.mod` returns `connectrpc.com/connect v1.19.2`
- No `go mod tidy` or `go get` error about Go version incompatibility
</acceptance_criteria>

<action>
Must run after Task 06-01.1 (go directive must be 1.26 before connect-go can upgrade).
```bash
go get connectrpc.com/connect@v1.19.2
grep "connectrpc.com/connect" go.mod
```
</action>

---

## Task 06-01.3: Upgrade google.golang.org/protobuf to v1.36.11

<read_first>
- `./go.mod` — verify current `google.golang.org/protobuf v1.34.2`
</read_first>

<acceptance_criteria>
- `grep "google.golang.org/protobuf" go.mod` returns `v1.36.11`
</acceptance_criteria>

<action>
```bash
go get google.golang.org/protobuf@v1.36.11
grep "google.golang.org/protobuf" go.mod
```
</action>

---

## Task 06-01.4: Upgrade golang.org/x/crypto to v0.50.0

<read_first>
- `./go.mod` — verify current `golang.org/x/crypto v0.23.0`
</read_first>

<acceptance_criteria>
- `grep "golang.org/x/crypto" go.mod` returns `v0.50.0`
</acceptance_criteria>

<action>
```bash
go get golang.org/x/crypto@v0.50.0
grep "golang.org/x/crypto" go.mod
```
</action>

---

## Task 06-01.5: Upgrade golang.org/x/exp to v0.0.0-20260410095643-746e56fc9e2f

<read_first>
- `./go.mod` — verify current `golang.org/x/exp v0.0.0-20231006140011-7918f672742d`
</read_first>

<acceptance_criteria>
- `grep "golang.org/x/exp" go.mod` returns `v0.0.0-20260410095643-746e56fc9e2f`
</acceptance_criteria>

<action>
```bash
go get golang.org/x/exp@v0.0.0-20260410095643-746e56fc9e2f
grep "golang.org/x/exp" go.mod
```
</action>

---

## Task 06-01.6: Upgrade github.com/go-git/go-git/v5 to v5.19.0

<read_first>
- `./go.mod` — verify current `github.com/go-git/go-git/v5 v5.9.0`
</read_first>

<acceptance_criteria>
- `grep "github.com/go-git/go-git/v5" go.mod` returns `v5.19.0`
</acceptance_criteria>

<action>
```bash
go get github.com/go-git/go-git/v5@v5.19.0
grep "github.com/go-git/go-git/v5" go.mod
```
</action>

---

## Task 06-01.7: Upgrade github.com/stretchr/testify to v1.11.1

<read_first>
- `./go.mod` — verify current `github.com/stretchr/testify v1.8.4`
</read_first>

<acceptance_criteria>
- `grep "github.com/stretchr/testify" go.mod` returns `v1.11.1`
</acceptance_criteria>

<action>
```bash
go get github.com/stretchr/testify@v1.11.1
grep "github.com/stretchr/testify" go.mod
```
</action>

---

## Task 06-01.8: Upgrade google.golang.org/grpc to v1.81.0 (let tidy handle if not direct)

<read_first>
- `./go.mod` — check if `google.golang.org/grpc` appears as a direct require (currently not present as direct)
</read_first>

<acceptance_criteria>
- After `go mod tidy` (in Plan 06-02), `grep "google.golang.org/grpc" go.mod` returns `v1.81.0`
- If upgrade is needed as a direct require, it must be at v1.81.0
</acceptance_criteria>

<action>
```bash
# Upgrade grpc as a direct require to lock in the target version
go get google.golang.org/grpc@v1.81.0
grep "google.golang.org/grpc" go.mod
```
</action>

---

## Task 06-01.9: Update Dockerfile base image from golang:1.22-alpine to golang:1.26-alpine

<read_first>
- `./Dockerfile` — verify current `FROM golang:1.22-alpine AS builder` on line 1
</read_first>

<acceptance_criteria>
- `grep "^FROM golang:" Dockerfile` returns `FROM golang:1.26-alpine AS builder`
- No other `golang:` references with a version older than 1.26
</acceptance_criteria>

<action>
Edit line 1 of `Dockerfile`:
```
- FROM golang:1.22-alpine AS builder
+ FROM golang:1.26-alpine AS builder
```
Command:
```bash
sed -i '' 's/^FROM golang:1\.22-alpine AS builder$/FROM golang:1.26-alpine AS builder/' Dockerfile
grep "^FROM golang:" Dockerfile
```
</action>

---

## Task 06-01.10: Upgrade golangci-lint to v2.12.2

<read_first>
- `./.golangci.yml` — verify lint config is present
- Check current golangci-lint version: `golangci-lint version`
</read_first>

<acceptance_criteria>
- `golangci-lint version` returns a version that supports Go 1.26 (v2.x, specifically v2.12.2 target)
- `golangci-lint --version` exits successfully
</acceptance_criteria>

<action>
golangci-lint is a CLI tool installed separately from go.mod dependencies. Update the binary:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.12.2
# or on macOS:
# brew upgrade golangci-lint
golangci-lint version
```
Note: golangci-lint is NOT a go.mod dependency — it is a standalone binary. No go.mod/go.sum changes.
</action>

---

## Exit Criteria for Plan 06-01

All tasks complete when:
1. `go.mod` declares `go 1.26`
2. `go.mod` has `connectrpc.com/connect v1.19.2`
3. `go.mod` has `google.golang.org/protobuf v1.36.11`
4. `go.mod` has `golang.org/x/crypto v0.50.0`
5. `go.mod` has `golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f`
6. `go.mod` has `github.com/go-git/go-git/v5 v5.19.0`
7. `go.mod` has `github.com/stretchr/testify v1.11.1`
8. `go.mod` has `google.golang.org/grpc v1.81.0` (direct or via tidy)
9. `Dockerfile` uses `golang:1.26-alpine`
10. `golangci-lint v2.12.2` is installed

Then proceed to **Plan 06-02**.

---

*Plan 06-01 created: 2026-05-07*
