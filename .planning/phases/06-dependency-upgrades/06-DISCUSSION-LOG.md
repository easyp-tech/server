# Phase 6: Dependency Upgrades - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-07
**Phase:** 6-Dependency Upgrades
**Areas discussed:** Upgrade strategies, Docker update, Verification approach

---

## Upgrade Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Go mod tidy last | Update go.mod directly, run go mod tidy, fix any errors. Simple, works for most upgrades. | |
| Incremental upgrades | Update dependencies incrementally in stages. Safer but slower. | ✓ |
| Manual version pinning | Use go-getter or GOPROXY to fetch latest versions directly. | |

**User's choice:** Incremental upgrades
**Notes:** User prefers structure and safety over speed

---

## Incremental Approach

| Option | Description | Selected |
|--------|-------------|----------|
| Big bang | Update all at once, go mod tidy, fix errors. Fast, catches conflicts early. | |
| By layer | Batch by dependency type: Go toolchain first, then Connect, then others. Structured but slower. | ✓ |
| One by one | One dep at a time. Most control but slowest. | |

**User's choice:** By layer
**Notes:** Structured batch approach: Go toolchain → Connect → protobuf → others

---

## Docker Update

| Option | Description | Selected |
|--------|-------------|----------|
| Update Dockerfile | Update Dockerfile to use golang:1.26-alpine. Required for the upgrade. | ✓ |
| Skip Dockerfile | Skip Dockerfile update for now. | |

**User's choice:** Update Dockerfile
**Notes:** Dockerfile must stay in sync with go.mod

---

## Build Verification

| Option | Description | Selected |
|--------|-------------|----------|
| go build ./... | Standard build verification. | |
| go build + smoke test | Build + basic connectivity check. | |
| go build + go test ./... | Full test suite if available. | ✓ |

**User's choice:** go build + go test ./...
**Notes:** Need to confirm both compile and tests pass after upgrades

---

## Deferred Ideas

None — discussion stayed within phase scope.
