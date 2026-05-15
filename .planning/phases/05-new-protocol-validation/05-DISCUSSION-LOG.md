# Phase 5: New Protocol Validation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-07
**Phase:** 5-New Protocol Validation
**Areas discussed:** Content-type fix strategy, New RPC discovery approach, buf dep update test approach

---

## Content-type fix strategy

### Q1: How should we approach the content-type mismatch?

| Option | Description | Selected |
|--------|-------------|----------|
| Investigate first | Run existing smoke test v1.69.0 subtest with verbose logging, capture actual RPC request/response exchange | ✓ |
| Pre-analyze protocol diff | Diff v1.30.1 vs v1.69.0 proto definitions before writing tests | |
| You decide | Let the planner/researcher determine the best approach | |

**User's choice:** Investigate first
**Notes:** The rootHandler returns text/plain, but connect-go RPC handlers should handle content types automatically. Investigation will reveal the true source.

### Q2: How should we capture diagnostics?

| Option | Description | Selected |
|--------|-------------|----------|
| Debug logging | Set proxy log level to debug in tests, capture full server subprocess output on failure | ✓ |
| Custom middleware logging | Add HTTP middleware logging request/response content-types | |
| You decide | Let the planner/researcher pick | |

**User's choice:** Debug logging
**Notes:** Minimal code change — just pass debug log level to test config.

### Q3: Should the fix be in the same plan as investigation?

| Option | Description | Selected |
|--------|-------------|----------|
| Fix immediately | Fix the root cause in the same plan as the investigation | ✓ |
| Separate plans | Investigation in plan 05-01, fixes in plan 05-02 | |
| You decide | Let the planner decide based on findings | |

**User's choice:** Fix immediately

---

## New RPC discovery approach

### Q1: How should we discover what new RPCs the modern buf CLI requires?

| Option | Description | Selected |
|--------|-------------|----------|
| Empirical discovery | Write tests, run them, capture server debug logs showing actual RPC calls | ✓ |
| Pre-analyze + pre-stub | Diff proto definitions, pre-implement stubs for likely-needed RPCs | |
| Hybrid | Quick proto diff for awareness, but tests drive implementation | |

**User's choice:** Empirical discovery
**Notes:** Minimal wasted effort — no over-engineering.

### Q2: If v1.69.0 CLI calls an unimplemented RPC, how should we handle it?

| Option | Description | Selected |
|--------|-------------|----------|
| Return Unimplemented | If CLI tolerates, no fix needed. If CLI fails, fix that specific RPC | ✓ |
| Minimal stubs | Implement minimal stubs for all discovered RPCs | |
| You decide | Let the planner/researcher decide | |

**User's choice:** Return Unimplemented
**Notes:** Follows the 'minimal implementation' principle.

---

## buf dep update test approach

### Q1: Should the NEW-02 test use the actual `buf dep update` command?

| Option | Description | Selected |
|--------|-------------|----------|
| Real buf dep update | Test the actual command with v1.69.0 — validates real user workflow | ✓ |
| Reuse Phase 4 pattern | Two-step buf mod update — simpler but doesn't test real command | |
| You decide | Let the planner decide | |

**User's choice:** Real buf dep update
**Notes:** v1.69.0 has a real `buf dep update` command. Testing it directly may reveal protocol differences.

### Q2: Where should the buf dep update logic live?

| Option | Description | Selected |
|--------|-------------|----------|
| New testutil helper | Add RunBufDepUpdate to e2e/testutil/ — follows Phase 3 pattern | ✓ |
| Inline in test file | Simpler for a one-off test | |
| You decide | Let the planner decide | |

**User's choice:** New testutil helper

---

## Claude's Discretion

- Content-type fix implementation details (connect-go config, middleware, or handler changes) — depends on investigation results
- Test file location and naming — follow existing e2e/ convention
- Whether to start from existing smoke test v1.69.0 subtest or write new dedicated test
- Debug logging detail level
- Whether plans 05-01 and 05-02 should remain separate or merge

## Deferred Ideas

None — discussion stayed within phase scope.
