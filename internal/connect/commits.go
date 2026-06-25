package connect

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/source"
	"github.com/easyp-tech/server/internal/reqid"
	"github.com/easyp-tech/server/internal/shake256"
	"google.golang.org/protobuf/encoding/protowire"
)

type commitInfoCache struct {
	commitID string // buf-issued deterministic id (the one buf sends in DownloadRequest)
	commit   string // resolved git commit hash (the one the upstream actually points at)
	ownerID  string
	moduleID string
	digest   []byte
}

type commitServiceHandler struct {
	api *api

	commitMu  sync.RWMutex
	commitMap map[string]moduleRef       // commitID → owner/module
	infoCache map[string]commitInfoCache // "owner/module" → cached commit info
	filesMap  map[string][]content.File  // commitID → cached files
	// knownOwners is a deterministic-id → owner-name lookup populated
	// at startup from the configured repositories. Used by the
	// buf.registry.owner.v1.OwnerService/GetOwners handler to answer
	// owner-info lookups from the buf CLI during `buf dep update`.
	knownOwners map[string]string
	// singleModule is the sole configured module when the deployment
	// serves exactly one (the common case). Used by the
	// ModuleService/GetModules foreign-module-id fallback; nil otherwise
	// so the fallback never guesses across multiple modules.
	singleModule *moduleRef

	// prewarmOnce guards prewarmHeads against double-execution.
	prewarmOnce sync.Once
	// missCache holds the time a commit_id was last confirmed absent from
	// every configured source (probe all-fail). Used by probeCommitID to
	// skip repeated upstream fan-out for known-bogus shas within ProbeTTL.
	missCache map[string]time.Time

	// runtime knobs (set in connect.New from config.Connect.WithDefaults)
	prewarmEnabled   bool
	prewarmTimeout   time.Duration
	probeEnabled     bool
	probeNegativeTTL time.Duration
	probeTimeout     time.Duration

	// probeSem bounds the number of concurrent upstream sha probes, capping
	// the N×(sources) fan-out an attacker can trigger by flooding distinct
	// unknown shas. Acquired/released around probeCommitID's body.
	probeSem chan struct{}
}

// maxConcurrentProbes caps simultaneous probeCommitID fan-outs. Each probe
// issues up to len(sources) upstream GetMeta calls, so this bounds worst-case
// upstream load from the probe path.
const maxConcurrentProbes = 4

// hlog returns a logger that carries the per-request correlation id so
// handler decision-trace lines join up with the access log.
func (h *commitServiceHandler) hlog(r *http.Request) *slog.Logger {
	if id := reqid.From(r.Context()); id != "" {
		return h.api.log.With(slog.String("request_id", id))
	}
	return h.api.log
}

// protocolLabel returns the buf API protocol name implied by a request path.
func protocolLabel(isV1 bool) string {
	if isV1 {
		return "v1"
	}
	return "v1beta1"
}

func (h *commitServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logHandlerError(r, w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.badRequest(r, w, "reading body", slog.String("read_error", err.Error()))
		return
	}

	refs := parseResourceRefs(body)
	isV1 := !strings.Contains(r.URL.Path, "v1beta1")
	h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
		slog.String("handler", "ServeHTTP"),
		slog.String("procedure", "CommitService/GetCommits"),
		slog.String("protocol", protocolLabel(isV1)),
		slog.Int("refs", len(refs)),
		slog.Int("body_bytes", len(body)),
	)
	if len(refs) == 0 {
		h.badRequest(r, w, "no resource refs", slog.Int("body_bytes", len(body)))
		return
	}

	type commitInfo struct {
		ownerID  string
		moduleID string
		commitID string
		digest   []byte
	}
	commits := make([]commitInfo, 0, len(refs))
	for _, ref := range refs {
		h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
			slog.String("handler", "ServeHTTP"),
			slog.String("procedure", "CommitService/GetCommits"),
			slog.String("branch", "resolve_meta"),
			slog.String("owner", ref.owner),
			slog.String("module", ref.module),
			slog.String("repo", ref.module),
		)
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, "")
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("resolving %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("upstream_error", err.Error()))
			return
		}
		cid := meta.Commit
		h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
			slog.String("handler", "ServeHTTP"),
			slog.String("procedure", "CommitService/GetCommits"),
			slog.String("branch", "compute_digest"),
			slog.String("owner", ref.owner),
			slog.String("module", ref.module),
			slog.String("repo", ref.module),
			slog.String("commit", meta.Commit),
			slog.String("commit_id", cid),
			slog.Bool("is_v1", isV1),
		)
		digest, err := h.computeB4Digest(r, ref, meta.Commit)
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("computing digest for %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", cid),
				slog.String("upstream_error", err.Error()))
			return
		}
		if isV1 {
			digest, err = toB5Digest(digest)
			if err != nil {
				h.upstreamError(r, w, fmt.Sprintf("wrapping digest for %s/%s", ref.owner, ref.module),
					slog.String("owner", ref.owner), slog.String("module", ref.module),
					slog.String("commit", meta.Commit),
					slog.String("commit_id", cid),
					slog.String("upstream_error", err.Error()))
				return
			}
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeHTTP"),
				slog.String("procedure", "CommitService/GetCommits"),
				slog.String("branch", "digest_b5_wrap"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", cid),
				slog.Bool("is_v1", isV1),
			)
		} else {
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeHTTP"),
				slog.String("procedure", "CommitService/GetCommits"),
				slog.String("branch", "digest_b4_keep"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", cid),
				slog.Bool("is_v1", isV1),
			)
		}
		commits = append(commits, commitInfo{
			ownerID:  ref.owner,
			moduleID: ref.owner + "/" + ref.module,
			commitID: cid,
			digest:   digest,
		})
		h.commitMu.Lock()
		h.commitMap[cid] = ref
		h.infoCache[ref.owner+"/"+ref.module] = commitInfoCache{
			commitID: cid,
			commit:   meta.Commit,
			ownerID:  ref.owner,
			moduleID: ref.owner + "/" + ref.module,
			digest:   digest,
		}
		h.commitMu.Unlock()
	}

	var respMsg []byte
	for _, c := range commits {
		var commit []byte
		commit = protowire.AppendTag(commit, 1, protowire.BytesType)
		commit = protowire.AppendString(commit, c.commitID)
		commit = protowire.AppendTag(commit, 3, protowire.BytesType)
		commit = protowire.AppendString(commit, c.ownerID)
		commit = protowire.AppendTag(commit, 4, protowire.BytesType)
		commit = protowire.AppendString(commit, c.moduleID)
		// Field 5: Digest (DigestType=1/B4, value=64-byte shake256)
		var digest []byte
		digest = protowire.AppendTag(digest, 1, protowire.VarintType)
		digest = protowire.AppendVarint(digest, 1) // B4
		digest = protowire.AppendTag(digest, 2, protowire.BytesType)
		digest = protowire.AppendBytes(digest, c.digest)
		commit = protowire.AppendTag(commit, 5, protowire.BytesType)
		commit = append(commit, protowire.AppendVarint(nil, uint64(len(digest)))...)
		commit = append(commit, digest...)
		respMsg = protowire.AppendTag(respMsg, 1, protowire.BytesType)
		respMsg = append(respMsg, protowire.AppendVarint(nil, uint64(len(commit)))...)
		respMsg = append(respMsg, commit...)
	}

	w.Header().Set("Content-Type", "application/proto")
	_, _ = w.Write(respMsg)
}

// ServeGraph handles v1beta1 GraphService/GetGraph.
// Returns a minimal graph with one commit per module ref and no edges
// (no transitive dependencies for our single-module proxy use case).
func (h *commitServiceHandler) ServeGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logHandlerError(r, w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.badRequest(r, w, "reading body", slog.String("read_error", err.Error()))
		return
	}

	// Parse GetGraphRequest - handle both v1 and v1beta1 formats:
	// v1beta1: field 1 = repeated GetGraphRequest_ResourceRef { ResourceRef, Registry }
	isV1 := !strings.Contains(r.URL.Path, "v1beta1")
	var refs []moduleRef
	if isV1 {
		refs = parseGetGraphResourceRefsV1(body)
	} else {
		refs = parseGetGraphResourceRefs(body)
	}
	h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
		slog.String("handler", "ServeGraph"),
		slog.String("procedure", "GraphService/GetGraph"),
		slog.String("protocol", protocolLabel(isV1)),
		slog.Int("refs", len(refs)),
		slog.Int("body_bytes", len(body)),
		slog.String("branch", "request_parsed"),
	)
	if len(refs) == 0 {
		// Return empty graph
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write(nil)
		return
	}

	type commitInfo struct {
		ownerID  string
		moduleID string
		commitID string
		owner    string
		module   string
		digest   []byte
	}
	commits := make([]commitInfo, 0, len(refs))
	for _, ref := range refs {
		key := ref.owner + "/" + ref.module
		h.commitMu.RLock()
		cached, ok := h.infoCache[key]
		h.commitMu.RUnlock()
		if ok {
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeGraph"),
				slog.String("procedure", "GraphService/GetGraph"),
				slog.String("branch", "info_cache_hit"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", cached.commit),
				slog.String("commit_id", cached.commitID),
				slog.Bool("is_v1", isV1),
			)
			commits = append(commits, commitInfo{
				ownerID:  cached.ownerID,
				moduleID: cached.moduleID,
				commitID: cached.commitID,
				owner:    ref.owner,
				module:   ref.module,
				digest:   cached.digest,
			})
			continue
		}
		h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
			slog.String("handler", "ServeGraph"),
			slog.String("procedure", "GraphService/GetGraph"),
			slog.String("branch", "info_cache_miss"),
			slog.String("owner", ref.owner),
			slog.String("module", ref.module),
			slog.String("repo", ref.module),
		)
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, "")
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("resolving %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("upstream_error", err.Error()))
			return
		}
		cid := meta.Commit
		digest, err := h.computeB4Digest(r, ref, meta.Commit)
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("computing digest for %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", cid),
				slog.String("upstream_error", err.Error()))
			return
		}
		if isV1 {
			digest, err = toB5Digest(digest)
			if err != nil {
				h.upstreamError(r, w, fmt.Sprintf("converting digest for %s/%s", ref.owner, ref.module),
					slog.String("owner", ref.owner), slog.String("module", ref.module),
					slog.String("repo", ref.module),
					slog.String("commit", meta.Commit),
					slog.String("commit_id", cid),
					slog.String("upstream_error", err.Error()))
				return
			}
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeGraph"),
				slog.String("procedure", "GraphService/GetGraph"),
				slog.String("branch", "digest_b5_wrap"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", cid),
				slog.Bool("is_v1", isV1),
			)
		} else {
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeGraph"),
				slog.String("procedure", "GraphService/GetGraph"),
				slog.String("branch", "digest_b4_keep"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", cid),
				slog.Bool("is_v1", isV1),
			)
		}
		// Write the freshly-resolved commit back into the local cache so that a
		// subsequent DownloadService/Download call (buf 1.69.0+ v1 workflow:
		// GetModules -> GetGraph -> Download) finds the commit_id without first
		// requiring CommitService/GetCommits. Without this, ServeDownload's
		// commit_id_lookup branch returns ref_found=false and replies 400
		// "unknown commit id: must call CommitService/GetCommits first".
		h.commitMu.Lock()
		h.commitMap[cid] = ref
		h.infoCache[ref.owner+"/"+ref.module] = commitInfoCache{
			commitID: cid,
			commit:   meta.Commit,
			ownerID:  ref.owner,
			moduleID: ref.owner + "/" + ref.module,
			digest:   digest,
		}
		h.commitMu.Unlock()
		h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
			slog.String("handler", "ServeGraph"),
			slog.String("procedure", "GraphService/GetGraph"),
			slog.String("branch", "info_cache_writeback"),
			slog.String("owner", ref.owner),
			slog.String("module", ref.module),
			slog.String("repo", ref.module),
			slog.String("commit", meta.Commit),
			slog.String("commit_id", cid),
			slog.Bool("is_v1", isV1),
		)
		commits = append(commits, commitInfo{
			ownerID:  ref.owner,
			moduleID: ref.owner + "/" + ref.module,
			commitID: cid,
			owner:    ref.owner,
			module:   ref.module,
			digest:   digest,
		})
	}

	// Build Graph response.
	// v1:       Graph.commits = repeated Commit (direct, no wrapper, no registry)
	// v1beta1:  Graph.commits = repeated Graph_Commit { Commit, Registry }
	// Both:     GetGraphResponse { field 1: Graph { field 1: commits, field 2: edges (empty) } }
	var graphMsg []byte
	for _, c := range commits {
		commit := buildCommitRaw(c.commitID, c.ownerID, c.moduleID, c.digest)

		if isV1 {
			// v1: Commit goes directly into Graph.commits (field 1)
			graphMsg = protowire.AppendTag(graphMsg, 1, protowire.BytesType)
			graphMsg = append(graphMsg, protowire.AppendVarint(nil, uint64(len(commit)))...)
			graphMsg = append(graphMsg, commit...)
		} else {
			// v1beta1: wrap in Graph_Commit { field 1 = Commit, field 2 = Registry }
			var graphCommit []byte
			graphCommit = protowire.AppendTag(graphCommit, 1, protowire.BytesType)
			graphCommit = append(graphCommit, protowire.AppendVarint(nil, uint64(len(commit)))...)
			graphCommit = append(graphCommit, commit...)
			graphCommit = protowire.AppendTag(graphCommit, 2, protowire.BytesType)
			graphCommit = protowire.AppendString(graphCommit, h.api.domain)

			graphMsg = protowire.AppendTag(graphMsg, 1, protowire.BytesType)
			graphMsg = append(graphMsg, protowire.AppendVarint(nil, uint64(len(graphCommit)))...)
			graphMsg = append(graphMsg, graphCommit...)
		}
	}

	// Wrap Graph in GetGraphResponse: field 1 (Graph)
	var respMsg []byte
	respMsg = protowire.AppendTag(respMsg, 1, protowire.BytesType)
	respMsg = append(respMsg, protowire.AppendVarint(nil, uint64(len(graphMsg)))...)
	respMsg = append(respMsg, graphMsg...)

	w.Header().Set("Content-Type", "application/proto")
	_, _ = w.Write(respMsg)
}

// ServeDownload handles v1beta1 DownloadService/Download.
func (h *commitServiceHandler) ServeDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logHandlerError(r, w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.badRequest(r, w, "reading body", slog.String("read_error", err.Error()))
		return
	}

	var ref *moduleRef
	commitID := parseResourceRefID(body)
	if commitID != "" {
		h.commitMu.RLock()
		if mapped, ok := h.commitMap[commitID]; ok {
			r := mapped
			ref = &r
		}
		h.commitMu.RUnlock()
	}
	// Build lookup-attrs in one place: when ref is found, include owner/module/commit
	// so the line is independently useful for correlation with prior GetCommits traffic.
	// When ref is NOT found, we still want commit_id visible to operators.
	lookupAttrs := []slog.Attr{
		slog.String("handler", "ServeDownload"),
		slog.String("procedure", "DownloadService/Download"),
		slog.String("branch", "commit_id_lookup"),
		slog.String("commit_id", commitID),
		slog.Bool("ref_found", ref != nil),
		slog.Int("body_bytes", len(body)),
	}
	if ref != nil {
		lookupAttrs = append(lookupAttrs,
			slog.String("owner", ref.owner),
			slog.String("module", ref.module),
			slog.String("repo", ref.module),
		)
		// Resolve the git commit this commit_id was minted from. infoCache is the
		// only place we keep it; commitMap only stores the ref.
		h.commitMu.RLock()
		if info, ok := h.infoCache[ref.owner+"/"+ref.module]; ok && info.commitID == commitID {
			lookupAttrs = append(lookupAttrs, slog.String("commit", info.commit))
		}
		h.commitMu.RUnlock()
	}
	h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision", lookupAttrs...)
	if ref == nil {
		// Foreign / cached commit_id fallback.
		//
		// commitMap is keyed only by ids this proxy mints, but a buf CLI client
		// may send a commit_id it cached from a different registry (e.g. real
		// buf.build, pinned in buf.lock). That id will never match commitMap,
		// yet the module the client wants is one this proxy serves. Before
		// rejecting, try to resolve the module identity:
		//
		//   1. If infoCache has exactly one entry, the proxy is serving a single
		//      active module (the common single-module deployment) — use it.
		//   2. Otherwise we cannot tell which module a foreign id refers to;
		//      fall through to the 400 so we never serve the wrong content.
		//
		// On a successful resolution, register the foreign commit_id as an alias
		// of the resolved module's ref in commitMap so subsequent identical
		// requests are served directly without re-running the fallback.
		resolved := h.resolveForeignCommitID(commitID)
		if resolved != nil {
			ref = resolved
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeDownload"),
				slog.String("procedure", "DownloadService/Download"),
				slog.String("branch", "foreign_commit_id_fallback"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("foreign_commit_id", commitID),
			)
		}
	}
	if ref == nil && h.probeEnabled {
		// Last resort: ask each configured source whether it owns this sha.
		// Recovers any real commit the proxy never resolved this session
		// (multi-module deployments where the single-module fallback cannot
		// disambiguate). A git sha is unique to one repo, so there is no
		// cross-module ambiguity. Negative-cached on all-fail.
		probed, ok := h.probeCommitID(r.Context(), commitID)
		if ok && probed != nil {
			ref = probed
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeDownload"),
				slog.String("procedure", "DownloadService/Download"),
				slog.String("branch", "commit_id_probe_hit"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", commitID),
				slog.String("commit_id", commitID),
			)
		} else {
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeDownload"),
				slog.String("procedure", "DownloadService/Download"),
				slog.String("branch", "commit_id_probe_miss"),
				slog.String("commit_id", commitID),
				slog.Bool("negative_cached", h.missCached(commitID)),
			)
		}
	}
	if ref == nil {
		// Truly unresolvable: no commitMap hit and no module identity we can
		// fall back to. Surface that explicitly, including the id itself so
		// operators can correlate with prior GetCommits traffic.
		h.badRequest(r, w, "unknown commit id: must call CommitService/GetCommits first",
			slog.String("commit_id", commitID),
			slog.Int("body_bytes", len(body)))
		return
	}

	var cid string
	h.commitMu.RLock()
	cached, infoOK := h.infoCache[ref.owner+"/"+ref.module]
	cachedFiles := h.filesMap[cached.commitID]
	h.commitMu.RUnlock()

	h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
		slog.String("handler", "ServeDownload"),
		slog.String("procedure", "DownloadService/Download"),
		slog.String("branch", "files_cache_lookup"),
		slog.String("owner", ref.owner),
		slog.String("module", ref.module),
		slog.String("repo", ref.module),
		slog.String("commit", cached.commit),
		slog.String("commit_id", cached.commitID),
		slog.Bool("info_cache_hit", infoOK),
		slog.Int("cached_files", len(cachedFiles)),
	)

	var files []content.File
	var digest []byte
	if infoOK && len(cachedFiles) > 0 {
		cid = cached.commitID
		files = cachedFiles
		digest = cached.digest
		h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
			slog.String("handler", "ServeDownload"),
			slog.String("procedure", "DownloadService/Download"),
			slog.String("branch", "files_cache_hit"),
			slog.String("owner", ref.owner),
			slog.String("module", ref.module),
			slog.String("repo", ref.module),
			slog.String("commit", cached.commit),
			slog.String("commit_id", cached.commitID),
			slog.Int("files", len(files)),
		)
	} else {
		h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
			slog.String("handler", "ServeDownload"),
			slog.String("procedure", "DownloadService/Download"),
			slog.String("branch", "files_cache_miss"),
			slog.String("owner", ref.owner),
			slog.String("module", ref.module),
			slog.String("repo", ref.module),
			slog.String("commit_id", commitID),
		)
		// Fetch the content for the requested commit, not always HEAD.
		// commit_id is a raw git sha (post 688f058): fetching by it returns
		// the exact content the client asked for, including non-HEAD commits
		// recovered via probeCommitID. A foreign-id alias
		// (resolveForeignCommitID) is not a real sha, so GetMeta(commitID)
		// fails and we fall back to the resolved HEAD commit (cached.commit)
		// the alias was bound to — preserving prior single-module behavior.
		fetchCommit := commitID
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, fetchCommit)
		if err != nil && cached.commit != "" && cached.commit != commitID {
			fetchCommit = cached.commit
			meta, err = h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, fetchCommit)
		}
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("resolving %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit_id", commitID),
				slog.String("fetch_commit", fetchCommit),
				slog.String("upstream_error", err.Error()))
			return
		}
		files, err = h.api.repo.GetFiles(r.Context(), ref.owner, ref.module, meta.Commit)
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("getting files for %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", commitID),
				slog.String("upstream_error", err.Error()))
			return
		}
		cid = meta.Commit
		digest, _ = h.computeB4DigestFromFiles(files)
		isV1 := !strings.Contains(r.URL.Path, "v1beta1")
		if isV1 {
			digest, _ = toB5Digest(digest)
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeDownload"),
				slog.String("procedure", "DownloadService/Download"),
				slog.String("branch", "digest_b5_wrap"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", cid),
				slog.Bool("is_v1", isV1),
			)
		} else {
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeDownload"),
				slog.String("procedure", "DownloadService/Download"),
				slog.String("branch", "digest_b4_keep"),
				slog.String("owner", ref.owner),
				slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
				slog.String("commit_id", cid),
				slog.Bool("is_v1", isV1),
			)
		}
	}

	commit := buildCommitRaw(cid, cached.ownerID, cached.moduleID, digest)

	// repeated File: each File has path=1(string) and content=2(bytes)
	var filesMsg []byte
	for _, f := range files {
		var fileMsg []byte
		fileMsg = protowire.AppendTag(fileMsg, 1, protowire.BytesType)
		fileMsg = protowire.AppendString(fileMsg, f.Path)
		fileMsg = protowire.AppendTag(fileMsg, 2, protowire.BytesType)
		fileMsg = protowire.AppendBytes(fileMsg, f.Data)
		filesMsg = protowire.AppendTag(filesMsg, 2, protowire.BytesType)
		filesMsg = append(filesMsg, protowire.AppendVarint(nil, uint64(len(fileMsg)))...)
		filesMsg = append(filesMsg, fileMsg...)
	}

	// Content: field 1 (Commit), field 2 (repeated File)
	var content []byte
	content = protowire.AppendTag(content, 1, protowire.BytesType)
	content = append(content, protowire.AppendVarint(nil, uint64(len(commit)))...)
	content = append(content, commit...)
	content = append(content, filesMsg...)

	// DownloadResponse: field 1 (repeated Content)
	var respMsg []byte
	respMsg = protowire.AppendTag(respMsg, 1, protowire.BytesType)
	respMsg = append(respMsg, protowire.AppendVarint(nil, uint64(len(content)))...)
	respMsg = append(respMsg, content...)

	w.Header().Set("Content-Type", "application/proto")
	_, _ = w.Write(respMsg)
}
func toB5Digest(b4Digest []byte) ([]byte, error) {
	// B5 digest wraps B4 (shake256) value: SHA3-Shake256("shake256:" + hex(b4_hash))
	// This matches buf's getB5DigestForBucketAndDepDigests with zero dependencies.
	digestStr := "shake256:" + hex.EncodeToString(b4Digest)
	hash, err := shake256.SHA3Shake256([]byte(digestStr))
	if err != nil {
		return nil, err
	}
	return hash[:], nil
}


func (h *commitServiceHandler) computeB4Digest(r *http.Request, ref moduleRef, commit string) ([]byte, error) {
	files, err := h.api.repo.GetFiles(r.Context(), ref.owner, ref.module, commit)
	if err != nil {
		return nil, err
	}
	digest, err := h.computeB4DigestFromFiles(files)
	if err != nil {
		return nil, err
	}
	cid := commit
	h.commitMu.Lock()
	h.filesMap[cid] = files
	h.commitMu.Unlock()
	return digest, nil
}

func (h *commitServiceHandler) computeB4DigestFromFiles(files []content.File) ([]byte, error) {
	var manifest bytes.Buffer
	for _, f := range files {
		fmt.Fprintf(&manifest, digestFormat, f.Hash.String(), f.Path)
	}
	hash, err := shake256.SHA3Shake256(manifest.Bytes())
	if err != nil {
		return nil, err
	}
	return hash[:], nil
}

// logHandlerError logs structured error context before writing an HTTP error response.
// All handler-level errors pass through here to ensure consistent attribute naming (ERR-05).
func (h *commitServiceHandler) logHandlerError(r *http.Request, w http.ResponseWriter, msg string, code int, attrs ...slog.Attr) {
	protocol := "v1beta1"
	if !strings.Contains(r.URL.Path, "v1beta1") {
		protocol = "v1"
	}

	logAttrs := []slog.Attr{
		slog.String("server", h.api.domain),
		slog.String("protocol", protocol),
		slog.String("request_id", RequestIDFrom(r.Context())),
		slog.String("error", msg),
		slog.Int("status", code),
		slog.String("error_class", errorClass(code)),
	}
	logAttrs = append(logAttrs, attrs...)

	level := slog.LevelWarn
	if code >= 500 {
		level = slog.LevelError
	}
	h.api.log.LogAttrs(r.Context(), level, "handler error", logAttrs...)

	http.Error(w, msg, code)
}

// errorClass maps an HTTP status code to a short, grep-friendly class name
// used in structured logs. Three buckets only:
//   - "bad_request": 4xx codes caused by client input.
//   - "upstream": 5xx codes caused by talking to a back-end we don't control.
//   - "internal": 5xx codes caused by this proxy itself.
func errorClass(code int) string {
	switch {
	case code >= 400 && code < 500:
		return "bad_request"
	case code == http.StatusBadGateway || code == http.StatusServiceUnavailable:
		return "upstream"
	default:
		return "internal"
	}
}

// badRequest writes a 400 response with the given message and any extra
// structured log attributes. Use when the client sent a request we cannot
// parse or that fails our own validation rules.
func (h *commitServiceHandler) badRequest(r *http.Request, w http.ResponseWriter, msg string, attrs ...slog.Attr) {
	h.logHandlerError(r, w, msg, http.StatusBadRequest, attrs...)
}

// upstreamError writes a 502 response with the given message and any extra
// structured log attributes. Use when the request was well-formed but
// talking to the back-end registry/provider failed.
func (h *commitServiceHandler) upstreamError(r *http.Request, w http.ResponseWriter, msg string, attrs ...slog.Attr) {
	h.logHandlerError(r, w, msg, http.StatusBadGateway, attrs...)
}

// resolveForeignCommitID attempts to recover the module identity for a
// commit_id this proxy never minted (e.g. one a buf CLI client cached from a
// different registry). It is called by ServeDownload when commitMap lookup
// misses, before falling back to a 400.
//
// Resolution strategy:
//
//  1. If infoCache has exactly one entry, the proxy is serving a single
//     active module (the common single-module deployment). Use that entry.
//  2. Otherwise we cannot tell which module a foreign id refers to without
//     more information (ResourceRef.name is not parsed from the download
//     wire format); return nil so the caller surfaces a 400 rather than
//     guessing and serving the wrong module.
//
// On success, the foreign commit_id is registered as an alias of the
// resolved module's ref in commitMap so subsequent identical requests are
// served directly without re-running this fallback.
//
// Returns nil when the module identity cannot be recovered.
func (h *commitServiceHandler) resolveForeignCommitID(commitID string) *moduleRef {
	if commitID == "" {
		return nil
	}
	h.commitMu.Lock()
	defer h.commitMu.Unlock()
	if len(h.infoCache) != 1 {
		return nil
	}
	for key, info := range h.infoCache {
		// Reject if the cached entry is for an id that does not match: this
		// happens when the single entry itself is for a *different* foreign
		// id we previously aliased. We trust the entry's commitID only as a
		// resolved-id hint, not as an equality check — the whole point of the
		// fallback is that commitID differs from info.commitID.
		owner, module, ok := splitOwnerModule(key)
		if !ok {
			return nil
		}
		ref := moduleRef{owner: owner, module: module}
		// Register the foreign id as an alias of this module's ref so future
		// requests for the same foreign id skip the fallback entirely.
		h.commitMap[commitID] = ref
		// Keep ownerID/moduleID consistent: also alias info.commitID -> ref
		// is already present (set when the entry was written); nothing to do.
		_ = info
		return &ref
	}
	return nil
}

// registerResolved records a resolved module for a commit id (the canonical
// git sha) under commitMu. commitMap[sha]=ref lets future Downloads of the
// same sha hit directly. infoCache is populated with a digest-less entry —
// ServeDownload's miss-branch recomputes files+digest on first use, so the
// digest field is not load-bearing here.
//
// If an infoCache entry already exists for the module (e.g. a prior
// ServeGraph computed the digest and cached files), its digest is preserved —
// only the resolved-commit identity is refreshed. Without this, a late
// pre-warm would clobber a computed digest with nil and the next cache-hit
// Download would serve a zero digest.
func (h *commitServiceHandler) registerResolved(sha, owner, module string) {
	key := owner + "/" + module
	h.commitMu.Lock()
	h.commitMap[sha] = moduleRef{owner: owner, module: module}
	if existing, ok := h.infoCache[key]; ok {
		existing.commitID = sha
		existing.commit = sha
		existing.ownerID = owner
		existing.moduleID = key
		h.infoCache[key] = existing
	} else {
		h.infoCache[key] = commitInfoCache{
			commitID: sha,
			commit:   sha,
			ownerID:  owner,
			moduleID: key,
		}
	}
	h.commitMu.Unlock()
}

// prewarmHeads resolves the current HEAD commit of every configured module
// and registers it, so that a client sending a cached current-HEAD sha hits
// the commit map without a prior in-session GetCommits. Best-effort and
// idempotent (prewarmOnce): failures are logged and skipped; a later
// probeCommitID call still recovers any sha pre-warm missed. GetMeta-only —
// no file fetch. Intended to run in a background goroutine launched from
// connect.New.
func (h *commitServiceHandler) prewarmHeads() {
	h.prewarmOnce.Do(func() {
		sources := h.api.repo.Repositories()
		log := h.api.log.With(slog.String("component", "prewarm"))
		log.LogAttrs(context.Background(), slog.LevelInfo, "prewarm starting",
			slog.Int("sources", len(sources)),
			slog.Duration("per_call_timeout", h.prewarmTimeout))
		var ok, fail int
		for _, s := range sources {
			owner, module := s.Owner(), s.RepoName()
			if owner == "" || module == "" {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), h.prewarmTimeout)
			meta, err := s.GetMeta(ctx, "") // empty commit = HEAD
			cancel()
			if err != nil || meta.Commit == "" {
				fail++
				log.LogAttrs(context.Background(), slog.LevelWarn, "prewarm source miss",
					slog.String("owner", owner), slog.String("module", module),
					slog.String("repo", module))
				continue
			}
			h.registerResolved(meta.Commit, owner, module)
			ok++
			log.LogAttrs(context.Background(), slog.LevelInfo, "prewarm source resolved",
				slog.String("owner", owner), slog.String("module", module),
				slog.String("repo", module), slog.String("commit", meta.Commit))
		}
		log.LogAttrs(context.Background(), slog.LevelInfo, "prewarm complete",
			slog.Int("resolved", ok), slog.Int("failed", fail))
	})
}

// missCached reports whether sha was recently confirmed absent from every
// configured source (within ProbeNegativeTTL). Caller must NOT hold commitMu.
func (h *commitServiceHandler) missCached(sha string) bool {
	h.commitMu.RLock()
	t, ok := h.missCache[sha]
	h.commitMu.RUnlock()
	return ok && time.Since(t) < h.probeNegativeTTL
}

func (h *commitServiceHandler) rememberMiss(sha string) {
	h.commitMu.Lock()
	h.missCache[sha] = time.Now()
	h.commitMu.Unlock()
}

// sweepMisses periodically drops expired negative-cache entries so missCache
// cannot grow without bound as distinct bogus shas arrive. Intended to run in
// a background goroutine; the interval is half the TTL (Nyquist-ish). Stops
// only when the process exits (no graceful-shutdown context exists).
func (h *commitServiceHandler) sweepMisses(ctx context.Context) {
	interval := h.probeNegativeTTL / 2
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			cutoff := now
			h.commitMu.Lock()
			for sha, t := range h.missCache {
				if cutoff.Sub(t) >= h.probeNegativeTTL {
					delete(h.missCache, sha)
				}
			}
			h.commitMu.Unlock()
		}
	}
}

// probeCommitID resolves a Download commit_id (= git sha) that was neither
// minted in-session nor recoverable by the single-module fallback, by asking
// each configured source whether it owns the sha. A git sha is unique to one
// repo, so at most one source succeeds — no cross-module ambiguity. On hit the
// sha is registered as an alias (future identical requests hit directly); on
// all-fail the sha is negative-cached (rememberMiss) so retries within TTL do
// not re-probe. Returns (ref, true) on a hit.
//
// Callers must NOT hold commitMu. ctx should be the request context so a
// disconnecting client bounds the fan-out; each per-source call additionally
// gets its own timeout (probeTimeout).
func (h *commitServiceHandler) probeCommitID(ctx context.Context, sha string) (*moduleRef, bool) {
	if sha == "" || h.missCached(sha) {
		return nil, false
	}
	// Re-check commitMap under the lock: a concurrent resolver may have
	// already registered this sha while we were waiting on the semaphore.
	h.commitMu.RLock()
	ref, already := h.commitMap[sha]
	h.commitMu.RUnlock()
	if already {
		r := ref
		return &r, true
	}

	// Bound concurrent probes so a flood of distinct unknown shas cannot
	// amplify to unbounded upstream load. Non-blocking acquire: if the cap is
	// reached, decline (the caller 400s; the client retries and hits the
	// negative cache only after a probe eventually runs).
	if h.probeSem != nil {
		select {
		case h.probeSem <- struct{}{}:
			defer func() { <-h.probeSem }()
		default:
			return nil, false
		}
	}

	sources := h.api.repo.Repositories()
	if len(sources) == 0 {
		return nil, false
	}

	type probeResult struct {
		ref moduleRef
		ok  bool
	}
	// Buffered enough to never block a successful goroutine; first success wins.
	results := make(chan probeResult, len(sources))
	// transient is set if any source returned a transient error (timeout /
	// cancellation / network). In that case the all-fail result is
	// inconclusive and must NOT be negative-cached — a brief upstream outage
	// should not make a real sha unavailable for ProbeNegativeTTL.
	var transient atomic.Bool
	var wg sync.WaitGroup
	for _, s := range sources {
		wg.Add(1)
		go func(s source.Source) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, h.probeTimeout)
			defer cancel()
			meta, err := s.GetMeta(pctx, sha)
			if err != nil {
				if isTransientErr(err) {
					transient.Store(true)
				}
				return
			}
			if meta.Commit == "" {
				return
			}
			results <- probeResult{
				ref: moduleRef{owner: s.Owner(), module: s.RepoName()},
				ok:  true,
			}
		}(s)
	}
	wg.Wait()
	close(results)

	for r := range results {
		// First (only) success. Register alias and return.
		h.registerResolved(sha, r.ref.owner, r.ref.module)
		ref := r.ref
		return &ref, true
	}
	// Only negative-cache when every source returned a definitive not-found.
	// A transient failure (timeout/network) leaves the sha retryable.
	if !transient.Load() {
		h.rememberMiss(sha)
	}
	return nil, false
}

// isTransientErr reports whether err looks like a transient upstream failure
// (timeout, cancellation, or a network error) rather than a definitive
// "commit not found". Used to avoid negative-caching shas during brief
// upstream outages.
func isTransientErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// splitOwnerModule splits an infoCache key of the form "owner/module" back
// into its parts. Returns ok=false if the key is malformed (no slash or
// empty parts). The owner part may itself contain a slash for some deploy
// topologies, so we split on the FIRST slash and treat the rest as module.
func splitOwnerModule(key string) (owner, module string, ok bool) {
	idx := strings.IndexByte(key, '/')
	if idx <= 0 || idx == len(key)-1 {
		return "", "", false
	}
	return key[:idx], key[idx+1:], true
}

// ServeGetModules handles v1/v1beta1 ModuleService/GetModules.
func (h *commitServiceHandler) ServeGetModules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logHandlerError(r, w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.badRequest(r, w, "reading body", slog.String("read_error", err.Error()))
		return
	}

	// Parse GetModulesRequest: repeated ModuleRef module_refs = 1
	// ModuleRef { oneof value { string id = 1; Name name = 2; } }
	// Name { string owner = 1; string module = 2; }
	h.commitMu.RLock()
	// Build moduleID → "owner/module" lookup
	moduleLookup := make(map[string]string, len(h.infoCache))
	for k, v := range h.infoCache {
		moduleLookup[v.moduleID] = k
	}
	h.commitMu.RUnlock()

	h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
		slog.String("handler", "ServeGetModules"),
		slog.String("procedure", "ModuleService/GetModules"),
		slog.String("branch", "request_received"),
		slog.Int("body_bytes", len(body)),
		slog.Int("info_cache_size", len(h.infoCache)),
		slog.String("raw_body_hex", hex.EncodeToString(body)),
	)

	type moduleKey struct {
		owner  string
		module string
	}
	var keys []moduleKey
	var refsSeen, refsMatched, refsRejected int
	msg := body
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			break
		}
		msg = msg[n:]
		if num == 1 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			refsSeen++
			if key := parseModuleRefByID(v, moduleLookup); key != nil {
				keys = append(keys, *key)
				refsMatched++
				h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
					slog.String("handler", "ServeGetModules"),
					slog.String("procedure", "ModuleService/GetModules"),
					slog.String("branch", "parse_module_ref"),
					slog.String("outcome", "matched"),
					slog.Int("ref_bytes", len(v)),
					slog.String("owner", key.owner),
					slog.String("module", key.module),
				)
			} else {
				refsRejected++
				h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
					slog.String("handler", "ServeGetModules"),
					slog.String("procedure", "ModuleService/GetModules"),
					slog.String("branch", "parse_module_ref"),
					slog.String("outcome", "rejected"),
					slog.Int("ref_bytes", len(v)),
					slog.String("ref_hex", hex.EncodeToString(v)),
				)
			}
		} else {
			n = protowire.ConsumeFieldValue(num, typ, msg)
			if n < 0 {
				break
			}
			msg = msg[n:]
		}
	}
	if len(keys) == 0 {
		// Foreign-module-id fallback. The buf client usually sends
		// ModuleRef.id (the id it cached from a prior GetModules response).
		// If that id was minted by an older proxy build (hashed) or by a
		// different registry (real buf.build), it will not match the raw
		// "owner/module" ids this build emits, so every ref rejects and we
		// would 400. When the deployment serves exactly one module, serve
		// it rather than failing — mirrors the Download foreign-commit_id
		// fallback. Multi-module deployments stay strict (singleModule nil).
		if h.singleModule != nil {
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeGetModules"),
				slog.String("procedure", "ModuleService/GetModules"),
				slog.String("branch", "module_id_fallback"),
				slog.String("reason", "no_module_refs_after_parse"),
				slog.Int("refs_seen", refsSeen),
				slog.Int("refs_matched", refsMatched),
				slog.Int("refs_rejected", refsRejected),
				slog.String("owner", h.singleModule.owner),
				slog.String("module", h.singleModule.module),
				slog.String("repo", h.singleModule.module),
			)
			keys = append(keys, moduleKey{
				owner:  h.singleModule.owner,
				module: h.singleModule.module,
			})
		} else {
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeGetModules"),
				slog.String("procedure", "ModuleService/GetModules"),
				slog.String("branch", "rejection"),
				slog.String("reason", "no_module_refs_after_parse"),
				slog.Int("refs_seen", refsSeen),
				slog.Int("refs_matched", refsMatched),
				slog.Int("refs_rejected", refsRejected),
				slog.Int("info_cache_size", len(h.infoCache)),
			)
			h.badRequest(r, w, "no module refs", slog.Int("body_bytes", len(body)))
			return
		}
	}

	// Build GetModulesResponse: repeated Module modules = 1
	var respMsg []byte
	for _, k := range keys {
		mod := buildModule(k.owner, k.module)
		respMsg = protowire.AppendTag(respMsg, 1, protowire.BytesType)
		respMsg = append(respMsg, protowire.AppendVarint(nil, uint64(len(mod)))...)
		respMsg = append(respMsg, mod...)
	}

	w.Header().Set("Content-Type", "application/proto")
	_, _ = w.Write(respMsg)
}
