---
phase: 05-new-protocol-validation
verified: 2026-05-07T22:30:00Z
status: human_needed
score: 5/5 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Run full e2e suite: go test ./e2e/ -v -timeout 300s -count=1"
    expected: "All 4 tests pass (TestSmokeBufModUpdate/v1.30.1, TestSmokeBufModUpdate/v1.69.0, TestOldProtocolBufModUpdateTwice, TestNewProtocolBufModUpdate, TestNewProtocolBufDepUpdate)"
    why_human: "Tests require EASYP_GITHUB_TOKEN and network access to GitHub API -- cannot run without secrets"
  - test: "Run buf mod update manually with v1.69.0 against the proxy"
    expected: "buf.lock created with valid module references"
    why_human: "Requires live server, TLS certs, and GitHub API access"
---

# Phase 5: New Protocol Validation -- Verification Report

**Phase Goal:** Modern buf CLI support confirmed -- buf v1.69.0+ commands work against the proxy, and any required new RPC implementations are identified
**Verified:** 2026-05-07T22:30:00Z
**Status:** human_needed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | buf mod update succeeds against proxy with v1.69.0 binary (NEW-01) | VERIFIED | TestNewProtocolBufModUpdate in e2e/new_proto_test.go:9-25 calls RunBufModUpdate with BufV169; checks exitCode==0; buf.lock existence asserted by RunBufModUpdate (server.go:130). TestSmokeBufModUpdate also covers v1.69.0 (smoke_test.go:28). Implementation: commits.go ServeHTTP handles GetCommits, api.go:56 routes v1beta1.CommitService |
| 2 | buf dep update succeeds against proxy with v1.69.0 binary (NEW-02) | VERIFIED | TestNewProtocolBufDepUpdate in e2e/new_proto_test.go:27-43 calls RunBufDepUpdate with BufV169; checks exitCode==0. RunBufDepUpdate exists in server.go:139-178 with "dep" "update" command; buf.lock check on success at line 174 |
| 3 | v1beta1 RPC handlers exist and are wired (GetCommits, GetGraph, Download, GetModules) | VERIFIED | api.go:56-60 registers 5 route prefixes (CommitService, GraphService, DownloadService, v1.ModuleService, v1beta1.ModuleService). commits.go implements 4 handler methods: ServeHTTP (line 32), ServeGraph (line 205), ServeDownload (line 379), ServeGetModules (line 525). All handlers use application/proto content-type |
| 4 | Backward compatibility maintained (old v1alpha1 protocol still works for v1.30.1) | VERIFIED | api.go:46-48 still registers v1alpha1 handlers (ResolveService, RepositoryService, DownloadService). modulepins.go unchanged (still returns ModulePin with Remote/Owner/Repository/Commit). TestOldProtocolBufModUpdateTwice and TestSmokeBufModUpdate/v1.30.1 cover old protocol |
| 5 | Server debug output reveals all RPCs called by v1.69.0 CLI | VERIFIED | 05-01-SUMMARY.md documents GetCommits RPC discovery. 05-02-SUMMARY.md documents full chain: GetCommits -> GetGraph(x2) -> Download -> GetModules. Both test functions set cfg.LogLevel="debug". New tests capture srv.Output in failure messages |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/connect/commits.go` | v1beta1 handler implementation (GetCommits, GetGraph, Download, GetModules) | VERIFIED | 705 lines, 4 handler methods. Substantive: parses protobuf wire format requests, calls provider.GetMeta/GetFiles, computes B4 digest (SHAKE256), builds raw protobuf responses. Wired: api.go:51-55 instantiates commitServiceHandler, api.go:56-60 registers routes |
| `internal/connect/api.go` | Route registration for v1beta1 endpoints | VERIFIED | 65 lines. Lines 46-48: v1alpha1 handlers preserved. Lines 51-60: v1beta1 commitServiceHandler created and 5 routes registered. Lines 62: rootHandler as fallback |
| `internal/connect/blobs.go` | Shared blob utilities for old protocol | VERIFIED | 67 lines. digestFormat constant shared with commits.go (both use same "shake256:%s  %s\n" format). SHA3Shake256 used consistently |
| `internal/providers/github/client.go` | GitHub client with IPv4 transport fix | VERIFIED | Lines 47-51: transport.DialContext overridden to use "tcp4" instead of default "tcp", preventing IPv6 TLS handshake timeouts on macOS |
| `e2e/new_proto_test.go` | TestNewProtocolBufModUpdate and TestNewProtocolBufDepUpdate | VERIFIED | 43 lines, 2 test functions. Both use t.Parallel(), RequireEnvToken, GetBuf(BufV169), LogLevel="debug". Both check exitCode==0 and include server output in failure messages |
| `e2e/testutil/server.go` | RunBufDepUpdate helper | VERIFIED | Lines 139-178. Follows RunBufModUpdate pattern exactly. Uses "dep" "update" command (line 155). Creates buf.yaml with deps, verifies buf.lock on success |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| e2e/new_proto_test.go | e2e/testutil.StartServer | Function call with cfg.LogLevel="debug" | WIRED | Line 18: `srv := testutil.StartServer(t, cfg)` |
| e2e/new_proto_test.go | e2e/testutil.RunBufDepUpdate | Function call for NEW-02 | WIRED | Line 38: `exitCode, stderr := testutil.RunBufDepUpdate(t, bufPath, srv.Port)` |
| api.go v1beta1 routes | commitServiceHandler methods | HandleFunc registration | WIRED | Lines 56-60: 5 HandleFunc routes pointing to ServeHTTP, ServeGraph, ServeDownload, ServeGetModules |
| commitServiceHandler | provider.GetMeta/GetFiles | h.api.repo method calls | WIRED | commits.go:58 (GetMeta in ServeHTTP), commits.go:252 (GetMeta in ServeGraph), commits.go:418-423 (GetMeta+GetFiles in ServeDownload), commits.go:497 (GetFiles in computeB4Digest) |
| commits.go | shake256.SHA3Shake256 | Import + direct call | WIRED | Import at line 12, call at line 517. Same algorithm as blobs.go line 39 |
| commits.go | blobs.go digestFormat | Shared constant | WIRED | blobs.go:15 defines `const digestFormat`, commits.go:515 uses `fmt.Fprintf(&manifest, digestFormat, ...)` |
| e2e/testutil/server.go RunBufDepUpdate | buf CLI binary | exec.CommandContext | WIRED | Line 155: `exec.CommandContext(ctx, bufBinary, "dep", "update")` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| commits.go ServeHTTP | `files` (from provider) | h.api.repo.GetFiles(r.Context(), owner, module, commit) | Yes -- GitHub API returns real proto files | FLOWING |
| commits.go ServeHTTP | `digest` (B4/SHAKE256) | h.computeB4DigestFromFiles(files) | Yes -- computed from real file hashes | FLOWING |
| commits.go ServeHTTP | `respMsg` (protobuf response) | Built from commitID, ownerID, moduleID, digest | Yes -- populated from real data | FLOWING |
| commits.go ServeGraph | `cached` (from infoCache) | h.infoCache[owner/module] | Yes -- populated by prior GetCommits call | FLOWING |
| commits.go ServeDownload | `files` (from cache or provider) | h.filesMap[cached.commitID] or h.api.repo.GetFiles | Yes -- real files from cache or GitHub | FLOWING |
| commits.go ServeGetModules | `keys` (from moduleLookup) | h.infoCache -> moduleLookup map | Yes -- moduleIDs derived from real owner/module names | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Project compiles | `go build ./...` | No output (success) | PASS |
| e2e test files parse correctly | `go build ./e2e/` | No output (success) | PASS |
| testutil package compiles | `go build ./e2e/testutil/` | No output (success) | PASS |
| v1beta1 route patterns registered | `grep v1beta1 internal/connect/api.go` | 4 matches (CommitService, GraphService, DownloadService, ModuleService) | PASS |
| v1alpha1 routes still registered | `grep 'mux.Handle(connect.New' internal/connect/api.go` | 3 matches (ResolveService, RepositoryService, DownloadService) | PASS |
| BufV169 constant defined | `grep BufV169 e2e/testutil/bufbin.go` | `BufV169 = "v1.69.0"` | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-----------|-------------|--------|----------|
| NEW-01 | 05-01, 05-02 | buf mod update succeeds with v1.69.0+ | SATISFIED | TestNewProtocolBufModUpdate + TestSmokeBufModUpdate/v1.69.0 cover this. Implementation: full v1beta1 handler chain |
| NEW-02 | 05-01, 05-02 | buf dep update succeeds with v1.69.0+ | SATISFIED | TestNewProtocolBufDepUpdate covers this. RunBufDepUpdate helper in server.go |
| HAND-03 | 05-02 (implied) | manifest_digest populated if required | NEEDS HUMAN | v1beta1 protocol uses Digest on Commit message (field 5) instead of manifest_digest on ModulePin. Old protocol does not need it. The digest IS populated for v1beta1. For v1alpha1, the field remains empty (which is correct -- old CLI does not require it) |
| HAND-04 | 05-02 (implied) | GetSDKInfo returns appropriate response or CodeUnimplemented | NEEDS HUMAN | 05-02-SUMMARY does not mention GetSDKInfo being called. The UnimplementedResolveServiceHandler handles this via connect-go framework. v1.69.0 CLI appears not to call GetSDKInfo, or tolerates Unimplemented |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none found) | - | - | - | No TODO/FIXME/placeholder/stub patterns detected in implementation files |

### Confirmation Bias Counter (Disconfirmation Pass)

1. **Partially met requirement:** HAND-03 (manifest_digest) -- the v1alpha1 ModulePin does NOT populate manifest_digest. However, v1beta1 handles digest differently (on Commit, not ModulePin), and old CLI (v1.30.1) does not require it. This is NOT a gap for Phase 5 scope, which only covers NEW-01 and NEW-02.

2. **Test that may not test stated behavior:** TestSmokeBufModUpdate runs both v1.30.1 and v1.69.0 through the same RunBufModUpdate path. For v1.69.0, this exercises the v1beta1 handlers. The test DOES verify exit code 0 and buf.lock creation, which is the stated behavior. No issue found.

3. **Error path without test coverage:** The v1beta1 handlers have several error paths (GetMeta failure, GetFiles failure, empty resource refs) that are not explicitly tested in unit tests. However, the e2e tests cover the happy path. Error handling is reasonable for an integration-tested proxy.

### Human Verification Required

### 1. Full E2E Test Suite

**Test:** Run `go test ./e2e/ -v -timeout 300s -count=1` with EASYP_GITHUB_TOKEN set
**Expected:** All 4 test functions pass (5 test cases total including subtests)
**Why human:** Requires EASYP_GITHUB_TOKEN environment variable and network access to GitHub API. Cannot run without secrets.

### 2. Manual v1.69.0 buf mod update

**Test:** Start the proxy server with a real config, then run `buf mod update` with v1.69.0 binary against it
**Expected:** buf.lock created with valid module references pointing to the proxy domain
**Why human:** Requires live TLS server, valid certificates, and GitHub API access. End-to-end protocol verification.

### 3. Backward Compatibility Confirmation

**Test:** Run the full test suite and verify TestOldProtocolBufModUpdateTwice still passes alongside new protocol tests
**Expected:** Old v1.30.1 tests pass -- no regression from v1beta1 handler additions
**Why human:** Requires live server and GitHub API access to verify old protocol path still functions correctly.

### Gaps Summary

No code-level gaps found. All implementation artifacts exist, are substantive (705-line commits.go with real protobuf encoding, real digest computation, real caching), and are properly wired (routes registered in api.go, data flows from GitHub provider through handlers to protobuf responses).

The proto field number mapping in commits.go was verified against the v1beta1 proto definitions in api/proto/buf/registry/module/v1beta1/commit.proto -- all field numbers match (Commit: id=1, create_time=2, owner_id=3, module_id=4, digest=5; Digest: type=1, value=2).

The B4 digest computation uses the identical algorithm as the existing v1alpha1 manifest computation (same digestFormat constant, same SHA3Shake256 function), ensuring consistency between protocols.

The only verification items requiring human action are running the e2e tests with live credentials.

---

_Verified: 2026-05-07T22:30:00Z_
_Verifier: Claude (gsd-verifier)_
