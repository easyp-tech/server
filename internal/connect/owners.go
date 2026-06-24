package connect

import (
	"io"
	"log/slog"
	"net/http"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/easyp-tech/server/internal/providers/source"
)

// ownerRef is a parsed OwnerRef from a GetOwnersRequest body.
// Exactly one of id or name is populated per the proto oneof rule.
type ownerRef struct {
	id   string
	name string
}

// ServeGetOwners handles buf.registry.owner.v1.OwnerService/GetOwners.
//
// The buf CLI calls this RPC during `buf dep update` to resolve the owner
// of each dependency (e.g. "googleapis" of googleapis/googleapis). The
// request body is a GetOwnersRequest with repeated OwnerRef entries —
// each OwnerRef is a oneof, either an id (buf-style deterministic id) or
// a name (the owner name like "googleapis").
//
// We respond with a synthetic Organization for every requested owner that
// matches a known configured owner, in the same order as requested.
// Unknown owners are omitted (buf CLI treats an empty slot in the
// response list as "owner not found", which is the right answer for
// owners this proxy does not serve).
//
// The handler mirrors the wire-level approach used by the other v1
// handlers in this package: raw protowire.Append* calls, no generated
// proto types. This avoids pulling the buf.build v1 owner module as a
// Go dependency and keeps the handler consistent with the v1
// CommitService/GraphService/DownloadService/ModuleService handlers.
func (h *commitServiceHandler) ServeGetOwners(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logHandlerError(r, w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.badRequest(r, w, "reading body", slog.String("read_error", err.Error()))
		return
	}

	refs := parseGetOwnersRequest(body)
	h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
		slog.String("handler", "ServeGetOwners"),
		slog.String("procedure", "OwnerService/GetOwners"),
		slog.Int("refs", len(refs)),
		slog.Int("body_bytes", len(body)),
	)
	if len(refs) == 0 {
		h.badRequest(r, w, "no owner refs", slog.Int("body_bytes", len(body)))
		return
	}

	// Resolve each requested owner against the configured set. We match by
	// id first (the buf CLI usually sends the deterministic id it already
	// has cached) and fall back to name lookup.
	var respMsg []byte
	for _, ref := range refs {
		name := h.resolveOwnerName(ref)
		if name == "" {
			h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
				slog.String("handler", "ServeGetOwners"),
				slog.String("procedure", "OwnerService/GetOwners"),
				slog.String("branch", "owner_resolve"),
				slog.String("outcome", "not_found"),
				slog.String("requested_id", ref.id),
				slog.String("requested_name", ref.name),
			)
			continue
		}
		h.hlog(r).LogAttrs(r.Context(), slog.LevelInfo, "handler decision",
			slog.String("handler", "ServeGetOwners"),
			slog.String("procedure", "OwnerService/GetOwners"),
			slog.String("branch", "owner_resolve"),
			slog.String("outcome", "matched"),
			slog.String("owner", name),
		)
		owner := buildOwnerOrganization(name, name)
		respMsg = protowire.AppendTag(respMsg, 1, protowire.BytesType)
		respMsg = append(respMsg, protowire.AppendVarint(nil, uint64(len(owner)))...)
		respMsg = append(respMsg, owner...)
	}

	w.Header().Set("Content-Type", "application/proto")
	_, _ = w.Write(respMsg)
}

// resolveOwnerName maps a parsed OwnerRef to the owner name string,
// using the knownOwners map built at startup. Returns empty string if
// the owner is not in the configured set.
func (h *commitServiceHandler) resolveOwnerName(ref ownerRef) string {
	if ref.id != "" {
		if name, ok := h.knownOwners[ref.id]; ok {
			return name
		}
		return ""
	}
	if ref.name != "" {
		// Validate the name is in our known set so we don't fabricate
		// owners we don't serve. Owner ids are the raw owner name, so
		// the id-keyed and name-keyed lookups share one key space.
		if _, ok := h.knownOwners[ref.name]; ok {
			return ref.name
		}
		return ""
	}
	return ""
}

// buildKnownOwners builds the id → name lookup used by ServeGetOwners.
// Owner ids are the raw owner name, so the map is keyed by name.
//
// The map is built once at startup from the source list. Owners can be
// added by configuring a new repository; the proxy does not need to
// populate this map dynamically from request traffic.
func buildKnownOwners(repos []source.Source) map[string]string {
	out := make(map[string]string, len(repos))
	for _, repo := range repos {
		name := repo.Owner()
		if name == "" {
			continue
		}
		out[name] = name
	}
	return out
}

// parseGetOwnersRequest extracts the repeated OwnerRef entries from a
// GetOwnersRequest body.
//
// GetOwnersRequest: repeated OwnerRef owner_refs = 1
// OwnerRef: oneof { string id = 1; string name = 2 }
func parseGetOwnersRequest(msg []byte) []ownerRef {
	var refs []ownerRef
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			break
		}
		msg = msg[n:]
		if num == 1 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			if ref := parseOwnerRef(v); ref != nil {
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

// parseOwnerRef extracts the id or name from a single OwnerRef message.
// Per the proto oneof, at most one of id or name will be set.
func parseOwnerRef(msg []byte) *ownerRef {
	var ref ownerRef
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			break
		}
		msg = msg[n:]
		if num == 1 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			ref.id = string(v)
		} else if num == 2 && typ == protowire.BytesType {
			v, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			ref.name = string(v)
		} else {
			n = protowire.ConsumeFieldValue(num, typ, msg)
			if n < 0 {
				break
			}
			msg = msg[n:]
		}
	}
	if ref.id == "" && ref.name == "" {
		return nil
	}
	return &ref
}

// buildOwnerOrganization builds an Owner message wrapping an
// Organization. The Owner proto is a oneof with User (field 1) and
// Organization (field 2); we emit field 2.
//
// Organization fields: id=1, create_time=2, update_time=3, name=4.
func buildOwnerOrganization(id, name string) []byte {
	org := buildOrganization(id, name)
	// Owner.organization = field 2, length-delimited submessage
	var owner []byte
	owner = protowire.AppendTag(owner, 2, protowire.BytesType)
	owner = append(owner, protowire.AppendVarint(nil, uint64(len(org)))...)
	owner = append(owner, org...)
	return owner
}

// buildOrganization builds an Organization message with id, name, and
// zero timestamps. We deliberately omit create_time/update_time to keep
// the response minimal — buf CLI does not require them to be populated
// for owner lookup during `buf dep update`.
func buildOrganization(id, name string) []byte {
	var org []byte
	// id = field 1 (string)
	org = protowire.AppendTag(org, 1, protowire.BytesType)
	org = protowire.AppendString(org, id)
	// name = field 4 (string)
	org = protowire.AppendTag(org, 4, protowire.BytesType)
	org = protowire.AppendString(org, name)
	return org
}
