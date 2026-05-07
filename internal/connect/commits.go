package connect

import (
	"encoding/binary"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/protobuf/encoding/protowire"
)

const connectEnvelopeHeaderSize = 5

type commitServiceHandler struct {
	api *api
}

func (h *commitServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	header := make([]byte, connectEnvelopeHeaderSize)
	if _, err := io.ReadFull(r.Body, header); err != nil {
		http.Error(w, "reading envelope header", http.StatusBadRequest)
		return
	}
	msgLen := binary.BigEndian.Uint32(header[1:5])
	msgBytes := make([]byte, msgLen)
	if _, err := io.ReadFull(r.Body, msgBytes); err != nil {
		http.Error(w, "reading message body", http.StatusBadRequest)
		return
	}

	refs := parseResourceRefs(msgBytes)
	if len(refs) == 0 {
		http.Error(w, "no resource refs", http.StatusBadRequest)
		return
	}

	type commitInfo struct {
		ownerID  string
		moduleID string
		commitID string
	}
	commits := make([]commitInfo, 0, len(refs))
	for _, ref := range refs {
		meta, err := h.api.repo.GetMeta(r.Context(), ref.owner, ref.module, "")
		if err != nil {
			http.Error(w, fmt.Sprintf("resolving %s/%s: %v", ref.owner, ref.module, err), http.StatusInternalServerError)
			return
		}
		commits = append(commits, commitInfo{
			ownerID:  deterministicID(ref.owner),
			moduleID: deterministicID(ref.owner + "/" + ref.module),
			commitID: deterministicID(meta.Commit),
		})
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
		respMsg = protowire.AppendTag(respMsg, 1, protowire.BytesType)
		respMsg = append(respMsg, protowire.AppendVarint(nil, uint64(len(commit)))...)
		respMsg = append(respMsg, commit...)
	}

	w.Header().Set("Content-Type", "application/proto")
	envelope := make([]byte, connectEnvelopeHeaderSize+len(respMsg))
	envelope[0] = 0
	binary.BigEndian.PutUint32(envelope[1:5], uint32(len(respMsg)))
	copy(envelope[connectEnvelopeHeaderSize:], respMsg)
	_, _ = w.Write(envelope)
}

type moduleRef struct {
	owner  string
	module string
}

func parseResourceRefs(msg []byte) []moduleRef {
	var refs []moduleRef
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			break
		}
		msg = msg[n:]
		if num == 1 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			if ref := parseResourceRef(v); ref != nil {
				refs = append(refs, *ref)
			}
		} else {
			n = protowire.ConsumeFieldValue(num, typ, msg)
			if n < 0 {
				break
			}
			msg = msg[n:]
		}
	}
	return refs
}

func parseResourceRef(msg []byte) *moduleRef {
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return nil
		}
		msg = msg[n:]
		if num == 2 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			return parseResourceRefName(v)
		}
		n = protowire.ConsumeFieldValue(num, typ, msg)
		if n < 0 {
			return nil
		}
		msg = msg[n:]
	}
	return nil
}

func parseResourceRefName(msg []byte) *moduleRef {
	var owner, module string
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			break
		}
		msg = msg[n:]
		if num == 1 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			owner = string(v)
		} else if num == 2 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			module = string(v)
		} else {
			n = protowire.ConsumeFieldValue(num, typ, msg)
			if n < 0 {
				break
			}
			msg = msg[n:]
		}
	}
	if owner != "" && module != "" {
		return &moduleRef{owner: owner, module: module}
	}
	return nil
}

func deterministicID(input string) string {
	var h uint64
	for _, c := range input {
		h = h*31 + uint64(c)
	}
	return fmt.Sprintf("%016x%016x", h, h^0xdeadbeefcafebabe)
}
