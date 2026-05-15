# Phase 3: Test Infrastructure - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-07
**Phase:** 3-Test Infrastructure
**Areas discussed:** Server lifecycle, Buf binary management, Helper packaging

---

## Server lifecycle

| Option | Description | Selected |
|--------|-------------|----------|
| Subprocess (current) | Keep go run subprocess approach from existing smoke test. Tests the real compiled binary end-to-end. Slow (compiles each run) but highest fidelity. | ✓ |
| In-process (httptest) | Import the handler, create httptest.NewTLSServer(). Fast, deterministic cleanup. But doesn't test the real binary's TLS wiring. | |
| Hybrid | Build both: in-process by default, subprocess for full-fidelity. More helper code to maintain. | |

**User's choice:** Subprocess (current)
**Notes:** Keeps high-fidelity approach, tests the real binary.

| Option | Description | Selected |
|--------|-------------|----------|
| TCP poll (current) | Start, poll TCP port until accepting, return port + cleanup. Proven, works reliably (30s timeout). | ✓ |
| Readiness signal | Server writes to pipe/file when ready. Faster but requires modifying main.go. | |
| You decide | Claude picks best approach. | |

**User's choice:** TCP poll (current)
**Notes:** No changes to production code needed.

| Option | Description | Selected |
|--------|-------------|----------|
| Config struct → YAML | Helper accepts config struct and generates YAML into t.TempDir(). Current approach formalized. | ✓ |
| Config file path | Helper accepts path to existing config file. Simpler helper but shifts config work to tests. | |

**User's choice:** Config struct → YAML
**Notes:** Formalizes the existing inline pattern from smoke_test.go.

---

## Buf binary management

| Option | Description | Selected |
|--------|-------------|----------|
| Path + version check | Helper checks binaries exist at paths and asserts version via `buf --version`. Fail fast if missing. Simple. | |
| Auto-download | Helper downloads pinned buf versions from GitHub releases if not found locally. Self-contained but adds download logic. | ✓ |
| PATH discovery | Helper finds buf binaries by version string via PATH lookup. Simpler config but less deterministic. | |

**User's choice:** Auto-download
**Notes:** Makes test suite self-contained. Binaries cached locally.

| Option | Description | Selected |
|--------|-------------|----------|
| User cache dir | Store in ~/.cache/easyp-buf-proxy/. Reused across runs. Standard pattern. | |
| Project testdata/ | Store in project's testdata/ directory. More portable but needs gitignore. | ✓ |
| Temp dir per test | Store in t.TempDir(). Simple but re-downloads every run. | |

**User's choice:** Project testdata/
**Notes:** Binaries cached at testdata/buf/v{version}/buf.

| Option | Description | Selected |
|--------|-------------|----------|
| GitHub releases | Download from github.com/bufbuild/buf/releases. Deterministic, platform-specific. | ✓ |
| go install | Use `go install github.com/bufbuild/buf/cmd/buf@v1.30.1`. Simpler but requires Go toolchain. | |

**User's choice:** GitHub releases

---

## Helper packaging

| Option | Description | Selected |
|--------|-------------|----------|
| internal/testutil/ | Shared helpers importable by any test. Go convention. | |
| e2e/testutil/ | Helpers scoped to integration tests alongside e2e tests. | ✓ |
| e2e package | Keep helpers in e2e/ as exported functions. Simplest. | |

**User's choice:** e2e/testutil/
**Notes:** Scoped to e2e integration tests. Phases 4/5 import from here.

| Option | Description | Selected |
|--------|-------------|----------|
| Split by concern | server.go, bufbin.go, config.go. Clear separation. | ✓ |
| Single file | helpers.go with all functions. Simpler but may grow. | |

**User's choice:** Split by concern

---

## Claude's Discretion

- Config struct field names/types (follow existing config.go patterns)
- go run vs pre-build binary optimization
- Platform detection for binary download
- Checksum verification on downloads
- How to handle existing e2e/smoke_test.go (refactor or leave)

## Deferred Ideas

None — discussion stayed within phase scope.
