# Phase 10-03: Shared Download Helper + HTTP Hardening — Summary

**Phase:** 10-code-quality-fixes
**Plan:** 03
**Wave:** 1
**Status:** Complete ✓

---

## What Was Built

Extracted duplicate download-hash-accumulate logic into a shared helper and hardened HTTP clients with configurable timeouts and response body limits.

---

## Changes Made

### New File: `internal/providers/content/download.go`

Created shared download helper with:
- `FileEntry` struct with `Orig` and `Name` fields
- `GetFiles()` function — downloads, hashes, and returns sorted content.File slice
- `FilterEntries()` generic function — applies repo.Check() filtering

### Refactored: `internal/providers/github/getfiles.go`

- Removed duplicate `fileFiltered` struct and `filterEntries` function
- Now uses `content.GetFiles()` and `content.FilterEntries()`
- Reduced code duplication significantly

### Refactored: `internal/providers/bitbucket/getfiles.go`

- Removed duplicate `fileFiltered` struct and `filterEntries` function
- Now uses `content.GetFiles()` and `content.FilterEntries()`

### Hardened: `internal/providers/bitbucket/client.go`

- Added `http.Client{Timeout: 30s}` (defaultHTTPTimeout)
- Added `bodyLimit: 50MB` (defaultBodyLimit)
- Replaced `http.DefaultClient` with `c.client.Do()`
- Wrapped `io.ReadAll` with `io.LimitReader`

### Hardened: `internal/providers/cache/artifactory/artifactory.go`

- Added `http.Client{Timeout: timeout}` to struct
- Added `bodyLimit: int64` field
- Replaced all `http.DefaultClient.Do()` with `c.client.Do()`
- Wrapped `io.ReadAll` with `io.LimitReader` in Get method
- Fixed inverted status code check (was `< 200 && >= 300`, now `>= 300`)

### Extended Config: `cmd/easyp/internal/config/config.go`

Added `Timeout int` and `BodyLimit int64` fields to:
- `GithubRepo`
- `BitBucketRepo`
- `Artifactory`

### Updated Wiring: `cmd/easyp/main.go`

- `buildCache()` now passes timeout and bodyLimit to `artifactory.New()`

---

## Bug Fixes Applied

1. **Artifactory PUT status check** — Changed `resp.StatusCode < http.StatusOK && resp.StatusCode >= http.StatusMultipleChoices` to `resp.StatusCode >= http.StatusMultipleChoices`. The old condition was always false.

---

## Verification

| Check | Result |
|-------|--------|
| `go build ./...` | ✓ Pass |
| `go vet ./...` | ✓ Pass |
| `go test ./...` | ✓ Pass |

---

## Commit

```
feat(10-03): shared download helper + HTTP hardening
```