---
gsd_wave: 1
depends_on: []
files_modified:
  - .gitmodules
  - api/_third_party/buf
  - api/_third_party/buf-v1.69.0
autonomous: false
---

# Phase 9 Plan: Submodule Cleanup

## Context

The project currently has two buf submodules:
- `api/_third_party/buf` — old protocol (v1.9.0), deprecated
- `api/_third_party/buf-v1.69.0` — modern protocol (v1.9.0-1748), currently in use

The `api/proto/generate.go` script copies proto files from `../_third_party/buf-v1.69.0/proto/buf` to `api/proto/buf/` before running `buf generate`. After this cleanup, the modern version should become the canonical `api/_third_party/buf`.

## Scope

1. Remove old `api/_third_party/buf` submodule
2. Rename `api/_third_party/buf-v1.69.0` to `api/_third_party/buf`
3. Update `.gitmodules` to reflect the rename
4. Update `api/proto/generate.go` to reference the new path
5. Regenerate proto code and verify build/tests pass

## Must-Haves (Goal-Backward Verification)

- [ ] `.gitmodules` contains only one `[submodule "api/_third_party/buf"]` entry (old removed, new renamed)
- [ ] `api/_third_party/buf-v1.69.0` directory no longer exists
- [ ] `api/_third_party/buf` exists and points to buf v1.9.0-1748+ commit
- [ ] `api/proto/generate.go` copies from `../_third_party/buf/proto/buf`
- [ ] `go generate` completes without errors
- [ ] `go build ./...` exits 0
- [ ] E2E tests pass

---

## Tasks

### Task 1: Remove old `buf` submodule

**Purpose:** Remove the deprecated `api/_third_party/buf` submodule entirely.

**Read first:**
- `.gitmodules` — shows submodule configuration
- `api/_third_party/buf` — directory that will be removed

**Action:**

Remove the old `api/_third_party/buf` submodule using git submodule commands:

```bash
git submodule deinit --force api/_third_party/buf
git rm api/_third_party/buf
```

Then remove the now-empty `.git` file and directory if they remain:

```bash
rm -rf api/_third_party/buf
```

**Acceptance criteria:**
- [ ] `git submodule status` does not list `api/_third_party/buf`
- [ ] `.gitmodules` no longer contains `[submodule "api/_third_party/buf"]` entry
- [ ] `api/_third_party/buf` directory does not exist in filesystem

---

### Task 2: Rename `buf-v1.69.0` to `buf`

**Purpose:** Promote the modern buf submodule to be the canonical `api/_third_party/buf`.

**Read first:**
- `.gitmodules` — current submodule configuration

**Action:**

Rename the `buf-v1.69.0` submodule directory using git mv to preserve history:

```bash
git mv api/_third_party/buf-v1.69.0 api/_third_party/buf
```

**Acceptance criteria:**
- [ ] `api/_third_party/buf-v1.69.0` directory does not exist
- [ ] `api/_third_party/buf` directory exists
- [ ] `git mv` rename was used (directory is tracked under new name with preserved history)
- [ ] `git submodule status` shows `api/_third_party/buf` pointing to `88829eb3bd5b9ee297b6005ffdf3675e23842511` or later

---

### Task 3: Update `.gitmodules` for renamed submodule

**Purpose:** Update the `.gitmodules` file to reflect the rename from `buf-v1.69.0` to `buf`.

**Read first:**
- `.gitmodules` — current content after rename

**Action:**

The `.gitmodules` entry currently reads:

```
[submodule "api/_third_party/buf-v1.69.0"]
	path = api/_third_party/buf-v1.69.0
	url = https://github.com/bufbuild/buf
```

Change it to:

```
[submodule "api/_third_party/buf"]
	path = api/_third_party/buf
	url = https://github.com/bufbuild/buf
```

**Acceptance criteria:**
- [ ] `.gitmodules` contains `[submodule "api/_third_party/buf"]`
- [ ] `.gitmodules` no longer contains `buf-v1.69.0` path or name
- [ ] `.gitmodules` contains exactly one buf submodule entry

---

### Task 4: Update `api/proto/generate.go` to reference new path

**Purpose:** The generate script currently copies from `buf-v1.69.0`, must be updated to copy from the renamed `buf`.

**Read first:**
- `api/proto/generate.go` — current script content

**Action:**

Update the `cp` command in `api/proto/generate.go` to reference the renamed submodule:

Current line:
```bash
//go:generate cp -r ../_third_party/buf-v1.69.0/proto/buf ./
```

Change to:
```bash
//go:generate cp -r ../_third_party/buf/proto/buf ./
```

**Acceptance criteria:**
- [ ] `api/proto/generate.go` line 4 contains `../_third_party/buf/proto/buf`
- [ ] `api/proto/generate.go` does not contain `buf-v1.69.0`
- [ ] `grep -n "buf-v1.69.0" api/proto/generate.go` returns no matches

---

### Task 5: Regenerate proto code with `go generate`

**Purpose:** Verify the generate script works with the new submodule path and produce fresh proto code.

**Read first:**
- `api/proto/generate.go` — to understand the generation flow
- `api/proto/buf.gen.yaml` — code generation configuration (no path changes expected)

**Action:**

```bash
cd api/proto && go generate
```

**Acceptance criteria:**
- [ ] `go generate` exits 0 (no errors)
- [ ] `api/proto/buf/` directory contains proto files copied from the new submodule
- [ ] `gen/proto/` directory contains regenerated Go code
- [ ] `buf generate` completed successfully (visible in output)
- [ ] `grep -r "buf-v1.69.0\|buf-v1.30" api/proto/buf.gen.yaml` returns no matches (D-04: buf.gen.yaml does not reference old submodule)

---

### Task 6: Verify `go build ./...` passes

**Purpose:** Confirm all generated code compiles correctly with the updated submodule structure.

**Read first:**
- `go.mod` — Go version and dependencies
- `internal/connect/api.go` — handler that imports generated packages

**Action:**

```bash
go build ./...
```

**Acceptance criteria:**
- [ ] `go build ./...` exits 0
- [ ] No import errors or missing symbols
- [ ] All packages compile including `gen/proto/` packages

---

### Task 7: Run E2E tests to verify functionality

**Purpose:** Confirm end-to-end functionality with the cleaned-up submodule structure.

**Read first:**
- `e2e/smoke_test.go` — main E2E test suite
- `e2e/new_proto_test.go` — tests for modern protocol
- `e2e/old_proto_test.go` — tests for old protocol (if applicable)

**Action:**

```bash
go test ./e2e/... -v
```

**Acceptance criteria:**
- [ ] `go test ./e2e/...` exits 0
- [ ] Tests pass (may skip if `EASYP_GITHUB_TOKEN` not set, but test code must compile and not error)

---

## Verification Summary

After all tasks complete:

| Check | Command | Expected Result |
|-------|---------|-----------------|
| Old submodule removed | `git submodule status` | No `api/_third_party/buf (v1.9.0)` entry |
| New submodule exists | `git submodule status` | `api/_third_party/buf` at v1.9.0-1748+ |
| `.gitmodules` updated | `cat .gitmodules` | Single buf entry, path=`api/_third_party/buf` |
| `generate.go` updated | `grep buf-v1.69.0 api/proto/generate.go` | No matches |
| Build passes | `go build ./...` | Exit 0 |
| E2E tests pass | `go test ./e2e/... -v` | Exit 0 |

---

*Plan: 09-submodule-cleanup*
*Wave: 1*
*Tasks: 7 (Wave 1: 1-3, Wave 2: 4-7)*