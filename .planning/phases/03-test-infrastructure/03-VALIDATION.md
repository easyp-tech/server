---
phase: 3
slug: test-infrastructure
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-07
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + `stretchr/testify` v1.8.4 |
| **Config file** | none — Go test framework uses convention (`*_test.go`) |
| **Quick run command** | `go test ./e2e/testutil/ -count=1 -timeout 120s -run TestHelper` |
| **Full suite command** | `go test ./e2e/... -count=1 -timeout 300s` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./e2e/testutil/ -count=1 -timeout 120s`
- **After every plan wave:** Run `go test ./e2e/... -count=1 -timeout 300s`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 03-01-01 | 01 | 1 | TINF-01 | — | Config file written with mode 0600 | unit | `go test ./e2e/testutil/ -run TestStartServer -count=1` | ❌ W0 | ⬜ pending |
| 03-01-02 | 01 | 1 | TINF-05 | — | Each test gets unique port via net.Listen :0 | unit | `go test ./e2e/ -run TestSmoke -count=1` | ❌ W0 | ⬜ pending |
| 03-02-01 | 02 | 1 | TINF-02 | T-3-01 | HTTPS download from GitHub CDN | unit | `go test ./e2e/testutil/ -run TestGetBuf -count=1` | ❌ W0 | ⬜ pending |
| 03-02-02 | 02 | 1 | TINF-06 | — | Env-only config (EASYP_GITHUB_TOKEN) | unit | `go test ./e2e/testutil/ -run TestConfig -count=1` | ❌ W0 | ⬜ pending |
| 03-03-01 | 03 | 2 | TINF-03 | — | Token read from env, skip if missing | integration | `go test ./e2e/ -run TestSmoke -count=1` | ❌ W0 | ⬜ pending |
| 03-03-02 | 03 | 2 | TINF-04 | — | Config targets googleapis/googleapis | integration | `go test ./e2e/ -run TestSmoke -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `e2e/testutil/server.go` — StartServer helper for TINF-01
- [ ] `e2e/testutil/bufbin.go` — GetBuf helper for TINF-02
- [ ] `e2e/testutil/config.go` — TestConfig struct and YAML generation for TINF-03, TINF-04
- [ ] `e2e/testutil/testutil_test.go` — Internal validation tests

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| CI execution with env vars only | TINF-06 | Requires CI environment | Verify test suite runs with only EASYP_GITHUB_TOKEN env var set |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 120s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
