---
status: partial
phase: 22-fix-v1-30-1-v1alpha1-read-path-uuid-handling-verify-v1-proto
source: [22-01-VERIFICATION.md]
started: 2026-07-08T15:35:00Z
updated: 2026-07-08T15:35:00Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. PR-22-4 live e2e gate (TestGenerateWithPinnedBufLock/v1.30.1)

Run with a GitHub token:

```
EASYP_GH_TOKEN=<token> go test ./e2e/ -run 'TestGenerateWithPinnedBufLock/v1.30.1' -count=1
```

expected: buf v1.30.1 generate exits 0 and produces `gen/go/google/type/*.pb.go` containing `package google.type`.
result: [pending]

## Summary

total: 1
passed: 0
issues: 0
pending: 1
skipped: 0
blocked: 0

## Gaps
