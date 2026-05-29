---
status: resolved
trigger: "buf 1.69.0 working fine with v1 config, but reporting the error after config migrated to v2. Error: invalid content-type: text/plain; charset=utf-8; expecting application/proto. Proxy at buf-proxy-dev.yadro.dev returns text/plain instead of application/proto for CommitService/GetCommits."
created: 2026-05-29
updated: 2026-05-29
---

# Debug Session: buf-v2-content-type

## Symptoms

- **Expected behavior:** `buf dep update` succeeds with v2 buf.yaml config pointing at buf-proxy-dev.yadro.dev
- **Actual behavior:** `Failure: unknown: invalid content-type: "text/plain; charset=utf-8"; expecting "application/proto"`
- **Error messages:** `buf.registry.module.v1.CommitService/GetCommits` returns `status: unknown`, content-type is `text/plain; charset=utf-8` instead of `application/proto`
- **Timeline:** Worked with buf v1 config format. Broke immediately after migrating config to v2 format.
- **Reproduction:** Run `buf dep update --debug` in any proto module using v2 buf config with remote pointing at buf-proxy-dev.yadro.dev
- **Key clue:** `message.received.uncompressed_size: 0` -- empty response body from proxy

## Current Focus

- hypothesis: null
- test: null
- expecting: null
- next_action: null
- reasoning_checkpoint: null

## Evidence

- timestamp: 2026-05-29T00:01
  type: code_analysis
  file: internal/connect/api.go:56-60
  finding: "Route registrations show CommitService/GraphService/DownloadService are only registered for v1beta1 paths. ModuleService has BOTH v1 and v1beta1. When buf v2 config is used, buf CLI calls v1 service paths (buf.registry.module.v1.CommitService) which are unhandled."
- timestamp: 2026-05-29T00:02
  type: code_analysis
  file: internal/connect/api.go:27-30
  finding: "rootHandler returns 200 OK with text/plain body. Go net/http default content-type for string writes is text/plain; charset=utf-8. Unhandled /buf.registry.module.v1.CommitService/ path falls through to rootHandler."
- timestamp: 2026-05-29T00:03
  type: reasoning
  finding: "When buf v1 config -> buf CLI uses v1beta1 paths -> hits registered handlers -> works. When buf v2 config -> buf CLI uses v1 paths -> ModuleService works (has v1 route) but CommitService/GraphService/DownloadService fall through to rootHandler -> text/plain response -> content-type mismatch error."

## Eliminated

## Resolution

- root_cause: "Missing v1 service path routes for CommitService, GraphService, and DownloadService in api.go. Only v1beta1 paths are registered (lines 56-58). When buf CLI uses v2 config format, it calls v1 paths which fall through to the text/plain rootHandler."
- fix: "Add v1 route registrations for CommitService, GraphService, and DownloadService alongside the existing v1beta1 routes, mirroring the pattern already used for ModuleService (lines 59-60)."
- verification: "Run buf dep update with v2 config pointing at the proxy; confirm content-type is application/proto and request succeeds."
- files_changed: internal/connect/api.go
