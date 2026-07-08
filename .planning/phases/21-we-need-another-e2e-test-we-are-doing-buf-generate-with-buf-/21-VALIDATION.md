---
phase: 21
slug: we-need-another-e2e-test-we-are-doing-buf-generate-with-buf
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-08
---

# Phase 21 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (stdlib) + `testify/require` for e2e testutil |
| **Config file** | none (Go test config is implicit) |
| **Quick run command** | `go test ./e2e/ -run TestGenerateWithPinnedBufLock -count=1` (requires token + cached buf binaries) |
| **Full suite command** | `go test ./... -count=1` (includes e2e/ skip-cleanly checks) |
| **Estimated runtime** | ~30s for skip-cleanly; ~3-5min for full e2e with token + cached binaries |

---

## Sampling Rate

- **After every task commit:** `go test ./e2e/testutil/ -count=1` (regression-guard testutil unit tests) + `go vet ./e2e/...` (compile + vet)
- **After every plan wave:** `go test ./... -count=1` (full suite, including e2e skip-cleanly checks)
- **Before `/gsd-verify-work`:** Full suite green + manual e2e run with `EASYP_GH_TOKEN` set
- **Max feedback latency:** 60 seconds (the testutil unit tests are sub-second; `go vet` is sub-second; the full suite is gated on network which is the slowest step)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 21-01-01 | 01 | 1 | SC-21-1 | T-19-01 | Token written to 0600 config file; never logged | e2e | `go test ./e2e/ -run TestGenerateWithPinnedBufLock -count=1` | ❌ W0 | ⬜ pending |
| 21-01-01 | 01 | 1 | SC-21-2 | — | Test iterates AvailableBufVersions | e2e | `go test ./e2e/ -list 'TestGenerate.*'` | ❌ W0 | ⬜ pending |
| 21-01-01 | 01 | 1 | regression guard Phase 18/20 | — | probeCommitID resolves a valid UUID | e2e | same | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `e2e/testutil/server.go` — add `RunBufGenerateWithPinnedLock` public wrapper and `runBufGenerate` private helper; import `strings`
- [ ] `e2e/generate_test.go` — new test file with `TestGenerateWithPinnedBufLock` + `generatePinnedRef` constant
- [ ] `e2e/testutil/testutil_test.go` — confirm existing testutil unit tests still pass after the server.go changes (regression guard for the runBufGenerate addition)

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `buf generate` exits 0 against a pre-existing buf.lock pinned to a valid (older) commit | SC-21-1 | Requires `EASYP_GH_TOKEN` + cached buf binaries (not available in this environment) | Set `EASYP_GH_TOKEN`, run `go test ./e2e/ -run TestGenerateWithPinnedBufLock -count=1` with cached buf binaries under `testdata/buf/`, assert exit 0 + generated `gen/go/google/type/*.pb.go` files exist |

*If none: "All phase behaviors have automated verification."*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
