---
phase: 20
slug: fix-the-problem-with-refs-discovered-on-phase-19
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-07
---

# Phase 20 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (stdlib) |
| **Config file** | none (Go test config is implicit) |
| **Quick run command** | `go test ./internal/connect/... ./internal/providers/... -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/connect/... ./internal/providers/... -count=1`
- **After every plan wave:** Run `go test ./... -count=1` (includes e2e/ skip-cleanly checks)
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** ~30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 20-01-01 | 01 | 1 | Phase 20-1 (parseResourceRefName reads field 4) | V5 Input Validation | ref is bound to buf client's Name.ref, not label_name | unit | `go test ./internal/connect/ -run TestParseResourceRefName -count=1` | ✅ (commits_helpers_test.go:372) — needs update | ⬜ pending |
| 20-01-02 | 01 | 1 | Phase 20-2 (label_name at field 3 is not read as ref) | V5 Input Validation | label_name at field 3 parses to empty ref | unit | `go test ./internal/connect/ -run TestParseResourceRefName -count=1` | ❌ W0 (new test) | ⬜ pending |
| 20-01-03 | 01 | 1 | SC-19-1, SC-19-2, SC-19-3 (Phase 19 e2e still pass) | — | e2e tests compile + list + skip cleanly | e2e (skip) | `go test ./e2e/ -run TestRefRespected -count=1` | ✅ (e2e/ref_test.go) | ⬜ pending |
| 20-01-04 | 01 | 1 | HAND-02 (TestSmokeBufModUpdate) | — | smoke test still passes for both v1.30.1 and v1.69.0 | e2e (skip) | `go test ./e2e/ -run TestSmokeBufModUpdate -count=1` | ✅ (e2e/smoke_test.go) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/connect/commits_helpers.go:196` — change `num == 3` to `num == 4` in the `parseResourceRefName` `else if` arm that captures `ref`
- [ ] `internal/connect/commits_helpers_test.go:379` — change `protowire.AppendTag(name, 3, protowire.BytesType)` to `protowire.AppendTag(name, 4, protowire.BytesType)` so `TestParseResourceRefName_ReadsRef` writes the ref at the correct field number
- [ ] `internal/connect/commits_helpers_test.go` (new test) — `TestParseResourceRefName_LabelNameIsField3` that asserts a Name with `label_name="main"` at field 3 parses to `moduleRef.ref == ""` (the label_name must NOT be read as the ref)

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `TestRefRespected_*` e2e tests pass with `EASYP_GH_TOKEN` | SC-19-1, SC-19-2, SC-19-3 | Requires a real GitHub token and cached buf binaries under `testdata/buf/` (neither available in this environment) | Set `EASYP_GH_TOKEN`, populate `testdata/buf/` with buf v1.30.1 + v1.69.0 binaries, run `go test ./e2e/ -run TestRefRespected -count=1` |
| `TestSmokeBufModUpdate` passes for v1.30.1 | HAND-02 | Same as above — real GitHub token + cached buf binaries required | Same as above; expect exit 0 for both v1.30.1 and v1.69.0 subtests |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
