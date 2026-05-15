# Plan 06-02: Run `go mod tidy` and verify `go build ./...` passes

**Phase:** 6 — Dependency Upgrades
**Goal:** Go 1.26, connect-go v1.19.x, and all dependencies updated to latest compatible versions with a clean `go mod tidy`
**Requirements addressed:** DEPS-01, DEPS-02, DEPS-03, DEPS-04
**Depends on:** Plan 06-01 (all tasks complete)
**Wave:** 1 (sequential after 06-01)

---

## Pre-flight: Verify preconditions from Plan 06-01

<read_first>
- `./go.mod` — verify `go 1.26`, all target versions are present
- `./Dockerfile` — verify `golang:1.26-alpine`
</read_first>

<acceptance_criteria>
- `grep "^go " go.mod` returns `go 1.26`
- `grep "connectrpc.com/connect" go.mod` contains `v1.19.2`
- `grep "^FROM golang:" Dockerfile` returns `golang:1.26-alpine`
- `go.mod` has no entries with version older than declared targets
</acceptance_criteria>

<action>
```bash
grep "^go " go.mod
grep "connectrpc.com/connect" go.mod
grep "^FROM golang:" Dockerfile
```
If any preconditions fail, return to Plan 06-01 and fix the missing upgrade first.
</action>

---

## Task 06-02.1: Run `go mod tidy`

<read_first>
- `./go.mod` — current state after Plan 06-01
- `./go.sum` — current state after Plan 06-01
</read_first>

<acceptance_criteria>
- `go mod tidy` exits with code 0 (no errors)
- `go.mod` and `go.sum` are updated with all transitive dependencies
- No "unused" or "missing" dependency warnings
- `go mod tidy` output contains no error lines
</acceptance_criteria>

<action>
```bash
go mod tidy
echo "Exit code: $?"
# Verify clean output — look for errors
go mod tidy 2>&1 | grep -i "error\|invalid\|conflict\|requires\|mismatch" || echo "No errors found"
```
Note: `go mod tidy` may upgrade or pin additional transitive dependencies. This is expected and correct — let tidy resolve to highest compatible versions.
</action>

---

## Task 06-02.2: Verify `go build ./...` passes

<read_first>
- `./go.mod` — confirm go directive is `1.26`
- `./cmd/easyp/main.go` — entry point
- `./internal/connect/` — Connect handler implementations
</read_first>

<acceptance_criteria>
- `go build ./...` exits with code 0
- No compile errors, linker errors, or type errors in output
- All packages in `./cmd/`, `./internal/`, `./pkg/` build successfully
</acceptance_criteria>

<action>
```bash
go build ./...
echo "Build exit code: $?"
```
</action>

---

## Task 06-02.3: Verify `go test ./...` passes

<read_first>
- `./` — all `*_test.go` files
- `.planning/phases/03-test-infrastructure/03-CONTEXT.md` — test patterns used
</read_first>

<acceptance_criteria>
- `go test ./...` exits with code 0
- All tests pass (no FAIL output)
- No test compilation errors
- `go test ./...` output shows `ok` for all packages
</acceptance_criteria>

<action>
```bash
go test ./...
echo "Test exit code: $?"
```
</action>

---

## Task 06-02.4: Verify Dockerfile still builds

<read_first>
- `./Dockerfile` — confirm `golang:1.26-alpine` on line 1
- `./go.mod` — confirm `go 1.26`
- `./go.sum` — present and populated by tidy
</read_first>

<acceptance_criteria>
- `docker build -t easyp-build-test .` exits with code 0
- Docker image tag `easyp-build-test` is created
- No Docker build errors in output
</acceptance_criteria>

<action>
```bash
docker build -t easyp-build-test .
echo "Docker build exit code: $?"
docker rmi easyp-build-test 2>/dev/null || true
```
Note: This validates that Dockerfile and go.mod/go.sum are in sync. If Docker build fails, check that go.mod and go.sum are both committed.
</action>

---

## Task 06-02.5: Verify final go.mod state

<read_first>
- `./go.mod` — final state
- `./go.sum` — final state (must be committed alongside go.mod)
</read_first>

<acceptance_criteria>
- `go.mod` declares `go 1.26`
- `go.mod` contains all direct dependencies at target versions:
  - `connectrpc.com/connect v1.19.2`
  - `google.golang.org/protobuf v1.36.11`
  - `golang.org/x/crypto v0.50.0`
  - `golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f`
  - `github.com/go-git/go-git/v5 v5.19.0`
  - `github.com/stretchr/testify v1.11.1`
  - `google.golang.org/grpc v1.81.0`
- `go.mod` has no indirect dependencies with duplicate/conflicting versions
- `go mod verify` passes (verifies go.sum integrity)
</acceptance_criteria>

<action>
```bash
echo "=== go.mod directive ===" && grep "^go " go.mod
echo "=== Direct dependencies ===" && grep -A 20 "^require (" go.mod | head -20
echo "=== go mod verify ===" && go mod verify
echo "=== go list -m all (first 20) ===" && go list -m all | head -20
```
</action>

---

## Exit Criteria for Plan 06-02

All tasks complete when:
1. `go mod tidy` exits with code 0 — no errors, no conflicts
2. `go build ./...` exits with code 0 — all packages compile
3. `go test ./...` exits with code 0 — all tests pass
4. `docker build -t easyp-build-test .` exits with code 0 — Dockerfile syncs with go.mod
5. `go mod verify` passes — go.sum integrity confirmed
6. `go.mod` declares `go 1.26` and all target dependency versions are present

**Phase 6 is complete when all Plan 06-02 exit criteria are met.**

---

## Phase 6 Completion Summary

After both plans:

| Requirement | Plan.Task | Verification |
|-------------|-----------|-------------|
| DEPS-01: Build passes with Go 1.26 | 06-02.2 | `go build ./...` exits 0 |
| DEPS-02: connect-go upgraded to v1.19.x | 06-01.2 | `grep connectrpc.com/connect go.mod` → v1.19.2 |
| DEPS-03: All other deps updated to latest | 06-01.3–08, 06-02.5 | All target versions present in go.mod |
| DEPS-04: `go mod tidy` clean | 06-02.1 | `go mod tidy` exits 0, no conflicts |

---

*Plan 06-02 created: 2026-05-07*
