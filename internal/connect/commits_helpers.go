package connect

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

type moduleRef struct {
	owner  string
	module string
}

// commitUUID returns the buf-style 32-character dashless UUID for a git
// commit SHA. The buf CLI (v1.32+) validates the commit id with
// uuidutil.FromDashless, which requires exactly 32 hex characters and
// rejects anything else — including a raw 40-char git SHA — with
// "expected dashless uuid to be of length 32 but was 40".
//
// We synthesize a stable UUIDv4-shaped id from the SHA-256 of the input
// commit. SHA-256 is overkill for non-security id-minting but lets us
// reuse the stdlib without pulling google/uuid. The version nibble (4) and
// RFC 4122 variant bits are stamped in so the result round-trips through
// the buf client's parser as a syntactically-valid UUID.
//
// Determinism is the property that matters: the same git SHA must always
// map to the same UUID within a process and across processes, so that a
// client caching the id from one buf dep update finds it again on the
// next. A random UUID per call would force the client to re-resolve on
// every restart and break foreign-id caching in buf.lock.
func commitUUID(gitSHA string) string {
	if gitSHA == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(gitSHA))
	var uuid [16]byte
	copy(uuid[:], sum[:16])
	// Set version 4 (random) in the high nibble of byte 6.
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant RFC 4122 in the high two bits of byte 8.
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return hex.EncodeToString(uuid[:])
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

// parseGetGraphResourceRefs parses GetGraphRequest to extract module references.
// GetGraphRequest has: field 1 (resource_refs) repeated GetGraphRequest_ResourceRef
// Each GetGraphRequest_ResourceRef has: field 1 (ResourceRef), field 2 (Registry)
func parseGetGraphResourceRefs(msg []byte) []moduleRef {
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
			// v is GetGraphRequest_ResourceRef: field 1 = ResourceRef, field 2 = Registry
			// Extract field 1 (ResourceRef) first
			resRef := extractField1(v)
			if ref := parseResourceRef(resRef); ref != nil {
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

// parseGetGraphResourceRefsV1 parses v1 GetGraphRequest where field 1 contains ResourceRef directly.
// v1 GetGraphRequest: field 1 = repeated ResourceRef { Name { owner, module, ref } }
// (no GetGraphRequest_ResourceRef wrapper)
func parseGetGraphResourceRefsV1(msg []byte) []moduleRef {
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
			// v is ResourceRef directly (not wrapped in GetGraphRequest_ResourceRef)
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

func extractField1(msg []byte) []byte {
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return nil
		}
		msg = msg[n:]
		if num == 1 && typ == protowire.BytesType {
			v, _ := protowire.ConsumeBytes(msg)
			return v
		}
		n = protowire.ConsumeFieldValue(num, typ, msg)
		if n < 0 {
			return nil
		}
		msg = msg[n:]
	}
	return nil
}

// parseResourceRefID extracts the commit id string from a DownloadRequest.
// DownloadRequest { DownloadRequest_ResourceRef resource_ref = 1; ... }
// DownloadRequest_ResourceRef { ResourceRef resource_ref = 1; ... }
// ResourceRef { oneof value { string id = 1; Name name = 2; } }
func parseResourceRefID(msg []byte) string {
	// field 1 of DownloadRequest = DownloadRequest_ResourceRef wrapper
	wrapper := extractField1(msg)
	if wrapper == nil {
		return ""
	}
	// field 1 of wrapper = ResourceRef
	resRef := extractField1(wrapper)
	if resRef == nil {
		return ""
	}
	// field 1 of ResourceRef = id (string)
	idBytes := extractField1(resRef)
	if idBytes == nil {
		return ""
	}
	return string(idBytes)
}

// buildCommitRaw creates a Commit message: id=1, create_time=2, owner_id=3, module_id=4, digest=5.
func buildCommitRaw(cid, ownerID, moduleID string, digestValue []byte) []byte {
	var commit []byte
	commit = protowire.AppendTag(commit, 1, protowire.BytesType)
	commit = protowire.AppendString(commit, cid)

	var ts []byte
	ts = protowire.AppendTag(ts, 1, protowire.VarintType)
	ts = protowire.AppendVarint(ts, 0)
	ts = protowire.AppendTag(ts, 2, protowire.VarintType)
	ts = protowire.AppendVarint(ts, 0)
	commit = protowire.AppendTag(commit, 2, protowire.BytesType)
	commit = append(commit, protowire.AppendVarint(nil, uint64(len(ts)))...)
	commit = append(commit, ts...)

	commit = protowire.AppendTag(commit, 3, protowire.BytesType)
	commit = protowire.AppendString(commit, ownerID)
	commit = protowire.AppendTag(commit, 4, protowire.BytesType)
	commit = protowire.AppendString(commit, moduleID)

	var digest []byte
	digest = protowire.AppendTag(digest, 1, protowire.VarintType)
	digest = protowire.AppendVarint(digest, 1) // B4
	digest = protowire.AppendTag(digest, 2, protowire.BytesType)
	digest = protowire.AppendBytes(digest, digestValue)
	commit = protowire.AppendTag(commit, 5, protowire.BytesType)
	commit = append(commit, protowire.AppendVarint(nil, uint64(len(digest)))...)
	commit = append(commit, digest...)

	return commit
}

func parseModuleRefByID(msg []byte, moduleLookup map[string]string) *struct {
	owner  string
	module string
} {
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return nil
		}
		msg = msg[n:]
		if num == 1 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			id := string(v)
			if key, ok := moduleLookup[id]; ok {
				parts := strings.SplitN(key, "/", 2)
				if len(parts) == 2 {
					return &struct {
						owner  string
						module string
					}{owner: parts[0], module: parts[1]}
				}
			}
			return nil
		} else if num == 2 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			return parseModuleName(v)
		}
		n = protowire.ConsumeFieldValue(num, typ, msg)
		if n < 0 {
			return nil
		}
		msg = msg[n:]
	}
	return nil
}

func parseModuleName(msg []byte) *struct {
	owner  string
	module string
} {
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
		return &struct {
			owner  string
			module string
		}{owner: owner, module: module}
	}
	return nil
}

// buildModule creates a Module message:
// id=1, create_time=2, update_time=3, name=4, owner_id=5,
// visibility=6(PUBLIC=1), state=7(ACTIVE=1), default_label_name=10.
func buildModule(owner, module string) []byte {
	var m []byte
	m = protowire.AppendTag(m, 1, protowire.BytesType)
	m = protowire.AppendString(m, owner+"/"+module)

	// create_time: Timestamp { seconds=1, nanos=2 }
	var ts []byte
	ts = protowire.AppendTag(ts, 1, protowire.VarintType)
	ts = protowire.AppendVarint(ts, 0)
	ts = protowire.AppendTag(ts, 2, protowire.VarintType)
	ts = protowire.AppendVarint(ts, 0)
	m = protowire.AppendTag(m, 2, protowire.BytesType)
	m = append(m, protowire.AppendVarint(nil, uint64(len(ts)))...)
	m = append(m, ts...)

	// update_time = create_time
	m = protowire.AppendTag(m, 3, protowire.BytesType)
	m = append(m, protowire.AppendVarint(nil, uint64(len(ts)))...)
	m = append(m, ts...)

	m = protowire.AppendTag(m, 4, protowire.BytesType)
	m = protowire.AppendString(m, module)
	m = protowire.AppendTag(m, 5, protowire.BytesType)
	m = protowire.AppendString(m, owner)

	// visibility = MODULE_VISIBILITY_PUBLIC = 1
	m = protowire.AppendTag(m, 6, protowire.VarintType)
	m = protowire.AppendVarint(m, 1)
	// state = MODULE_STATE_ACTIVE = 1
	m = protowire.AppendTag(m, 7, protowire.VarintType)
	m = protowire.AppendVarint(m, 1)
	// default_label_name = "main"
	m = protowire.AppendTag(m, 10, protowire.BytesType)
	m = protowire.AppendString(m, "main")

	return m
}
