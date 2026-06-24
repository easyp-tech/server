---
status: resolved
trigger: "Failure: could not get module data for remote module \"buf-proxy.yadro.dev/googleapis/googleapis\": unknown: invalid content-type: \"text/plain; charset=utf-8\"; expecting \"application/proto\" — buf CLI v1 path /buf.registry.owner.v1.OwnerService/GetOwners falls through to text/plain rootHandler"
created: 2026-06-24
updated: 2026-06-24
---

# Debug Session: buf-v1-owner-route-missing

## Symptoms

- **Expected behavior:** `buf dep update` succeeds when remote module is `buf-proxy.yadro.dev/googleapis/googleapis`
- **Actual behavior:** `Failure: could not get module data for remote module "buf-proxy.yadro.dev/googleapis/googleapis": unknown: invalid content-type: "text/plain; charset=utf-8"; expecting "application/proto"`
- **Error messages:** buf CLI receives `text/plain; charset=utf-8` body of 62 bytes (the "Hello!" health-check string) when calling `POST /buf.registry.owner.v1.OwnerService/GetOwners`
- **Timeline:** Surfaced after the v1 module/commit/graph/download routes were registered (commits `8cf1da5` and `2e88320`). Same class of bug as the resolved `buf-v2-content-type` issue, but for the Owner service.
- **Reproduction:** `buf dep update` against `buf-proxy.yadro.dev/googleapis/googleapis` from a v2 buf.yaml config.
- **Key clue:** Log line `request completed ... path:"/buf.registry.owner.v1.OwnerService/GetOwners" status:200 size:62 content_type:"application/proto" duration:22781` — the 62-byte body is exactly the `rootHandler` "Hello! This is Buf Proxy Service and this is its Health Check!" string. The `content_type:"application/proto"` is the request content-type, not the response; the response content-type is `text/plain; charset=utf-8` (Go's default for `w.Write([]byte(...))`).

## Current Focus

- hypothesis: "The proxy has no handler registered for /buf.registry.owner.v1.OwnerService/ so requests fall through to the text/plain rootHandler."
- test: "Verify by checking route registrations in internal/connect/api.go and matching against log entries for /buf.registry.owner.v1.OwnerService/*."
- expecting: "api.go missing mux.Handle on the v1 OwnerService path, matching exactly the prior buf-v2-content-type bug class."
- next_action: "Confirm and apply the fix (register the v1 OwnerService handler)."
- reasoning_checkpoint: null

## Evidence

- timestamp: 2026-06-24T12:35
  type: log_search
  file: logs/prod-buf-proxy-buf-proxy-858bbb4bb7-6dcdn-buf-proxy.log
  finding: "Log line 203-204: request_id=8c54d3c86a92e29368ac75aa90f13909, method=POST, path=/buf.registry.owner.v1.OwnerService/GetOwners, body_size=36, content_type=application/proto, user_agent=connect-go/1.19.2 (go1.26.3), status=200, size=62, duration=22781ns. size=62 matches the rootHandler string byte length exactly."

- timestamp: 2026-06-24T12:35
  type: log_search
  file: logs/prod-buf-proxy-buf-proxy-858bbb4bb7-6dcdn-buf-proxy.log
  finding: "Log line 222-223 and 1125-1126: identical pattern (status=200, size=62) for /buf.registry.owner.v1.OwnerService/GetOwners from client_ip=192.168.53.189, with body_size=36 and content_type=application/proto. Each call is a clean 200 with a 62-byte body — Go net/http default for the rootHandler text response."

- timestamp: 2026-06-24T12:35
  type: code_analysis
  file: internal/connect/api.go:47-72
  finding: "Registered routes are: v1alpha1 (Resolve/Repository/Download via connect-generated handlers), v1 + v1beta1 (Commit/Graph/Download/Module) for buf CLI 1.69.0+ compatibility. NO registration for /buf.registry.owner.v1.OwnerService/ — that path is not handled by any mux.Handle and falls through to the catch-all rootHandler at line 70."

- timestamp: 2026-06-24T12:35
  type: code_analysis
  file: internal/connect/api.go:29-32
  finding: "rootHandler does w.WriteHeader(200) + w.Write([]byte('Hello! This is Buf Proxy Service and this is its Health Check!')) — Go's net/http sets Content-Type to text/plain; charset=utf-8 by default for []byte writes without a prior Set/Header call. The string is exactly 62 bytes, matching the response size in the log."

- timestamp: 2026-06-24T12:35
  type: code_analysis
  file: gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect/owner.connect.go
  finding: "The v1alpha1 owner.connect.go is generated and exported (v1alpha1connect.NewOwnerServiceHandler, v1alpha1connect.UnimplementedOwnerServiceHandler), but the API has no v1 path registration for OwnerService. The buf CLI uses /buf.registry.owner.v1.OwnerService/GetOwners — a v1 path that has no handler. The v1alpha1 path is registered via mux.Handle on line 49-51, but only for Resolve/Repository/Download."

- timestamp: 2026-06-24T12:35
  type: history_check
  file: .planning/debug/buf-v2-content-type.md (resolved)
  finding: "Identical class of bug resolved previously: missing v1 path registrations for CommitService/GraphService/DownloadService caused text/plain fallthrough. Fix pattern was to add v1 mux.HandleFunc calls mirroring the v1beta1 routes. The same pattern needs to be applied for OwnerService at /buf.registry.owner.v1.OwnerService/."

## Eliminated

- hypothesis: "Proxy returns text/plain because the request body itself is wrong."
  evidence: "Request body is 36 bytes with content_type=application/proto (line 203). The buf CLI is sending the correct request format. The 200 status with size=62 body is the proxy's own response, not the request."
  timestamp: 2026-06-24T12:35

- hypothesis: "The text/plain response comes from a health-check handler accidentally matching the path."
  evidence: "The rootHandler at api.go:29 IS the health-check handler. It matches via the catch-all / route at api.go:70 because no more specific route is registered for /buf.registry.owner.v1.OwnerService/. Same mechanism as the resolved buf-v2-content-type bug."
  timestamp: 2026-06-24T12:35

- hypothesis: "The buf CLI should be calling a v1alpha1 path, not a v1 path, so this is a client misconfiguration."
  evidence: "Buf CLI v1.69.0+ uses v1 paths (as documented in the resolved buf-v1-graph-format session) for module operations. The buf CLI is correctly using /buf.registry.owner.v1.OwnerService/GetOwners — it's the proxy that fails to register this route."
  timestamp: 2026-06-24T12:35

## Resolution

- root_cause: "Missing v1 route registration for /buf.registry.owner.v1.OwnerService/ in internal/connect/api.go. The buf CLI calls OwnerService/GetOwners as part of the v1 module-dep workflow; without a registered handler the request fell through to the text/plain rootHandler (returning 62 bytes of the health-check string), which the buf CLI rejected with the content-type mismatch error."
- fix: "Add a hand-written v1 OwnerService handler that parses GetOwnersRequest, looks up each requested owner against the configured repository set (id and name forms both supported), and returns a synthetic GetOwnersResponse with one Owner{Organization{id, name}} per matched owner. Wire it up in api.go alongside the existing v1/v1beta1 module/owner/commit/graph/download routes. Use raw protowire.Append* to stay consistent with the v1 commit/graph/download handlers and avoid pulling the buf.build v1 owner module as a Go dependency."
- verification: "go build ./... clean. go vet ./... clean. All existing tests pass. 6 new tests pass: route registered, application/proto response for known owner, by-id and by-name lookup forms, unknown-owner omitted, empty body returns 400, GET returns 405."
- files_changed:
  - internal/connect/api.go (add Repositories() to provider interface, build knownOwners map at startup, register the new route)
  - internal/connect/owners.go (new — ServeGetOwners handler + parse/build helpers)
  - internal/connect/commits.go (add knownOwners field to commitServiceHandler struct)
  - internal/connect/api_test.go (mockSource + Repositories on mockProvider + 6 new tests)
