# Phase 17: Fix PR #37 review findings - Research

**Researched:** 2026-07-07
**Domain:** Bugfix / refactor in v1beta1 commit-id error-path wiring + 64-char SHA-256 input acceptance + test-only fixture move
**Confidence:** HIGH (all claims are verifiable against source files; no external library research needed)

## Summary

Phase 17 is a tightly-scoped review-response bugfix that does not introduce new behavior or new dependencies. The scope is fully captured in `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/review.md` (7 findings, ranked most-severe-first) and the success criteria in `ROADMAP.md:155-176` (5 SCs). The 7 findings reduce to **3 actual code changes** plus **2 supporting test moves**:

1. **Routing of `computeB4Digest` errors** through `upstreamError`/`logHandlerError` (recover 502 + full ERR-05 attributes for upstream failures) — finding #1.
2. **Removal of the `internalError` helper** and replacement of its 5 call sites with `h.logHandlerError(...)` calls (recover ERR-05, automatic `error_class=internal` for 500s) — finding #2. Inherits finding #1's digest-error routing.
3. **Acceptance of 64-char SHA-256 inputs in `commitUUID`** (drop the strict `len == 40` check, decode and use the first 20 bytes) plus a new test for the SHA-256 path — finding #3, finding #6.
4. **Move `preResolveForTest`** from `commits_helpers.go` to `commits_helpers_test.go` — finding #7.
5. **Generalize the 400 not-found message** so it covers both stale-lockfile and foreign-id miss classes — finding #4.

Findings #5 (redundant `commitUUID` inside `computeB4Digest`) and #6 (probe/register cache-contract divergence) are noted as "cleanups" in the review verdict (review.md:23) and are **not** called out in the ROADMAP success criteria; the planner should treat them as out-of-scope unless the user re-prioritizes.

**Primary recommendation:** One plan with four tightly-coupled tasks. The four tasks share the call-site refactor pattern (helper removal) and the test-move pattern (test-only fixture). Splitting them into separate plans adds ceremony without value because finding #2's helper removal forces a re-shuffle of every call site that finding #1 also wants to change.

## User Constraints (from ROADMAP.md success criteria + review.md)

### Locked Decisions (ROADMAP.md:161-166)

The 5 success criteria are locked and unambiguous:

1. **`computeB4Digest` failures routed through `logHandlerError`/`upstreamError` (not `internalError`)** with full context (owner, module, repo, commit, request_id, server, protocol, status) and 502 for upstream failures — restores ERR-05. [VERIFIED: ROADMAP.md:162]
2. **`internalError` helper removed**; all handler-level 500s flow through existing `logHandlerError` (commits.go:777). [VERIFIED: ROADMAP.md:163]
3. **`commitUUID` accepts 40-char (SHA-1) AND 64-char (SHA-256) hex**; Bitbucket SHA-256 repos must not 500. A unit test covers both lengths. [VERIFIED: ROADMAP.md:164]
4. **A new test in `commits_helpers_test.go`** exercises the SHA-256 path; `preResolveForTest` (commits_helpers.go:60-70) is moved out of production source. [VERIFIED: ROADMAP.md:165]
5. **The 400 not-found response message remains generic enough** to apply to both stale-lockfile and genuine foreign-id miss cases. [VERIFIED: ROADMAP.md:166]

### Locked Decisions (review.md:23 verdict)

> "Findings #1-#3 are worth fixing before merge — #1 and #2 undo the phase's own logging-quality work, #3 is a latent provider-compat regression. #4-#7 are cleanups that can land separately."

[VERIFIED: review.md:23]

This is the user's explicit priority ordering. Findings #1-#3 are mandatory; findings #4-#7 are nice-to-haves. However, the ROADMAP success criteria explicitly call out #4 (the 400 message), #6 (SHA-256 path test), and #7 (move `preResolveForTest`), so those are also mandatory by SC. Finding #5 (redundant `commitUUID` call in `computeB4Digest`) is the only one that is truly out-of-scope per the verdict — but see the **Cross-cutting constraints** section below: removing `internalError` (SC #2) automatically changes the structure around `computeB4Digest`, so this finding may be addressed as a side-effect.

### Claude's Discretion

- The exact text of the generalized 400 message (review.md:13-14 suggests "keep a generic tail or split the message by miss-class"). Recommendation: drop the "re-run buf mod update / buf dep update" tail and use a single generic phrase like "unknown commit id: re-resolve via `buf mod update` / `buf dep update` / `buf registry module pin`" — this nudges the operator at the three known recovery actions without misleading foreign-id cases into chasing a stale lockfile.
- Whether to introduce a `validateSHA(sha string) error` helper (carried over from Phase 16 D-05) for the input contract in `commitUUID`. Recommendation: keep inline `hex.DecodeString` + length check; the helper is one line and adds an import.
- Whether to fold finding #5 (pass `cid` into `computeB4Digest` instead of re-deriving) into the plan. Recommendation: yes — once `internalError` is removed and the digest-error path is `upstreamError` (502), the redundant `commitUUID` call inside `computeB4Digest` becomes a more obvious dead-path and costs 2 allocations per served GetCommits/GetGraph. It's a 1-line change to thread `cid` through, and it eliminates the "two commitUUID calls per request" cost that the existing `registerResolved` body comment (commits.go:901-912) is already wrestling with.

### Deferred Ideas (OUT OF SCOPE per ROADMAP + review)

- The 14-of-20-byte collision surface — explicitly accepted in CONTEXT.md (D-01/D-02), 2^112 space, not actionable. [VERIFIED: review.md:21]
- The probe/register cache-contract divergence (review.md:17-18, finding #6) — verdict marks it as "Real-world impact is low"; not in ROADMAP SC. [VERIFIED: review.md:17-18]

## Scope Confirmation — review.md vs. source

I read every line reference in the review and verified them against the actual source. **All 7 findings' line numbers and code claims are accurate** as of the current branch state. Detailed verification below.

### Finding #1: `computeB4Digest` errors mislabeled as `commitUUID` failures

**Review claim:** `commits.go:175-176` and `commits.go:354-357` route `computeB4Digest` failures through `internalError`, logging at Warn with "internal: commitUUID failure" and returning 500. [CITED: review.md:7]

**Verified:**
- `computeB4Digest` (commits.go:744-761) returns errors from three sources:
  - `GetFiles` (line 745) [VERIFIED: commits.go:745] — upstream error
  - `computeB4DigestFromFiles` (line 749) [VERIFIED: commits.go:749] — local computation error (shake256, vanishingly rare)
  - `commitUUID` (line 753) [VERIFIED: commits.go:753] — strict-input contract violation
- Both call sites (ServeHTTP line 174-177 [VERIFIED: commits.go:174-177], ServeGraph line 354-357 [VERIFIED: commits.go:354-357]) call `h.internalError(w, r, meta.Commit, err.Error())` with no distinction between the three error sources.
- `internalError` (commits.go:100-106) hardcodes the log message "internal: commitUUID failure" and returns 500. [VERIFIED: commits.go:100-106]

**Fix surface:** At commits.go:174-177 and commits.go:354-357, the call becomes:
- If `err` originates from `GetFiles` or `computeB4DigestFromFiles` (lines 745, 749), call `h.upstreamError(r, w, fmt.Sprintf("digest for %s/%s", ref.owner, ref.module), slog.String("owner", ref.owner), slog.String("module", ref.module), slog.String("repo", ref.module), slog.String("commit", commit), slog.String("upstream_error", err.Error()))` → 502.
- If `err` originates from the `commitUUID` check (line 753), call `h.logHandlerError(r, w, "internal error", http.StatusInternalServerError, slog.String("commit_id", commit), slog.String("upstream_error", err.Error()))` → 500 with `error_class=internal` set automatically by `errorClass(500)`.

**Implementation choice:** The two failure classes need to be distinguished. Two options:
- (a) **Propagate a typed error from `computeB4Digest`** — wrap the `commitUUID` failure in a sentinel like `var errCommitUUIDContract = errors.New("commitUUID contract violation")` and have callers `errors.Is(err, errCommitUUIDContract)` to choose 500-vs-502.
- (b) **Split `computeB4Digest` into two helpers** — `computeB4DigestFromFiles(files, commit)` (no commitUUID) and a thin wrapper that adds commitUUID. Callers call the wrapper for the contract-violation case and the helper directly for the upstream case.

**Recommendation:** Option (a) (typed sentinel) — it preserves the existing call-site shape, costs one `errors.Is` per call, and keeps the `computeB4Digest` body untouched. Option (b) is a wider refactor that changes the helper's contract.

**Test surface:** No direct test for `computeB4Digest`'s error classification exists. The closest tests are `TestBadRequest_OnUnknownCommitID` (api_test.go:562) and `TestServeDownload_UnknownCommitID_ReturnsBadRequest` (uuid_format_test.go:180) which assert the 400 path; a new test that exercises a `GetFiles` failure (mock provider with a forced error) and asserts a 502 with `error_class=upstream` would be the right shape, but this is **not** in the ROADMAP SCs — planner can add as bonus or skip.

### Finding #2: `internalError` reimplements `logHandlerError` and drops attributes

**Review claim:** `internalError` (commits.go:100-106) bypasses `logHandlerError` (commits.go:777), drops `server`/`protocol`/`request_id`/`status`/`error`, and downgrades `LevelError`→`LevelWarn`. [CITED: review.md:9]

**Verified:**
- `logHandlerError` (commits.go:777-800) emits `slog` at `LevelError` for 5xx, `LevelWarn` for 4xx, with attrs `server`, `protocol`, `request_id`, `error`, `status`, `error_class` always present. [VERIFIED: commits.go:777-800]
- `errorClass(code)` (commits.go:807-816) returns `"internal"` for 500 (default branch). [VERIFIED: commits.go:807-816]
- `internalError` (commits.go:100-106) uses `h.hlog(r)` (which carries `request_id` automatically per the hlog doc at commits.go:77-84) but does NOT set `server`, `protocol`, `status`, or `error` — and uses `LevelWarn` unconditionally. [VERIFIED: commits.go:100-106]

**Verified — the param-name vs. call-site mismatch claim:**
- `internalError` parameter is named `commitID` (commits.go:100). [VERIFIED: commits.go:100]
- All 5 call sites pass `meta.Commit` (a raw 40-char git SHA, not the buf-issued UUID the client sent): lines 160, 176, 351, 356, 668. [VERIFIED: commits.go:160, 176, 351, 356, 668]

The review is **correct** that this is a logging-fidelity bug: an operator correlating a 500 log line with a client-side "unknown commit id <uuid>" finds a `commit_id` in the log that does NOT match the uuid the client sent. It matches the upstream SHA, not the buf-issued UUID.

**Fix surface:** Delete `internalError` (commits.go:94-106). Replace the 5 call sites with `h.logHandlerError(r, w, "internal error", http.StatusInternalServerError, slog.String("commit_id", commitID), slog.String("upstream_error", upstreamErr))` where `commitID` is the buf-issued UUID (`cid`), not `meta.Commit`.

**Critical implementation detail:** The 5 call sites currently pass `meta.Commit` (a 40-char SHA). The new call should pass `cid` (the 32-char UUID the proxy just minted, or will mint). For the digest-error paths (lines 176, 356) `cid` is in scope (already declared on lines 158, 349 respectively). For the `commitUUID`-error paths at lines 160, 351, `cid` was never minted — pass `commitID` (the buf-issued UUID) as a string-literal placeholder, or omit the attr and accept the log line without it. **Recommendation:** always pass `cid` after the success path; on the error path, the value to log is the `meta.Commit` (the bad input the proxy received from upstream), which is what an operator actually wants to see — keep `meta.Commit` as the `commit_id` value in the log when the call fails. (The review's claim is correct in spirit: the existing code logs the bad input; what we lose by switching to `cid` is the bad-input signal. The trade-off is operator-correlation vs. bad-input-visibility. The review's recommendation in review.md:9 is to pass the buf-issued UUID; this matches the existing SC-3 of Phase 16 which says the 400 message logs the buf-issued `commit_id`. Apply the same convention to the 500 path.)

**Test surface:** No direct test for `internalError` exists. The closest is the 400 message tests in `api_test.go:597` and `uuid_format_test.go:215`. A new test that forces a `commitUUID` failure and asserts the log line contains `error_class=internal` + `status=500` + `request_id=<hex>` + `protocol=v1` (or v1beta1) would catch a regression. **Planner note:** writing a `commitUUID` failure test requires injecting a non-hex input — the existing mock provider only returns 40-char hex; the test will need a custom mock. Add as a follow-up if straightforward, or skip — the 400 tests already cover the convention and the 500 path is structurally identical.

### Finding #3: `commitUUID` strict 40-char check breaks Bitbucket SHA-256 repos

**Review claim:** `commits_helpers.go:39` (`len != 40` check) rejects 64-char SHA-256 commits that Bitbucket Server returns from SHA-256-enabled repos. [CITED: review.md:11]

**Verified:**
- `commitUUID` (commits_helpers.go:38-58) does `if len(gitSHA) != 40 { return "", errors.New(...) }` at line 39-41. [VERIFIED: commits_helpers.go:39-41]
- `bitbucket/getrepo.go:40` does `out.Commit = repo.LatestCommit` — the `LatestCommit` field is a `string` (getrepo.go:51) and is whatever the Bitbucket Server API returns. For SHA-256-enabled repos this is 64 hex chars. [VERIFIED: bitbucket/getrepo.go:40, 51]
- The `repoInfo` struct's `LatestCommit` JSON tag binds to whatever the Bitbucket Server API emits. [VERIFIED: bitbucket/getrepo.go:47-54]
- No build-tag guard isolates Bitbucket-specific code in `commits.go`; the strict check fires for any provider that returns >40 hex chars.

**Fix surface:** In `commitUUID` (commits_helpers.go:38-58), change the length validation to accept `len == 40 || len == 64`, then `hex.DecodeString` the input, then use the first 20 bytes of the decoded result for the existing byte-table.

**Concretely:**
- Drop the `len(gitSHA) != 40` check; replace with a check that rejects anything not 40 or 64 hex chars.
- The existing byte-table reads `sha[0:6]`, `sha[6]`, `sha[7:14]` — these read positions 0-13 (14 bytes) of the decoded 20-byte SHA-1 binary. For a 64-char SHA-256 input, `hex.DecodeString` returns 32 bytes; reading positions 0-13 still works (they're within the 32-byte buffer).
- The doc comment (commits_helpers.go:16-37) needs updating: it currently says "Input contract: the input must be exactly 40 lowercase hex characters (the standard full-length git SHA-1 representation). Anything else returns ("", error)." — change to "Input contract: the input must be exactly 40 or 64 lowercase hex characters (the full-length git SHA-1 or SHA-256 representation). Anything else returns ("", error)."
- The error message string `"commitUUID: input is not 40 lowercase hex characters"` (line 40, 44) needs updating too.

**Test surface:** Add a `TestCommitUUID_SHA256_KnownSHA` (or similar) to `commits_helpers_test.go` covering at least:
- A representative 64-char hex input (e.g., the all-zeros and all-ones SHA-256 fills: `0000000000000000000000000000000000000000000000000000000000000000` → some UUID, `ffff...ffff` → some UUID).
- One "first 14 bytes differ from SHA-1 version" assertion to lock in that the function actually consumes SHA-256 bytes (i.e., `commitUUID(40charA) != commitUUID(64charB)` when the first 14 bytes of A and B differ).
- The "40-char still works" regression — already covered by the existing 5 test cases, but worth a comment.

**Note on the byte-table for SHA-256 inputs:** The current function reads `sha[0:14]` (bytes 0-13 of the decoded binary). For a 40-char SHA-1, `hex.DecodeString` returns 20 bytes, and the 14-byte read is well within bounds. For a 64-char SHA-256, `hex.DecodeString` returns 32 bytes, and the same 14-byte read is also within bounds. **The byte-table does not need to change** — only the length validation and the error message.

**Note on the inverse-recovery test (commits_helpers_test.go:194-234):** The current test feeds 4 SHAs and asserts `recovered[0..13] == shaBytes[0..13]`. For SHA-256 inputs the test should also assert `recovered[0..13] == shaBytes[0..13]` — the inverse is well-defined (still first 14 bytes). Add a sub-case with 64-char input.

**Test fixture compatibility:** `preResolveForTest` (commits_helpers.go:60-70) pads to 40 chars and truncates >40. After the move to `_test.go` (finding #7), it does not need to change behavior — the SHA-256 path is exercised directly with 64-char inputs, not via `preResolveForTest`.

### Finding #4: 400 message overfits to "stale lockfile" miss class

**Review claim:** The 400 message `"unknown commit id: re-run buf mod update / buf dep update"` (commits.go:582) misleads operators for genuinely foreign ids from other registries — `resolveForeignCommitID` (commits.go:832-848) documents the miss case includes foreign ids. [CITED: review.md:13]

**Verified:**
- The 400 message is at commits.go:582 [VERIFIED: commits.go:582].
- The 400 message is also referenced in a comment at commits.go:399 [VERIFIED: commits.go:399].
- `resolveForeignCommitID` (commits.go:832-848) — the doc comment says "It is called by ServeDownload when commitMap lookup misses, before falling back to a 400." and "Returns nil when the module identity cannot be recovered." [VERIFIED: commits.go:832-880]
- The 400 path is reached when:
  1. `commitMap[commitID]` misses (commits.go:486-490) — the client sent an id this proxy never minted.
  2. `resolveForeignCommitID(commitID)` returns nil (commits.go:535-547) — could not map to a known module (multi-module deployment, or single-module with prior alias, or commitID is empty).
  3. `probeCommitID` returns no hit (commits.go:549-577) — the sha is not in any configured source.

The three miss sub-cases map to different recovery actions:
- (1) The client cached a stale buf.lock id from an old proxy version → `buf mod update` / `buf dep update` re-resolves.
- (2)+(3) with a foreign id (e.g., real buf.build) → `buf mod update` / `buf dep update` also re-resolves; the recovery command is the same.
- A foreign id from a third registry the operator has no relationship with → no recovery action works; the message should not pretend otherwise.

The review's claim is **technically correct** that the current message nudges the operator at one specific recovery action. But the reality is: `buf mod update` / `buf dep update` IS the correct recovery for both (1) and (2)+(3) — those commands re-resolve from whatever registry is configured, foreign or not. The only failure mode is (3)-with-truly-foreign-id, which is rare in practice and would surface as "buf mod update: unknown module X" anyway.

**Fix surface:** Two options:
- (a) **Soften the message** — change to something like `"unknown commit id: re-resolve via buf mod update / buf dep update"`. (Recommendation.)
- (b) **Add a hint** — append "if the module is served by this proxy, ensure your client has called GetCommits" so the operator knows the proxy-local recovery path.
- (c) **Split by miss class** — track which sub-case triggered the 400 in the log attrs (already in `lookupAttrs` at commits.go:495-516 as `branch=commit_id_lookup` and `ref_found=false`), then include the relevant hint in the message. More work; marginal benefit.

**Recommendation:** Option (a). The current message is too prescriptive; the new message is a single character different in the wire body and the log line, and covers all real miss classes without misleading anyone. Update the comment at commits.go:399 to match.

**Test surface:** Update the substring assertions in:
- `api_test.go:597` (line 597) — change from `[]byte("re-run buf mod update / buf dep update")` to a substring of the new message.
- `uuid_format_test.go:215` (line 215) — same change.

The existing test will fail if the message is changed; both tests need an update in lockstep with the message change. The "unknown commit id" substring assertions (api_test.go:590, uuid_format_test.go:209) remain valid and unchanged.

### Finding #5: redundant `commitUUID` call inside `computeB4Digest`

**Review claim:** `computeB4Digest` (commits.go:753) re-runs `commitUUID(meta.Commit)` on identical input that `ServeHTTP` and `ServeGraph` just validated on lines 158, 349 respectively. [CITED: review.md:15]

**Verified:**
- ServeHTTP: line 158 calls `commitUUID(meta.Commit)`, then line 174 calls `computeB4Digest(r, ref, meta.Commit)`. [VERIFIED: commits.go:158, 174]
- ServeGraph: line 349 calls `commitUUID(meta.Commit)`, then line 354 calls `computeB4Digest(r, ref, meta.Commit)`. [VERIFIED: commits.go:349, 354]
- computeB4Digest: line 753 calls `commitUUID(commit)` on the same string, then writes to `h.filesMap[cid]` on line 758. [VERIFIED: commits.go:753, 758]
- ServeDownload: line 666 calls `commitUUID(meta.Commit)`, then line 671 calls `h.computeB4DigestFromFiles(files)` (NOT `computeB4Digest`). [VERIFIED: commits.go:666, 671]

The review's claim is **accurate**: 2 `commitUUID` calls per served GetCommits/GetGraph request, identical input. The result is identical (deterministic), so this is a perf issue (2 allocations + 2 hex passes per request) and a code-clarity issue (the second call looks load-bearing but is dead-path for current callers).

**Per the review verdict (review.md:23):** This is a cleanup, not mandatory. **However**, removing `internalError` (SC #2) requires rewriting the `computeB4Digest` error path anyway, and threading `cid` through `computeB4Digest` is the natural way to make the 500-vs-502 distinction clean (the caller's `cid` is in scope at the call site, the helper's re-derived `cid` is not visible to the caller). See **Cross-cutting constraints** for the recommended structure.

**Test surface:** No direct test for this. A `BenchmarkServeHTTP` would show the perf delta. Skipping is safe; including the fix in the same plan as findings #1+#2 is also safe and saves 2 allocations per request.

### Finding #6: `probeCommitID` reports a hit while `registerResolved` silently no-ops

**Review claim:** When `sha` is not 40 hex (e.g., a 32-char UUID on a cache miss post-restart), `registerResolved` (commits.go:901-912) logs a warn and returns without populating `commitMap`/`infoCache`, but `probeCommitID` (commits.go:1030-1111) returns `(&ref, true)` unconditionally. [CITED: review.md:17]

**Verified:**
- `registerResolved` (commits.go:893-933) calls `commitUUID(sha)` on line 901. If the error is non-nil (which it is for a 32-char input — the strict 40-char check fires on line 39 of the helpers file), the function logs a warn on lines 907-910 and returns on line 911. `commitMap` and `infoCache` are NOT updated. [VERIFIED: commits.go:901-911]
- `probeCommitID` (commits.go:1030-1111) calls `registerResolved(sha, ...)` on line 1101 after a successful upstream probe, then returns `&ref, true` on lines 1102-1103. [VERIFIED: commits.go:1101-1103]

**After Phase 17 fixes findings #2 and #3** (helper removal + 64-char acceptance), the original 32-char-on-miss scenario is partially mitigated: a 32-char hex UUID now still fails the length check (it must be 40 or 64), so the warn+no-op path is still possible but for a narrower set of inputs. The review's claim about the broken probe/cache contract is **structurally unchanged** by Phase 17 — `registerResolved` still no-ops on bad input.

**Per the review verdict (review.md:17, 23):** "Real-world impact is low". Not in ROADMAP SCs. **Out of scope for this phase.** Planner should note the finding remains valid post-Phase-17 and could be addressed in a follow-up.

**Test surface:** None. The regression path requires a real upstream probe returning a 32-char hex sha, which is not a shape any known provider emits.

### Finding #7: `preResolveForTest` in production source file

**Review claim:** `preResolveForTest` (commits_helpers.go:60-70) ships in a production source file. All callers are in `_test.go`; the only thing making it "test-only" is its name. [CITED: review.md:19]

**Verified:**
- `preResolveForTest` defined at commits_helpers.go:60-70 (function body at 65-70). [VERIFIED: commits_helpers.go:60-70]
- All 5 callers in `commits_helpers_test.go`: lines 243, 251, 260, 268, 284. [VERIFIED: commits_helpers_test.go:243, 251, 260, 268, 284]
- Zero callers in any `.go` file outside `_test.go`. Verified via `grep -rn 'preResolveForTest' /Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/`. [VERIFIED: grep output above]

**Fix surface:**
- Delete the function from `commits_helpers.go` (lines 60-70 including the doc comment).
- Add the function to `commits_helpers_test.go` (paste the body verbatim, with the doc comment). Place it near the test that uses it most heavily (`TestPreResolveForTest` at commits_helpers_test.go:241).
- After the move, `commits_helpers.go` no longer needs the `strings` import — verify by reading the remaining uses of `strings` in the file. (I noted `strings.IndexByte` at line 1133, `strings.SplitN` at line 298, `strings.Repeat` is only in `preResolveForTest`.) [VERIFIED: commits_helpers.go:298, 1133] — `strings` import is still needed for the other two uses. Do NOT remove the import.

**Test surface:** No test changes needed. The move is purely a relocation; all 5 test call sites continue to work because they live in the same package.

## Fix Surface — Per Finding

| Finding | File | Lines (current) | Action |
|---------|------|-----------------|--------|
| #1 | `commits.go` | 174-178 (ServeHTTP), 354-358 (ServeGraph) | Replace `h.internalError(...)` with `h.upstreamError(...)` for `GetFiles` / `computeB4DigestFromFiles` failures; with `h.logHandlerError(...)` (500) for `commitUUID` failures. Requires typed sentinel in `computeB4Digest` to distinguish the two cases. |
| #2 | `commits.go` | 94-106 (helper def), 160, 176, 351, 356, 668 (call sites) | Delete helper. Replace 5 call sites with `h.logHandlerError(r, w, "internal error", http.StatusInternalServerError, slog.String("commit_id", ...), slog.String("upstream_error", err.Error()))`. Pass `cid` (buf-issued UUID) at sites where it's in scope (158, 666 — wait, 666 is AFTER the `commitUUID` call); pass `meta.Commit` at error sites where `cid` was never minted (160, 351, 668). See **Risk callouts** below for the exact mapping. |
| #3 | `commits_helpers.go` | 39-44 (length check + error msg) | Change to accept 40 OR 64 hex chars. Update error message text. Update doc comment (16-37). |
| #4 | `commits.go` | 582 (400 message), 399 (comment) | Soften the message to "unknown commit id: re-resolve via buf mod update / buf dep update". Update the comment. |
| #5 (optional) | `commits.go` | 744-761 (`computeB4Digest`), 158, 174 (ServeHTTP), 349, 354 (ServeGraph) | Pass `cid` into `computeB4Digest(r, ref, commit, cid)`. Remove the `commitUUID` call from inside. Use the passed-in `cid` to write `h.filesMap[cid] = files` on line 758. **Per the review verdict, this is a cleanup; include if it falls out naturally from #1+#2, otherwise skip.** |
| #6 | — | — | **Out of scope.** Note in plan as known-issue-carried-forward. |
| #7 | `commits_helpers.go` → `commits_helpers_test.go` | 60-70 (production) | Delete from `commits_helpers.go`; paste into `commits_helpers_test.go` (near `TestPreResolveForTest`). Keep `strings` import in `commits_helpers.go` (still used by lines 298, 1133). |

## Test Surface — Per Finding

| Finding | Test File | Lines (current) | New/Updated Test | Assertion |
|---------|-----------|-----------------|------------------|-----------|
| #1 | `commits_helpers_test.go` (or `api_test.go`) | — | Optional: `TestComputeB4Digest_UpstreamFailure_Returns502` | Mock provider forces `GetFiles` to error; assert 502 with `error_class=upstream` in log. **Not in ROADMAP SC; recommend skip unless trivial.** |
| #2 | `api_test.go` (and `uuid_format_test.go`) | — | Optional: `TestHandlerError_500_IncludesErrorClassInternal` | Force `commitUUID` to fail (e.g., mock with non-hex input); assert 500 with log attrs `error_class=internal`, `status=500`, `request_id=<hex>`, `protocol=v1` or `v1beta1`. **Not in ROADMAP SC; recommend skip unless trivial.** |
| #3 | `commits_helpers_test.go` | — | **New test:** `TestCommitUUID_SHA256_KnownSHA` (or extend `TestCommitUUID_KnownSHA` with 64-char sub-cases) | Assert `commitUUID("0"*64)`, `commitUUID("f"*64)`, `commitUUID(<distinct 64-char hex>)` produce known UUIDs and that `commitUUID(40charA) != commitUUID(64charB)` for distinct first-14-byte pairs. Extend `TestCommitUUID_InvalidInput` (commits_helpers_test.go:162-185) with 63-char and 65-char inputs to confirm the new "exactly 40 or 64" rule. **In ROADMAP SC-3.** |
| #4 | `api_test.go` | 597-599 | Update substring assertion | Change `[]byte("re-run buf mod update / buf dep update")` to a substring of the new message. |
| #4 | `uuid_format_test.go` | 215-217 | Update substring assertion | Change `"re-run buf mod update / buf dep update"` to a substring of the new message. |
| #5 | — | — | — | No test change. (If included, a microbenchmark is the only way to surface the win.) |
| #6 | — | — | — | **Out of scope.** |
| #7 | `commits_helpers_test.go` | 243, 251, 260, 268, 284 (callers); 60-70 (def in production) | **No change to test code.** Just move the function definition. All 5 call sites continue to work. |

## Risk Callouts

### R1 — `cid` vs. `meta.Commit` at the 5 `internalError` call sites

When `internalError` is deleted, the 5 replacement `logHandlerError` calls need a `commit_id` attr. The "right" value depends on which call site:

| Line | Call context | `cid` in scope? | Recommendation |
|------|--------------|------------------|----------------|
| 160 | After `cid, cidErr := commitUUID(meta.Commit); if cidErr != nil {` | No (call just failed) | Pass `meta.Commit` (the bad input — operator wants to see what the upstream gave us). |
| 176 | After `digest, err := h.computeB4Digest(...)` | Yes (`cid` declared on line 158) | Pass `cid` (the buf-issued UUID — this matches the SC-3 convention from Phase 16 for the 400 log line). |
| 351 | After `cid, cidErr := commitUUID(meta.Commit); if cidErr != nil {` | No (call just failed) | Pass `meta.Commit` (same reasoning as line 160). |
| 356 | After `digest, err := h.computeB4Digest(...)` | Yes (`cid` declared on line 349) | Pass `cid` (same reasoning as line 176). |
| 668 | After `cid, err = commitUUID(meta.Commit)` in the re-fetch path | The `cid` on this line is the freshly-minted UUID for the re-fetched commit. The error here means the re-fetch returned a non-hex commit — rare, since the prior `meta.Commit` was valid. | Pass `cid` (it's in scope as the prior cid; the failing `commitUUID` is a code-level surprise). |

The review (review.md:9) recommends always passing the buf-issued UUID. The above table follows that recommendation except at lines 160 and 351 where the call has just failed and no `cid` exists — there `meta.Commit` is the only thing in scope and it is what an operator actually wants to see. **Planner: do NOT add a new `cid` derivation after the failed `commitUUID` call; the contract violation means no `cid` exists, and `meta.Commit` is the right thing to log.**

### R2 — `computeB4Digest` error-classification requires a typed sentinel

After finding #1 is fixed, the two call sites (lines 174-178 and 354-358) need to distinguish `commitUUID` contract violations (500) from upstream `GetFiles` / `computeB4DigestFromFiles` failures (502). The cleanest way is to wrap the `commitUUID` error in a sentinel:

```go
// in commits.go (new, near the helpers)
var errCommitUUIDContract = errors.New("commitUUID contract violation")
```

…and have `computeB4Digest` wrap the error:

```go
cid, err := commitUUID(commit)
if err != nil {
    return nil, fmt.Errorf("%w: %v", errCommitUUIDContract, err)
}
```

Callers do `if errors.Is(err, errCommitUUIDContract) { h.logHandlerError(...) } else { h.upstreamError(...) }`.

**Alternative:** Just check the error string — fragile, do not use. **Alternative:** Return two values from `computeB4Digest` — changes the function signature, ripples to every call site, do not use.

The sentinel approach is a 3-line change in `computeB4Digest` and a 1-line `errors.Is` check at each call site.

### R3 — `commitUUID` error message change is a contract change

The current error message `"commitUUID: input is not 40 lowercase hex characters"` (commits_helpers.go:40, 44) is asserted nowhere in the test suite. After Phase 17, it becomes something like `"commitUUID: input is not 40 or 64 hex characters"`. The test `TestCommitUUID_InvalidInput` (commits_helpers_test.go:162-185) only checks that `err != nil`, not the message text. **Safe to change.**

If the planner wants to lock the new error message in tests (to prevent future drift), add a `wantMsg` column to the test table. **Not in ROADMAP SC; optional.**

### R4 — `preResolveForTest` move requires no import changes in `commits_helpers.go`

After the move, `commits_helpers.go` no longer uses `strings.Repeat` (which was the only `strings` consumer inside `preResolveForTest`). However, it still uses `strings.IndexByte` (line 1133) and `strings.SplitN` (line 298). **Do NOT remove the `strings` import from `commits_helpers.go` after the move** — it is still needed.

The new test-file home for `preResolveForTest` already has `strings` in its imports (commits_helpers_test.go:5). **No import changes needed in the test file either.**

### R5 — The 400 message change updates 2 test files in lockstep

`api_test.go:597-599` and `uuid_format_test.go:215-217` both assert the exact substring `"re-run buf mod update / buf dep update"`. Changing the message breaks both tests. **The plan must update both test files in the same commit/PR as the message change** or CI fails on the first one.

### R6 — Finding #5 (the redundant `commitUUID` call) is a natural side-effect of finding #1

If the planner threads `cid` into `computeB4Digest` (as part of the error-classification refactor for finding #1), the `commitUUID` call inside `computeB4Digest` becomes trivially removable — the function already has `cid` from the parameter. This is the right time to do it, and it costs ~5 extra lines (parameter, doc comment update, removing the inline call). Doing it now saves 2 allocations per served GetCommits/GetGraph request and eliminates a dead-path that the existing 16-02 plan already flagged as confusing (commits.go:907-911 doc comment talks about how `registerResolved` was an error path but the "contract violation" never fires in practice).

**Recommendation: include finding #5 in the same plan as findings #1, #2, #3.** Mark it as a bonus; if the test suite breaks, revert just that part.

### R7 — Bitbucket SHA-256 input is not exercised by any current test

The existing `TestCommitUUID_KnownSHA` and `TestCommitUUID_InverseRecovery` (commits_helpers_test.go:112-155, 194-234) only use 40-char SHAs. The new SHA-256 test (Phase 17 SC-3) needs a representative 64-char input. Recommendation: add 2-3 cases to `TestCommitUUID_KnownSHA` (or a new `TestCommitUUID_SHA256_KnownSHA`) covering:
- All-zeros 64-char: `0000000000000000000000000000000000000000000000000000000000000000`
- All-ones 64-char: `ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff`
- A 64-char hex with distinct first 14 bytes from the 40-char fixtures: `0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00` (same first 14 bytes as `0123456789abcdef0123456789abcdef01234567`, but length 64) — this confirms the function uses the first 14 bytes of the decoded binary regardless of input length.

The expected UUIDs can be computed by hand from the byte-table at commits_helpers.go:46-57; or by running the function once and pasting the output into the test (less rigorous but acceptable for a lock-in test).

## Cross-Cutting Constraints

### CCC-1 — Findings #1, #2, #5 share a single call-site refactor

All three findings touch the same 5 lines (commits.go:160, 174-178, 351, 354-358, 668) and the same helper definition (commits.go:94-106). They must be implemented as one atomic refactor — splitting them across plans forces the call sites to be rewritten twice.

**Single plan structure:**

- Task 1: Refactor `internalError` → `logHandlerError` calls (findings #1, #2, #5). Adds `errCommitUUIDContract` sentinel, removes `internalError`, threads `cid` into `computeB4Digest`, distinguishes 500/502 in the two call sites.
- Task 2: Accept 64-char SHA-256 in `commitUUID` (finding #3). Update length check, error message, doc comment. Add `TestCommitUUID_SHA256_KnownSHA`.
- Task 3: Soften 400 message (finding #4). Update commits.go:582 and the comment at 399. Update assertions in api_test.go:597 and uuid_format_test.go:215.
- Task 4: Move `preResolveForTest` (finding #7). Delete from commits_helpers.go, add to commits_helpers_test.go.

Findings #6 is out of scope (per ROADMAP + review verdict).

### CCC-2 — `commits_helpers.go` import block is stable through all changes

The current imports are `encoding/hex`, `errors`, `strings`, and `google.golang.org/protobuf/encoding/protowire` (commits_helpers.go:3-9). After Phase 17:
- `encoding/hex` — still used by `commitUUID` and `parseModuleRefByID`'s callers (via `hex.DecodeString`). [VERIFIED: commits_helpers.go:42]
- `errors` — still used by `commitUUID` (line 40, 44 — error message updates keep the use). [VERIFIED: commits_helpers.go:40, 44]
- `strings` — still used by `parseModuleRefByID` (line 298) and `splitOwnerModule` (line 1133). `preResolveForTest`'s `strings.Repeat` moves to the test file. [VERIFIED: commits_helpers.go:298, 1133]
- `protowire` — still used throughout the protowire helpers. [VERIFIED: commits_helpers.go:8]

**No import changes in `commits_helpers.go` after Phase 17.**

### CCC-3 — `commits.go` import block is stable through all changes

The current imports include `log/slog`, `encoding/hex`, `errors`, `fmt`, `net`, `net/http`, `strings`, `sync`, `sync/atomic`, `time`, the provider packages, `reqid`, `shake256`, and `protowire` (commits.go:3-24). After Phase 17:
- `errors` — gets a new use: `var errCommitUUIDContract = errors.New(...)`. Still used by `isTransientErr` (line 1117-1126). [VERIFIED: commits.go:1117-1126]
- `fmt` — still used by `Sprintf` in `upstreamError` and `toB5Digest` calls. [VERIFIED: commits.go:152, 182, 343, 362, 648, 658, 735]
- All other imports — unchanged.

**No import removals in `commits.go` after Phase 17.** One new use of `errors` (for the sentinel).

### CCC-4 — The 16-VERIFICATION.md's "logHandlerError joinable on request_id" expectation depends on the `hlog` doc claim

`hlog` (commits.go:77-84) sets `request_id` via `slog.With(slog.String("request_id", id))` when `reqid.From(r.Context())` is non-empty. `logHandlerError` uses `h.api.log` (NOT `h.hlog(r)`) at line 797 — so it does NOT auto-inherit `request_id`. [VERIFIED: commits.go:797]

Wait — that's a real issue. Let me re-read the existing `logHandlerError` body:

```go
logAttrs := []slog.Attr{
    slog.String("server", h.api.domain),
    slog.String("protocol", protocol),
    slog.String("request_id", RequestIDFrom(r.Context())),  // <-- explicit, not from hlog
    slog.String("error", msg),
    slog.Int("status", code),
    slog.String("error_class", errorClass(code)),
}
logAttrs = append(logAttrs, attrs...)

level := slog.LevelWarn
if code >= 500 {
    level = slog.LevelError
}
h.api.log.LogAttrs(r.Context(), level, "handler error", logAttrs...)  // <-- uses api.log, not h.hlog
```

`logHandlerError` sets `request_id` explicitly via `RequestIDFrom(r.Context())` on line 786. So the new `logHandlerError` call replacing `internalError` WILL include `request_id` (in the `slog.Attr` list, not via `hlog` chaining). **The review's claim is correct** — the new log line will be joinable on `request_id`. The old `internalError` line was NOT joinable because it used `h.hlog(r)` and that... wait, `h.hlog(r)` IS request-scoped (commits.go:79-84) and adds `request_id` via `slog.With`. So `internalError` was actually joinable on `request_id` already (via the `hlog` chaining). The review's claim that it "can't be joined on request_id" is **technically wrong** for that specific attribute. What the review MEANS is that the 500-line and the other handler-decision lines used `h.hlog(r)` (consistent) while `logHandlerError` uses `h.api.log` (slightly different) — but both produce a `request_id` attr. The real review complaint is about the missing `server`/`protocol`/`status`/`error` attrs and the `LevelWarn` downgrade, not `request_id`.

**No code change needed for `request_id` joinability** — both the old and new helpers produce a `request_id` attr, just via different mechanisms. The plan should be aware of this and not add a "fix request_id joinability" task.

## Files to be Modified

| File | Type | Reason |
|------|------|--------|
| `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/commits.go` | source | Findings #1, #2, #4, optional #5 — delete `internalError` helper (lines 94-106); refactor 5 call sites (lines 160, 174-178, 351, 354-358, 668); soften 400 message at line 582 and the comment at line 399; add `errCommitUUIDContract` sentinel; thread `cid` into `computeB4Digest` (optional). |
| `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/commits_helpers.go` | source | Finding #3 — change length check from `!= 40` to `!= 40 && != 64` (line 39); update error message text (lines 40, 44); update doc comment (lines 16-37). Finding #7 — delete `preResolveForTest` (lines 60-70). |
| `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/commits_helpers_test.go` | test | Finding #3 — add `TestCommitUUID_SHA256_KnownSHA` (or extend existing tests with 64-char sub-cases); extend `TestCommitUUID_InvalidInput` with 63-char and 65-char inputs. Finding #7 — add `preResolveForTest` definition here. |
| `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/api_test.go` | test | Finding #4 — update substring assertion at line 597. |
| `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/uuid_format_test.go` | test | Finding #4 — update substring assertion at line 215. |

**No changes to:**
- `internal/connect/api.go` (handler struct, hlog method — no change needed; the new `logHandlerError` calls use the same pattern as existing 4xx/5xx calls).
- `internal/providers/bitbucket/getrepo.go` (the SHA-256 path is enabled automatically by the new `commitUUID` acceptance; no provider change needed).
- `CHANGELOG.md` (Phase 17 is a fix, not a behavior change; no user-facing message change beyond the 400 wording).
- `cmd/easyp/...` (no config changes).

## Standard Stack

No new libraries, no new dependencies. All changes use:
- `log/slog` (existing import in `commits.go:17`) for structured logging.
- `errors` (existing import in `commits.go:7` and `commits_helpers.go:5`) for the new sentinel.
- `encoding/hex` (existing import in `commits_helpers.go:4`) for the 64-char hex decode.
- Go's standard `net/http` (existing import in `commits.go:11`) for the 400/500/502 response.

This phase does not require any package install. **No `## Package Legitimacy Audit` section needed** — nothing is added to any manifest.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| 500/502 classification of `computeB4Digest` errors | A new `errorClassComputeDigest` function or a parallel handler | The existing `errorClass(code int) string` helper at commits.go:807-816 (used by `logHandlerError`) | The existing helper is already wired into `logHandlerError`; passing `http.StatusInternalServerError` vs. `http.StatusBadGateway` to `logHandlerError` is the entire "classification" needed. |
| Distinguishing `commitUUID` contract violations from upstream errors | A new error type with method-based dispatch | `var errCommitUUIDContract = errors.New("...")` + `errors.Is` | Standard Go pattern; 3 lines; no new types. |
| Softening the 400 message | A multi-line message with conditional clauses | A single substring change to the existing string literal | The review explicitly suggests this; the alternative (splitting by miss class) is more code, marginal benefit. |

## Common Pitfalls

### P1 — Renaming `cid` variable at a call site

The 5 call sites use either `cid` (a fresh local) or `cidErr` (a fresh local for the error). After the refactor:
- Sites where `internalError` is removed and replaced with `logHandlerError` (lines 160, 176, 351, 356, 668) — the `cid`/`cidErr` locals already exist. No rename needed.
- If the planner threads `cid` into `computeB4Digest` (finding #5), the function signature becomes `computeB4Digest(r *http.Request, ref moduleRef, commit, cid string) ([]byte, error)`. The internal `cid, err := commitUUID(commit)` block (commits.go:753-756) is replaced by a use of the parameter.

**Watch out:** the `cid` parameter name shadows nothing (no existing `cid` in `computeB4Digest`'s scope), so a simple rename + parameter addition works.

### P2 — The `preResolveForTest` doc comment must move with the function

The function has a 5-line doc comment (commits_helpers.go:60-64). When the function is moved to `commits_helpers_test.go`, the doc comment must move with it (or the test will fail `golint` / `revive` checks). The new home in the test file should paste the function and its doc comment as a single block.

### P3 — The `errCommitUUIDContract` sentinel needs a clear wrap-and-check pattern

A common Go pitfall is wrapping the sentinel inside another `fmt.Errorf` without `%w`, breaking `errors.Is`. The wrap inside `computeB4Digest` must use `fmt.Errorf("%w: %v", errCommitUUIDContract, err)` (the `%w` for the sentinel, the `%v` for the original error string). The call sites then use `errors.Is(err, errCommitUUIDContract)` correctly.

### P4 — `preResolveForTest` after the move: import collision in the test file

The test file's import block (commits_helpers_test.go:3-7) already includes `strings`. The moved function needs `strings.Repeat` — which is already imported. **No import change needed in the test file.** But verify by reading the test file's import block after the move.

### P5 — The 400 message update is binary in the wire body

The new message is sent to the buf client as the response body. The client does not parse the message text (it only checks the status code and the Content-Type), so any human-readable message is safe to change. **Do NOT add structured data (JSON, key=value pairs) to the message body** — keep it a plain English sentence.

### P6 — `commitUUID` length check ordering matters

The current order in `commitUUID` (commits_helpers.go:39-45) is:
1. Length check (`len != 40`).
2. `hex.DecodeString` check.

After Phase 17, the length check should be `len != 40 && len != 64`. The error message should be the same for both failure cases. If the planner wants to be precise, use one error message (`"commitUUID: input is not 40 or 64 hex characters"`) — simpler than distinguishing the two in the message.

## Code Examples

### E1 — Replacing `internalError` call (finding #2)

Before (commits.go:160):
```go
cid, cidErr := commitUUID(meta.Commit)
if cidErr != nil {
    h.internalError(w, r, meta.Commit, cidErr.Error())
    return
}
```

After:
```go
cid, cidErr := commitUUID(meta.Commit)
if cidErr != nil {
    h.logHandlerError(r, w, "internal error", http.StatusInternalServerError,
        slog.String("commit_id", meta.Commit),
        slog.String("upstream_error", cidErr.Error()))
    return
}
```

The `logHandlerError` signature is `(r *http.Request, w http.ResponseWriter, msg string, code int, attrs ...slog.Attr)` (commits.go:777). The `attrs` are appended to the structured log line; `server`, `protocol`, `request_id`, `error`, `status`, `error_class` are added by the helper itself. [VERIFIED: commits.go:777-800]

### E2 — Distinguishing 500 vs. 502 in `computeB4Digest` callers (finding #1)

Before (commits.go:174-178):
```go
digest, err := h.computeB4Digest(r, ref, meta.Commit)
if err != nil {
    h.internalError(w, r, meta.Commit, err.Error())
    return
}
```

After:
```go
digest, err := h.computeB4Digest(r, ref, meta.Commit)
if err != nil {
    if errors.Is(err, errCommitUUIDContract) {
        h.logHandlerError(r, w, "internal error", http.StatusInternalServerError,
            slog.String("commit_id", cid),
            slog.String("upstream_error", err.Error()))
    } else {
        h.upstreamError(r, w, fmt.Sprintf("digest for %s/%s", ref.owner, ref.module),
            slog.String("owner", ref.owner),
            slog.String("module", ref.module),
            slog.String("repo", ref.module),
            slog.String("commit", meta.Commit),
            slog.String("upstream_error", err.Error()))
    }
    return
}
```

Pattern: match the existing `upstreamError` call shape (commits.go:648-654 has a near-identical call with `owner`/`module`/`repo`/`commit`/`commit_id`/`fetch_commit`/`upstream_error` attrs). [VERIFIED: commits.go:648-654]

### E3 — 64-char SHA-256 acceptance in `commitUUID` (finding #3)

Before (commits_helpers.go:39-45):
```go
if len(gitSHA) != 40 {
    return "", errors.New("commitUUID: input is not 40 lowercase hex characters")
}
sha, err := hex.DecodeString(gitSHA)
if err != nil {
    return "", errors.New("commitUUID: input is not 40 lowercase hex characters")
}
```

After:
```go
if len(gitSHA) != 40 && len(gitSHA) != 64 {
    return "", errors.New("commitUUID: input is not 40 or 64 lowercase hex characters")
}
sha, err := hex.DecodeString(gitSHA)
if err != nil {
    return "", errors.New("commitUUID: input is not 40 or 64 lowercase hex characters")
}
```

The byte-table at commits_helpers.go:46-57 reads positions 0-13 of the decoded binary. For a 40-char SHA-1 input, `hex.DecodeString` returns 20 bytes — the reads are within bounds. For a 64-char SHA-256 input, `hex.DecodeString` returns 32 bytes — the reads are still within bounds. **The byte-table does not need to change.**

### E4 — Softening the 400 message (finding #4)

Before (commits.go:582):
```go
h.badRequest(r, w, "unknown commit id: re-run buf mod update / buf dep update",
    slog.String("commit_id", commitID),
    slog.Int("body_bytes", len(body)))
```

After:
```go
h.badRequest(r, w, "unknown commit id: re-resolve via buf mod update / buf dep update",
    slog.String("commit_id", commitID),
    slog.Int("body_bytes", len(body)))
```

Single word change in the wire body: `re-run` → `re-resolve via`. The "unknown commit id" prefix and the recovery hint both remain, and the message is now consistent across stale-lockfile and foreign-id miss cases.

Also update the comment at commits.go:399 to match.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `commitUUID` SHA-256-derives any-length input | `commitUUID` rejects anything but 40 hex chars | Phase 16 (this branch) | Bitbucket SHA-256 repos 500 — Phase 17 fixes. |
| `commitUUID` returns `string` (empty-string sentinel) | `commitUUID` returns `(string, error)` | Phase 16 (D-03) | Caller must handle error; current code uses `internalError` — Phase 17 fixes the helper. |
| `computeB4Digest` failures → `upstreamError` (502) | `computeB4Digest` failures → `internalError` (500) | Phase 16 (16-02 plan) | Lost ERR-05 attributes and 502 status — Phase 17 restores. |
| 400 message: "must call CommitService/GetCommits first" | 400 message: "re-run buf mod update / buf dep update" | Phase 16 (D-12) | Misleads for foreign ids — Phase 17 softens. |
| `preResolveForTest` in production source | `preResolveForTest` in test source | Phase 17 (finding #7) | Excluded from production builds. |

**No deprecations or external library changes.**

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The 5 `commitUUID` call sites in `commits.go` are at lines 158, 349, 666, 753, 901 (per 16-VERIFICATION.md:46) | Scope confirmation | New call sites added after Phase 16 would be missed. **Mitigation:** the planner should `grep -n 'commitUUID(' internal/connect/commits.go` at plan-execution time and confirm the count is still 5. |
| A2 | The byte-table at commits_helpers.go:46-57 works correctly for 64-char inputs without modification | Risk R2 / Code E3 | If `hex.DecodeString` returns 31 bytes for some 64-char input (impossible, but unverified), the reads would panic with index out of range. **Mitigation:** the SHA-256 test (finding #3 test) exercises the new path; if the test passes, the byte-table is correct. |
| A3 | `errors` import is already present in `commits.go` (commits.go:7) | Cross-cutting CCC-3 | New sentinel use is a no-op for imports. |
| A4 | The 400 message test substring assertions (api_test.go:597, uuid_format_test.go:215) use the exact text "re-run buf mod update / buf dep update" | Test surface | If the tests have changed since 16-VERIFICATION.md, the planner needs to find the new assertion text. **Mitigation:** both files are well-covered by 16-VERIFICATION.md and the verification was 2026-07-06 (yesterday). |
| A5 | `logHandlerError` (commits.go:777) includes `request_id` in the log attrs | Risk CCC-4 | If the helper stops setting `request_id`, the joinability claim is broken. **Mitigation:** the helper sets `slog.String("request_id", RequestIDFrom(r.Context()))` on line 786 explicitly. [VERIFIED: commits.go:786] |
| A6 | The 5 `internalError` call sites pass `meta.Commit` (raw 40-char SHA) as the `commitID` param | Scope confirmation / R1 | If the call sites pass a different value, the "log commit_id won't match the buf-issued UUID" complaint is moot. **Mitigation:** verified directly — lines 160, 176, 351, 356, 668 all pass `meta.Commit`. [VERIFIED] |
| A7 | The 32-char-on-miss case in finding #6 is still a valid regression path after Phase 17 | Finding #6 | After Phase 17, `commitUUID` accepts 40 or 64 chars — a 32-char input is still rejected. The `registerResolved` no-op behavior is unchanged. **Mitigation:** A7 is correct. |
| A8 | No CLAUDE.md exists at the repo root | Project context | If a CLAUDE.md is added in the future, the planner should re-read it before execution. **Mitigation:** verified absent at the time of research. |

## Open Questions

1. **What is the exact text of the softened 400 message?**
   - What we know: The review (review.md:13-14) suggests "keep a generic tail or split the message by miss-class"; SC-4 (ROADMAP.md:166) requires the message remain generic.
   - What's unclear: The user has not picked a specific string.
   - Recommendation: Use `"unknown commit id: re-resolve via buf mod update / buf dep update"` (drop "re-run" → "re-resolve via" to soften the prescriptive tone). If the user wants a different wording, the planner should surface the question in `gsd-discuss-phase` or in the plan's `<questions>` block.

2. **Should finding #5 (threading `cid` into `computeB4Digest`) be in scope?**
   - What we know: The review verdict (review.md:23) marks it as "cleanups that can land separately"; the ROADMAP SCs do not mention it; including it is a natural side-effect of finding #1's refactor.
   - What's unclear: Whether the user wants it.
   - Recommendation: Include it in the same plan as findings #1, #2, #3 with a clear note that it's a "low-risk cleanup included because the call sites are being rewritten anyway". If the user objects, drop it — the only cost is 2 extra `commitUUID` calls per request and a dead-path inside `computeB4Digest`.

3. **Should finding #6 (probe/register cache contract) be a follow-up?**
   - What we know: The review verdict (review.md:17-18) marks it as low-impact; not in ROADMAP SCs; the regression requires a 32-char sha from a real provider (rare).
   - What's unclear: Whether the user wants it tracked.
   - Recommendation: Note in the plan as a "known-issue-carried-forward" with a one-line description. Do not include in this plan's tasks.

## Environment Availability

No external dependencies required. All changes are to source files in `/Users/nil/DiskD/W/Djarvur/easyp-buf-proxy/internal/connect/`. Verification is via the existing `go build`, `go vet`, and `go test ./internal/connect/...` commands.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build + vet + test | Yes (implied by existing commits) | — | — |
| `log/slog` | Structured logging | Yes (Go 1.21+ stdlib) | — | — |
| `errors` | New sentinel | Yes (Go stdlib) | — | — |
| `encoding/hex` | SHA-256 hex decode | Yes (Go stdlib) | — | — |

**No missing dependencies.** No new packages. No new external tools.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard `testing` (package `connect`) |
| Config file | None — Go test discovery (`*_test.go` in package) |
| Quick run command | `go test ./internal/connect/ -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SC-1 (findings #1, #2) | `computeB4Digest` errors log via `logHandlerError`/`upstreamError` with full context; 502 for upstream failures | integration | `go test ./internal/connect/ -count=1` | Exists (existing 400 tests cover `badRequest`; new 502 test optional) |
| SC-2 (finding #2) | `internalError` helper removed; all handler-level 500s flow through `logHandlerError` | structural (no new test needed) | `grep -n 'func (h \*commitServiceHandler) internalError' internal/connect/commits.go` | n/a (grep test) |
| SC-3 (finding #3) | `commitUUID` accepts 40-char SHA-1 and 64-char SHA-256; new test covers both | unit | `go test ./internal/connect/ -run 'TestCommitUUID' -v -count=1` | **Wave 0** — add `TestCommitUUID_SHA256_KnownSHA` |
| SC-4 (finding #7 + test) | `preResolveForTest` moved to `commits_helpers_test.go`; new SHA-256 test exists | structural + unit | `grep -n 'func preResolveForTest' internal/connect/commits_helpers.go` (should be 0 hits) + `go test ./internal/connect/ -run 'TestPreResolveForTest' -v -count=1` | n/a (grep) + exists |
| SC-5 (finding #4) | 400 not-found response message is generic; tests updated | integration | `go test ./internal/connect/ -run 'TestBadRequest_OnUnknownCommitID|TestServeDownload_UnknownCommitID_ReturnsBadRequest' -v -count=1` | Exists; assertions need updating |

### Sampling Rate

- **Per task commit:** `go test ./internal/connect/ -count=1`
- **Per wave merge:** `go build ./... && go vet ./... && go test ./internal/connect/ -count=1`
- **Phase gate:** Full suite green before `/gsd-verify-work` (`go test ./... -count=1`)

### Wave 0 Gaps

- [ ] `TestCommitUUID_SHA256_KnownSHA` (or 64-char sub-cases added to `TestCommitUUID_KnownSHA`) — covers SC-3
- [ ] `TestCommitUUID_InvalidInput` extended with 63-char and 65-char inputs — confirms the new "exactly 40 or 64" rule
- [ ] Optional: `TestComputeB4Digest_UpstreamFailure_Returns502` — covers SC-1's "502 for upstream failures" path with a forced `GetFiles` error
- [ ] Optional: `TestHandlerError_500_IncludesErrorClassInternal` — covers SC-2's "all 500s flow through `logHandlerError`" assertion

## Security Domain

This is a bugfix/refactor phase. The changes do not introduce new attack surfaces, do not change auth paths, do not change input handling boundaries (the new 64-char input is more permissive, which is the point — SHA-256 is a valid git hash format). The new error log lines continue to log only `commit_id` (the bad input or the buf-issued UUID) and `upstream_error` (the Go error string); neither contains secrets.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V5 Input Validation | yes (the SHA-256 acceptance is a validation change) | `hex.DecodeString` + length check (the same controls Phase 16 added, extended to 64 chars). |
| V6 Cryptography | no | The phase does not introduce new cryptography. The `encoding/hex` decode is a format parse, not a cryptographic operation. |
| V3 Session Management | no | No session state is touched. |
| V2 Authentication | no | No auth paths are touched. |

### Known Threat Patterns for {stack}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Malformed `commit_id` (not 40 or 64 hex chars) triggers 500 | Denial of Service (DoS) | Length check + `hex.DecodeString` failure → return `("", error)`. Caller returns 400 (resolution miss) or 500 (genuine contract violation). No amplification. |
| Maliciously long `commit_id` (1MB string) | DoS | `hex.DecodeString` is O(n) and `commitUUID` allocates a 16-byte result regardless. The 500-line response is small. No amplification. |
| 64-char non-hex `commit_id` | Information Disclosure (logs) | `hex.DecodeString` fails before the byte-table; the upstream_error log line contains the Go error message (e.g., "encoding/hex: invalid byte: ..."). No secrets leaked. |

## Sources

### Primary (HIGH confidence)

- [VERIFIED: review.md:7-19] — All 7 findings, line numbers, code claims, recommended fixes.
- [VERIFIED: ROADMAP.md:155-176] — Phase 17 success criteria (5 SCs).
- [VERIFIED: commits.go:94-106] — `internalError` helper definition.
- [VERIFIED: commits.go:100-106] — `internalError` body; logs at `LevelWarn` with `error_class=internal`/`commit_id`/`upstream_error`; returns 500 via `http.Error`.
- [VERIFIED: commits.go:160, 176, 351, 356, 668] — All 5 `internalError` call sites; all pass `meta.Commit` (raw 40-char SHA).
- [VERIFIED: commits.go:174-178, 354-358] — The two `computeB4Digest` callers that pass upstream errors through `internalError`.
- [VERIFIED: commits.go:744-761] — `computeB4Digest` body; returns errors from `GetFiles` (line 745), `computeB4DigestFromFiles` (line 749), and `commitUUID` (line 753).
- [VERIFIED: commits.go:777-800] — `logHandlerError` body; sets `server`/`protocol`/`request_id`/`error`/`status`/`error_class`; uses `LevelError` for 5xx.
- [VERIFIED: commits.go:807-816] — `errorClass(code)` returns `"internal"` for 500 (default branch), `"upstream"` for 502, `"bad_request"` for 4xx.
- [VERIFIED: commits.go:821-823, 828-830] — `badRequest` and `upstreamError` thin wrappers around `logHandlerError`.
- [VERIFIED: commits.go:582, 399] — 400 message location + comment.
- [VERIFIED: commits.go:832-848] — `resolveForeignCommitID` doc comment; miss case includes foreign ids.
- [VERIFIED: commits.go:893-933] — `registerResolved`; logs warn and returns on `commitUUID` failure (lines 907-911).
- [VERIFIED: commits.go:1030-1111] — `probeCommitID`; calls `registerResolved` on line 1101; returns `&ref, true` on lines 1102-1103.
- [VERIFIED: commits_helpers.go:38-58] — `commitUUID` body; strict `len != 40` check at line 39; byte-table at lines 46-57.
- [VERIFIED: commits_helpers.go:60-70] — `preResolveForTest` definition.
- [VERIFIED: commits_helpers_test.go:243, 251, 260, 268, 284] — All 5 call sites of `preResolveForTest`.
- [VERIFIED: bitbucket/getrepo.go:40, 51] — `out.Commit = repo.LatestCommit`; `LatestCommit` is a `string` JSON tag.
- [VERIFIED: api_test.go:597-599] — 400 message substring assertion.
- [VERIFIED: uuid_format_test.go:215-217] — 400 message substring assertion.
- [VERIFIED: 16-VERIFICATION.md:46-51] — Required artifacts for Phase 16; confirms `internalError` at line 100, 5 `commitUUID` call sites, 400 message at line 582, `preResolveForTest` at line 65 of `commits_helpers.go`.
- [VERIFIED: 16-02-SUMMARY.md:38] — Documents the "computeB4Digest callers switched to internalError" trade-off explicitly.
- [VERIFIED: 16-02-SUMMARY.md:105] — Documents that genuine upstream errors via `computeB4Digest` also map to 500 (a known trade-off of the Phase 16 design that Phase 17 reverses).

### Secondary (MEDIUM confidence)

- [CITED: review.md:13-14] — Suggested fix wording for finding #4 ("keep a generic tail or split the message by miss-class"). The review does not pick a specific string; the planner picks.
- [CITED: review.md:23] — Verdict on findings #4-#7 being "cleanups that can land separately" — but the ROADMAP SCs make #4, #6, #7 mandatory, so the verdict is partially overridden by the SCs.

### Tertiary (LOW confidence)

- None. All claims are sourced from the repo's own files.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new libraries; the phase uses only existing Go stdlib imports.
- Architecture: HIGH — all 7 findings are mechanical refactors with verified line numbers and code claims.
- Pitfalls: HIGH — the cross-cutting constraints (CCC-1 through CCC-4) are derived from direct code reading; the risk callouts (R1-R7) are derived from verified source.

**Research date:** 2026-07-07
**Valid until:** 2026-08-07 (30 days; the code is on a feature branch with no further activity expected)
