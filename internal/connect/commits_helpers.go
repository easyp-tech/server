package connect

import (
	"encoding/hex"
	"errors"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

type moduleRef struct {
	owner  string
	module string
	// ref is the buf BSR Name.ref field (proto field 3): the branch or
	// tag the client asked for. Empty when the client did not send one
	// (older buf CLI versions, or v1beta1 callers that omit the field);
	// the providers treat empty ref as HEAD.
	ref string
}

// commitUUID returns the buf-style 32-character dashless UUID for a git
// commit SHA. The buf CLI (v1.32+) validates the commit id with
// uuidutil.FromDashless, which requires exactly 32 hex characters and
// rejects anything else — including a raw 40-char git SHA — with
// "expected dashless uuid to be of length 32 but was 40".
//
// The 16-byte UUID is built from the first 14 bytes of the decoded 20-byte
// git SHA plus the standard UUID version-4 and RFC 4122 variant bits at
// positions 6 and 8. SHA bytes 14-19 are not represented in the id; the
// inverse (UUID -> SHA prefix) recovers sha[0..13]. The result is
// hex-encoded to 32 lowercase chars, a syntactically-valid dashless UUID.
//
// Determinism is the property that matters: the same git SHA must always
// map to the same UUID within a process and across processes, so that a
// client caching the id from one buf dep update finds it again on the
// next. A random UUID per call would force the client to re-resolve on
// every restart and break foreign-id caching in buf.lock.
//
// Input contract: the input must be exactly 40 or 64 lowercase hex characters
// (the standard full-length git SHA-1 or SHA-256 representation). Anything else
// returns ("", error). Production callers always pass full SHAs from
// upstream GetMeta, so any non-conforming input is a contract violation.
// 64-char SHA-256 input is required for Bitbucket Server on SHA-256-enabled
// repos (bitbucket/getrepo.go:40), which returns out.Commit as 64 chars.
func commitUUID(gitSHA string) (string, error) {
	if len(gitSHA) != 40 && len(gitSHA) != 64 {
		return "", errors.New("commitUUID: input is not 40 or 64 lowercase hex characters")
	}
	sha, err := hex.DecodeString(gitSHA)
	if err != nil {
		return "", errors.New("commitUUID: input is not 40 or 64 lowercase hex characters")
	}
	var result [16]byte
	// SHA bytes 0..5 -> result bytes 0..5.
	copy(result[0:6], sha[0:6])
	// Result byte 6 = UUID version-4 nibble (high nibble = 4, low nibble = 0).
	result[6] = 0x40
	// SHA byte 6 -> result byte 7.
	result[7] = sha[6]
	// Result byte 8 = RFC 4122 variant bits (high two bits = 10, low six bits = 0).
	result[8] = 0x80
	// SHA bytes 7..13 -> result bytes 9..15.
	copy(result[9:16], sha[7:14])
	return hex.EncodeToString(result[:]), nil
}

// isSHA reports whether s is a 40-char (SHA-1) or 64-char (SHA-256)
// lowercase hex string. The hex check rejects refs like "main/v2" and
// buf-issued UUIDs (32 chars). Used by providers to decide whether to
// treat a GetMeta commit arg as a raw SHA (fast path) or as a ref to
// resolve through the commit-fetch API.
func isSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// isUUID reports whether s is exactly 32 lowercase hex characters — the
// shape of a buf-issued dashless UUID. Used by probeCommitID to detect
// UUID inputs and route them through commitUUIDInverse.
func isUUID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// commitUUIDInverse recovers the first 28 hex characters (14 bytes) of
// the git SHA that produced the given buf-issued dashless UUID. The
// full SHA cannot be recovered: commitUUID drops the last 6 bytes
// during the forward mapping (14-of-20-byte collision surface — 2^112
// space, accepted as out-of-scope for collision avoidance). The
// recovered prefix is sufficient to identify a specific source among
// the configured providers (each source's commit space is disjoint)
// and to scope a probeCommitID fan-out to the right repository.
//
// Input contract: the input must be exactly 32 lowercase hex characters
// (the standard dashless UUID shape that uuidutil.FromDashless
// accepts). Any other input returns ("", error).
//
// Inverse: if uuid == commitUUID(sha) for some 40- or 64-char sha,
// then commitUUIDInverse(uuid) == hex(sha[0:14]). The inverse does NOT
// require knowledge of the original sha length — it recovers the same
// 14 bytes regardless of whether the input was SHA-1 or SHA-256.
func commitUUIDInverse(uuid string) (string, error) {
	if len(uuid) != 32 {
		return "", errors.New("commitUUIDInverse: input is not 32 lowercase hex characters")
	}
	u, err := hex.DecodeString(uuid)
	if err != nil {
		return "", errors.New("commitUUIDInverse: input is not 32 lowercase hex characters")
	}
	// Mirror commitUUID's byte-table in reverse. Bytes 6 and 8 of u are
	// version/variant and were overwritten by commitUUID; they are not
	// recoverable. The other 14 bytes are the first 14 bytes of the SHA.
	var sha [20]byte
	copy(sha[0:6], u[0:6])
	sha[6] = u[7]
	copy(sha[7:14], u[9:16])
	return hex.EncodeToString(sha[:14]), nil
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
	var owner, module, ref string
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
		} else if num == 3 && typ == protowire.BytesType {
			// buf BSR Name.ref (branch/tag). Optional: older buf clients
			// do not send it; the ref-aware code paths tolerate an empty
			// value (treated as HEAD by the providers).
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			ref = string(v)
		} else {
			n = protowire.ConsumeFieldValue(num, typ, msg)
			if n < 0 {
				break
			}
			msg = msg[n:]
		}
	}
	if owner != "" && module != "" {
		return &moduleRef{owner: owner, module: module, ref: ref}
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
