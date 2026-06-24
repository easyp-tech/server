package connect

import (
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

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
