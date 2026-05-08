# Requirements: EasyP Buf Proxy — Dependency Modernization

**Defined:** 2026-05-07

**Core Value:** The proxy must correctly serve both old (v1.30.1) and modern (v1.69.0+) Buf CLI clients simultaneously

## v1 Requirements

### Dependency Upgrades

- [x] **DEPS-01**: Build passes with Go 1.26
- [x] **DEPS-02**: connect-go upgraded to v1.19.x (requires Go 1.24+)
- [x] **DEPS-03**: All other dependencies updated to latest compatible versions
- [x] **DEPS-04**: `go mod tidy` produces no version conflicts
- [x] **DEPS-05**: Regenerated proto code compiles against new connect-go
- [x] **DEPS-06**: E2E tests pass with both buf v1.30.1 and v1.69.0+ after upgrades
- [x] **DEPS-07**: Handler structs compile with new generated Unimplemented* types

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Performance

- **PERF-01**: Response time < 100ms for cached responses
- **PERF-02**: Memory usage < 50MB under normal load

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Update buf proto to newer version | Already at v1.69.0, update in future milestone |
| Add new API endpoints | Focus is dependency modernization, not features |
| Change cache strategy | Existing caching works, no reason to change |
| BitBucket provider testing | GitHub provider is sufficient |
| Local git provider testing | Not needed for protocol correctness |
| Artifactory cache testing | Not relevant to protocol correctness |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| DEPS-01 | Phase 6 | Done |
| DEPS-02 | Phase 6 | Done |
| DEPS-03 | Phase 6 | Done |
| DEPS-04 | Phase 6 | Done |
| DEPS-05 | Phase 7 | Done |
| DEPS-06 | Phase 7 | Done |
| DEPS-07 | Phase 7 | Done |

**Coverage:**

- v1 requirements: 7 total
- Mapped to phases: 7
- Unmapped: 0 ✓
- Complete: 7/7 ✓

---

*Requirements defined: 2026-05-07*
*Last updated: 2026-05-08 after Phase 7 completion — all v1 requirements satisfied*
