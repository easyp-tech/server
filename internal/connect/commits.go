package connect

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"log/slog"

	"github.com/easyp-tech/server/internal/providers/content"
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
}

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
		cid := deterministicID(meta.Commit)
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
			ownerID:  deterministicID(ref.owner),
			moduleID: deterministicID(ref.owner + "/" + ref.module),
			commitID: cid,
			digest:   digest,
		})
		h.commitMu.Lock()
		h.commitMap[cid] = ref
		h.infoCache[ref.owner+"/"+ref.module] = commitInfoCache{
			commitID: cid,
			commit:   meta.Commit,
			ownerID:  deterministicID(ref.owner),
			moduleID: deterministicID(ref.owner + "/" + ref.module),
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
		cid := deterministicID(meta.Commit)
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
		commits = append(commits, commitInfo{
			ownerID:  deterministicID(ref.owner),
			moduleID: deterministicID(ref.owner + "/" + ref.module),
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
		// The DownloadService wire format is just a commit id produced by a prior
		// GetCommits call. If we have never seen that id, the caller skipped the
		// warm-up step — surface that explicitly, including the id itself so
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
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, "")
		if err != nil {
			h.upstreamError(r, w, fmt.Sprintf("resolving %s/%s", ref.owner, ref.module),
				slog.String("owner", ref.owner), slog.String("module", ref.module),
				slog.String("repo", ref.module),
				slog.String("commit_id", commitID),
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
		cid = deterministicID(meta.Commit)
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
	cid := deterministicID(commit)
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

	type moduleKey struct {
		owner  string
		module string
	}
	var keys []moduleKey
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
			if key := parseModuleRefByID(v, moduleLookup); key != nil {
				keys = append(keys, *key)
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
		h.badRequest(r, w, "no module refs", slog.Int("body_bytes", len(body)))
		return
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
