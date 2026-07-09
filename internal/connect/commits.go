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
	// cidPinned is true when this entry was minted for a request whose
	// Name.ref was a buf-issued 32-hex cid (the normal buf.lock state).
	// infoCache is keyed by owner/module only, so without this flag a
	// pinned-cid writeback would silently poison every subsequent HEAD /
	// SHA / tag request on the same module (WR-01): the cid-gate can't
	// tell the entry apart from a HEAD-minted one, because commitID is
	// always a cid (the output of commitUUID). cidPinned records the
	// request shape that produced the entry so the gate can force a
	// re-resolve when a non-pinned request lands on a pinned entry.
	cidPinned bool
}

type commitServiceHandler struct {
	api *api

	commitMu  sync.RWMutex
	commitMap map[string]moduleRef       // commitID → owner/module
	infoCache map[string]commitInfoCache // "owner/module" → cached commit info
	filesMap  map[string][]content.File  // commitID → cached files
	// cidSha remembers the full 40-/64-char git SHA a buf-issued 32-hex
	// commit_id was minted from. Populated at every mint site (GetCommits,
	// ServeGraph writeback, ServeDownload mint, registerResolvedAlias). The
	// map is the source of truth ServeGraph and ServeDownload consult before
	// touching upstream when a request pins a dependency via its cid (the
	// normal buf.lock state) — without it the proxy cannot recover the real
	// SHA (commitUUID is intentionally lossy) and would either forward the
	// cid to the upstream (422) or serve a differently-cached commit (HEAD).
	cidSha map[string]string // cid (32-hex) → full git sha (40/64-hex)
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

	// missCache holds the time a commit_id was last confirmed absent from
	// every configured source (probe all-fail). Used by probeCommitID to
	// skip repeated upstream fan-out for known-bogus shas within ProbeTTL.
	missCache map[string]time.Time

	// runtime knobs (set in connect.New from config.Connect.WithDefaults)
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

// errCommitUUIDContract marks errors that arise when commitUUID rejects an
// input that an upstream provider returned. The contract assumes callers have
// already validated the sha shape, so a non-conforming value is a real
// programming/upstream bug (not a transient upstream outage) and routes
// through h.logHandlerError as 500 internal error. Other digest errors
// (GetFiles / computeB4DigestFromFiles) route through h.upstreamError as
// 502 because they reflect provider health, not contract violations.
var errCommitUUIDContract = errors.New("commitUUID contract violation")

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
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, ref.ref)
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("resolving %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("upstream_error", err.Error()))
			return
		}
		cid, cidErr := commitUUID(meta.Commit)
		if cidErr != nil {
			h.logHandlerError(r, w, "internal error", http.StatusInternalServerError,
				slog.String("commit_id", meta.Commit),
				slog.String("upstream_error", cidErr.Error()))
			return
		}
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
		digest, err := h.computeB4Digest(r, ref, meta.Commit, cid)
		if err != nil {
			if errors.Is(err, errCommitUUIDContract) {
				h.logHandlerError(r, w, "internal error", http.StatusInternalServerError,
					slog.String("commit_id", cid),
					slog.String("upstream_error", err.Error()))
				return
			}
			h.upstreamError(r, w, fmt.Sprintf("digest for %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
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
		h.cidSha[cid] = meta.Commit
		h.infoCache[ref.owner+"/"+ref.module] = commitInfoCache{
			commitID:  cid,
			commit:    meta.Commit,
			ownerID:   ref.owner,
			moduleID:  ref.owner + "/" + ref.module,
			digest:    digest,
			cidPinned: isUUID(ref.ref),
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
		// infoCache is keyed by owner/module only, so the cid-gate must be
		// SYMMETRIC (WR-01):
		//   - A cid-pinned request must only hit an entry minted for the SAME
		//     cid (a HEAD/tag/SHA-minted entry has a different commitID).
		//   - A non-pinned request (HEAD/SHA/tag, i.e. ref.ref is not a cid)
		//     must only hit an entry that was itself minted by a non-pinned
		//     request. Without this reverse check, a single pinned-cid
		//     writeback would stick every subsequent HEAD request on that
		//     module to the pinned commit until restart.
		requestIsUUID := isUUID(ref.ref)
		if ok && ((!requestIsUUID && !cached.cidPinned) || (requestIsUUID && cached.commitID == ref.ref)) {
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
		// UUID-resolution branch: when the client pinned a dependency via a
		// buf-issued 32-hex cid (the standard buf.lock state), the raw cid
		// MUST NOT be forwarded to GetMeta — GitHub rejects it with 422
		// ("No commit found for SHA") because the real SHA is 40 hex and
		// commitUUID is intentionally lossy. Resolve cid → full git SHA via
		// the cidSha map (warm) or commitUUIDInverse + upstream prefix probe
		// (cold), then proceed with the resolved SHA. Empty/branch/tag/SHA
		// refs flow the existing path untouched.
		fetchRef := ref.ref
		if isUUID(ref.ref) {
			resolved, ok := h.resolveUUIDRef(r.Context(), ref, ref.ref)
			if ok {
				fetchRef = resolved
				h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
					slog.String("handler", "ServeGraph"),
					slog.String("procedure", "GraphService/GetGraph"),
					slog.String("branch", "uuid_ref_resolved"),
					slog.String("owner", ref.owner),
					slog.String("module", ref.module),
					slog.String("repo", ref.module),
					slog.String("commit_id", ref.ref),
					slog.String("commit", resolved),
				)
			} else {
				h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
					slog.String("handler", "ServeGraph"),
					slog.String("procedure", "GraphService/GetGraph"),
					slog.String("branch", "uuid_ref_unresolved"),
					slog.String("owner", ref.owner),
					slog.String("module", ref.module),
					slog.String("repo", ref.module),
					slog.String("commit_id", ref.ref),
				)
			}
		}
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, fetchRef)
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("resolving %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("upstream_error", err.Error()))
			return
		}
		cid, cidErr := commitUUID(meta.Commit)
		if cidErr != nil {
			h.logHandlerError(r, w, "internal error", http.StatusInternalServerError,
				slog.String("commit_id", meta.Commit),
				slog.String("upstream_error", cidErr.Error()))
			return
		}
		digest, err := h.computeB4Digest(r, ref, meta.Commit, cid)
		if err != nil {
			if errors.Is(err, errCommitUUIDContract) {
				h.logHandlerError(r, w, "internal error", http.StatusInternalServerError,
					slog.String("commit_id", cid),
					slog.String("upstream_error", err.Error()))
				return
			}
			h.upstreamError(r, w, fmt.Sprintf("digest for %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit", meta.Commit),
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
		// "unknown commit id: re-resolve via buf mod update / buf dep update".
		h.commitMu.Lock()
		h.commitMap[cid] = ref
		h.cidSha[cid] = meta.Commit
		h.infoCache[ref.owner+"/"+ref.module] = commitInfoCache{
			commitID:  cid,
			commit:    meta.Commit,
			ownerID:   ref.owner,
			moduleID:  ref.owner + "/" + ref.module,
			digest:    digest,
			cidPinned: isUUID(ref.ref),
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
		h.badRequest(r, w, "unknown commit id: re-resolve via buf mod update / buf dep update",
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
	// Files-cache hit only counts when the cached entry is for the SAME cid
	// the request pinned. infoCache is keyed by owner/module, so without
	// this gate a pinned-cid request served after a HEAD request would
	// return HEAD's files under the pinned cid ("no content returned for
	// commit ID <pinned>"). When the cids differ, fall through to the
	// fetch path which resolves the pinned cid via cidSha.
	if infoOK && cached.commitID == commitID && len(cachedFiles) > 0 {
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
		// commitID is typically a buf-issued 32-hex cid (the value buf.lock
		// pins), not a raw git sha — forwarding it to GetMeta fails upstream
		// (422, "No commit found for SHA"). Resolve cid→sha via cidSha first;
		// only when that misses do we fall back to the prior behavior of
		// treating commitID as a raw sha and then to cached.commit (HEAD).
		fetchCommit := commitID
		if sha, ok := h.cidShaLookup(commitID); ok && sha != "" {
			fetchCommit = sha
		}
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, fetchCommit)
		if err != nil && fetchCommit == commitID && cached.commit != "" && cached.commit != commitID {
			// commitID was not a known cid; try the resolved HEAD commit the
			// foreign-id alias was bound to (preserves prior single-module
			// behavior for genuinely-foreign ids with no cidSha entry).
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
		cid, err = commitUUID(meta.Commit)
		if err != nil {
			h.logHandlerError(r, w, "internal error", http.StatusInternalServerError,
				slog.String("commit_id", meta.Commit),
				slog.String("upstream_error", err.Error()))
			return
		}
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
		// Write back the full resolution so the next identical pinned-cid
		// Download hits the files-cache directly (WR-03). Without this, every
		// repeat request re-ran cidShaLookup (hit) → GetMeta → GetFiles →
		// re-compute digest, because the infoCache entry still held the stale
		// pre-resolution state and filesMap[cid] was never populated. Mirror
		// the ServeGraph writeback (and ServeHTTP's) so all three handlers
		// converge on the same cached state.
		h.commitMu.Lock()
		h.commitMap[cid] = *ref
		h.cidSha[cid] = meta.Commit
		h.infoCache[ref.owner+"/"+ref.module] = commitInfoCache{
			commitID:  cid,
			commit:    meta.Commit,
			ownerID:   ref.owner,
			moduleID:  ref.owner + "/" + ref.module,
			digest:    digest,
			cidPinned: isUUID(commitID),
		}
		h.filesMap[cid] = files
		h.commitMu.Unlock()
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


func (h *commitServiceHandler) computeB4Digest(r *http.Request, ref moduleRef, commit, cid string) ([]byte, error) {
	files, err := h.api.repo.GetFiles(r.Context(), ref.owner, ref.module, commit)
	if err != nil {
		return nil, err
	}
	digest, err := h.computeB4DigestFromFiles(files)
	if err != nil {
		return nil, err
	}
	// The caller has already validated commitUUID(meta.Commit); re-check the
	// same input here so an upstream contract violation surfaces with the
	// errCommitUUIDContract sentinel and the dispatcher can route it as 500
	// (a real bug), not 502 (a transient upstream outage). Use the caller-
	// supplied cid to populate filesMap — re-deriving it would just be a
	// second sha->UUID pass for the same input.
	if _, uidErr := commitUUID(commit); uidErr != nil {
		return nil, fmt.Errorf("%w: %v", errCommitUUIDContract, uidErr)
	}
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

// cidShaLookup is the read-side of the cidSha map. Returns the full git SHA
// the cid was minted from, plus ok=true. Caller must NOT hold commitMu.
func (h *commitServiceHandler) cidShaLookup(cid string) (string, bool) {
	h.commitMu.RLock()
	sha, ok := h.cidSha[cid]
	h.commitMu.RUnlock()
	return sha, ok
}

// resolveUUIDRef recovers the full git SHA a buf-issued 32-hex cid was minted
// from, so the caller can pass a real SHA to GetMeta instead of forwarding
// the raw cid upstream (which GitHub rejects with 422 — commitUUID is
// intentionally lossy and the cid is not a SHA prefix).
//
// Resolution ladder:
//
//  1. cidSha map hit — the cid was minted in-session (GetCommits, a prior
//     ServeGraph writeback, ServeDownload, or probeCommitID). Zero round-trips.
//  2. cidSha miss — recover the first 28 hex chars (14 bytes) of the
//     original SHA via commitUUIDInverse (Phase 18 technique) and probe the
//     upstream with the prefix. Real providers (GitHub, Bitbucket) resolve
//     short-SHA prefixes via their commit-fetch API; the 28-hex prefix is
//     well above both providers' minimum length. The returned meta.Commit
//     MUST start with the recovered prefix — otherwise the source does not
//     own this cid and the probe misses (prefix-match validation closes the
//     wrong-source-alias risk, mirroring probeCommitID's contract).
//
// On hit the cid→sha mapping is cached so subsequent identical requests take
// the zero-round-trip path. On any failure returns ("", false); the caller
// falls through to the existing GetMeta path with the original ref (which
// will still fail upstream, but no worse than before).
//
// Caller must NOT hold commitMu. ctx should be the request context.
func (h *commitServiceHandler) resolveUUIDRef(ctx context.Context, ref moduleRef, cid string) (string, bool) {
	if cid == "" {
		return "", false
	}

	// (a) Warm map — the common path once a cid has been minted in-session.
	if sha, ok := h.cidShaLookup(cid); ok && sha != "" {
		return sha, true
	}

	// (b) Cold cache — recover the 28-hex SHA prefix and probe upstream.
	//
	// This probe has the same shape as probeCommitID's per-source GetMeta, so
	// it MUST inherit the same four defenses (CR-01/WR-02): the probeSem cap
	// on concurrent upstream probes, the missCache negative cache, the
	// probeTimeout per-call bound, and the isTransientErr classification that
	// keeps a brief upstream outage from permanently negative-caching a real
	// cid. Without these an unauthenticated client flooding ServeGraph with
	// distinct 32-hex cids causes one unbounded upstream GetMeta per request
	// with no concurrency cap and no negative caching — exactly the
	// amplification probeSem exists to prevent.
	if h.missCached(cid) {
		return "", false
	}
	// commitUUIDInverse recovers the first 28 hex chars (14 bytes) of the
	// original SHA. The 2^112 collision space is large enough that at most
	// one commit in any realistic repo starts with this prefix; the
	// HasPrefix check below verifies the upstream's answer honors it.
	// (IN-01: 2^112 uniqueness assumption, verified by probeCommitID's
	// identical HasPrefix validation.)
	prefix, err := commitUUIDInverse(cid)
	if err != nil {
		h.rememberMiss(cid)
		return "", false
	}
	// Bound concurrent probes so a flood of distinct unknown cids cannot
	// amplify to unbounded upstream load (mirrors probeCommitID's acquire).
	// Non-blocking: if the cap is reached, decline and let the caller fall
	// through — the client retries and hits the negative cache once a probe
	// eventually runs.
	if h.probeSem != nil {
		select {
		case h.probeSem <- struct{}{}:
			defer func() { <-h.probeSem }()
		default:
			return "", false
		}
	}
	// Per-call timeout: request contexts alone are an insufficient bound on
	// upstream hangs (proxies behind proxies, idle-pinned connections).
	// probeCommitID wraps every per-source GetMeta the same way (WR-02). A
	// zero probeTimeout (enhancements disabled) means "use the request
	// context as-is" — mirrors how the rest of ServeGraph behaves when the
	// CommitResolution knobs are off.
	pctx := ctx
	if h.probeTimeout > 0 {
		var cancel context.CancelFunc
		pctx, cancel = context.WithTimeout(ctx, h.probeTimeout)
		defer cancel()
	}
	meta, err := h.api.repo.GetMeta(pctx, ref.owner, ref.module, prefix)
	if err != nil {
		// Only negative-cache on a definitive not-found. A transient error
		// (timeout/cancel/network) leaves the cid retryable so a brief
		// upstream outage does not lock out a real cid for ProbeNegativeTTL.
		if !isTransientErr(err) {
			h.rememberMiss(cid)
		}
		return "", false
	}
	if meta.Commit == "" || !strings.HasPrefix(meta.Commit, prefix) {
		// Prefix-match validation: the upstream resolved the prefix to a
		// commit that does NOT start with the recovered prefix. The
		// source does not own this cid — do not trust the answer.
		h.rememberMiss(cid)
		return "", false
	}

	// Cache so the next request for this cid takes the zero-round-trip path.
	h.commitMu.Lock()
	h.cidSha[cid] = meta.Commit
	h.commitMu.Unlock()
	return meta.Commit, true
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

// resolveCommitForRead is the thin wrapper invoked by the v1alpha1
// DownloadManifestAndBlobs handler (via the CommitResolver interface on *api)
// to resolve a buf-issued commit id to the 40-/64-char git SHA the upstream
// source points at. It mirrors the ServeDownload decision ladder verbatim in
// shape: (a) commitMap lookup, (b) single-module foreign-id fallback, (c)
// probe fan-out. The probe step is where 32-char UUID inputs are handled via
// commitUUIDInverse + prefix-match — this wrapper does NOT call those helpers
// directly (RESEARCH.md "Don't Hand-Roll" table); probeCommitID owns them.
//
// On a total miss the method returns a wrapped error so the caller surfaces a
// Connect error and never falls back to HEAD (RESEARCH.md anti-pattern
// "Returning HEAD when resolution fails"). Callers must NOT hold commitMu
// when calling this method; the method acquires/releases the lock internally
// and probeCommitID mandates a lock-free caller.
func (h *commitServiceHandler) resolveCommitForRead(
	ctx context.Context, owner, module, id string,
) (string, error) {
	// (a) commitMap — the in-session cache populated by GetCommits.
	h.commitMu.RLock()
	if ref, ok := h.commitMap[id]; ok {
		info := h.infoCache[ref.owner+"/"+ref.module]
		h.commitMu.RUnlock()
		if info.commit != "" {
			return info.commit, nil
		}
	} else {
		h.commitMu.RUnlock()
	}

	// (b) single-module foreign-id fallback (commitMap miss).
	if ref := h.resolveForeignCommitID(id); ref != nil {
		h.commitMu.RLock()
		info := h.infoCache[ref.owner+"/"+ref.module]
		h.commitMu.RUnlock()
		if info.commit != "" {
			return info.commit, nil
		}
	}

	// (c) probe fan-out — handles UUID inputs via commitUUIDInverse inside
	// probeCommitID. Caller must NOT hold commitMu (satisfied: all locks
	// above were released).
	if h.probeEnabled {
		if ref, ok := h.probeCommitID(ctx, id); ok && ref != nil {
			h.commitMu.RLock()
			info := h.infoCache[ref.owner+"/"+ref.module]
			h.commitMu.RUnlock()
			if info.commit != "" {
				return info.commit, nil
			}
		}
	}

	// (d) All three steps missed. Surface an error — do NOT fall back to HEAD.
	return "", fmt.Errorf("commit id %q not resolved by any configured source", id)
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
// probeCommitID resolves a Download commit_id that was neither minted
// in-session nor recoverable by the single-module fallback, by asking
// each configured source whether it owns the id. Three input shapes
// are supported:
//
//   - raw 40- or 64-char SHA: probed as-is. A git sha is unique to one
//     repo, so at most one source succeeds.
//   - buf-issued 32-char UUID: the SHA prefix is recovered via
//     commitUUIDInverse (the first 14 bytes of the original SHA), and
//     each source is probed with the prefix. The returned meta.Commit
//     MUST start with the recovered prefix — otherwise the source does
//     not own this UUID and the probe must miss (prefix-match
//     validation closes review.md finding #6: a wrong-source match
//     would silently alias a real commit id to a wrong module).
//   - anything else: not a SHA, not a UUID — providers cannot resolve
//     it, so the probe records a miss and returns.
//
// On hit, registerResolvedAlias stores both the buf-issued id (the
// primary key the client will use) and the resolved SHA (as an alias
// for any caller that sends a raw sha). On all-fail, the id is
// negative-cached so retries within TTL do not re-probe.
//
// Callers must NOT hold commitMu. ctx should be the request context so
// a disconnecting client bounds the fan-out; each per-source call
// additionally gets its own timeout (probeTimeout).
func (h *commitServiceHandler) probeCommitID(ctx context.Context, id string) (*moduleRef, bool) {
	if id == "" || h.missCached(id) {
		return nil, false
	}
	// Re-check commitMap under the lock: a concurrent resolver may have
	// already registered this id while we were waiting on the semaphore.
	h.commitMu.RLock()
	ref, already := h.commitMap[id]
	h.commitMu.RUnlock()
	if already {
		r := ref
		return &r, true
	}

	// If id is a buf-issued UUID, derive the SHA prefix and probe with the
	// prefix. The 14-byte recovery is lossy but 2^112 — sufficient to
	// identify a single source among the configured set. The prefix-match
	// validation below rules out collisions.
	probeArg := id
	if isUUID(id) {
		prefix, err := commitUUIDInverse(id)
		if err != nil {
			// id looks UUID-shaped but isn't hex — treat as a miss.
			h.rememberMiss(id)
			return nil, false
		}
		probeArg = prefix
	} else if !isSHA(id) {
		// Not a UUID, not a SHA — providers can't resolve it.
		h.rememberMiss(id)
		return nil, false
	}

	// Bound concurrent probes so a flood of distinct unknown ids cannot
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
		ref    moduleRef
		commit string
		ok     bool
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
			meta, err := s.GetMeta(pctx, probeArg)
			if err != nil {
				if isTransientErr(err) {
					transient.Store(true)
				}
				return
			}
			if meta.Commit == "" {
				return
			}
			// Prefix-match validation: when probeArg is a SHA prefix
			// (28 chars from commitUUIDInverse), the source's commit
			// MUST start with it. Otherwise the collision went the wrong
			// way and the source does not actually own this UUID.
			if isUUID(id) && !strings.HasPrefix(meta.Commit, probeArg) {
				return
			}
			results <- probeResult{
				ref:    moduleRef{owner: s.Owner(), module: s.RepoName()},
				commit: meta.Commit,
				ok:     true,
			}
		}(s)
	}
	wg.Wait()
	close(results)

	for r := range results {
		// First (only) success. Register both the buf-issued id and the
		// resolved SHA as aliases so future identical requests hit
		// directly.
		h.registerResolvedAlias(id, r.commit, r.ref.owner, r.ref.module)
		ref := r.ref
		return &ref, true
	}
	// Only negative-cache when every source returned a definitive not-found.
	// A transient failure (timeout/network) leaves the id retryable.
	if !transient.Load() {
		h.rememberMiss(id)
	}
	return nil, false
}

// registerResolvedAlias writes a commit id → moduleRef mapping and the
// matching infoCache entry. The id arg is what the buf client sent
// (typically a buf-issued 32-char UUID); sha is what the upstream
// resolved to (a 40- or 64-char hex). Both are stored in commitMap so
// future identical requests hit directly, and sha is also kept as an
// alias for any caller (debug tool, foreign-id probe) that sends a
// raw sha.
func (h *commitServiceHandler) registerResolvedAlias(id, sha, owner, module string) {
	key := owner + "/" + module
	h.commitMu.Lock()
	h.commitMap[id] = moduleRef{owner: owner, module: module}
	if sha != "" && sha != id {
		h.commitMap[sha] = moduleRef{owner: owner, module: module}
	}
	// Remember the cid→sha mapping so a subsequent ServeGraph request that
	// pins this cid short-circuits without re-probing. id is what the buf
	// client sent (a buf-issued 32-hex UUID when this path was reached via
	// probeCommitID); sha is the real 40-/64-hex git SHA the upstream
	// returned. Guard against degenerate id==sha (raw-sha callers).
	if isUUID(id) && sha != "" {
		h.cidSha[id] = sha
	}
	if existing, ok := h.infoCache[key]; ok {
		existing.commitID = id
		existing.commit = sha
		existing.ownerID = owner
		existing.moduleID = key
		existing.cidPinned = isUUID(id)
		h.infoCache[key] = existing
	} else {
		h.infoCache[key] = commitInfoCache{
			commitID:  id,
			commit:    sha,
			ownerID:   owner,
			moduleID:  key,
			cidPinned: isUUID(id),
		}
	}
	h.commitMu.Unlock()
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
