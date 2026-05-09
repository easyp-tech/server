# Phase 10-01: Critical Bug Fixes — Summary

**Phase:** 10-code-quality-fixes
**Plan:** 01
**Wave:** 2
**Status:** Complete ✓

---

## What Was Fixed

5 critical bugs that caused incorrect behavior, crashes, or data corruption.

---

## Bug Fixes

### 1. `splitRepoName()` Panic (CQ-01)

**File:** `internal/connect/bynames.go`

**Problem:** Function accessed `fields[1]` without checking if the array had at least 2 elements, causing panic on malformed input.

**Fix:**
```go
func splitRepoName(name string) (string, string) {
    fields := strings.Split(name, "/")
    if len(fields) != 2 {
        return "", ""
    }
    return fields[0], fields[1]
}
```

### 2. Repository Name Validation (CQ-01)

**File:** `internal/connect/bynames.go`

**Problem:** `resolveRepoByFullName` would call `GetMeta` with empty repository name when input had no slash.

**Fix:**
```go
owner, repositoryName := splitRepoName(name)
if repositoryName == "" {
    return nil, fmt.Errorf("invalid repository name %q: expected owner/repo format", name)
}
```

### 3. `resolveReposByFullNames` Returns Partial Results (CQ-02)

**File:** `internal/connect/bynames.go`

**Problem:** On error during iteration, returned partial results (`return out, error`).

**Fix:** Changed to `return nil, error` — ensures clean error propagation.

### 4. `multisource.GetFiles` Returns Partial Results (CQ-02)

**File:** `internal/providers/multisource/repo.go`

**Problem:** On source error, returned partial file list with error.

**Fix:** Changed to `return nil, fmt.Errorf(...)`.

### 5. `resolveModulePins` Returns Partial Results (CQ-02)

**File:** `internal/connect/modulepins.go`

**Problem:** On resolution error, returned partial module pins.

**Fix:** Changed to `return nil, fmt.Errorf(...)`.

---

## Verification

| Check | Result |
|-------|--------|
| `go build ./...` | ✓ Pass |
| `go vet ./...` | ✓ Pass |
| `go test ./internal/connect/... -run TestSplitRepoName` | ✓ Pass |

---

## Commit

```
fix(10-01): critical bug fixes
```