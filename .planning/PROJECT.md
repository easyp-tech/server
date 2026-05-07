# EasyP Buf Proxy — Protocol Modernization

## What This Is

A Go-based proxy server that translates Buf CLI registry requests into VCS API calls (GitHub, BitBucket, local git). The server currently implements the deprecated Buf `registry.v1alpha1` protocol (last compatible version: buf v1.30.1) via Connect RPC. We are adding support for the modern Buf protocol (v1.69.0+) while keeping the old protocol active for backward compatibility.

## Core Value

The proxy must correctly serve both old (v1.30.1) and modern (v1.69.0+) Buf CLI clients simultaneously, so that existing users are not broken during migration.

## Requirements

### Validated

- ✓ Buf registry proxy for deprecated `registry.v1alpha1` protocol — existing
- ✓ Multi-provider architecture (local git, GitHub, BitBucket) — existing
- ✓ Cache layer (noop, local filesystem, Artifactory) — existing
- ✓ TLS with optional mTLS support — existing
- ✓ Structured logging with sensitive header masking — existing

### Active

- [ ] Test suite verifying the server works correctly with buf v1.30.1 (old protocol) using real `buf` binary + TLS server + real GitHub API
- [ ] Modern Buf protocol (v1.69.0) implemented alongside the existing deprecated protocol — both served simultaneously
- [ ] Test suite verifying the server works correctly with buf v1.69.0+ (modern protocol) using real `buf` binary + TLS server + real GitHub API

### Out of Scope

- BitBucket provider testing — GitHub provider is sufficient for validation
- Local git provider testing — not needed for protocol validation
- Removing the old v1alpha1 protocol — both protocols must coexist
- Artifactory cache testing — not relevant to protocol correctness
- UI changes — this is a backend-only project

## Context

- The existing codebase uses Connect RPC (`connectrpc.com/connect` v1.11.1) to implement Buf's `registry.v1alpha1` gRPC-compatible services
- Modern Buf proto definitions are already available at `api/_third_party/buf-v1.69.0/proto/buf/` (git submodule)
- The old proto definitions are at `api/_third_party/buf/` — these generated the current `gen/proto/` code
- Code generation is done via `buf generate` configured in `api/proto/buf.gen.yaml` using go, go-grpc, and connect-go plugins
- TLS certs for local testing are at `~/local-tls/server/` (self-signed, added to local CA)
- The server is stateless — no database, relies on external VCS APIs and optional caching
- Go version is 1.22

## Constraints

- **Tech Stack**: Go 1.22, Connect RPC, protobuf — must stay within existing stack
- **Protocol Compatibility**: Old protocol must continue working unchanged while new protocol is added
- **Proto Definitions**: Modern protocol proto files are already available in the repo as a git submodule
- **TLS**: Required for all tests — buf CLI mandates TLS. Use `~/local-tls/server/` certs
- **Testing**: Use real `buf` CLI binaries (v1.30.1 and v1.69.0+) against a real TLS server hitting the real GitHub API
- **GitHub API**: Tests require a valid GitHub token configured in test environment

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Both protocols active simultaneously | Backward compatibility — existing clients must not break during migration | — Pending |
| GitHub-only provider testing | GitHub is the primary provider; testing one real provider is sufficient for protocol validation | — Pending |
| Real buf binary + real server + TLS for tests | Tests must prove the actual buf CLI can communicate with the proxy — anything less wouldn't catch protocol issues | — Pending |
| Proto diff as part of work | We don't know exact differences between old and new protocol — will analyze during research/planning | — Pending |
| buf v1.69.0 content-type mismatch | Modern buf expects `application/proto` but proxy returns `text/plain; charset=utf-8` — Connect RPC protocol version difference | Escalated to Phase 5 |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-05-07 after Phase 2 completion*
