---
phase: 4
slug: old-protocol-validation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-07
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + `stretchr/testify` v1.8.4 |
| **Config file** | none — Go test framework uses convention (`*_test.go`) |
| **Quick run command** | `go test ./e2e/ -count=1 -timeout 120s -run TestOldProtocol -v` |
| **Full suite command** | `go test ./e2e/... -count=1 -timeout 300s -v` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./e2e/ -count=1 -timeout 120s -run TestOldProtocol -v`
- **After every plan wave:** Run `go test ./e2e/... -count=1 -timeout 300s -v`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 04-01-01 | 01 | 1 | OLD-01 | — | N/A | e2e (existing) | `go test ./e2e/ -run TestSmokeBufModUpdate/buf_v1.30.1 -count=1` | ✅ | ⬜ pending |
| 04-01-02 | 01 | 1 | OLD-02 | — | N/A | e2e (new) | `go test ./e2e/ -run TestOldProtocol -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `e2e/old_proto_test.go` — covers OLD-02 with two-step `buf mod update` pattern

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| None | — | — | — |

All phase behaviors have automated verification.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 120s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
