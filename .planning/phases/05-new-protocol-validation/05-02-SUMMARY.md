---
phase: 05-new-protocol-validation
plan: 02
subsystem: connect-handlers
tags: [buf, v1beta1, protobuf-wire, connect-protocol, caching]

requires:
  - phase: 05-new-protocol-validation
    plan: 01
    provides: "Discovered v1beta1 RPC call chain and content-type requirements"
provides:
  - "Full v1beta1 protocol handler implementation (GetCommits, GetGraph, Download, GetModules)"
  - "B4 digest computation matching buf CLI expectations"
  - "Caching layer preventing redundant GitHub API calls across RPC chain"
  - "IPv4-only transport fix for GitHub client"
affects: []

tech-stack:
  added: ["google.golang.org/protobuf/encoding/protowire for manual protobuf encoding"]
  patterns: ["in-memory caching across RPC chain", "manual protobuf wire format construction"]

key-files:
  created: []
  modified: ["internal/connect/commits.go", "internal/connect/api.go", "internal/connect/blobs.go", "internal/providers/github/client.go"]

key-decisions:
  - "Manual protobuf wire encoding instead of generated types — avoids complex proto dependencies"
  - "Cache files and commit info in handler maps — GetCommits is the only expensive call"
  - "IPv4-only dialer in GitHub client to avoid IPv6 TLS timeouts on macOS"

patterns-established:
  - "Manual protobuf wire encoding for v1beta1 responses"
  - "In-memory request-scoped caching across RPC chain"

requirements-completed: [NEW-01, NEW-02]

duration: 45min
completed: 2026-05-07
---

# Phase 5 Plan 02 Summary

**Full v1beta1 protocol implementation — all RPCs working, buf v1.69.0 passes e2e**

## Performance

- **Duration:** 45 min
- **Started:** 2026-05-07T20:15:00Z
- **Completed:** 2026-05-07T21:00:00Z
- **Tasks:** 6
- **Files modified:** 4

## Accomplishments

1. Implemented `GetCommits` handler with file download, B4 digest computation, and caching
2. Implemented `GetGraph` handler using cached commit info (instant after GetCommits)
3. Implemented `Download` handler with correct v1beta1 wire format (`repeated Content contents`)
4. Implemented `GetModules` handler with ID-based module lookups
5. Fixed IPv6 timeout issue in GitHub client (IPv4-only transport)
6. Registered all v1beta1 routes in api.go

## Key Technical Discoveries

**v1beta1 RPC call chain from buf v1.69.0:**
1. `CommitService/GetCommits` — expensive, downloads files from GitHub
2. `GraphService/GetGraph` (x2) — cheap, uses cached data
3. `DownloadService/Download` — cheap, uses cached files
4. `ModuleService/GetModules` — cheap, uses cached module info

**Connect protocol:** `application/proto` content-type = raw protobuf body, no 5-byte envelope header.

**B4 Digest:** SHAKE256 of sorted manifest (`shake256:<hash>  <path>\n` per line), then SHAKE256 of the full manifest.

**DownloadResponse v1beta1:** `{ repeated Content contents=1 }` where `Content { Commit commit=1, repeated File files=2 }`.

## Errors Fixed

1. Wrong DownloadResponse format (used v1alpha1 instead of v1beta1)
2. IPv6 TLS handshake timeout to raw.githubusercontent.com
3. GetModules returning text/plain (route not registered)
4. GetModules 400 "no module refs" (only handled Name refs, not ID refs)
5. Digest verification failed (fake digest replaced with real B4 computation)
6. Network timeouts from redundant file downloads (added caching)

## Deviations from Plan

Plan 05-02 was originally scoped as "fix blockers discovered by 05-01." The scope expanded to full v1beta1 protocol implementation because the entire API surface was new. All fixes were implemented incrementally with test-driven discovery.

## Next Phase Readiness

Phase 5 goal fully achieved:
- `buf mod update` works with v1.69.0 (TestNewProtocolBufModUpdate: PASS)
- `buf dep update` works with v1.69.0 (TestNewProtocolBufDepUpdate: PASS)
- Old protocol still works (TestOldProtocolBufModUpdateTwice: PASS)

---
*Phase: 05-new-protocol-validation*
*Completed: 2026-05-07*
