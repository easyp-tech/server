---
phase: 25
slug: address-pr-39-post-merge-review-findings-pin-multi-default-b
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-07-10
---

# Phase 25 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` (go1.26.4); `github.com/stretchr/testify/require` for assertions |
| **Config file** | none (Go convention; `go test ./...`) |
| **Quick run command** | `go test ./internal/providers/github/... ./internal/providers/bitbucket/... ./internal/connect/... -count=1` |
| **Full suite command** | `go test ./... -count=1` (unit) + `EASYP_GH_TOKEN=... go test ./e2e/ -count=1` (e2e) |
| **Estimated runtime** | ~30-60s unit / ~5m with e2e |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -count=1` (unit tests only)
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| F-1 | 01 | 1 | — | T-25-01 | `GetMeta` resolves `ref="main"` to branch tip when repo has a real branch named "main" not the default | unit | `go test ./internal/providers/github/... ./internal/providers/bitbucket/... -count=1` | ✅ (existing tests updated) | ⬜ pending |
| F-1 (no regression) | 01 | 1 | — | — | Removing carve-out doesn't break v1.30.1 googleapis path (ref="" branch) | e2e | `EASYP_GH_TOKEN=... go test ./e2e/ -run 'TestRefRespected' -count=1` | ✅ (Phase 19/23 tests) | ⬜ pending |
| F-2 | 01 | 1 | — | — | Error wrap says "GetFiles" not "GetRepository" | code review + grep | `grep -c 'a.repo.GetRepository' internal/connect/blobs.go` (0 after fix) | structural | ⬜ pending |
| F-3 | 02 | 2 | — | — | v1alpha1 `TestGenerateWithPinnedBufLock/v1.30.1` passes against real GitHub | e2e | `EASYP_GH_TOKEN=... go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1 -v` | ✅ (exists, was TLS-blocked) | ⬜ pending |
| F-4 | 01 | 1 | — | — | `isSHA` and `isConventionalDefaultName` no longer defined in provider packages | structural | `grep -c 'func isSHA\|func isConventionalDefaultName' internal/providers/github/getrepo.go internal/providers/bitbucket/getrepo.go` (0) | structural | ⬜ pending |
| F-5 | 01 | 1 | — | — | Proto path comment references an existing file | code review | `grep -c 'api/proto/buf/registry/module/v1beta1/resource.proto' internal/connect/commits_helpers_test.go` (0) | structural | ⬜ pending |
| F-6 | 02 | 2 | — | — | `commitLineRE` defined only in e2e/testutil/server.go | structural | `grep -c 'commitLineRE' e2e/ref_test.go e2e/testutil/server.go` (1 in server.go, 0 in ref_test.go) | structural | ⬜ pending |
| F-7 | 01 | 1 | — | — | `commitResolver` assignment documented with startup-not-nil assertion | code review + audit | Review field assignment in `api.go:124` post-fix | structural | ⬜ pending |

---

## Wave 0 Requirements

- [ ] `internal/providers/content/helpers_test.go` — test cases for extracted `IsSHA` and `IsConventionalDefaultName` (moved from provider copies)

*Existing infrastructure covers all other phase requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Error wrap string F-2 | — | Single-line string fix; automated assertion would be over-engineered | `grep -c 'a.repo.GetRepository' internal/connect/blobs.go` — must be 0 (string replaced with `GetFiles`) |
| Proto path comment F-5 | — | Comment-only fix; automated assertion unnecessary | `grep -c 'api/proto/buf/registry/module/v1beta1/resource.proto' internal/connect/commits_helpers_test.go` — must be 0 |
| Post-construction mutation F-7 | — | Code review of api.go constructor assignment | Verify `a.commitResolver = commitHandler` at api.go:124 has doc comment and/or guard |

*All other phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have automated verify or structural assertions
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (helpers_test.go)
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending