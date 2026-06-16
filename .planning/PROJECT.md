# EasyP Buf Proxy — Protocol Modernization

## Current Milestone: v1.3 Diagnostic Logging

**Goal:** Improve logging across all request/response paths so that 400 errors and other failures are diagnosable without requiring source code access

**Target features:**
- Configurable debug-level logging for full request/response tracing
- Structured diagnostic information on all error paths
- Log level configuration (e.g., via environment variable or config file)
- Sensitive data masking preserved in enhanced logging

## What This Is

A Go-based proxy server that translates Buf CLI registry requests into VCS API calls (GitHub, BitBucket, local git). The server implements both the deprecated Buf `registry.v1alpha1` protocol (buf v1.30.1) and the modern Buf protocol (v1.69.0+) via Connect RPC, serving both simultaneously for backward compatibility.

## Core Value

The proxy must correctly serve both old (v1.30.1) and modern (v1.69.0+) Buf CLI clients simultaneously, so that existing users are not broken during migration.

## Requirements

### Validated

- ✓ Buf registry proxy for deprecated `registry.v1alpha1` protocol — v1.0
- ✓ Multi-provider architecture (local git, GitHub, BitBucket) — v1.0
- ✓ Cache layer (noop, local filesystem, Artifactory) — v1.0
- ✓ TLS with optional mTLS support — v1.0
- ✓ Structured logging with sensitive header masking — v1.0
- ✓ v1beta1 API support for modern buf CLI — v1.1
- ✓ B4 digest computation (SHAKE256) — v1.1
- ✓ E2E tests for both buf versions — v1.1
- ✓ Go 1.26 + connect-go v1.19.x — v1.2
- ✓ Proto regenerated from buf v1.69.0 — v1.2
- ✓ All dependencies at latest compatible versions — v1.2
- ✓ Code quality fixes + unit test suite — v1.2

### Active

- [ ] Performance benchmarking and optimization
- [ ] New API endpoints as needed

### Out of Scope

- BitBucket provider testing — GitHub provider is sufficient for validation
- Local git provider testing — not needed for protocol validation
- Removing the old v1alpha1 protocol — both protocols must coexist
- Artifactory cache testing — not relevant to protocol correctness
- UI changes — this is a backend-only project

## Context

- Tech stack: Go 1.26, Connect RPC v1.19.x, protobuf, buf v1.69.0 proto definitions
- Codebase: ~324K LOC Go (including generated proto code)
- Submodule: `api/_third_party/buf` points to buf v1.69.0 proto definitions
- TLS certs for local testing at `~/local-tls/server/` (self-signed)
- Server is stateless — no database, relies on external VCS APIs and optional caching
- Unit test suite: 14 tests across 4 packages
- E2E tests require `EASYP_GITHUB_TOKEN` environment variable
- HTTP clients hardened with 30s timeout and 50MB body limit

## Constraints

- **Tech Stack**: Go 1.26, Connect RPC, protobuf — must stay within existing stack
- **Protocol Compatibility**: Old protocol must continue working unchanged while new protocol is active
- **Proto Definitions**: Modern protocol proto files available in repo as git submodule
- **TLS**: Required for all tests — buf CLI mandates TLS
- **Testing**: Use real `buf` CLI binaries against real TLS server hitting real GitHub API

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Both protocols active simultaneously | Backward compatibility — existing clients must not break during migration | ✓ Good |
| GitHub-only provider testing | GitHub is the primary provider; testing one real provider is sufficient | ✓ Good |
| Real buf binary + real server + TLS for tests | Tests must prove actual buf CLI can communicate with the proxy | ✓ Good |
| Go 1.26 minimum version | connect-go v1.19.x requires Go 1.24+; Go 1.26 is latest stable | ✓ Good |
| Old buf submodule removed | buf-v1.69.0 is canonical; old v1.9.0 defs are deprecated | ✓ Good |
| All error paths return nil | Prevents silent data corruption from partial results | ✓ Good |
| Configurable HTTP timeouts/body limits | Hardening without sacrificing configurability | ✓ Good |
| Shared download helper (DRY) | GitHub and BitBucket had identical download-hash-accumulate logic | ✓ Good |

## Evolution

This document evolves at phase transitions and milestone boundaries.

---
*Last updated: 2026-06-16 after v1.3 milestone start*
