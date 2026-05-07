---
title: EasyP Buf Proxy Milestones
description: >
  Track shipped milestones and their outcomes.
  Add new entries at the top when a milestone completes.
  Do not reorder existing entries.
---

# Milestones

## v1.2 Dependency Modernization — In Progress

**Goal:** Upgrade Go to 1.26, update all dependencies, verify build and tests

**Started:** 2026-05-07

**Target:**
- Upgrade Go from 1.22 to 1.26
- Update connect-go to v1.19.x
- Update all other dependencies to latest
- Update buf proto submodule to latest
- Build and tests pass

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