---
phase: 2
slug: handler-adaptation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-07
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) + stretchr/testify v1.8.4 |
| **Config file** | none — Wave 0 creates `e2e/` directory |
| **Quick run command** | `go build ./... && go vet ./...` |
| **Full suite command** | `go test ./e2e/ -v -count=1 -timeout 120s` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go build ./... && go vet ./...`
- **After every plan wave:** Run `go test ./e2e/ -v -count=1 -timeout 120s`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 02-01-1 | 01 | 1 | HAND-01 | — | N/A | compilation | `go build ./... && go vet ./...` | N/A | ⬜ pending |
| 02-01-1 | 01 | 1 | HAND-03 | — | N/A | compilation | `grep -c "ManifestDigest" gen/proto/buf/alpha/module/v1alpha1/module.pb.go` | N/A | ⬜ pending |
| 02-01-1 | 01 | 1 | HAND-04 | — | N/A | compilation | `go build ./...` (Unimplemented embedding) | N/A | ⬜ pending |
| 02-01-2 | 01 | 1 | HAND-02 | — | N/A | E2E smoke | `go test ./e2e/ -run TestSmokeBufModUpdate -v -count=1 -timeout 120s` | Wave 0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `e2e/smoke_test.go` — E2E smoke test for buf mod update with both CLI versions
- [ ] `e2e/` directory created

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| None | — | — | — |

All phase behaviors have automated verification.

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
