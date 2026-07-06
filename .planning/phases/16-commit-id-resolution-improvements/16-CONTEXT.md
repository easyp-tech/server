# Phase 16: Commit ID Resolution Improvements - Context

**Gathered:** 2026-07-06
**Status:** Ready for planning

<domain>
## Phase Boundary

Make commit-id resolution more robust (accept short git SHAs, fall back to upstream probe on cache miss) and the not-found failure mode diagnosable (clear error response and structured log line).

This phase bundles three changes into one ROADMAP entry:

1. **Format change**: drop the SHA-256 derivation in `commitUUID` in favor of the first 16 bytes of the git SHA, with UUID version/variant bits stamped in at the standard positions. The minted id remains 32 lowercase hex chars (a syntactically valid dashless UUID), but it is now a deterministic function of the git SHA rather than a hash of it. The unit test must verify determinism for both full 40-char and short (7-byte / 14-byte) inputs.

2. **Resolution path**: when a `DownloadService/Download` request carries a commit id that is not in `commitMap` and `resolveForeignCommitID` cannot disambiguate, the handler probes every configured source for the sha and uses the first match. Single-source deployments keep the existing `resolveForeignCommitID` fast path as a first attempt; the probe is the recovery.

3. **Failure mode**: the 400 response returned for an unresolvable commit id names the id itself in both the wire body and the structured log line, so an operator can correlate a client-side "unknown commit id" with a prior `CommitService/GetCommits` log entry without re-reading the request.

In scope: `internal/connect/commits.go`, `internal/connect/commits_helpers.go`, `internal/connect/commits_helpers_test.go`, `internal/connect/uuid_format_test.go`, the 400 message in `ServeDownload`, and the CHANGELOG.

Out of scope: the deprecated `v1alpha1` handlers (already covered by INFR-02 from Phase 12), the upstream provider's GetMeta implementation (no changes there), buf CLI changes (the client is external).
</domain>

<decisions>
## Implementation Decisions

### Commit ID format

- **D-01:** `commitUUID` is rewritten to take 14 bytes from the git SHA-1 (bytes 0–13 of the 20-byte decoded binary) and place them in the 14 non-UUID positions of a 16-byte result. The 2 UUID positions (byte 6 = version-4, byte 8 = variant) hold pure UUID standard bits. SHA bytes 14–19 are not used in the result. The final 16-byte array is hex-encoded to 32 lowercase chars. (User explicit construction: "use 14 bytes from the git commit id, and combine them with the bytes at positions 6 and 8 of uuid standard. distribute them to make uuid bytes unused. this is sha to uuid procedure. for uuid to sha procedure we are taking 16 uuid bytes, then shifting parts to remove uuid bytes and get the 14 sha bytes back".)
- **D-02:** Concrete construction (one valid implementation; planner may use this or a simpler form):

  ```text
  result[0]  = sha[0]
  result[1]  = sha[1]
  result[2]  = sha[2]
  result[3]  = sha[3]
  result[4]  = sha[4]
  result[5]  = sha[5]
  result[6]  = 0x40                  // UUID version-4 (high nibble), low nibble = 0
  result[7]  = sha[6]
  result[8]  = 0x80                  // UUID RFC 4122 variant (high 2 bits), low 6 bits = 0
  result[9]  = sha[7]
  result[10] = sha[8]
  result[11] = sha[9]
  result[12] = sha[10]
  result[13] = sha[11]
  result[14] = sha[12]
  result[15] = sha[13]
  hex.EncodeToString(result[:]) → 32 lowercase hex chars
  ```

  Inverse: take `result[0..5] = sha[0..5]`, skip `result[6]`, take `result[7] = sha[6]`, skip `result[8]`, take `result[9..15] = sha[7..13]`. This recovers `sha[0..13]` (14 bytes; the remaining 6 bytes of the 20-byte SHA-1 are not encoded in the id).

### Short SHA support

- **D-03:** `commitUUID` signature changes from `func commitUUID(gitSHA string) string` to `func commitUUID(gitSHA string) (string, error)`. Returns empty string + non-nil error for input that is not exactly 40 valid lowercase or uppercase hex chars. (User explicit choice — cleanest signature; empty-string-as-sentinel is fragile.)
- **D-04:** Callers handle the error by logging a structured warn (with `error_class=internal`, the offending `commit_id`, and the upstream error) and returning 500 Internal Server Error. This is treated as a programming bug, not a client error: callers always pass full SHAs from upstream `GetMeta`, so any short input is a contract violation worth flagging. (User explicit choice.)
- **D-05:** No new pre-resolve helper. Callers of `commitUUID` already get full 40-char SHAs from upstream `s.GetMeta(ctx, sha)`, so in production the function is always called with valid input. The unit test pre-resolves 7-byte (14-hex-char) and 14-byte (28-hex-char) short inputs via a test fixture function (e.g., `preResolveForTest(sha string) string` that pads the short hex string out to 40 chars) to verify the function's contract for both shapes. (User explicit choice.)
- **D-06:** All current call sites of `commitUUID` are updated to handle the new `(string, error)` signature:

  - `internal/connect/commits.go:144` (ServeHTTP)
  - `internal/connect/commits.go:336` (ServeGraph)
  - `internal/connect/commits.go:654` (ServeDownload)
  - `internal/connect/commits.go:737` (computeB4Digest)
  - `internal/connect/commits.go:882` (registerResolved)

  Each call site wraps the result in the `if err != nil { log warn + return 500 }` pattern from D-04. (Mechanical change.)

### Probe gating

- **D-07:** `probeCommitID` runs unconditionally when `probeEnabled` is true, regardless of source count. The ROADMAP wording "multi-module only" is interpreted as the primary use case for the probe, not a gating rule. Single-source deployments probe too. (User explicit choice — matches current code on the fix branch; ROADMAP wording treated as descriptive of the motivating scenario.)
- **D-08:** The probe is called only from `ServeDownload`. `ServeGraph` already has the module ref from the wire and uses `GetMeta` directly. No other handlers need the probe. (User explicit choice.)
- **D-09:** Existing config defaults are kept as-is: `probe.enabled=true`, `probe.per_call_timeout=8s`, `probe.negative_ttl=5m`, `max_concurrent_probes=4`; `prewarm.enabled=true`, `prewarm.per_call_timeout=10s`. No config schema changes.

### Migration / wire format

- **D-10:** Hard cutover. Existing `buf.lock` entries that reference the old SHA-256-derived UUIDs will be rejected by the proxy after the upgrade (they look like unknown commit ids). Clients must re-run `buf mod update` or `buf dep update` to repopulate `buf.lock` with the new-format ids. (User explicit choice — cleanest; the change is internal to the proxy and the in-memory `commitMap` / `infoCache` is reset on restart, so no persistent state needs migration.)
- **D-11:** Document the format change in `CHANGELOG.md` with a single entry under the next version. Entry text: "commit-id format change: the proxy now mints the first 16 bytes of the git SHA (with UUID version/variant bits) instead of the SHA-256 of the git SHA. Existing `buf.lock` entries are invalidated; clients must re-run `buf mod update` or `buf dep update` after upgrading." No startup log line, no config flag, no transitional period. (User explicit choice — CHANGELOG only.)
- **D-12:** Update the 400 response message in `ServeDownload` from `"unknown commit id: must call CommitService/GetCommits first"` to `"unknown commit id: re-run buf mod update / buf dep update"`. The `commit_id` remains in the wire body and the structured log line for correlation. (User explicit choice — the new message nudges clients to re-resolve, which covers both "you forgot to call GetCommits" and "your cached id is from a different version of the proxy".)
- **D-13:** Update existing tests in `internal/connect/commits_helpers_test.go` and `internal/connect/uuid_format_test.go` to match the new format. Same test names, new expected values. Add new tests that:
  - Verify the new format for known git SHAs (e.g., the empty SHA, a well-known commit like `6.0.0-beta.1`'s SHA, or a stable test fixture).
  - Verify `commitUUID("")`, `commitUUID("abc")`, `commitUUID("not-hex-but-40-chars-zzzzzzzzzzzzzzzzzzzz")` all return `("", error)`.
  - Verify the inverse: extracting bytes at positions 0..5, 7, 9..15 from the id recovers the first 14 bytes of the SHA.
  - Exercise the 7-byte and 14-byte short SHA pre-resolve path via the test fixture. (User explicit choice — update existing tests; same names, new values.)

### Claude's Discretion

- Whether to introduce a `validateSHA(sha string) error` helper for the input validation in `commitUUID`, or inline the hex-decode + length check. Pick whichever is shorter.
- The exact name and signature of the test fixture function (e.g., `preResolveForTest`, `expandShortSHA`). Pick a name consistent with existing test conventions in the package.
- Whether the `(string, error)` change in `commitUUID` warrants a doc-comment update referencing the new contract, or whether the doc-comment stays the same with the new return values inferred from the function signature.

### Folded Todos

None — `cross_reference_todos` returned no matches for Phase 16.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap

- `.planning/ROADMAP.md` — Phase 16 success criteria (the three locked criteria this phase must satisfy)
- `.planning/PROJECT.md` — project context and the v1.3 milestone scope (Diagnostic Logging); relevant because Phase 16 was added as a follow-on to the logging work
- `.planning/REQUIREMENTS.md` — full v1.3 requirements (FOUND/INFR/ERR/PROV/OPS-01) and the deferred v1.4 items; Phase 16 itself is not in the requirements table, but the table's traceability section confirms the gap (potential follow-up: add Phase 16 to the v1.3 traceability)

### Codebase Maps

- `.planning/codebase/ARCHITECTURE.md` — system overview, `multisource.Repo.Repositories()` semantics, cache-aside pattern (relevant for understanding what `probeCommitID` does and why single-source deployments can fall through to the fast path)
- `.planning/codebase/INTEGRATIONS.md` — Buf Registry Protocol (Connect/gRPC) section, which describes the wire format the buf CLI sends and the error semantics the proxy must match
- `.planning/codebase/CONCERNS.md` — "Sequential file downloads in GitHub and BitBucket providers" and "No response compression" are not directly related to Phase 16 but document adjacent performance issues that future phases may need to address

### Existing Code Touched

- `internal/connect/commits.go` — `commitServiceHandler`, `ServeHTTP`, `ServeGraph`, `ServeDownload`, `resolveForeignCommitID`, `probeCommitID`, `registerResolved`, `prewarmHeads`, `computeB4Digest`, `toB5Digest`, `commitMap`, `infoCache`, `filesMap`, `singleModule`, `missCache`, `sweepMisses`, `maxConcurrentProbes`. This is the file that contains most of the work.
- `internal/connect/commits_helpers.go` — `commitUUID` (the function being rewritten in D-01/D-02/D-03), and the surrounding protowire helpers that stay as-is.
- `internal/connect/api.go` — `NewWithConfig` and the `Connect` config wiring (sets `probeEnabled`, `probeTimeout`, `probeNegativeTTL`, `prewarmEnabled`, `prewarmTimeout`, `probeSem`). Read to understand how the new format change interacts with the existing init path; the wiring is unchanged.
- `cmd/easyp/internal/config/config.go` — `Connect`, `PrewarmConfig`, `ProbeConfig`, `WithDefaults`. Read to confirm the defaults in D-09 match the code and no schema changes are needed.
- `internal/connect/api_test.go` — existing tests that exercise `commitServiceHandler` end-to-end; may need fixture updates if test inputs assumed the old UUID format.

### Tests

- `internal/connect/commits_helpers_test.go` — unit tests for the current `commitUUID` (length, hex, version, variant, determinism, distinctness). To be updated per D-13.
- `internal/connect/uuid_format_test.go` — 400-error regression tests that lock in the wire format the buf client parses. To be updated per D-13.
- `internal/connect/bynames_test.go`, `internal/connect/modulepins_test.go` — read for context on how test fixtures in this package are structured (the test fixture function from D-05 should match this style).

### Migration Document

- `CHANGELOG.md` (repo root) — to be created or updated per D-11. If `CHANGELOG.md` does not exist yet, the planner should create it; if it exists, the planner should add a new entry under the next version heading.

### External (out-of-tree)

- buf CLI `uuidutil.FromDashless` (in the buf repo, not this one) — validates the 32-char id with `uuid.Parse` (length == 32 + version-4 + variant bits). The format in D-01/D-02 satisfies this validator; planners should not need to re-derive the buf client behavior from source — the success criteria in ROADMAP are the source of truth.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `s.GetMeta(ctx, sha)` on every configured source (`internal/providers/source/source.go`) — the probe and prewarm paths already use this; the new code in D-06 reuses it. No changes to the source interface are needed.
- `s.Owner()`, `s.RepoName()` — the probe path uses these to construct the `moduleRef` on a hit. Same as current code.
- `registerResolved(sha, owner, module)` — the existing function that registers a sha (and its derived UUID) into `commitMap` and `infoCache`. The new `commitUUID` will be called from inside this function (line 882 of `commits.go`); the function signature changes propagate up.
- `commitMap[uuid]` and `commitMap[sha]` dual-keying in `registerResolved` (lines 884–887) — keeps the existing "buf client sends UUID, probe path sends raw sha" pattern working. Under the new format, the UUID and the sha are different (UUID is the first 14 SHA bytes + UUID bits, sha is the full 40 hex chars), so the dual-keying remains correct.

### Established Patterns

- **Logger is dependency-injected and request-scoped** (`h.hlog(r)`) — the new 500-return in D-04 should use `h.hlog(r).LogAttrs(...)` with `error_class=internal` to match the existing `logHandlerError` / `badRequest` / `upstreamError` helpers. Read `commits.go:756-811` for the pattern.
- **Config struct pattern** — `internal/connect/api.go:104-110` reads `cfg.PrewarmEnabled`, `cfg.PrewarmTimeout`, `cfg.ProbeEnabled`, `cfg.ProbeNegativeTTL`, `cfg.ProbeTimeout`. No changes; D-09 keeps the existing fields.
- **`logHandlerError` / `badRequest` / `upstreamError`** — the existing helpers at `commits.go:756-811` are the standard way to return a structured error response. D-04 says "log warn + return 500" which means: either add a 5th helper `internalError(...)`, or inline the pattern in each new call site. Planner can decide.

### Integration Points

- `internal/connect/commits.go:570-573` — the 400 message in `ServeDownload` is the one being updated per D-12. The line is:
  ```go
  h.badRequest(r, w, "unknown commit id: must call CommitService/GetCommits first",
      slog.String("commit_id", commitID),
      slog.Int("body_bytes", len(body)))
  ```
  Change the string literal to `"unknown commit id: re-run buf mod update / buf dep update"`. The structured attributes stay.
- `internal/connect/commits.go:144`, `:336`, `:654`, `:737`, `:882` — the five `commitUUID` call sites. Each becomes `cid, err := commitUUID(meta.Commit); if err != nil { ... return 500 ... }`.
- `internal/connect/commits.go:34-70` — the `commitServiceHandler` struct. No new fields; `probeSem`, `prewarmEnabled`, `probeEnabled` etc. stay.
- `cmd/easyp/main.go:57` — `cc := cfg.Connect.WithDefaults()`. No change.
- `CHANGELOG.md` (to be created) — root of the repo. D-11 says add a single entry. If no CHANGELOG exists, planner should create one with a single initial version entry.

</code_context>

<specifics>
## Specific Ideas

- The user's exact words for the format construction: *"use 14 bytes from the git commit id, and combine them with 6 and 8 bytes of uuid standard"*. The "6 and 8" refers to byte positions 6 and 8 in the resulting 16-byte array (the standard UUID version-4 and variant byte positions). The "distribute them to make uuid bytes unused" wording means the 14 SHA bytes occupy the 14 positions that are not 6 and 8, so the SHA bytes are placed in a non-contiguous slice of the result (positions 0-5, 7, 9-15). The inverse (uuid → sha) is unambiguous: skip positions 6 and 8, concatenate the rest, recover the first 14 bytes of the SHA.
- The 400 message change is the main operator-facing difference between pre- and post-Phase-16. Old logs from before the upgrade will show "unknown commit id: must call CommitService/GetCommits first"; new logs after the upgrade show "unknown commit id: re-run buf mod update / buf dep update". Operators correlating client-side errors with proxy logs can spot the message change as a marker of the upgrade.
- The user's preference for the `commitUUID` signature change is explicit: "Return (id string, err error)" — they considered the empty-string-as-sentinel alternative (current code uses this) and the panic alternative, and chose the explicit error return.
- The probe in D-07 is interpreted as unconditional by the user, even though the ROADMAP text suggests multi-module. The user's reasoning (in their choice description) was that matching the current code is simpler than introducing a new gating condition. Planners should not re-introduce a source-count check.

## Specifics from prior work

The Phase 16 work was started on the `fix/buf-v1.69-commit-uuid-format` branch (commits 77e387a, 659f4af, 3297549, 151ca0d, 6610ee9). The current code on that branch already implements items 2 and 3 from the ROADMAP success criteria. Phase 16's main code delta is item 1 (the format change), plus the message update in D-12. The branch's commits are:

- `77e387a fix(e2e): skip matrix tests gracefully when testdata/buf is absent`
- `659f4af fix(connect): mint 32-char dashless UUID for commit ids; add e2e matrix`
- `3297549 fix(connect): commit-id resolution — multi-module 400s, raw ids, probe + prewarm (#34)`
- `151ca0d fix(connect): register v1 OwnerService route to fix text/plain fallthrough (#33)`
- `6610ee9 fix(connect): write back resolved commits in ServeGraph; add per-ref parse trace in ServeGetModules (#32)`

The `659f4af` commit is the one that introduced the current SHA-256 derivation. Phase 16 reverses that decision (D-01/D-02).

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. The `buf.lock` re-resolution is a one-time client-side action, not a feature.

</deferred>

---

*Phase: 16-commit-id-resolution-improvements*
*Context gathered: 2026-07-06 via /gsd-discuss-phase*
