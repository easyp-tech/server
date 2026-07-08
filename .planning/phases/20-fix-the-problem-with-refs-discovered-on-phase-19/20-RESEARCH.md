# Phase 20: fix-the-problem-with-refs-discovered-on-phase-19 - Research

**Researched:** 2026-07-07
**Domain:** Bugfix in the v1beta1 Name-ref wire-parse path; minor defensive update to the provider ref-resolution gate
**Confidence:** HIGH (all claims verified against the source files; both regressions trace to a single root cause; field-number fix is byte-exact against the buf v1beta1/v1 schema)

## Summary

Phase 19 e2e tests caught two regressions introduced by Phase 18's ref-honoring refactor. Both regressions are the visible symptoms of a single root cause: Phase 18 read proto field 3 from the `Name.child` oneof as `ref`, but the buf v1beta1/v1 `Name` proto defines `label_name = 3` and `ref = 4`. The wrong field number is a one-line bug in `internal/connect/commits_helpers.go:196`.

Once `parseResourceRefName` reads field 4 correctly, both regressions resolve. Regression 1 (v1.30.1 → `label_name="main"` misread as `ref`, then `GetMeta("main")` → 422) goes away because the v1.30.1 no-ref request has empty field 4, so `ref` parses as `""`, and the providers' empty-commit branch returns HEAD without any `repos.GetCommit` round-trip. Regression 2 (v1.69.0 ref-pinned → HEAD) goes away because the modern client's `ref` at field 4 is now actually read.

**Primary recommendation:** Change `num == 3` to `num == 4` in `parseResourceRefName` (commits_helpers.go:196), update the matching unit test (`commits_helpers_test.go:372-389`) to write the ref at field 4, and add a new unit test that asserts a `label_name` at field 3 is NOT read as `ref`. A separate, optional defensive update to the provider `isSHA(commit)` gate (re-introducing the `commit != "main"` carve-out in github/getrepo.go:48-58 and bitbucket/getrepo.go:43-53) is NOT required to fix the regressions but is a reasonable belt-and-suspenders if reviewers want to preserve the pre-Phase-18 default-branch handling. See "Open Questions" for the recommendation on whether to include the defensive update.

## User Constraints

No `CONTEXT.md` exists for Phase 20; the user selected "continue without context". No locked decisions, no discretion areas, no deferred ideas. The user's job description in this prompt is the only spec: "Fix the two regressions that Phase 19 e2e tests caught in the Phase 18 ref-honoring code path." Success criteria (per `19-01-SUMMARY.md`) require all three Phase 19 e2e tests to pass with `EASYP_GH_TOKEN` set, plus the pre-existing `TestSmokeBufModUpdate` to keep passing.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| v1beta1 Name ref wire-parse | API / Backend (connect handler) | — | `parseResourceRefName` lives in `internal/connect/commits_helpers.go` and is invoked only by the v1beta1 handlers (`ServeHTTP` for `CommitService/GetCommits`, `ServeGraph` for `GraphService/GetGraph`) |
| Provider ref-resolution gate (isSHA vs ref) | API / Backend (connect handler) | Providers (github, bitbucket) | The gate is enforced in BOTH the connect-package callers (commits.go:344) AND the per-provider `GetMeta` functions. Phase 18 moved the ref-resolution call from the connect layer into the providers (so each provider can use its native ref-resolution API). |
| Provider commit-fetch API (repos.GetCommit / /commits/{id}) | Providers (github, bitbucket) | — | The actual ref → SHA resolution is provider-specific. The github provider uses `c.repos.GetCommit(ctx, owner, repo, ref, nil)`; the bitbucket provider uses `c.getCommit(ctx, ref)` which calls `GET /commits/{id}`. |
| v1alpha1 ModuleReference path (modulepins.go, bynames.go) | API / Backend (connect handler) | — | v1alpha1 uses a SEPARATE proto with a `ModuleReference.reference` field at field 4 (see `api/_third_party/buf/proto/buf/alpha/module/v1alpha1/module.proto:84-90`). It does NOT go through `parseResourceRefName`. The regressions are confined to the v1beta1 path. |
| Cache writeback (infoCache) | API / Backend (connect handler) | — | `infoCache` is keyed by `"owner/module"` (commits.go:39, 224, 312, 417). Ref-resolution is a one-shot per request; the cache is reused for follow-up requests with the same module identity, but the ref itself is not part of the cache key. |

## Root Cause Analysis

### REGRESSION 1 (v1.30.1 `buf mod update`, no ref → GitHub 422)

**Code path:**

1. buf v1.30.1 calls `buf.registry.module.v1beta1.CommitService/GetCommits` with a `Name` that contains `owner`, `module`, and `label_name="main"` (the default label name). The `ref` field at field 4 is absent.
2. The proxy's `ServeHTTP` (commits.go:99) reads the body via `parseResourceRefs` (commits_helpers.go:134) → `parseResourceRef` (commits_helpers.go:159) → `parseResourceRefName` (commits_helpers.go:180).
3. **`parseResourceRefName` reads field 3 as `ref`** (commits_helpers.go:196). Field 3 in the v1.30.1 wire payload carries `label_name="main"`, so the returned `moduleRef.ref = "main"`.
4. `ServeHTTP` calls `h.api.repo.GetMeta(ctx, "googleapis", "googleapis", "main")` (commits.go:141).
5. In `internal/providers/github/getrepo.go:48-58`, the Phase 18 logic is:
   - `commit = "main"` is non-empty
   - `isSHA("main")` is false
   - → falls into the `else` branch, calls `c.repos.GetCommit(ctx, owner, repoName, "main", nil)`
6. GitHub's `repos.GetCommit` returns 422 for this case (the proxy currently surfaces 502 via `upstreamError` at commits.go:144-147).

**Why Phase 18's old code worked:**

Pre-Phase-18 carve-out (visible at `internal/providers/github/getrepo.go:21-23` in the commit just before `6a49450`):
```go
if commit != "" && commit != "main" {
    meta.Commit = commit
}
```
The `commit != "main"` check silently treated "main" as a HEAD request — the provider never called `repos.GetCommit`, and `meta.Commit` stayed as the default-branch HEAD SHA returned by `getRepo` (github/getrepo.go:132).

**Why the regressions appear now:**

Phase 18 (commit `6a49450`) replaced the carve-out with an `isSHA(commit)` gate (github/getrepo.go:48-58, bitbucket/getrepo.go:43-53). For `commit="main"`, the new gate routes the request to `repos.GetCommit` and GitHub returns 422. The carve-out was removed because the field-number bug above means the proxy was about to start sending "main" to `GetMeta` for v1.30.1 no-ref requests — the field-number fix (see below) makes the carve-out unnecessary, since after the fix v1.30.1 no-ref requests send `ref=""` and the providers' `commit == ""` branch returns HEAD without any ref-resolution API call.

**Concrete file:line for the fix:**

- `internal/connect/commits_helpers.go:196` — change `num == 3` to `num == 4` in the `parseResourceRefName` `else if` arm that captures `ref`.
- `internal/connect/commits_helpers_test.go:379` — change `protowire.AppendTag(name, 3, protowire.BytesType)` to `protowire.AppendTag(name, 4, protowire.BytesType)` so `TestParseResourceRefName_ReadsRef` writes the ref at the correct field number.

### REGRESSION 2 (v1.69.0 ref-pinned → HEAD instead of ref's SHA)

**Code path:**

1. buf v1.69.0 calls `buf.registry.module.v1beta1.GraphService/GetGraph` (or `CommitService/GetCommits`) with a `Name` that contains `owner`, `module`, and `ref="common-protos-1_3_1"` at field 4. The `label_name` field at field 3 is absent.
2. The proxy's `ServeGraph` (commits.go:264) reads the body via `parseGetGraphResourceRefs` (commits_helpers.go:220) → `parseResourceRef` (commits_helpers.go:159) → `parseResourceRefName` (commits_helpers.go:180).
3. **`parseResourceRefName` reads field 3 as `ref`** (commits_helpers.go:196). Field 3 in the v1.69.0 wire payload is absent, so the returned `moduleRef.ref = ""`.
4. `ServeGraph` calls `h.api.repo.GetMeta(ctx, "googleapis", "googleapis", "")` (commits.go:344).
5. The github provider's `GetMeta` (getrepo.go:48) takes the `commit == ""` branch and returns HEAD from `getRepo`. The buf.lock pins to HEAD's UUID.

**Why it breaks:**

The `ref` at field 4 is silently dropped by the parser. After the field-number fix to `num == 4`, the v1.69.0 ref is captured, `GetMeta(ctx, owner, module, "common-protos-1_3_1")` is called, the github provider's `isSHA("common-protos-1_3_1")` is false, and the ref is resolved via `c.repos.GetCommit(ctx, owner, repo, "common-protos-1_3_1", nil)`. The returned SHA is the SHA at the ref.

**Proto evidence:**

`/Users/nil/DiskD/W/Djarvur/easyp-tech.save/api/proto/buf/registry/module/v1beta1/resource.proto:81-93`:
```proto
// If the oneof is present but empty, this should be treated as not present.
oneof child {
  // The name of the Label.
  //
  // If this value is present but empty, this should be treated as not present, that is
  // an empty value is the same as a null value.
  string label_name = 3 [(buf.validate.field).string.max_len = 250];
  // The untyped reference, applying the semantics as documented on the Name message.
  //
  // If this value is present but empty, this should be treated as not present, that is
  // an empty value is the same as a null value.
  string ref = 4;
}
```

Identical structure in `/Users/nil/DiskD/W/Djarvur/easyp-tech.save/api/proto/buf/registry/module/v1/resource.proto:81-93` (v1 schema).

**Concrete file:line for the fix:**

- `internal/connect/commits_helpers.go:196` — same one-line fix as Regression 1.
- `internal/connect/commits_helpers_test.go:379` — same one-line test update as Regression 1.
- Add a new test `TestParseResourceRefName_LabelNameIsField3` in `commits_helpers_test.go` that asserts a Name with `label_name="main"` at field 3 parses to `moduleRef.ref == ""` (the label_name must NOT be read as the ref).

### Single root cause vs two distinct bugs

Both regressions are visible symptoms of the SAME field-number bug. After the fix, both go away because:

- For v1.30.1 no-ref: `ref` parses to `""`, `GetMeta("")` is called, the providers' `commit == ""` branch (github/getrepo.go:48, bitbucket/getrepo.go:43) keeps HEAD from `getRepo`. No `repos.GetCommit` call. No 422.
- For v1.69.0 ref-pinned: `ref` parses to `"common-protos-1_3_1"`, `GetMeta(ctx, owner, module, "common-protos-1_3_1")` is called, the providers' `else` branch (github/getrepo.go:51-57, bitbucket/getrepo.go:47-51) resolves via the ref-fetch API, returns the SHA at the ref.

This is why the fix surface is small: a one-line code change, two test updates, and a new regression-guard test.

### Caching interaction (informational, not a fix target)

`infoCache` is keyed by `"owner/module"` (commits.go:39, 224, 312, 417). The v1.69.0 ref-pinned run and the v1.30.1 no-ref run use different modules in the e2e tests (`googleapis/googleapis` with a different `default_label_name` value in v1.30.1, etc.), so they do not collide. The ref itself is not part of the cache key. After the field-number fix, the cache will be re-populated correctly: a v1.69.0 ref-pinned run will write the ref-resolved SHA into `infoCache["googleapis/googleapis"]`, and a subsequent v1.30.1 run for the same module (with empty ref) will take the cache-hit path in `ServeGraph` (commits.go:312-335) and return the same SHA — which is the SAME behavior the v1.30.1 client expects (it does not distinguish between HEAD's SHA and the ref's SHA when the ref equals the default branch's HEAD, which is the case in the v1.30.1 smoke test where the dep has no `:ref` suffix). The diff test (`TestRefRespected_ModUpdate_DiffersFromHead`) runs two server instances per version (one with no ref, one with `common-protos-1_3_1`), so the per-server `infoCache` never sees both inputs.

## Standard Stack

N/A — this is a bug fix using existing patterns. No new libraries, no new dependencies, no `go.mod` / `go.sum` changes.

## Architecture Patterns

### Pattern 1: Field-number test mirrors production

The unit test `TestParseResourceRefName_ReadsRef` (commits_helpers_test.go:372-389) hand-builds a `Name` proto using `protowire.AppendTag` to set `owner=1`, `module=2`, `ref=3`. The test's job is to verify that the production parser reads the same field numbers. When the production code had a wrong field number, the test had the same wrong field number, so the test passed against the buggy code. **The fix MUST update both the production code AND the test in lockstep.** After the fix:

- `commits_helpers.go:196` reads `num == 4` for `ref`
- `commits_helpers_test.go:379` writes `protowire.AppendTag(name, 4, protowire.BytesType)` for the `ref` value

If only the production code is fixed and the test is not, `TestParseResourceRefName_ReadsRef` will fail because the test-encoded wire payload writes the ref at field 3, which the (now-corrected) production code will skip. Conversely, if only the test is fixed, the production code will continue to read `label_name` from field 3, and the test will fail. Both must move together.

### Pattern 2: Provider ref-resolution gate (isSHA vs ref)

The `isSHA(commit)` gate (github/getrepo.go:48-58, bitbucket/getrepo.go:43-53) is the current shape:

```go
if commit != "" {
    if isSHA(commit) {
        meta.Commit = commit
    } else {
        // resolve via the provider's commit-fetch API
    }
}
```

After the field-number fix, this gate is correct as-is. The empty-commit branch returns HEAD; the isSHA branch returns the raw SHA; the else branch resolves refs via the upstream API. No code change is required to fix the regressions.

**Optional defensive update** (NOT required for the regression fix — see "Open Questions"): Re-introduce the pre-Phase-18 `commit != "main"` carve-out inside the else branch as a fast-path for the default branch:

```go
} else if commit == "main" {
    // no-op: HEAD from getRepo is already correct
} else {
    // resolve via the provider's commit-fetch API
}
```

This is purely defensive — after the field-number fix, no caller path sends "main" to GetMeta. But if a future code change accidentally sends a literal "main" ref, this carve-out preserves the pre-Phase-18 default-branch behavior.

### Pattern 3: Test-side byte-table mirroring for crypto helpers

The e2e test `commitUUIDForTest` in `e2e/ref_test.go:250-270` duplicates the production `commitUUID` byte table to avoid an import cycle. This is the established pattern for e2e tests that need to derive the same UUID the proxy mints. No new test-side mirrors are required for Phase 20 (the field-number fix is in the proto parser, not the UUID minting logic).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Field number lookup | A separate `nameByField` map or similar lookup table | The literal `num == 4` in the existing `else if` arm | The production parser is a one-pass protowire walk (commits_helpers.go:180-215); introducing a lookup table would add overhead and obscure the wire format. The literal `4` is the actual proto field number — any change must touch this exact spot. |
| Provider ref-resolution | A new `ResolveRef` helper that wraps both github and bitbucket | The existing per-provider `getCommit` / `repos.GetCommit` call inside each `GetMeta` | Each provider has a different ref-resolution API (Bitbucket Server `/commits/{id}` JSON, GitHub `repos.GetCommit` via go-github). Cross-provider abstraction would force a least-common-denominator design. |
| "main" handling | A new default-branch-resolution helper | The existing `getRepo` function (github/getrepo.go:65-135) that already returns the default branch's HEAD SHA when called with `commit == ""` | The HEAD fast path is already in place; after the field-number fix, no caller sends "main" to GetMeta, so a separate "main" handler is unnecessary. The pre-Phase-18 carve-out is preserved as an optional defensive update (see Pattern 2). |
| e2e ref-honoring tests | A new e2e test that exercises the field-number fix directly | The existing `e2e/ref_test.go` tests that already exercise the regression path | The Phase 19 e2e tests (`TestRefRespected_*`) already cover both regressions end-to-end. Adding a fourth test would be redundant. |

**Key insight:** The fix is byte-exact. Phase 18 introduced the field-number bug as a literal `3` where the proto says `4`. The fix is a literal `4` where the production code has `3`. No abstraction, no helper, no new pattern.

## Common Pitfalls

### Pitfall 1: Test and production must move together

**What goes wrong:** Updating `commits_helpers.go:196` to `num == 4` without updating `commits_helpers_test.go:379` to also write at field 4. The test will start failing with "parseResourceRefName returned nil" because the production parser now correctly skips the test's field-3 `ref` value.
**Why it happens:** The existing test (commits_helpers_test.go:367-389) was written in lockstep with the buggy production code in Phase 18; both have the same wrong field number.
**How to avoid:** Make the test update in the same commit as the production code change. Verify with `go test ./internal/connect/... -run TestParseResourceRefName -count=1` after the change.
**Warning signs:** `TestParseResourceRefName_ReadsRef` fails with `ref.owner == "" || ref.module == "" || ref.ref == ""`.

### Pitfall 2: Re-introducing the "main" carve-out without realizing it changes the public contract

**What goes wrong:** Adding `else if commit == "main"` to the provider gates as a defensive measure. The carve-out means the proxy silently ignores a ref named "main" if the user explicitly pins a branch named "main" in buf.yaml. In the common case the user IS referring to the default branch, so the behavior is identical, but in the edge case (a user has both a default branch "main" and a non-default branch "main" in the same repo) the carve-out would silently pick the wrong one.
**Why it happens:** The pre-Phase-18 carve-out was load-bearing for a buggy parser; restoring it as a defensive measure is a design change, not a bug fix.
**How to avoid:** Document the carve-out as a deliberate design choice in the provider's godoc. If the carve-out is included, the test (`TestGetMeta_ResolvesRef` for github) must be updated to assert that a ref named "main" is treated as HEAD.
**Warning signs:** The github or bitbucket provider test starts failing because `TestGetMeta_ResolvesRef` uses a non-default ref. (The current test uses `main/v2` as the ref, so it's unaffected by a "main" carve-out, but a future test that uses "main" would break.)

### Pitfall 3: Conflating v1beta1 `Name.ref` (field 4) with v1alpha1 `ModuleReference.reference` (field 4)

**What goes wrong:** A future change assumes the v1alpha1 path (`modulepins.go:46`, `bynames.go:70`) and the v1beta1 path (`parseResourceRefName`) read the same field. They do NOT — v1alpha1 uses a different proto message (`module.ModuleReference.reference` at field 4, see `api/_third_party/buf/proto/buf/alpha/module/v1alpha1/module.proto:84-90`) and a different parser (the generated proto code via `connect-go`, not a hand-rolled protowire walk). Phase 18 did NOT modify the v1alpha1 path.
**Why it happens:** Both v1alpha1 and v1beta1 happen to put "ref" at field 4, but they are different protos with different message names.
**How to avoid:** Keep the v1alpha1 path (`modulepins.go`, `bynames.go`) untouched. The Phase 20 fix is confined to `commits_helpers.go:196` and the matching test.
**Warning signs:** Any test that touches `modulepins.go` or `bynames.go` regresses.

### Pitfall 4: Provider 422 is a symptom, not a cause

**What goes wrong:** Treating GitHub's 422 as the bug to fix. Adding a 422 handler to the github provider that maps 422 to HEAD.
**Why it happens:** The 422 is the visible error in the Phase 19 e2e test logs.
**How to avoid:** The 422 is the symptom of the proxy calling `repos.GetCommit("main")` with a default-branch name. The fix is to stop sending "main" to GetMeta, not to handle the 422 specially. After the field-number fix, the proxy never sends "main" to GetMeta (the v1.30.1 no-ref request sends `ref = ""`, which takes the empty-commit branch and returns HEAD without calling `repos.GetCommit`).
**Warning signs:** Provider code starts to special-case 422 responses.

### Pitfall 5: Bitbucket parity

**What goes wrong:** Updating only the github provider's `isSHA` gate (or only the bitbucket provider's). Phase 18 changed both providers in lockstep; Phase 20 must keep the lockstep.
**Why it happens:** The two providers have separate `isSHA` helpers (github/getrepo.go:19-29, bitbucket/getrepo.go:16-26) and separate ref-resolution code paths. The v1beta1 path through `parseResourceRefName` is provider-agnostic; both providers receive the same `moduleRef.ref` and apply the same gate.
**How to avoid:** Any change to one provider's gate MUST be mirrored to the other. If a defensive `commit == "main"` carve-out is added, it goes in both.
**Warning signs:** Provider tests pass for github but fail for bitbucket (or vice versa).

### Pitfall 6: e2e tests can't be run in this environment

**What goes wrong:** The Phase 19 e2e tests require `EASYP_GH_TOKEN` and cached buf binaries under `testdata/buf/`. Neither is available in this environment. The fix cannot be verified end-to-end in this session.
**Why it happens:** The Phase 19 e2e tests are gated on `testutil.RequireEnvToken` and skip cleanly without the token. Token-less CI exits 0 with SKIP lines.
**How to avoid:** The fix MUST be verified by unit + integration tests in `internal/connect/` and `internal/providers/`. The e2e tests MUST be confirmed to compile + list + skip cleanly. The "actual fix" is verified manually with a token and cached binaries (out of scope for this environment).
**Warning signs:** `go test ./e2e/ -list 'TestRefRespected.*'` does not list exactly 3 tests. `go test ./e2e/ -run TestRefRespected -count=1` exits non-zero. `go build ./e2e/...` fails.

## Code Examples

### Pre-Phase-18 carve-out (visible in commit `bbaac3f^`, removed in commit `6a49450`)

`internal/providers/github/getrepo.go` (before commit `6a49450`):
```go
if commit != "" && commit != "main" {
    meta.Commit = commit
}
```

`internal/providers/bitbucket/getrepo.go` (before commit `6a49450`):
```go
if commit != "" && commit != "main" {
    meta.Commit = commit
}
```

### Current buggy production code (commits_helpers.go:196)

```go
} else if num == 3 && typ == protowire.BytesType {
    // buf BSR Name.ref (branch/tag). Optional: older buf clients
    // do not send it; the ref-aware code paths tolerate an empty
    // value (treated as HEAD by the providers).
    v, mLen := protowire.ConsumeBytes(msg)
    msg = msg[mLen:]
    ref = string(v)
}
```

The comment is misleading: field 3 in the buf proto is `label_name`, not `ref`. Field 4 is `ref`.

### Correct proto (resource.proto)

`/Users/nil/DiskD/W/Djarvur/easyp-tech.save/api/proto/buf/registry/module/v1beta1/resource.proto:81-93`:
```proto
// If the oneof is present but empty, this should be treated as not present.
oneof child {
  // The name of the Label.
  //
  // If this value is present but empty, this should be treated as not present, that is
  // an empty value is the same as a null value.
  string label_name = 3 [(buf.validate.field).string.max_len = 250];
  // The untyped reference, applying the semantics as documented on the Name message.
  //
  // If this value is present but empty, this should be treated as not present, that is
  // an empty value is the same as a null value.
  string ref = 4;
}
```

The same shape appears at `/Users/nil/DiskD/W/Djarvur/easyp-tech.save/api/proto/buf/registry/module/v1/resource.proto:81-93` (v1 schema).

### Current buggy test (commits_helpers_test.go:372-389)

```go
// Build a Name { owner=1, module=2, ref=3 } message.
var name []byte
name = protowire.AppendTag(name, 1, protowire.BytesType)
name = protowire.AppendString(name, "cyp")
name = protowire.AppendTag(name, 2, protowire.BytesType)
name = protowire.AppendString(name, "cyp-net-listeners")
name = protowire.AppendTag(name, 3, protowire.BytesType)  // BUG: should be 4
name = protowire.AppendString(name, "main/v2")
```

The test passes against the buggy code because both the test and the production code use the wrong field number. After the fix, the test's field 3 must change to field 4 to keep passing.

### Provider isSHA gate (current)

`internal/providers/github/getrepo.go:48-58`:
```go
if commit != "" {
    if isSHA(commit) {
        meta.Commit = commit
    } else {
        rc, _, err := c.repos.GetCommit(ctx, owner, repoName, commit, nil)
        if err != nil {
            return meta, fmt.Errorf("resolving ref %q: %w", commit, err)
        }
        meta.Commit = rc.GetSHA()
    }
}
```

`internal/providers/bitbucket/getrepo.go:43-53`:
```go
if commit != "" {
    if isSHA(commit) {
        meta.Commit = commit
    } else {
        resolved, err := c.getCommit(ctx, commit)
        if err != nil {
            return meta, fmt.Errorf("resolving ref %q: %w", commit, err)
        }
        meta.Commit = resolved
    }
}
```

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | `go test` (stdlib) + `testify/require` for e2e testutil |
| Config file | none (Go test config is implicit) |
| Quick run command | `go test ./internal/connect/... ./internal/providers/... -count=1` |
| Full suite command | `go test ./... -count=1` (includes e2e/) |

### Phase Requirements → Test Map

Phase 20 has no `REQUIREMENTS.md` of its own. The success criteria are derived from Phase 19's `19-01-SUMMARY.md`:

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SC-19-1 (already met by Phase 19 tests) | `buf mod update` ref-pinned run pins to a different commit than no-ref run, for all cached buf versions | e2e | `go test ./e2e/ -run TestRefRespected_ModUpdate_DiffersFromHead -count=1` (requires token) | yes (e2e/ref_test.go:55) |
| SC-19-2 (already met by Phase 19 tests) | v1.69.0 ref-pinned lock matches upstream SHA at the ref | e2e | `go test ./e2e/ -run TestRefRespected_ModUpdate_MatchesUpstreamSHA -count=1` (requires token) | yes (e2e/ref_test.go:114) |
| SC-19-3 (already met by Phase 19 tests) | v1.69.0 `buf dep update` ref-pinned differs from no-ref | e2e | `go test ./e2e/ -run TestRefRespected_DepUpdate_DiffersFromHead -count=1` (requires token) | yes (e2e/ref_test.go:157) |
| HAND-02 (Phase 3) | `TestSmokeBufModUpdate` passes for both v1.30.1 and v1.69.0 | e2e | `go test ./e2e/ -run TestSmokeBufModUpdate -count=1` (requires token) | yes (e2e/smoke_test.go:12) |
| Phase 20-1 (new) | `parseResourceRefName` reads field 4 (ref), not field 3 (label_name) | unit | `go test ./internal/connect/ -run TestParseResourceRefName -count=1` | yes (commits_helpers_test.go:372) — needs update |
| Phase 20-2 (new) | `label_name` at field 3 is NOT read as `ref` | unit | `go test ./internal/connect/ -run TestParseResourceRefName -count=1` | NO — new test required |
| Phase 20-3 (new, optional) | `GetMeta("main")` does not call `repos.GetCommit` (HEAD fast path) | unit (provider) | `go test ./internal/providers/github/ -run TestGetMeta -count=1` | NO — new test required (only if defensive carve-out is added) |
| Phase 20-4 (new, optional) | bitbucket `getMeta("main")` does not call `/commits/main` | unit (provider) | `go test ./internal/providers/bitbucket/ -run TestGetMeta -count=1` | NO — new test required (only if defensive carve-out is added) |

### Sampling Rate

- **Per task commit:** `go test ./internal/connect/... ./internal/providers/... -count=1` (the regression-guard unit + integration tests)
- **Per wave merge:** `go test ./... -count=1` (full suite, including e2e skip-cleanly checks)
- **Phase gate:** Full suite green + manual e2e run with `EASYP_GH_TOKEN` set, before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `commits_helpers_test.go:379` — change `protowire.AppendTag(name, 3, protowire.BytesType)` to `protowire.AppendTag(name, 4, protowire.BytesType)` so `TestParseResourceRefName_ReadsRef` writes the ref at the correct field number
- [ ] `commits_helpers_test.go` (new test) — `TestParseResourceRefName_LabelNameIsField3` that asserts a Name with `label_name="main"` at field 3 parses to `moduleRef.ref == ""`
- [ ] `commits_helpers.go:196` — change `num == 3` to `num == 4`
- [ ] (Optional, only if defensive carve-out is added) `github/getrepo_test.go` — `TestGetMeta_MainRef` asserting `GetMeta("main")` returns HEAD without calling `repos.GetCommit`
- [ ] (Optional, only if defensive carve-out is added) `bitbucket/getrepo_test.go` — `TestGetMeta_MainRef` asserting `getMeta("main")` returns HEAD without calling `/commits/main`

If the defensive carve-out is NOT added, only the first three gaps are required. The "no new test" path is the minimum viable fix.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V5 Input Validation | yes | The Phase 18 `isSHA(commit)` gate is the input validation: 40/64 lowercase hex passes through, anything else is treated as a ref and routed through the provider's commit-fetch API. The Phase 20 field-number fix ensures that the `ref` field actually carries the user's input rather than the unrelated `label_name` field. |
| V6 Cryptography | no | The fix is a wire-protocol field-number correction. No crypto is involved. |
| V2 Authentication, V3 Session Management, V4 Access Control | no | The fix is in the v1beta1 Name parser; auth/session/access are handled at the request level, not the field-parse level. |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Adversarial ref injection | Tampering | After the field-number fix, the `ref` field is correctly bound to the buf client's `Name.ref`. The provider's ref-resolution API call (Bitbucket `/commits/{id}`, GitHub `repos.GetCommit`) accepts ref names and short SHAs, both of which are safe to route to upstream. The 7-char minimum SHA support in `repos.GetCommit` means a malicious short SHA cannot be turned into a false 40-char SHA. No new attack surface is introduced. |
| Provider 422 / 5xx amplification | Denial of Service | The Phase 19 422 (from `repos.GetCommit("main")`) is a one-off error per request, not a tight loop. The Phase 18 `probeSem` cap (commits.go:65) bounds concurrent upstream fan-out. No new DoS surface is introduced. |

## Sources

### Primary (HIGH confidence)

- `internal/connect/commits_helpers.go:180-215` — production `parseResourceRefName` with the `num == 3` bug
- `internal/connect/commits_helpers_test.go:367-409` — unit tests for `parseResourceRefName` (both pin the wrong field number)
- `internal/connect/commits.go:99-258, 264-482` — ServeHTTP and ServeGraph paths that call `parseResourceRefName` then `GetMeta`
- `internal/connect/bynames.go:64-88` — v1alpha1 `GetRepositoryByFullName` path (uses generated proto, not `parseResourceRefName`)
- `internal/connect/modulepins.go:45-57` — v1alpha1 `GetModulePins` path (uses generated proto, not `parseResourceRefName`)
- `internal/providers/github/getrepo.go:31-61, 65-135` — github provider `GetMeta` and `getRepo`
- `internal/providers/bitbucket/getrepo.go:28-56, 91-109` — bitbucket provider `getMeta` and `getRepo`
- `internal/providers/github/getrepo_test.go` — github provider tests (3 subtests, all green today; they use a 40-char SHA and `main/v2` ref, neither of which is affected by the field-number fix)
- `internal/providers/bitbucket/getrepo_test.go` — bitbucket provider tests (4 subtests, same)
- `internal/providers/github/mockrepos_test.go:84-96` — `WithGetCommit` and `WithGetCommitError` for asserting whether `repos.GetCommit` was called
- `e2e/ref_test.go:55-192` — three `TestRefRespected_*` e2e tests that caught the regression
- `e2e/testutil/server.go:96-202` — `RunBufModUpdate` / `RunBufDepUpdate` / `RunBufModUpdateWithRef` / `RunBufDepUpdateWithRef` / `runBufUpdate` helpers
- `e2e/ref_test.go:250-270` — `commitUUIDForTest` e2e-side byte-table mirror
- `e2e/smoke_test.go:12-46` — pre-existing `TestSmokeBufModUpdate` that regresses for v1.30.1
- `/Users/nil/DiskD/W/Djarvur/easyp-tech.save/api/proto/buf/registry/module/v1beta1/resource.proto:81-93` — buf v1beta1 `Name.child` oneof: `label_name=3, ref=4`
- `/Users/nil/DiskD/W/Djarvur/easyp-tech.save/api/proto/buf/registry/module/v1/resource.proto:81-93` — buf v1 `Name.child` oneof: `label_name=3, ref=4` (identical shape)
- `api/_third_party/buf/proto/buf/alpha/module/v1alpha1/module.proto:84-90` — buf v1alpha1 `ModuleReference.reference` at field 4 (different proto, different parser; not affected by this fix)
- `git log 6a49450 -- internal/connect/commits_helpers.go internal/providers/github/getrepo.go internal/providers/bitbucket/getrepo.go` — Phase 18 commit that introduced the bug and removed the `commit != "main"` carve-out
- `git log bbaac3f^ -- internal/providers/github/getrepo.go internal/providers/bitbucket/getrepo.go` — pre-Phase-18 code with the `commit != "main"` carve-out intact

### Secondary (MEDIUM confidence)

- `.planning/phases/19-we-need-e2e-tests-for-the-ref-specified-for-dependency-looks/19-01-SUMMARY.md:84` — the canonical description of both regressions (SC-19-1/2/3 met, but proxy regressions discovered)
- `.planning/phases/18-respect-buf-yaml-dependency-refs-not-always-head-fix-related/18-RESEARCH.md:22-33, 38-46, 96-98` — Phase 18's research note on the ref-parsing gap and the `commit != "main"` carve-out (verifies that the carve-out was deliberately removed, not accidentally)

### Tertiary (LOW confidence)

- None. All claims in this research are verified against the source files or the proto files. There are no `[ASSUMED]` claims.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new libraries; the fix uses existing patterns
- Architecture: HIGH — single root cause verified against the proto schema and the production code
- Pitfalls: HIGH — the test/production lockstep pitfall is verified by reading both files; the v1alpha1 path is verified by reading `modulepins.go` and `bynames.go`; the provider 422-as-symptom pitfall is verified by reading the github `GetMeta` logic

**Research date:** 2026-07-07
**Valid until:** 2026-08-07 (the buf v1beta1 schema is stable; the field numbers in `Name.child` are unlikely to change in this timeframe)

## Open Questions

1. **Should the defensive `commit == "main"` carve-out be re-introduced in the providers?**
   - **What we know:** After the field-number fix, no caller path sends `commit = "main"` to `GetMeta`. The v1.30.1 no-ref request sends `ref = ""` (the empty-commit branch returns HEAD). The v1.69.0 ref-pinned request sends `ref = "common-protos-1_3_1"` (the ref-resolution branch). The `repos.GetCommit("main")` 422 only happens today because the field-number bug makes the proxy send "main" instead of "".
   - **What's unclear:** Is the user (or the code reviewer) comfortable with the providers' current behavior of routing a literal `"main"` ref to `repos.GetCommit`? The pre-Phase-18 carve-out was deliberately removed in Phase 18 because it was load-bearing for the buggy parser; restoring it as a defensive measure changes the public contract (a user with a non-default branch named "main" would get HEAD instead of the ref's SHA).
   - **Recommendation:** Skip the defensive carve-out. The field-number fix alone resolves both regressions, and the carve-out adds design complexity (silent fallback for "main") without a clear benefit. The planner should NOT add the carve-out unless the user requests it explicitly. If the planner decides to add it anyway, the test updates in `github/getrepo_test.go` and `bitbucket/getrepo_test.go` are listed in the "Wave 0 Gaps" section.

2. **Does the v1beta1 path also need to handle `label_name` explicitly?**
   - **What we know:** The `Name.child` oneof contains `label_name` (field 3) and `ref` (field 4). The buf documentation says `label_name` is the name of a Label (e.g., "stable", "latest") that resolves to a specific commit. The proxy currently has no logic to resolve a `label_name` to a SHA.
   - **What's unclear:** Should the proxy treat a `Name` with a `label_name` (and no `ref`) as a ref to resolve? The buf CLI can send a `label_name` in the `Name` (e.g., when a user pins `buf.build/googleapis/googleapis:stable` in buf.yaml). Today's behavior: the proxy's `parseResourceRefName` reads field 3 as `ref` (the bug), so it tries to resolve "stable" as a ref. After the field-number fix, the proxy reads field 4 (the absent ref) as `""`, takes the HEAD fast path, and ignores the `label_name`.
   - **Recommendation:** This is OUT OF SCOPE for Phase 20. The buf CLI can send `label_name` in the `Name`, but Phase 20's regressions are confined to v1.30.1 sending `label_name="main"` and v1.69.0 sending `ref="common-protos-1_3_1"`. The v1.30.1 case is a no-ref request (the user has no `:ref` suffix in buf.yaml), and the proxy's HEAD fast path is the correct behavior. The v1.69.0 case is a ref-pinned request, and the proxy's ref-resolution path is the correct behavior. A future phase may need to add `label_name` resolution, but that's a feature, not a bug fix.

3. **Are there other call sites in the project that read proto field 3 from a `Name` message?**
   - **What we know:** A grep for `num == 3` in `internal/connect/` returns exactly one hit: `commits_helpers.go:196`. The other `num == X` arms in the same file (commits_helpers.go:142, 166, 188, 192, 228, 259, 284, 362, 376, 401, 405) are all for different message types (`ResourceRef`, `GetGraphRequest`, `ModuleRef`, `Name` in the v1alpha1 / non-`child` context), not the `Name.child` oneof.
   - **What's unclear:** None. The grep is exhaustive.
   - **Recommendation:** No additional call sites need updating. The single fix at `commits_helpers.go:196` is the only one.
