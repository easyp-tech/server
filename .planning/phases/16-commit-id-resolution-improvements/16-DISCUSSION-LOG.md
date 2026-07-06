# Phase 16: Commit ID Resolution Improvements - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-06
**Phase:** 16-commit-id-resolution-improvements
**Areas discussed:** Commit ID format, Short SHA support, Probe gating, Migration / wire format

---

## Commit ID format

| Option | Description | Selected |
|--------|-------------|----------|
| Stamp version/variant bits | Take first 16 bytes of SHA-1; overwrite byte 6 with version-4 nibble and byte 8 with RFC 4122 variant bits (preserves buf client compatibility). Idempotent on re-parse. (Recommended.) | |
| Literal first 16 bytes | Take first 16 bytes of SHA-1 as-is and hex-encode. Relies on buf v1.69 not strictly enforcing version/variant (or accepting any 32-char hex). | |
| Hash + stamp (status quo) | Keep SHA-256, but call it 'hash of the git SHA' rather than 'first 16 bytes' — preserves status quo. (Not aligned with ROADMAP success criteria #1.) | |

**User's choice:** "use 14 bytes from the git commit id, and combine them with 6 and 8 bytes of uuid standard"

**Notes:** The user's exact wording means: take 14 of the 20 bytes of the git SHA-1 and place them in the 14 non-UUID positions of a 16-byte array; the 2 remaining positions (6 and 8) hold the standard UUID version-4 and variant bits. The construction is non-contiguous in the SHA (bytes 6 and 8 are skipped, so SHA bytes 0-5 go to result positions 0-5, SHA byte 6 goes to result position 7, SHA bytes 7-13 go to result positions 9-15). This is reversible: skip positions 6 and 8 of the result to recover the first 14 bytes of the SHA.

---

## Short SHA support

| Option | Description | Selected |
|--------|-------------|----------|
| Pad with zeros | Pad short input with zero bytes on the right to 14 bytes before mixing. Deterministic for any length up to 14. (Recommended.) | |
| Pre-resolve to 40 chars | Caller resolves short SHA to full 40-char SHA via upstream before calling commitUUID. commitUUID itself only accepts ≥14 bytes (or full 20). | |
| Reject short input | commitUUID returns empty for short input. Probe path / prewarm path validates length before calling. | |

**User's choice:** "Pre-resolve to 40 chars" — and then "Strict: error on short input" for the runtime contract, "Return (id string, err error)" for the signature, "Log warn + return 500" for the caller's error handling, and "No helper; upstream already returns full" for the pre-resolve responsibility.

**Notes:** The user clarified the test fixture should pre-resolve 7-byte (14-hex-char) and 14-byte (28-hex-char) inputs to 40 chars via a test fixture function, then call commitUUID on the 40-char version. commitUUID itself rejects any input that isn't 40 valid hex chars with a non-nil error. Callers log structured warn (error_class=internal) and return 500. No pre-resolve helper in production; upstream GetMeta already returns full SHAs.

---

## Probe gating

| Option | Description | Selected |
|--------|-------------|----------|
| Multi-module only (ROADMAP) | probeCommitID runs only when len(sources) > 1. Single-source deployments skip the probe and use resolveForeignCommitID's fast path. Matches ROADMAP success criteria #2 verbatim. (Recommended.) | |
| Unconditional (current code) | probeCommitID runs whenever probeEnabled=true, regardless of source count. Matches current code (the fix branch). Slightly slower for single-source deployments, but uniform behavior. | ✓ |
| Single-source: fast path; multi-source: probe | probeCommitID runs for single-source too, but the fast path (resolveForeignCommitID) runs first. Only if fast path fails AND probeEnabled do we probe. (Hybrid — best of both.) | |

**User's choice:** "Unconditional (current code)" — and then "Keep defaults as-is" for config defaults, "ServeDownload only" for probe scope.

**Notes:** The user interpreted the ROADMAP wording "single-source deployments keep the existing resolveForeignCommitID fast path" as describing a feature (the fast path is still in place) rather than as a gating condition (the probe only runs in multi-module deployments). The probe is ServeDownload-only; config defaults stay (enabled=true, per_call_timeout=8s, negative_ttl=5m, max_concurrent=4).

---

## Migration / wire format

| Option | Description | Selected |
|--------|-------------|----------|
| Hard cutover, document | Hard cutover: existing buf.lock entries that reference the old SHA-256-derived UUIDs will be rejected (the new format produces different 32-char hex). Clients must re-run `buf mod update` or `buf dep update` after the proxy upgrade. Document the change in CHANGELOG / release notes. (Recommended — cleanest.) | ✓ |
| Soft transition: accept both | Accept both old and new formats during a transition window. The proxy detects the format by checking the bits at positions 6, 8 (version-4 + variant vs random) and dispatches accordingly. Dual-format logic is complex and the old format's random bits might collide with the new format's stamped bits, so this is fragile. | |
| Soft transition: alias map | Accept both old and new formats; the proxy translates old-format ids to new-format ids via a map (build it during the first GetCommits after upgrade, when the same module is encountered with both old and new ids). Adds significant complexity to commitMap lookups. | |

**User's choice:** "Hard cutover, document" — and then "CHANGELOG entry" for documentation, "Update message" for the 400 wording, "Update existing tests" for the test fixtures.

**Notes:** The 400 message changes from "unknown commit id: must call CommitService/GetCommits first" to "unknown commit id: re-run buf mod update / buf dep update". The commit_id remains in the body and log line for correlation. Existing tests in `uuid_format_test.go` and `commits_helpers_test.go` are updated with the same test names but new expected values matching the first-16-bytes format.

---

## Claude's Discretion

- Whether to introduce a `validateSHA(sha string) error` helper for the input validation in `commitUUID`, or inline the hex-decode + length check.
- The exact name and signature of the test fixture function (e.g., `preResolveForTest`, `expandShortSHA`).
- Whether the `(string, error)` change in `commitUUID` warrants a doc-comment update referencing the new contract, or whether the doc-comment stays the same with the new return values inferred from the function signature.

## Deferred Ideas

None — discussion stayed within phase scope.
