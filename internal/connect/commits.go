package connect

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/shake256"
	"google.golang.org/protobuf/encoding/protowire"
)

type commitInfoCache struct {
	commitID string
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

func (h *commitServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body", http.StatusBadRequest)
		return
	}

	refs := parseResourceRefs(body)
	if len(refs) == 0 {
		http.Error(w, "no resource refs", http.StatusBadRequest)
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
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, "")
		if err != nil {
			http.Error(w, fmt.Sprintf("resolving %s/%s: %v", ref.owner, ref.module, err), http.StatusInternalServerError)
			return
		}
		cid := deterministicID(meta.Commit)
		digest, err := h.computeB4Digest(r, ref, meta.Commit)
		if err != nil {
			http.Error(w, fmt.Sprintf("computing digest for %s/%s: %v", ref.owner, ref.module, err), http.StatusInternalServerError)
			return
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
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body", http.StatusBadRequest)
		return
	}

	// Parse GetGraphRequest: field 1 (resource_refs) repeated GetGraphRequest_ResourceRef
	// Each GetGraphRequest_ResourceRef has: field 1 (ResourceRef) + field 2 (Registry string)
	refs := parseGetGraphResourceRefs(body)
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
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, "")
		if err != nil {
			http.Error(w, fmt.Sprintf("resolving %s/%s: %v", ref.owner, ref.module, err), http.StatusInternalServerError)
			return
		}
		cid := deterministicID(meta.Commit)
		digest, err := h.computeB4Digest(r, ref, meta.Commit)
		if err != nil {
			http.Error(w, fmt.Sprintf("computing digest for %s/%s: %v", ref.owner, ref.module, err), http.StatusInternalServerError)
			return
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

	// Build Graph response:
	//   GetGraphResponse { field 1: Graph { field 1: [Graph_Commit...], field 2: [Graph_Edge...] } }
	//   Graph_Commit { field 1 (Commit), field 2 (Registry) }
	var graphMsg []byte
	for _, c := range commits {
		commit := buildCommitRaw(c.commitID, c.ownerID, c.moduleID, c.digest)

		// Build Graph_Commit wrapper: field 1 (Commit), field 2 (Registry)
		var graphCommit []byte
		graphCommit = protowire.AppendTag(graphCommit, 1, protowire.BytesType)
		graphCommit = append(graphCommit, protowire.AppendVarint(nil, uint64(len(commit)))...)
		graphCommit = append(graphCommit, commit...)
		graphCommit = protowire.AppendTag(graphCommit, 2, protowire.BytesType)
		graphCommit = protowire.AppendString(graphCommit, h.api.domain)

		// Append to Graph.commits (field 1, repeated)
		graphMsg = protowire.AppendTag(graphMsg, 1, protowire.BytesType)
		graphMsg = append(graphMsg, protowire.AppendVarint(nil, uint64(len(graphCommit)))...)
		graphMsg = append(graphMsg, graphCommit...)
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
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body", http.StatusBadRequest)
		return
	}

	var ref *moduleRef
	if commitID := parseResourceRefID(body); commitID != "" {
		h.commitMu.RLock()
		if mapped, ok := h.commitMap[commitID]; ok {
			r := mapped
			ref = &r
		}
		h.commitMu.RUnlock()
	}
	if ref == nil {
		http.Error(w, "no resource refs", http.StatusBadRequest)
		return
	}

	var cid string
	h.commitMu.RLock()
	cached, infoOK := h.infoCache[ref.owner+"/"+ref.module]
	cachedFiles := h.filesMap[cached.commitID]
	h.commitMu.RUnlock()

	var files []content.File
	var digest []byte
	if infoOK && len(cachedFiles) > 0 {
		cid = cached.commitID
		files = cachedFiles
		digest = cached.digest
	} else {
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, "")
		if err != nil {
			http.Error(w, fmt.Sprintf("resolving %s/%s: %v", ref.owner, ref.module, err), http.StatusInternalServerError)
			return
		}
		files, err = h.api.repo.GetFiles(r.Context(), ref.owner, ref.module, meta.Commit)
		if err != nil {
			http.Error(w, fmt.Sprintf("getting files: %v", err), http.StatusInternalServerError)
			return
		}
		cid = deterministicID(meta.Commit)
		digest, _ = h.computeB4DigestFromFiles(files)
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

// ServeGetModules handles v1/v1beta1 ModuleService/GetModules.
func (h *commitServiceHandler) ServeGetModules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body", http.StatusBadRequest)
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
		http.Error(w, "no module refs", http.StatusBadRequest)
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
