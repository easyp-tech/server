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

result: [pending]

Why human: the verifier sandbox has no token and no live egress to github.com. The underlying live-upstream integration check (`buf generate` resolving a buf.lock-pinned commit to its own content) is genuine external-service work that warrants a human run before declaring production-ready.

## Summary

total: 1
passed: 0
issues: 0
pending: 1
skipped: 0
blocked: 0

## Gaps
