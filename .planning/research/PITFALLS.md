# Domain Pitfalls: Buf Protocol Proxy Modernization

**Domain:** Buf registry proxy -- adding modern protocol (v1.69.0) alongside deprecated protocol (v1.30.1)
**Researched:** 2026-05-07

## Critical Pitfalls

Mistakes that cause rewrites or major issues.

### Pitfall 1: Assuming Old and New Protocols Need Separate Service Registrations

**What goes wrong:** The developer assumes they need to register two separate sets of Connect RPC handlers -- one for old buf clients, one for new -- mapping to different HTTP paths. They plan to generate two sets of proto stubs with different Go import paths and wire both into the HTTP mux.

**Why it happens:** The project has two proto submodules (`buf` for old, `buf-v1.69.0` for new). The intuitive assumption is "two proto versions means two generated codebases." The `generate.go` file explicitly copies from `_third_party/buf/proto/buf` which reinforces the idea that you must pick one or generate both.

**Reality (confirmed by proto diff):** Both proto sets use the **identical package name** (`buf.alpha.registry.v1alpha1`), **identical service names** (`DownloadService`, `ResolveService`, `RepositoryService`), and **identical HTTP paths** (e.g., `/buf.alpha.registry.v1alpha1.DownloadService/DownloadManifestAndBlobs`). The core proto files the server implements (download.proto, resolve.proto, repository.proto, push.proto, module.proto) differ only in their copyright header. New RPCs were ADDED (GetSDKInfo, GetCargoVersion, etc.) but NONE of the existing RPCs the proxy uses were modified incompatibly. The `is_bsr_head` field was reserved (removed) but the proxy never set it.

**Consequences:** If you try to generate both sets, you get Go package name collisions -- same `package v1alpha1connect`, same types, same handler constructors. If you try to serve them on different paths, neither old nor new buf CLI will find them because both expect the same canonical Connect RPC paths.

**Prevention:** Generate code from the NEW proto set only (`buf-v1.69.0`). The generated code is backward compatible because the proto package, services, and existing RPCs are identical. The buf CLI v1.30.1 and v1.69.0 both call the same HTTP endpoints. The `Unimplemented*Handler` types absorb any new RPCs the proxy does not implement.

**Detection:** Run `diff` on the proto files early (already done -- only copyright headers differ for the RPCs this proxy implements). Check that both buf CLI versions produce the same HTTP requests to the same paths.

**Phase:** Proto generation phase (first implementation phase). This decision shapes all subsequent work.

### Pitfall 2: Forgetting to Update generate.go to Point at the New Proto Submodule

**What goes wrong:** The `api/proto/generate.go` file contains `//go:generate cp -r ../_third_party/buf/proto/buf ./` -- it copies from the OLD proto submodule. If you regenerate after adding new proto-based features, you silently revert to the old proto definitions. Any new RPC stubs you implemented against the new protos disappear.

**Why it happens:** The generate.go was written for the original protocol. There is a second submodule (`buf-v1.69.0`) sitting right next to it, but generate.go has the old path hardcoded. After a clean `go generate`, the generated code is based on the old protos.

**Consequences:** Implemented handlers for new RPCs (GetSDKInfo, etc.) won't compile because the generated stubs don't include them. Or worse: the build succeeds because old stubs are still present, but the new RPCs return `Unimplemented` errors at runtime.

**Prevention:** Update `generate.go` to copy from `../_third_party/buf-v1.69.0/proto/buf` instead of `../_third_party/buf/proto/buf`. Verify by running `go generate ./api/proto/` and checking that generated files include the new RPCs. Keep the old submodule for reference but never generate from it again.

**Detection:** After any `go generate`, check generated files for `GetSDKInfo` or other new RPC names. If absent, generate.go is still pointing at old protos.

**Phase:** First implementation phase -- must be done before any proto regeneration.

### Pitfall 3: Testing with Wrong Buf CLI Binary and Getting Misleading Failures

**What goes wrong:** Tests use a single `buf` binary from `$PATH` instead of explicitly pinned binaries. The developer runs what they think is a v1.30.1 test but actually invokes v1.69.0 (or vice versa). Test failures are attributed to protocol issues when the real problem is the wrong binary.

**Why it happens:** The buf CLI binary name is always `buf` regardless of version. There is no `buf-1.30` vs `buf-1.69` naming convention. If both are installed, `$PATH` determines which runs. Developers may have their system buf at one version and not realize the test picks it up.

**Consequences:** Hours of debugging phantom protocol incompatibilities. A test that "proves" the new protocol works may actually be testing the old protocol. False confidence in protocol compatibility.

**Prevention:**
1. Download both binaries to explicit paths (e.g., `testdata/bin/buf-v1.30.1` and `testdata/bin/buf-v1.69.0`).
2. Never rely on `$PATH` for buf binary resolution in tests.
3. Add a test helper that asserts the buf version before running any protocol test: `buf --version` must match expected string.
4. Consider using `exec.Command` with the full path to the binary, not just `"buf"`.

**Detection:** Log `buf --version` output at the start of every test suite. If the logged version does not match the test's expectation, fail immediately with a clear error message.

**Phase:** Test infrastructure phase (must be in place before any protocol tests run).

### Pitfall 4: Self-Signed TLS Certificate Trust Issues in Tests

**What goes wrong:** The buf CLI refuses to connect to the test TLS server because it does not trust the self-signed CA certificate. The test fails with a TLS handshake error, not a protocol error. The developer misdiagnoses this as a protocol problem.

**Why it happens:** The project uses self-signed certs at `~/local-tls/server/`. Even though they are "added to the local CA," the buf CLI uses its own TLS configuration and may not respect the system CA store. Different operating systems handle this differently. The buf CLI is a Go binary and uses the Go TLS stack, which reads the system cert pool differently from macOS Keychain.

**Consequences:** Tests fail on machines where the CA is not properly trusted. Tests pass on one developer's machine but fail in CI. The failure message is a generic TLS error that gives no indication the issue is certificate trust, not the protocol implementation.

**Prevention:**
1. In tests, start the TLS server with certs from a known, test-controlled location (not `~/local-tls/server/` which may not exist on CI).
2. Generate test certs as part of the test setup using `crypto/tls` and `crypto/x509` -- Go's standard library can create ephemeral certs programmatically.
3. For the buf CLI invocation, set `SSL_CERT_FILE` environment variable to point to the test CA cert. Go's TLS stack respects this variable.
4. Alternatively, use `httptest.NewTLSServer` which creates a server with a test CA, then configure the buf CLI to trust that CA.

**Detection:** If `buf mod update` fails with `tls: certificate verification failed` or `x509: certificate signed by unknown authority`, the issue is TLS trust, not protocol. Always check the full error chain.

**Phase:** Test infrastructure phase. Must be solved before any protocol test can run.

### Pitfall 5: Flaky Tests from Real GitHub API Calls

**What goes wrong:** Tests that hit the real GitHub API fail intermittently due to API rate limits (5000 requests/hour), network latency, or GitHub downtime. The developer treats these flaky failures as protocol bugs and wastes time debugging.

**Why it happens:** The project explicitly requires "real buf binary + real TLS server + real GitHub API." This means tests are subject to GitHub's availability and rate limits. A burst of test runs during development can exhaust the rate limit. The proxy makes one tree request plus one request per file per `GetFiles` call -- a repository with 500 proto files uses 501 API calls per cache miss.

**Consequences:** Tests become unreliable. Developers start ignoring failures ("it's just GitHub rate limiting"), which means real bugs slip through. CI becomes noisy and trust in the test suite erodes.

**Prevention:**
1. Use a GitHub token with high rate limits (GitHub App tokens can have 15,000 requests/hour).
2. Use a small, stable repository for testing (e.g., a dedicated test repo with 2-3 proto files, not `googleapis/googleapis`).
3. Cache GitHub API responses at the test level: on the first run, record responses; on subsequent runs, replay them. This is NOT the same as mocking -- the first run proves the real integration works.
4. Mark real-API tests with a build tag (e.g., `//go:build integration`) so they can be excluded from fast test runs.
5. Add a GitHub API rate limit check at test start -- if remaining calls < threshold, skip with a clear message rather than fail.

**Detection:** Test failures that mention `403 Forbidden` with `rate limit` in the response body. Failures that only occur after running tests multiple times in quick succession.

**Phase:** Test implementation phase. Apply prevention from the start -- retrofitting a recording layer after tests are flaky is much harder.

## Moderate Pitfalls

### Pitfall 6: buf CLI Requires Specific Config File Format to Point at Custom Registry

**What goes wrong:** The buf CLI is configured via `buf.yaml` (or `buf.yaml` v2 format). The developer forgets to configure the test's `buf.yaml` to point `registry` at `https://localhost:<port>` instead of the default `buf.build`. The test silently hits the real BSR, which either succeeds (masking the bug) or fails with an unrelated error.

**Prevention:**
1. Each test case must have its own `buf.yaml` with the `registry` field set to the test server's address.
2. Use a temp directory for each test with a freshly written `buf.yaml` -- never rely on a pre-existing one.
3. Assert in the test that the proxy received the request (e.g., check proxy logs or request counter).

**Detection:** If the proxy logs show no incoming requests during a test run, the buf CLI is talking to the wrong server.

### Pitfall 7: Connect RPC Version Mismatch Between Generated Code and Runtime Library

**What goes wrong:** The project uses `connectrpc.com/connect v1.11.1` (from go.mod). After regenerating proto stubs from the new `buf-v1.69.0` protos, the generated code may use features or APIs not available in v1.11.1. Compilation fails with cryptic "undefined" errors.

**Why it happens:** The `connect-go` protoc plugin version used during generation may be newer than v1.11.1. The buf tool itself bundles a specific version of the `protoc-gen-connect-go` plugin, and that version may generate code requiring newer connect-go runtime features.

**Prevention:**
1. Update `connectrpc.com/connect` to the latest version in go.mod BEFORE regenerating proto stubs.
2. After regeneration, run `go build ./...` immediately. Any undefined references indicate a version mismatch.
3. Pin the `protoc-gen-connect-go` plugin version in `buf.gen.yaml` if possible, or verify that the buf CLI's bundled plugin version matches the runtime library version.

**Detection:** Build errors referencing `connect` package methods that don't exist in v1.11.1.

### Pitfall 8: HTTP/2 Required for gRPC Protocol but Server Only Supports HTTP/1.1

**What goes wrong:** The current server uses `http.ListenAndServeTLS` with no HTTP/2 configuration. The buf CLI may send gRPC protocol requests (not Connect protocol), which require HTTP/2. The server responds with HTTP/1.1 and the request fails.

**Why it happens:** Go's `net/http` package automatically enables HTTP/2 when using `ListenAndServeTLS` (via `http2.ConfigureServer`). However, if the TLS configuration is customized (which it is -- the server sets `TLSConfig` for mTLS), this automatic configuration may not happen. The current code calls `server.ListenAndServeTLS(certFile, keyFile)` which should auto-configure HTTP/2, but if `TLSConfig.NextProtos` is manually set, it could disable HTTP/2 negotiation.

**Prevention:**
1. Verify HTTP/2 support by testing with `grpcurl` or `buf curl` using the gRPC protocol explicitly.
2. If needed, explicitly configure HTTP/2: `http2.ConfigureServer(server, &http2.Server{})`.
3. The Connect protocol works over HTTP/1.1, but gRPC protocol requires HTTP/2. The buf CLI may use either. The server should support both.

**Detection:** Requests fail only when the buf CLI uses gRPC wire format (not Connect protocol). Test with both protocols explicitly.

### Pitfall 9: Test Server Port Conflicts When Running Tests in Parallel

**What goes wrong:** Two test functions start TLS servers on the same port. One fails to bind. The failure message says "address already in use" which looks like a transient issue, but it happens consistently when `-count=N` or parallel test flags are used.

**Why it happens:** The test code binds to a fixed port (e.g., `:8443`) rather than port 0 (OS-assigned). Running `go test -count=2` or multiple packages in parallel causes conflicts.

**Prevention:**
1. Always use `:0` as the listen address in tests -- the OS assigns a free port.
2. Read the actual assigned port from the listener before starting the test client.
3. Use `httptest.NewUnstartedServer` or manually create a `net.Listener` on `:0`.

**Detection:** Tests pass individually but fail when run with `-count=2` or `-parallel=4`.

### Pitfall 10: Forgetting to Handle Unimplemented RPCs Gracefully

**What goes wrong:** The new proto set adds RPCs (GetSDKInfo, GetCargoVersion, GetNugetVersion, GetCmakeVersion, AddRepositoryGroup, etc.) that the proxy does not implement. When the buf CLI v1.69.0 calls one of these, the proxy returns `Unimplemented`. The buf CLI may treat this as a fatal error and abort the entire operation, even though the proxy correctly handles the core RPCs the operation needs.

**Why it happens:** The `Unimplemented*Handler` types from the generated Connect code return `connect.CodeUnimplemented` for any RPC the server does not override. The buf CLI may probe for optional features (like GetSDKInfo) and fail hard if they are not available, rather than gracefully degrading.

**Prevention:**
1. Test with real buf CLI v1.69.0 early to discover which RPCs it actually calls during common operations (`buf mod update`, `buf build`, `buf export`).
2. If the CLI requires RPCs the proxy does not support, implement stub handlers that return empty/valid responses rather than `Unimplemented`.
3. Monitor buf CLI error messages -- they may indicate which RPC must be implemented.

**Detection:** buf CLI operations fail with errors like "unimplemented" or "not found" when the core data RPCs (download, resolve, repository) should be working.

## Minor Pitfalls

### Pitfall 11: Race Condition in Test Server Startup

**What goes wrong:** The test starts the server in a goroutine but immediately proceeds to run the buf CLI before the server is ready to accept connections. The first request fails with "connection refused."

**Prevention:** After starting the server, make a test HTTP request to the health check endpoint (`/`) to confirm the server is up before running buf CLI commands. Alternatively, use a channel or sync primitive to signal readiness.

### Pitfall 12: Test Cleanup Does Not Kill Server Process

**What goes wrong:** The test starts a server process but `t.Cleanup` does not properly shut it down. Leftover server processes hold ports open, causing subsequent test runs to fail with "address in use."

**Prevention:** Always use `t.Cleanup(func() { server.Shutdown(ctx) })` or equivalent. Use `httptest.NewServer` which handles cleanup automatically. If using `exec.Command` for the server, kill the process in cleanup.

### Pitfall 13: Generated Proto Code Committed to Git Causes Merge Conflicts

**What goes wrong:** The `gen/proto/` directory is committed to the repo. After regenerating with new protos, every generated file changes (copyright header years, new RPC stubs). If multiple developers regenerate independently, merge conflicts in generated code are difficult to resolve.

**Prevention:** Regenerate proto code in a controlled manner -- one developer, one PR. Do not regenerate as part of every build. Consider whether `gen/proto/` should be `.gitignore`d and regenerated on demand (requires `buf` CLI in CI), though this conflicts with the current approach of committing generated code.

### Pitfall 14: The `is_bsr_head` Field Removal Silently Breaking Old Clients

**What goes wrong:** The new proto set removes the `is_bsr_head` field (field number 4 in `LocalModuleResolveResult`, now `reserved`). If the proxy generates responses using the NEW proto types but an old buf CLI v1.30.1 expects this field, it may behave differently. Conversely, if the old client sends a request that includes `is_bsr_head`, the new proto parser silently drops it.

**Prevention:** The current proxy code never sets `is_bsr_head` (the `resolveModulePin` function in `modulepins.go` creates `ModulePin` without it, and the proxy does not implement `LocalResolveService`). The risk is LOW but should be verified by testing with old buf CLI after proto migration.

**Detection:** Test with buf CLI v1.30.1 after regenerating from new protos. If `buf mod update` still works, no issue.

### Pitfall 15: Buf.gen.yaml Option `M` Mappings Not Updated for New Proto Files

**What goes wrong:** The `buf.gen.yaml` file contains explicit `Mbuf/alpha/...` mappings for the `go` and `go-grpc` plugins. These mappings control the Go import paths for generated code. If new proto files are added in the new proto set (files not present in the old set), they will have incorrect or missing import path mappings, leading to Go compilation errors.

**Prevention:** After switching generate.go to the new proto submodule, run `buf generate` and check for any new proto files not covered by existing `M` mappings. Add mappings for any new files. Alternatively, configure the module's `go_package` option in the proto files themselves (though these are third-party files).

**Detection:** Go compilation errors like `package xxx is not in GOROOT` or undefined types referencing unexpected import paths.

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Proto generation setup | Pitfall 2: generate.go points at old protos | Update generate.go path FIRST, before any regeneration |
| Proto generation setup | Pitfall 7: Connect runtime version mismatch | Update connectrpc.com/connect in go.mod before regenerating |
| Proto generation setup | Pitfall 15: Missing M mappings for new proto files | Run `go build ./...` immediately after regeneration |
| Test infrastructure | Pitfall 3: Wrong buf binary used | Pin binaries to explicit paths, assert version in test helper |
| Test infrastructure | Pitfall 4: TLS trust issues | Generate test certs programmatically or use SSL_CERT_FILE |
| Test infrastructure | Pitfall 9: Port conflicts in parallel tests | Always bind to `:0` in test servers |
| Test infrastructure | Pitfall 11: Server not ready when test starts | Health check probe before running buf CLI |
| Old protocol tests | Pitfall 1: Unnecessary dual registration | Old protocol tests should pass with single new proto generation |
| Old protocol tests | Pitfall 14: is_bsr_head field removal | Verify buf v1.30.1 still works after proto migration |
| New protocol tests | Pitfall 5: GitHub API rate limiting | Use small test repo, record/replay, rate limit checks |
| New protocol tests | Pitfall 6: buf.yaml registry not pointing at test server | Create fresh buf.yaml per test in temp dir |
| New protocol tests | Pitfall 10: Unimplemented RPCs causing CLI failure | Discover which RPCs v1.69.0 actually calls |
| Protocol coexistence | Pitfall 1: Thinking two registrations needed | Single set of handlers serves both client versions |
| Protocol coexistence | Pitfall 8: HTTP/2 not configured for gRPC clients | Test with both Connect and gRPC protocol clients |

## Key Insight: This Is Simpler Than It Appears

The proto diff reveals that the core protocol is essentially unchanged between v1.30.1 and v1.69.0. Both versions of the buf CLI call the same RPCs on the same HTTP paths with the same message types. The new proto version adds RPCs that the proxy does not need to implement (the `Unimplemented` handlers will cover them). The primary risk is not protocol incompatibility but rather:

1. Test infrastructure complexity (TLS, subprocess management, port allocation)
2. External API flakiness (GitHub rate limits)
3. Build chain issues (generate.go pointing at wrong submodule, version mismatches)

The biggest pitfall is the psychological one: over-engineering a "dual protocol" solution when a single proto generation from the newer definitions serves both client versions.

## Sources

- Proto diff analysis: `api/_third_party/buf/` vs `api/_third_party/buf-v1.69.0/` (direct filesystem comparison)
- Connect RPC documentation: context7 `/connectrpc/connect-go` (HIGH confidence)
- Buf CLI stability policy: [github.com/bufbuild/buf README](https://github.com/bufbuild/buf) -- "no breaking changes within v1.x" (HIGH confidence)
- BSR on-prem TLS guidance: [buf.build/docs/bsr/admin/on-prem/installation/](https://buf.build/docs/bsr/admin/on-prem/installation/) (MEDIUM confidence)
- Connect protocol specification: [connectrpc.com/docs/protocol/](https://connectrpc.com/docs/protocol/) (HIGH confidence)
- Codebase analysis: `.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/CONCERNS.md`, `.planning/codebase/TESTING.md` (HIGH confidence -- direct analysis)

---

*Pitfalls analysis: 2026-05-07*
