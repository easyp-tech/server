# Phase 2: Handler Adaptation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-07
**Phase:** 2-Handler Adaptation
**Areas discussed:** GetSDKInfo strategy, manifest_digest handling, Verification approach

---

## GetSDKInfo strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Defer to Phase 5 | Leave Unimplemented. Phase 5 discovers requirements empirically. | ✓ |
| Stub an empty response | Return minimal valid GetSDKInfoResponse proactively. | |
| You decide | Let planner/researcher decide. | |

**User's choice:** Defer to Phase 5
**Notes:** User agreed with recommended safe default — no wasted effort on unproven requirements.

---

## manifest_digest handling

| Option | Description | Selected |
|--------|-------------|----------|
| Defer to Phase 5 | Leave manifest_digest empty in ModulePin. Phase 5 discovers if needed. | ✓ |
| Compute and populate now | Call GetFiles + SHAKE256 during resolve. Doubles GitHub API calls. | |
| You decide | Let planner/researcher decide. | |

**User's choice:** Defer to Phase 5
**Notes:** User agreed with recommended approach — avoids coupling resolve logic with download logic.

---

## Verification approach

| Option | Description | Selected |
|--------|-------------|----------|
| Smoke test each RPC | Start server, call each RPC with Connect protocol, verify response types. | |
| Compile-only verification | go build + go vet pass. Testing deferred to Phase 3/4. | |
| You decide | Let planner decide. | |

**User's choice:** Custom — "we have subset of buf operations to be supported by the proxy. these operations must be tested e2e"

**Follow-up question:** Scope of E2E in Phase 2

| Option | Description | Selected |
|--------|-------------|----------|
| Include E2E smoke test in Phase 2 | Start TLS server, run `buf mod update` with old buf binary. Phase 3 builds on this. | ✓ |
| Keep Phase 2 minimal | Compile + server start only. E2E in Phase 4. | |

**User's choice:** Include E2E smoke test in Phase 2 — "add e2e tests as early as possible"
**Notes:** User wants early validation that the whole pipeline works, even if formal test infrastructure comes later in Phase 3.

---

## Claude's Discretion

- E2E test structure and helper functions — simplest approach that validates the server works.
- Test file organization (table-driven, single function, etc.).

## Deferred Ideas

None — discussion stayed within phase scope.
