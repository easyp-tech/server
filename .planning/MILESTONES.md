---
title: EasyP Buf Proxy Milestones
description: >
  Track shipped milestones and their outcomes.
  Add new entries at the top when a milestone completes.
  Do not reorder existing entries.
---

# Milestones

## v1.2 Dependency Modernization — 2026-05-10

**Goal:** Upgrade Go to 1.26, update all dependencies, clean up codebase, add unit tests

**Outcome:** Complete

**What shipped:**
- Go 1.26 + connect-go v1.19.x + all dependencies upgraded
- Proto code regenerated from buf v1.69.0 with new connect-go
- Deprecated x/exp imports replaced with stdlib (slog, slices)
- Old buf submodule removed, buf-v1.69.0 promoted to canonical
- 5 critical bugs fixed (panic, inverted checks, partial results)
- HTTP hardening (timeouts, body limits) + shared download helper
- Unit test suite with 14 tests

**Phases:** 5 | **Plans:** 10 | **Tests:** 14 unit + 9 UAT
**Timeline:** 2 days (2026-05-07 → 2026-05-09)
**Known deferred items at close:** 2 (Phase 03/05 human tests from v1.1, see STATE.md)

**Accomplishments:**
1. Go 1.26 + all dependencies upgraded to latest compatible versions
2. Proto code regenerated with connect-go v1.19.x — 29 connect files
3. Deprecated golang.org/x/exp replaced with stdlib equivalents
4. Old buf submodule (v1.9.0) removed, buf-v1.69.0 promoted via git mv
5. 5 critical bugs fixed with nil-on-error pattern
6. HTTP clients hardened with configurable timeouts and body limits
7. Shared download helper extracted (DRY across GitHub/BitBucket)
8. Unit test suite established covering bug fixes and key API surfaces

## v1.1 Protocol Modernization — 2026-05-07

**Goal:** Add v1beta1 API support for modern buf CLI while keeping v1alpha1 for old clients

**Outcome:** ✓ Complete

**What shipped:**
- v1beta1 API handlers: GetCommits, GetGraph, Download, GetModules
- B4 digest computation (SHAKE256)
- In-memory caching across RPC chain
- IPv4-only GitHub transport for macOS compatibility
- E2E tests for both buf v1.30.1 and v1.69.0+

**Version:** v1.30.1
**Completed:** 2026-05-07