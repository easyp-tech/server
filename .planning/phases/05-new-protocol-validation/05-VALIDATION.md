---
phase: 5
slug: new-protocol-validation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-07
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing testutil infrastructure |
| **Quick run command** | `go test ./e2e/ -run TestNewProtocol -v -count=1` |
| **Full suite command** | `go test ./e2e/ -v -count=1` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go build ./...`
- **After every plan wave:** Run `go test ./e2e/ -v -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | NEW-01 | — | N/A | integration | `go test ./e2e/ -run TestNewProtocol/BufModUpdate -v` | ❌ W0 | ⬜ pending |
| 05-01-02 | 01 | 1 | NEW-01 | — | N/A | integration | `go test ./e2e/ -run TestNewProtocol/BufModUpdate -v` | ❌ W0 | ⬜ pending |
| 05-02-01 | 02 | 2 | NEW-02 | — | N/A | integration | `go test ./e2e/ -run TestNewProtocol/BufDepUpdate -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `e2e/new_proto_test.go` — new test file for NEW-01, NEW-02
- [ ] `e2e/testutil/bufbin.go` — RunBufDepUpdate helper addition

*Existing infrastructure covers all phase requirements except the new test file and helper.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| None | — | — | — |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
