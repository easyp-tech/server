---
phase: 22
slug: fix-v1-30-1-v1alpha1-read-path-uuid-handling-verify-v1-proto
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-08
---

# Phase 22 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (go1.26.4) + `github.com/stretchr/testify/require` |
| **Config file** | none — Go convention (`go test ./...`) |
| **Quick run command** | `go test ./internal/connect/ -count=1` |
| **Full suite command** | `go test ./... -count=1` (unit) + `EASYP_GH_TOKEN=... go test ./e2e/ -run TestGenerateWithPinnedBufLock -count=1` (e2e gate) |
| **Estimated runtime** | ~15 seconds (unit); ~60-120 seconds (e2e gate) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/connect/ -count=1` (+ `go build ./...`, `go vet ./...`)
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd-verify-work`:** Full unit suite green AND e2e gate (`TestGenerateWithPinnedBufLock/v1.30.1`) green
- **Max feedback latency:** ~15 seconds (unit); ~120 seconds (e2e gate)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 22-01-01 | 01 | 1 | PR-22-1 | — | N/A | unit | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ResolveUUID -count=1` | ❌ W0 | ⬜ pending |
| 22-01-02 | 01 | 1 | PR-22-2 | — | N/A | unit | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ProbeFallback -count=1` | ❌ W0 | ⬜ pending |
| 22-01-03 | 01 | 1 | PR-22-3 | — | No silent HEAD fallback on resolution failure | unit | `go test ./internal/connect/ -run TestDownloadManifestAndBlobs_ResolveError -count=1` | ❌ W0 | ⬜ pending |
| 22-01-04 | 01 | 2 | PR-22-4 | — | N/A | e2e | `EASYP_GH_TOKEN=... go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1` | ✅ | ⬜ pending |
| 22-01-05 | 01 | 2 | PR-22-5 | — | N/A | unit (guard) | `go test ./internal/connect/ -run 'TestCommitUUIDInverse|TestIsUUID|TestIsSHA' -count=1` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/connect/blobs_test.go` (new, same package — reuse package-private `mockProvider` from `api_test.go:37-67`) — covers PR-22-1, PR-22-2, PR-22-3. Asserts provider receives the resolved SHA, not the raw UUID.
- [ ] No framework install — Go `testing` + testify already in `go.mod`.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| v1.30.1 `buf generate` end-to-end against live GitHub | PR-22-4 | Requires `EASYP_GH_TOKEN` + network egress to api.github.com | `EASYP_GH_TOKEN=... go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s (unit)
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
