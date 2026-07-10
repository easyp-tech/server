---
status: partial
phase: 24-resolve-buf-cid-ref-in-servegraph-honor-pinned-commit
source: [24-VERIFICATION.md]
started: 2026-07-09T15:45:00.000Z
updated: 2026-07-09T15:45:00.000Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. Live-token e2e gate: pinned cid serves pinned commit, not HEAD

expected: With `EASYP_GH_TOKEN` set, `go test ./e2e/... -run TestGeneratePinnedCommit_NotHEAD -v` PASSES for every cached buf version. The server log must show the PINNED SHA as `commit=<pinnedSHA>` on a serving-decision branch line (`uuid_ref_resolved`, `info_cache_writeback`, `files_cache_hit`, `digest_b5_wrap`, `digest_b4_keep`, `commit_id_probe_hit`) — never HEAD's SHA. Without the token the test SKIPs cleanly by design (PLAN `user_setup`); the SKIP is not a code gap.

result: blocked — environment issue, NOT a phase-24 defect (confirmed with baseline control).

Attempted live run 2026-07-09 with EASYP_GH_TOKEN loaded from test.env:
`go test ./e2e/... -run TestGeneratePinnedCommit_NotHEAD -v` → FAIL (v1.69.0 + v1.30.1).

Root cause is buf-CLI-side, not the proxy:
- Proxy served every request 200. The Phase 24 code path fired correctly: `branch=uuid_ref_resolved commit_id=27156597fdf440fb8077004434d44091 commit=27156597fdf4fb77004434d4409154a230dc9a32` — the pinned cid resolved to the pinned SHA exactly as designed.
- buf CLI itself failed: `Failure: 403 Forbidden` (generate) / `the server hosted at that remote is unavailable` (dep update).

DECISIVE CONTROL: `testdata/buf/` (cached buf binaries) is gitignored, so the initial bisect worktree runs were silently SKIPping (0.4s "ok" with no buf binaries). Redoing it properly — checked out the pre-phase-24 baseline 9270e6f into a worktree, copied in testdata/buf + test.env, ran `TestGenerateWithPinnedBufLock/v1.69.0` → FAILS IDENTICALLY ("server hosted at that remote is unavailable"). A failure present at the baseline BEFORE phase 24 cannot be a phase-24 regression. The live GitHub e2e is broken in THIS environment for all pinned-dep tests (Phase 19/21/23/24 lineage), independent of the phase-24 code.

Conclusion: Phase 24 cid-resolution is verified working end-to-end at the proxy layer (unit suite 8/8 + live server logs showing uuid_ref_resolved). The live-token e2e gate cannot be satisfied in THIS environment until the buf-CLI env issue is resolved (network egress / token-propagation to buf's own git operations) — affects pre-existing tests, not Phase 24 specifically.

## Summary

total: 1
passed: 0
issues: 0
pending: 0
skipped: 0
blocked: 1

## Gaps
