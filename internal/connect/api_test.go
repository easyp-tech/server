package connect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	v1alpha1connect "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect"
	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/source"
	"github.com/easyp-tech/server/internal/shake256"
	"google.golang.org/protobuf/encoding/protowire"
)

// errUpstream is the sentinel error the upstream-failure test injects into
// the mock provider. It must be distinct from any other error and have a
// non-empty message so the test can confirm structured logging preserved it.
var errUpstream = errors.New("upstream is down")

// mockProvider implements provider for testing.
type mockProvider struct {
	meta  content.Meta
	files []content.File
	err   error
	repos []source.Source
	// Optional per-commit overrides. When non-nil, GetMeta/GetFiles serve the
	// entry for the requested commit (or a not-found error), letting tests
	// distinguish HEAD from an older sha. When nil, the legacy m.meta/m.files
	// behavior is used (ignoring the commit arg).
	byCommit     map[string]content.Meta
	filesByCommit map[string][]content.File
}

func (m *mockProvider) GetMeta(_ context.Context, _, _, commit string) (content.Meta, error) {
	if m.byCommit != nil {
		if meta, ok := m.byCommit[commit]; ok {
			return meta, nil
		}
		return content.Meta{}, fmt.Errorf("mock: commit %q not found", commit)
	}
	return m.meta, m.err
}

func (m *mockProvider) GetFiles(_ context.Context, _, _, commit string) ([]content.File, error) {
	if m.filesByCommit != nil {
		if files, ok := m.filesByCommit[commit]; ok {
			return files, nil
		}
		return nil, fmt.Errorf("mock: no files for commit %q", commit)
	}
	return m.files, m.err
}

func (m *mockProvider) Repositories() []source.Source {
	return m.repos
}

// mockSource is a source.Source that returns canned metadata for
// OwnerService tests. It only implements the methods needed for
// buildKnownOwners (Owner, RepoName) plus the Source interface contract
// that the rest of the package relies on.
//
// For pre-warm/probe tests, GetMeta is sha-aware: it owns exactly one commit
// (commit). A HEAD probe (commit arg "") resolves to it; any other commit arg
// is reported as not-owned (error). getMetaCalls counts calls so tests can
// assert the negative cache suppresses repeat probes.
type mockSource struct {
	owner    string
	repoName string
	typ      string
	commit   string

	getMetaCalls *atomic.Int32
	getMetaErr   error // when set, GetMeta always returns this error
}

func (s *mockSource) GetMeta(_ context.Context, commit string) (content.Meta, error) {
	if s.getMetaCalls != nil {
		s.getMetaCalls.Add(1)
	}
	if s.getMetaErr != nil {
		return content.Meta{}, s.getMetaErr
	}
	// HEAD (empty arg) resolves to the source's own commit; a specific
	// commit resolves only if it is the one this source owns. A SHA
	// prefix (a 28-char substring of s.commit) also resolves — used by
	// the post-restart probe path (commitUUIDInverse + prefix-match).
	if commit != "" && commit != s.commit && !strings.HasPrefix(s.commit, commit) {
		return content.Meta{}, fmt.Errorf("mock: commit %q not found in %s/%s", commit, s.owner, s.repoName)
	}
	return content.Meta{Commit: s.commit, DefaultBranch: "main"}, nil
}

func (s *mockSource) GetFiles(_ context.Context, _ string) ([]content.File, error) {
	return nil, nil
}

func (s *mockSource) ConfigHash() string { return "mock" }

func (s *mockSource) Name() string { return s.repoName }

func (s *mockSource) Owner() string { return s.owner }

func (s *mockSource) RepoName() string { return s.repoName }

func (s *mockSource) Type() string {
	if s.typ == "" {
		return "mock"
	}
	return s.typ
}

func testMux(p provider) *http.ServeMux {
	return testMuxWithLogger(p, slog.Default())
}

// testMuxWithLogger is like testMux but injects a custom logger. Used by
// tests that need to inspect the structured log output (server, commit_id,
// module, etc.).
func testMuxWithLogger(p provider, log *slog.Logger) *http.ServeMux {
	return New(log, p, "buf.example.com")
}

// testMuxWithConfig is like testMuxWithLogger but enables commit-resolution
// enhancements via NewWithConfig, so probe/prewarm behavior can be exercised.
func testMuxWithConfig(p provider, log *slog.Logger, cfg CommitResolution) *http.ServeMux {
	return NewWithConfig(log, p, "buf.example.com", cfg)
}

// buildGetCommitsRequest builds a protobuf-encoded GetCommits request
// with one resource ref for the given owner/module.
// ResourceRef { Name name = 2; Name { owner = 1; module = 2 } }
func buildGetCommitsRequest(owner, module string) []byte {
	// Name: owner=1, module=2
	var name []byte
	name = protowire.AppendTag(name, 1, protowire.BytesType)
	name = protowire.AppendString(name, owner)
	name = protowire.AppendTag(name, 2, protowire.BytesType)
	name = protowire.AppendString(name, module)

	// ResourceRef: name=2
	var ref []byte
	ref = protowire.AppendTag(ref, 2, protowire.BytesType)
	ref = append(ref, protowire.AppendVarint(nil, uint64(len(name)))...)
	ref = append(ref, name...)

	// GetCommitsRequest: resource_refs=1
	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType)
	req = append(req, protowire.AppendVarint(nil, uint64(len(ref)))...)
	req = append(req, ref...)
	return req
}

// buildGetGraphRequest builds a protobuf-encoded GetGraph request.
// GetGraphRequest { resource_refs = 1; GetGraphRequest_ResourceRef { resource_ref = 1; ResourceRef { name = 2; Name { owner=1; module=2 } } } }
func buildGetGraphRequest(owner, module string) []byte {
	var name []byte
	name = protowire.AppendTag(name, 1, protowire.BytesType)
	name = protowire.AppendString(name, owner)
	name = protowire.AppendTag(name, 2, protowire.BytesType)
	name = protowire.AppendString(name, module)

	var resRef []byte
	resRef = protowire.AppendTag(resRef, 2, protowire.BytesType)
	resRef = append(resRef, protowire.AppendVarint(nil, uint64(len(name)))...)
	resRef = append(resRef, name...)

	var graphRef []byte
	graphRef = protowire.AppendTag(graphRef, 1, protowire.BytesType)
	graphRef = append(graphRef, protowire.AppendVarint(nil, uint64(len(resRef)))...)
	graphRef = append(graphRef, resRef...)

	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType)
	req = append(req, protowire.AppendVarint(nil, uint64(len(graphRef)))...)
	req = append(req, graphRef...)
	return req
}

// buildV1GetGraphRequest builds a v1-format GetGraph request with ResourceRef directly.
// v1 GetGraphRequest: field 1 = repeated ResourceRef { name = 2; Name { owner=1; module=2 } }
// (no GetGraphRequest_ResourceRef wrapper)
func buildV1GetGraphRequest(owner, module string) []byte {
	var name []byte
	name = protowire.AppendTag(name, 1, protowire.BytesType)
	name = protowire.AppendString(name, owner)
	name = protowire.AppendTag(name, 2, protowire.BytesType)
	name = protowire.AppendString(name, module)

	var resRef []byte
	resRef = protowire.AppendTag(resRef, 2, protowire.BytesType)
	resRef = append(resRef, protowire.AppendVarint(nil, uint64(len(name)))...)
	resRef = append(resRef, name...)

	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType)
	req = append(req, protowire.AppendVarint(nil, uint64(len(resRef)))...)
	req = append(req, resRef...)
	return req
}

// buildV1GetGraphRequestWithRef builds a v1 GetGraph request whose Name carries
// a ref (proto field 4) — the branch/tag/commit the client pinned. buf.lock
// stores the proxy-minted 32-hex commit_id here, so ref is usually that id.
func buildV1GetGraphRequestWithRef(owner, module, ref string) []byte {
	var name []byte
	name = protowire.AppendTag(name, 1, protowire.BytesType)
	name = protowire.AppendString(name, owner)
	name = protowire.AppendTag(name, 2, protowire.BytesType)
	name = protowire.AppendString(name, module)
	name = protowire.AppendTag(name, 4, protowire.BytesType)
	name = protowire.AppendString(name, ref)

	var resRef []byte
	resRef = protowire.AppendTag(resRef, 2, protowire.BytesType)
	resRef = append(resRef, protowire.AppendVarint(nil, uint64(len(name)))...)
	resRef = append(resRef, name...)

	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType)
	req = append(req, protowire.AppendVarint(nil, uint64(len(resRef)))...)
	req = append(req, resRef...)
	return req
}

// recordingProvider is a mockProvider that records every commit arg passed to
// GetMeta, so tests can assert which ref the handler forwarded upstream.
type recordingProvider struct {
	meta    content.Meta
	bySha   map[string]content.Meta
	getMeta []string
	// head, when set, is returned for an empty commit arg (HEAD). Real
	// providers resolve HEAD to the default-branch tip; the mock cannot
	// infer which bySha entry that is, so it must be told explicitly.
	// Without this, the prefix-match loop would match every entry for ""
	// (HasPrefix(x, "") is always true) and return a nondeterministic one.
	head *content.Meta
}

func (r *recordingProvider) GetMeta(_ context.Context, _, _, commit string) (content.Meta, error) {
	r.getMeta = append(r.getMeta, commit)
	if commit == "" && r.head != nil {
		return *r.head, nil
	}
	if m, ok := r.bySha[commit]; ok {
		return m, nil
	}
	// Prefix match: real providers (GitHub via repos.GetCommit, Bitbucket
	// via its commit-fetch API) resolve short-SHA prefixes (>=7 hex) to the
	// full SHA. The mock must mirror that so ServeGraph's
	// commitUUIDInverse→28-hex-prefix probe path can be exercised end-to-end
	// against this fixture. Without it the mock would reject the 28-hex
	// prefix that resolveUUIDRef derives from a buf-issued 32-hex cid.
	// Only fire for non-empty args: HasPrefix(x, "") is always true, so an
	// empty commit must be handled by the head branch above (or miss).
	if commit != "" {
		for fullSHA, m := range r.bySha {
			if strings.HasPrefix(fullSHA, commit) {
				return m, nil
			}
		}
	}
	return content.Meta{}, fmt.Errorf("mock: upstream has no commit %q", commit)
}

func (r *recordingProvider) GetFiles(_ context.Context, _, _, _ string) ([]content.File, error) {
	return nil, nil
}
func (r *recordingProvider) Repositories() []source.Source { return nil }

// TestServeGraph_BufCommitIDRefNotForwardedToUpstream confirms Defect 1:
// when a client pins a dependency via the proxy-minted 32-hex buf commit_id
// (the value buf.lock stores), ServeGraph must NOT forward that 32-hex id to
// the upstream GetMeta as if it were a git SHA. The upstream only knows the
// 40-hex git SHA; sending the 32-hex id yields a 422 "No commit found for SHA".
//
// Reproduces prod failure: grpc-ecosystem/grpc-gateway pinned at
// commit_id e91b8a68fe214081808d79f1a1a4f09e (derived from git sha
// e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9). Expect: graph resolves to the
// pinned commit (200). Actual (bug): GetMeta is called with the 32-hex id and
// the request fails.
func TestServeGraph_BufCommitIDRefNotForwardedToUpstream(t *testing.T) {
	const (
		gitSHA = "e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9"
		cid    = "e91b8a68fe214081808d79f1a1a4f09e" // == commitUUID(gitSHA)
	)
	// Upstream recognizes ONLY the real 40-hex git SHA — mirrors GitHub,
	// which returns 422 for the 32-hex buf id.
	repo := &recordingProvider{
		bySha: map[string]content.Meta{
			gitSHA: {Commit: gitSHA, DefaultBranch: "main"},
		},
	}
	mux := testMux(repo)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/buf.registry.module.v1.GraphService/GetGraph",
		"application/proto", bytes.NewReader(buildV1GetGraphRequestWithRef("grpc-ecosystem", "grpc-gateway", cid)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (pinned commit must resolve)", resp.StatusCode, http.StatusOK)
	}
	for _, c := range repo.getMeta {
		if c == cid {
			t.Errorf("GetMeta forwarded buf commit_id %q to upstream as a git SHA — "+
				"upstream rejects it (422). Forward the 40-hex git SHA %q instead. calls=%v",
				cid, gitSHA, repo.getMeta)
		}
	}
}

// TestServeGraph_InfoCacheMustNotServeWrongCommit confirms Defect 2:
// infoCache is keyed only by owner/module, so once ANY commit for a module is
// cached, a later request pinning a DIFFERENT commit id returns the cached
// (wrong) commit's id+digest. The proxy must honor the requested commit.
//
// Reproduces prod "no content returned for commit ID e91b8a68...": the proxy
// served main-HEAD content under the pinned commit_id.
func TestServeGraph_InfoCacheMustNotServeWrongCommit(t *testing.T) {
	const (
		headSHA = "34a6674c253f287533e8e904d89eb530b574128d" // main HEAD
		pinSHA  = "e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9" // pinned older commit
	)
	headCID, _ := commitUUID(headSHA)
	pinCID, _ := commitUUID(pinSHA)

	repo := &recordingProvider{
		bySha: map[string]content.Meta{
			headSHA: {Commit: headSHA, DefaultBranch: "main"},
			pinSHA:  {Commit: pinSHA, DefaultBranch: "main"},
		},
	}
	mux := testMux(repo)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1) Prime infoCache with main HEAD (e.g. a prior HEAD/tag resolution).
	if _, err := http.Post(srv.URL+"/buf.registry.module.v1.GraphService/GetGraph",
		"application/proto", bytes.NewReader(buildV1GetGraphRequestWithRef("grpc-ecosystem", "grpc-gateway", headSHA))); err != nil {
		t.Fatalf("prime request failed: %v", err)
	}

	// 2) Now pin a different commit via its buf id. Must resolve to pinCID, not headCID.
	resp, err := http.Post(srv.URL+"/buf.registry.module.v1.GraphService/GetGraph",
		"application/proto", bytes.NewReader(buildV1GetGraphRequestWithRef("grpc-ecosystem", "grpc-gateway", pinCID)))
	if err != nil {
		t.Fatalf("pinned request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if bytes.Contains(body, []byte(headCID)) {
		t.Errorf("graph returned main-HEAD commit_id %q for a request pinning %q — "+
			"infoCache keyed by owner/module served the wrong commit. body has HEAD id.",
			headCID, pinCID)
	}
	if !bytes.Contains(body, []byte(pinCID)) {
		t.Errorf("graph did not return the pinned commit_id %q; body=%x", pinCID, body)
	}
}

// TestServeGraph_UUIDRefShortCircuitsFromCidShaMap confirms that once the
// proxy has minted a cid for a git SHA (and populated cidSha + infoCache),
// a subsequent ServeGraph request that pins that cid is served entirely
// from cache — zero GetMeta calls — and the response carries the pinned cid.
// This is the warm-cache path: cidSha hit short-circuits before any upstream
// touch, so no 422 risk and no round-trip.
func TestServeGraph_UUIDRefShortCircuitsFromCidShaMap(t *testing.T) {
	const (
		gitSHA = "e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9"
	)
	cid, _ := commitUUID(gitSHA)

	repo := &recordingProvider{
		bySha: map[string]content.Meta{
			gitSHA: {Commit: gitSHA, DefaultBranch: "main"},
		},
	}
	mux := testMux(repo)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Prime: a prior resolution populated cidSha[cid]=gitSHA and the
	// infoCache entry keyed by owner/module (commitID=cid). Send the git
	// SHA as the ref — the existing path resolves it, mints the cid, and
	// writes back both maps.
	if _, err := http.Post(srv.URL+"/buf.registry.module.v1.GraphService/GetGraph",
		"application/proto", bytes.NewReader(buildV1GetGraphRequestWithRef("grpc-ecosystem", "grpc-gateway", gitSHA))); err != nil {
		t.Fatalf("prime request failed: %v", err)
	}

	// The prime should have made exactly one GetMeta call (for the git SHA).
	primeCalls := len(repo.getMeta)

	// Pinned request: send the cid. infoCache has commitID=cid, so the
	// cache-hit gate serves directly. No GetMeta, no resolveUUIDRef probe.
	resp, err := http.Post(srv.URL+"/buf.registry.module.v1.GraphService/GetGraph",
		"application/proto", bytes.NewReader(buildV1GetGraphRequestWithRef("grpc-ecosystem", "grpc-gateway", cid)))
	if err != nil {
		t.Fatalf("pinned request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := len(repo.getMeta) - primeCalls; got != 0 {
		t.Errorf("pinned-cid request made %d new GetMeta call(s); want 0 (warm cidSha must short-circuit). calls=%v",
			got, repo.getMeta)
	}
	if !bytes.Contains(body, []byte(cid)) {
		t.Errorf("response does not carry the pinned cid %q; body=%x", cid, body)
	}
}

// TestServeGraph_UUIDRefColdCache_ProbesWithInversePrefix confirms the
// cold-cache path: when cidSha is empty for a cid, ServeGraph resolves via
// commitUUIDInverse → 28-hex prefix → upstream GetMeta(prefix). The upstream
// MUST be called with the prefix (not the cid), and the cid→sha mapping is
// cached afterwards so the next request short-circuits.
func TestServeGraph_UUIDRefColdCache_ProbesWithInversePrefix(t *testing.T) {
	const (
		gitSHA = "e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9"
	)
	cid, _ := commitUUID(gitSHA)
	prefix, _ := commitUUIDInverse(cid) // 28-hex

	repo := &recordingProvider{
		bySha: map[string]content.Meta{
			gitSHA: {Commit: gitSHA, DefaultBranch: "main"},
		},
	}
	mux := testMux(repo)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// No prime — cidSha is empty. ServeGraph must call GetMeta with the
	// 28-hex prefix (which the recordingProvider resolves via prefix-match,
	// mirroring real GitHub/Bitbucket behavior), NOT with the raw cid.
	resp, err := http.Post(srv.URL+"/buf.registry.module.v1.GraphService/GetGraph",
		"application/proto", bytes.NewReader(buildV1GetGraphRequestWithRef("grpc-ecosystem", "grpc-gateway", cid)))
	if err != nil {
		t.Fatalf("cold-cache pinned request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (prefix probe must resolve)", resp.StatusCode, http.StatusOK)
	}
	if len(repo.getMeta) == 0 {
		t.Fatal("expected at least one GetMeta call for the prefix probe; got zero")
	}
	// Every GetMeta call must be the prefix (or the resolved SHA), never the cid.
	for _, c := range repo.getMeta {
		if c == cid {
			t.Errorf("GetMeta forwarded the raw cid %q to upstream (must use the %q prefix); calls=%v",
				cid, prefix, repo.getMeta)
		}
	}
	// The first call should be the prefix (resolveUUIDRef's probe).
	if repo.getMeta[0] != prefix {
		t.Errorf("first GetMeta call = %q, want the 28-hex prefix %q; calls=%v",
			repo.getMeta[0], prefix, repo.getMeta)
	}
	if !bytes.Contains(body, []byte(cid)) {
		t.Errorf("response does not carry the resolved cid %q; body=%x", cid, body)
	}
}

// TestServeGraph_UUIDRefColdCache_NegativeCachesMiss confirms CR-01: an
// unknown cid (one the upstream does not own) is negative-cached after the
// first prefix probe, so a retry within TTL does NOT re-probe. This is the
// defense probeCommitID has against flooding distinct unknown ids; the
// resolveUUIDRef path must inherit it. Uses newTestCommitHandler so
// probeNegativeTTL is non-zero (testMux wires a zero TTL which disables
// negative caching, mirroring the production "enhancements off" default).
func TestServeGraph_UUIDRefColdCache_NegativeCachesMiss(t *testing.T) {
	const (
		// A 40-hex SHA the upstream does NOT own; commitUUID of its first
		// 14 bytes yields a cid whose 28-hex prefix the mock will reject.
		unknownSHA = "ffffffffffffffffffffffffffffffffffffffff"
	)
	unknownCID, _ := commitUUID(unknownSHA)

	// recordingProvider rejects any commit it doesn't have (no bySha entry
	// matches), returning a non-transient error so the miss is cacheable.
	repo := &recordingProvider{
		bySha: map[string]content.Meta{
			// Intentionally does NOT contain unknownSHA.
		},
	}
	h := newTestCommitHandler(repo)

	_, ok := h.resolveUUIDRef(context.Background(), moduleRef{owner: "o", module: "m"}, unknownCID)
	if ok {
		t.Fatal("unknown cid should not resolve")
	}
	firstCalls := len(repo.getMeta)
	if firstCalls != 1 {
		t.Fatalf("expected 1 GetMeta call on first probe, got %d", firstCalls)
	}
	if !h.missCached(unknownCID) {
		t.Fatal("unknown cid not negative-cached after a definitive miss")
	}

	// Retry within TTL: must NOT re-probe.
	_, ok2 := h.resolveUUIDRef(context.Background(), moduleRef{owner: "o", module: "m"}, unknownCID)
	if ok2 {
		t.Fatal("unknown cid should still not resolve on retry")
	}
	if got := len(repo.getMeta) - firstCalls; got != 0 {
		t.Errorf("negative-cached cid re-probed; %d new GetMeta call(s), want 0", got)
	}
}

// TestServeGraph_PinnedCidDoesNotPoisonHeadRequest confirms WR-01's reverse
// direction: after a pinned-cid request writes back an infoCache entry for a
// module, a subsequent HEAD (empty ref) request for the same module must
// RE-RESOLVE rather than be served the pinned cid's entry. Without the
// symmetric cid-gate the pinned writeback would stick the module to that
// commit for every later HEAD/SHA/tag request until restart.
func TestServeGraph_PinnedCidDoesNotPoisonHeadRequest(t *testing.T) {
	const (
		pinSHA  = "e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9" // pinned older commit
		headSHA = "34a6674c253f287533e8e904d89eb530b574128d" // main HEAD
	)
	pinCID, _ := commitUUID(pinSHA)
	headCID, _ := commitUUID(headSHA)

	repo := &recordingProvider{
		bySha: map[string]content.Meta{
			pinSHA:  {Commit: pinSHA, DefaultBranch: "main"},
			headSHA: {Commit: headSHA, DefaultBranch: "main"},
		},
		head: &content.Meta{Commit: headSHA, DefaultBranch: "main"},
	}
	mux := testMux(repo)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1) Prime: a pinned-cid request resolves pinSHA and writes back an
	//    infoCache entry minted for the cid (cidPinned=true).
	if _, err := http.Post(srv.URL+"/buf.registry.module.v1.GraphService/GetGraph",
		"application/proto", bytes.NewReader(buildV1GetGraphRequestWithRef("grpc-ecosystem", "grpc-gateway", pinCID))); err != nil {
		t.Fatalf("pinned prime failed: %v", err)
	}
	primeCalls := len(repo.getMeta)

	// 2) HEAD request (empty ref) for the same module. The symmetric gate
	//    must treat the pinned entry as a miss and re-resolve HEAD.
	//    buildV1GetGraphRequest (no ref field) yields ref.ref == "".
	resp, err := http.Post(srv.URL+"/buf.registry.module.v1.GraphService/GetGraph",
		"application/proto", bytes.NewReader(buildV1GetGraphRequest("grpc-ecosystem", "grpc-gateway")))
	if err != nil {
		t.Fatalf("HEAD request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD request after pin: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	// Must have re-resolved: at least one new GetMeta call (for HEAD).
	if got := len(repo.getMeta) - primeCalls; got < 1 {
		t.Errorf("HEAD request after pinned writeback made %d new GetMeta call(s); want >= 1 (symmetric gate must re-resolve). calls=%v",
			got, repo.getMeta)
	}
	// Must serve HEAD's cid, not the pinned cid.
	if bytes.Contains(body, []byte(pinCID)) && !bytes.Contains(body, []byte(headCID)) {
		t.Errorf("HEAD request served the pinned cid %q instead of re-resolving HEAD %q; body=%x",
			pinCID, headCID, body)
	}
	if !bytes.Contains(body, []byte(headCID)) {
		t.Errorf("HEAD request did not return HEAD commit_id %q; body=%x", headCID, body)
	}
}

// TestServeDownload_PinnedCidNotServedFromWrongInfoCache confirms the
// ServeDownload fix: when infoCache was minted for HEAD (cid=headCID) but
// the request pins a different cid (pinCID) whose cidSha is known, the
// files-cache hit is gated out (cid mismatch) and the fetch path prefers
// cidSha[pinCID] over the cached HEAD commit. The response carries the
// pinned cid and the pinned commit's files — not HEAD's.
//
// The handler is seeded directly (not via prime HTTP requests) so the
// infoCache genuinely holds the WRONG (HEAD) entry while cidSha holds the
// pinned cid → sha mapping — the exact prod state that produced "no content
// returned for commit ID <pinned>".
func TestServeDownload_PinnedCidNotServedFromWrongInfoCache(t *testing.T) {
	const (
		headSHA = "34a6674c253f287533e8e904d89eb530b574128d"
		pinSHA  = "e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9"
		owner   = "grpc-ecosystem"
		module  = "grpc-gateway"
	)
	headCID, _ := commitUUID(headSHA)
	pinCID, _ := commitUUID(pinSHA)
	ref := moduleRef{owner: owner, module: module}

	headFile := content.File{Path: "head_only.txt", Data: []byte("HEAD content")}
	pinFile := content.File{Path: "pin_only.txt", Data: []byte("pinned commit content")}

	repo := &mockProvider{
		byCommit: map[string]content.Meta{
			pinSHA: {Commit: pinSHA, DefaultBranch: "main"},
		},
		filesByCommit: map[string][]content.File{
			pinSHA: {pinFile},
		},
	}

	h := newTestCommitHandler(repo)
	// infoCache holds the WRONG entry: HEAD was minted, pinned cid was not.
	h.infoCache[owner+"/"+module] = commitInfoCache{
		commitID: headCID,
		commit:   headSHA,
		ownerID:  owner,
		moduleID: owner + "/" + module,
	}
	h.filesMap[headCID] = []content.File{headFile}
	// commitMap knows the pinned cid → module (e.g. a prior GetCommits
	// registered it, or the foreign-id fallback did).
	h.commitMap[pinCID] = ref
	// cidSha knows the pinned cid → real SHA. This is what ServeDownload
	// must prefer over the infoCache's HEAD commit.
	h.cidSha[pinCID] = pinSHA

	mux := http.NewServeMux()
	mux.HandleFunc("/buf.registry.module.v1.DownloadService/", h.ServeDownload)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/buf.registry.module.v1.DownloadService/Download",
		"application/proto", bytes.NewReader(buildDownloadRequest(pinCID)))
	if err != nil {
		t.Fatalf("download request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	// Response must carry the pinned cid, NOT HEAD's cid.
	if bytes.Contains(body, []byte(headCID)) {
		t.Errorf("download returned HEAD commit_id %q for a request pinning %q — "+
			"infoCache served the wrong commit", headCID, pinCID)
	}
	if !bytes.Contains(body, []byte(pinCID)) {
		t.Errorf("download did not return the pinned commit_id %q; body=%x", pinCID, body)
	}
	// Content must be the pinned commit's files, not HEAD's.
	if !bytes.Contains(body, pinFile.Data) {
		t.Errorf("download did not return the pinned commit's file content %q; body=%x",
			string(pinFile.Data), body)
	}
	if bytes.Contains(body, headFile.Data) {
		t.Errorf("download returned HEAD's file content %q for a request pinning %q",
			string(headFile.Data), pinCID)
	}
}

// dualCountingProvider wraps a provider to count GetMeta and GetFiles calls so
// tests can assert that a warm cache suppresses repeat upstream traffic. Unlike
// countingProvider (blobs_test.go, GetFiles-only via atomic pointer), this one
// tracks both methods locally for the WR-03 repeat-request assertion.
type dualCountingProvider struct {
	provider
	getMetaCalls  int
	getFilesCalls int
}

func (c *dualCountingProvider) GetMeta(ctx context.Context, owner, repo, commit string) (content.Meta, error) {
	c.getMetaCalls++
	return c.provider.GetMeta(ctx, owner, repo, commit)
}

func (c *dualCountingProvider) GetFiles(ctx context.Context, owner, repo, commit string) ([]content.File, error) {
	c.getFilesCalls++
	return c.provider.GetFiles(ctx, owner, repo, commit)
}

// TestServeDownload_PinnedCidRepeatHitsFilesCache confirms WR-03: after the
// first pinned-cid Download resolves and writes back infoCache + filesMap,
// the second identical pinned-cid Download makes ZERO new GetMeta/GetFiles
// calls (it must hit the files-cache directly).
func TestServeDownload_PinnedCidRepeatHitsFilesCache(t *testing.T) {
	const (
		pinSHA = "e91b8a68fe21818d79f1a1a4f09eaf4db7e810a9"
		owner  = "grpc-ecosystem"
		module = "grpc-gateway"
	)
	pinCID, _ := commitUUID(pinSHA)
	ref := moduleRef{owner: owner, module: module}
	pinFile := content.File{Path: "pin_only.txt", Data: []byte("pinned commit content")}

	base := &mockProvider{
		byCommit: map[string]content.Meta{
			pinSHA: {Commit: pinSHA, DefaultBranch: "main"},
		},
		filesByCommit: map[string][]content.File{
			pinSHA: {pinFile},
		},
	}
	repo := &dualCountingProvider{provider: base}

	h := newTestCommitHandler(repo)
	// commitMap + cidSha are pre-seeded so the first request resolves via the
	// fetch path (infoCache starts empty → files-cache miss → GetMeta/GetFiles
	// → writeback). This models the first-ever pinned-cid Download.
	h.commitMap[pinCID] = ref
	h.cidSha[pinCID] = pinSHA

	mux := http.NewServeMux()
	mux.HandleFunc("/buf.registry.module.v1.DownloadService/", h.ServeDownload)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	doDownload := func(label string) []byte {
		resp, err := http.Post(srv.URL+"/buf.registry.module.v1.DownloadService/Download",
			"application/proto", bytes.NewReader(buildDownloadRequest(pinCID)))
		if err != nil {
			t.Fatalf("%s download failed: %v", label, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want %d; body=%x", label, resp.StatusCode, http.StatusOK, body)
		}
		return body
	}

	firstBody := doDownload("first")
	firstMeta := repo.getMetaCalls
	firstFiles := repo.getFilesCalls
	if firstMeta == 0 {
		t.Fatal("first download should have made at least one GetMeta call (cold cache)")
	}
	if firstFiles == 0 {
		t.Fatal("first download should have made at least one GetFiles call (cold cache)")
	}
	if !bytes.Contains(firstBody, pinFile.Data) {
		t.Errorf("first download did not return the pinned file content; body=%x", firstBody)
	}

	// Second identical request must hit the files-cache: no new upstream calls.
	secondBody := doDownload("second")
	if repo.getMetaCalls != firstMeta {
		t.Errorf("second download made %d new GetMeta call(s); want 0 (files-cache must hit). before=%d after=%d",
			repo.getMetaCalls-firstMeta, firstMeta, repo.getMetaCalls)
	}
	if repo.getFilesCalls != firstFiles {
		t.Errorf("second download made %d new GetFiles call(s); want 0 (files-cache must hit). before=%d after=%d",
			repo.getFilesCalls-firstFiles, firstFiles, repo.getFilesCalls)
	}
	if !bytes.Contains(secondBody, pinFile.Data) {
		t.Errorf("second download did not return the pinned file content; body=%x", secondBody)
	}
}

// buildDownloadRequest builds a protobuf-encoded Download request using a commit ID.
func buildDownloadRequest(commitID string) []byte {
	// ResourceRef: id=1
	var resRef []byte
	resRef = protowire.AppendTag(resRef, 1, protowire.BytesType)
	resRef = protowire.AppendString(resRef, commitID)

	// DownloadRequest_ResourceRef: resource_ref=1
	var wrapper []byte
	wrapper = protowire.AppendTag(wrapper, 1, protowire.BytesType)
	wrapper = append(wrapper, protowire.AppendVarint(nil, uint64(len(resRef)))...)
	wrapper = append(wrapper, resRef...)

	// DownloadRequest: resource_ref=1
	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType)
	req = append(req, protowire.AppendVarint(nil, uint64(len(wrapper)))...)
	req = append(req, wrapper...)
	return req
}

// --- Route registration tests ---

func TestV1RoutesRegistered(t *testing.T) {
	p := &mockProvider{}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	paths := []struct {
		name string
		path string
	}{
		{"CommitService v1", "/buf.registry.module.v1.CommitService/GetCommits"},
		{"CommitService v1beta1", "/buf.registry.module.v1beta1.CommitService/GetCommits"},
		{"GraphService v1", "/buf.registry.module.v1.GraphService/GetGraph"},
		{"GraphService v1beta1", "/buf.registry.module.v1beta1.GraphService/GetGraph"},
		{"DownloadService v1", "/buf.registry.module.v1.DownloadService/Download"},
		{"DownloadService v1beta1", "/buf.registry.module.v1beta1.DownloadService/Download"},
		{"ModuleService v1", "/buf.registry.module.v1.ModuleService/GetModules"},
		{"ModuleService v1beta1", "/buf.registry.module.v1beta1.ModuleService/GetModules"},
	}

	for _, tc := range paths {
		t.Run(tc.name, func(t *testing.T) {
			// POST with empty body — handler should return 400, not fall through
			// to rootHandler (which returns 200 text/plain).
			// Any non-200 or a 200 with application/proto means the route is registered.
			resp, err := http.Post(server.URL+tc.path, "application/proto", bytes.NewReader(nil))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			ct := resp.Header.Get("Content-Type")
			if resp.StatusCode == http.StatusOK && ct == "text/plain; charset=utf-8" {
				t.Errorf("path %s not registered — fell through to rootHandler (200 text/plain)", tc.path)
			}
		})
	}
}

func TestV1RoutesNotReachingRootHandler(t *testing.T) {
	p := &mockProvider{
		meta: content.Meta{
			Commit:        "abc1230000000000000000000000000000000000",
			DefaultBranch: "main",
		},
		files: []content.File{
			{Path: "a.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	v1Paths := []struct {
		name string
		path string
		body []byte
	}{
		{"CommitService v1", "/buf.registry.module.v1.CommitService/GetCommits", buildGetCommitsRequest("owner", "repo")},
		{"GraphService v1", "/buf.registry.module.v1.GraphService/GetGraph", buildV1GetGraphRequest("owner", "repo")},
	}

	for _, tc := range v1Paths {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(server.URL+tc.path, "application/proto", bytes.NewReader(tc.body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			ct := resp.Header.Get("Content-Type")
			if ct != "application/proto" {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected Content-Type application/proto, got %q; body: %s", ct, body)
			}
		})
	}
}

// --- Handler content-type tests ---

func TestCommitServiceV1ReturnsProtobuf(t *testing.T) {
	p := &mockProvider{
		meta: content.Meta{
			Commit:        "deadbeef00000000000000000000000000000000",
			DefaultBranch: "main",
		},
		files: []content.File{
			{Path: "test.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	for _, path := range []string{
		"/buf.registry.module.v1.CommitService/GetCommits",
		"/buf.registry.module.v1beta1.CommitService/GetCommits",
	} {
		t.Run(path, func(t *testing.T) {
			body := buildGetCommitsRequest("owner", "repo")
			resp, err := http.Post(server.URL+path, "application/proto", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if ct := resp.Header.Get("Content-Type"); ct != "application/proto" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/proto")
			}
			if resp.StatusCode != http.StatusOK {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, respBody)
			}

			respBody, _ := io.ReadAll(resp.Body)
			if len(respBody) == 0 {
				t.Fatal("empty response body")
			}
			// Verify it's valid protobuf: should start with field tag
			_, _, n := protowire.ConsumeTag(respBody)
			if n < 0 {
				t.Fatalf("response is not valid protobuf: %x", respBody[:min(len(respBody), 32)])
			}
		})
	}
}

func TestGraphServiceV1ReturnsProtobuf(t *testing.T) {
	p := &mockProvider{
		meta: content.Meta{
			Commit:        "cafe123400000000000000000000000000000000",
			DefaultBranch: "main",
		},
		files: []content.File{
			{Path: "graph.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Pre-populate commit cache via CommitService so GraphService can look it up.
	commitResp, err := http.Post(
		server.URL+"/buf.registry.module.v1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(buildGetCommitsRequest("owner", "repo")),
	)
	if err != nil {
		t.Fatalf("pre-seed CommitService request failed: %v", err)
	}
	io.ReadAll(commitResp.Body)
	commitResp.Body.Close()

	testPaths := []struct {
			path string
			body []byte
		}{
			{"/buf.registry.module.v1.GraphService/GetGraph", buildV1GetGraphRequest("owner", "repo")},
			{"/buf.registry.module.v1beta1.GraphService/GetGraph", buildGetGraphRequest("owner", "repo")},
		}
		for _, tc := range testPaths {
			t.Run(tc.path, func(t *testing.T) {
				resp, err := http.Post(server.URL+tc.path, "application/proto", bytes.NewReader(tc.body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if ct := resp.Header.Get("Content-Type"); ct != "application/proto" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/proto")
			}
			if resp.StatusCode != http.StatusOK {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, respBody)
			}

			respBody, _ := io.ReadAll(resp.Body)
			if len(respBody) == 0 {
				t.Fatal("empty response body")
			}
		})
	}
}

func TestDownloadServiceV1ReturnsProtobuf(t *testing.T) {
	p := &mockProvider{
		meta: content.Meta{
			Commit:        "f00dcafe00000000000000000000000000000000",
			DefaultBranch: "main",
		},
		files: []content.File{
			{Path: "dl.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Pre-populate commit cache.
	commitResp, err := http.Post(
		server.URL+"/buf.registry.module.v1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(buildGetCommitsRequest("owner", "repo")),
	)
	if err != nil {
		t.Fatalf("pre-seed CommitService request failed: %v", err)
	}
	commitBody, _ := io.ReadAll(commitResp.Body)
	commitResp.Body.Close()

	// Extract commit ID from CommitService response: field 1 (repeated), sub-field 1 (string id).
	commitID := extractCommitID(commitBody)
	if commitID == "" {
		t.Fatal("failed to extract commit ID from CommitService response")
	}

	for _, path := range []string{
		"/buf.registry.module.v1.DownloadService/Download",
		"/buf.registry.module.v1beta1.DownloadService/Download",
	} {
		t.Run(path, func(t *testing.T) {
			body := buildDownloadRequest(commitID)
			resp, err := http.Post(server.URL+path, "application/proto", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if ct := resp.Header.Get("Content-Type"); ct != "application/proto" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/proto")
			}
			if resp.StatusCode != http.StatusOK {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, respBody)
			}

			respBody, _ := io.ReadAll(resp.Body)
			if len(respBody) == 0 {
				t.Fatal("empty response body")
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	p := &mockProvider{}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	paths := []string{
		"/buf.registry.module.v1.CommitService/GetCommits",
		"/buf.registry.module.v1.GraphService/GetGraph",
		"/buf.registry.module.v1.DownloadService/Download",
		"/buf.registry.module.v1.ModuleService/GetModules",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(server.URL + path)
			if err != nil {
				t.Fatalf("GET request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("GET %s: status = %d, want %d", path, resp.StatusCode, http.StatusMethodNotAllowed)
			}
		})
	}
}

// extractCommitID extracts the first commit ID from a GetCommits response.
// Response: repeated { id=1, ... }
func extractCommitID(msg []byte) string {
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			break
		}
		msg = msg[n:]
		if num == 1 && typ == protowire.BytesType {
			commit, mLen := protowire.ConsumeBytes(msg)
			msg = msg[mLen:]
			// Inside Commit: field 1 = id (string)
			for len(commit) > 0 {
				cNum, cTyp, cN := protowire.ConsumeTag(commit)
				if cN < 0 {
					break
				}
				commit = commit[cN:]
				if cNum == 1 && cTyp == protowire.BytesType {
					id, _ := protowire.ConsumeBytes(commit)
					return string(id)
				}
				cN = protowire.ConsumeFieldValue(cNum, cTyp, commit)
				if cN < 0 {
					break
				}
				commit = commit[cN:]
			}
		} else {
			n = protowire.ConsumeFieldValue(num, typ, msg)
			if n < 0 {
				break
			}
			msg = msg[n:]
		}
	}
	return ""
}

// --- Error classification tests ---

// TestBadRequest_OnUnknownCommitID pins the "truly unresolvable module"
// behavior of DownloadService/Download. When the proxy has no module
// identity it can fall back to (infoCache empty — no prior GetCommits /
// GetGraph in this session), a foreign commit_id cannot be served and must
// surface as 400 with an explicit message and the commit id logged so
// operators can correlate with prior GetCommits traffic.
//
// Note: under the foreign-commit_id fallback semantics, a request with an
// unknown commit_id but a populated single-entry infoCache is served (see
// TestServeDownload_ForeignCommitID_FallbackKnownModule). This test only
// passes because no CommitService/GetCommits call is made, so infoCache is
// empty and the fallback has nothing to resolve to.
func TestBadRequest_OnUnknownCommitID(t *testing.T) {
	const wantCommitID = "000000000000000000000000deadbeef"

	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	p := &mockProvider{}
	mux := testMuxWithLogger(p, log)
	server := httptest.NewServer(mux)
	defer server.Close()

	for _, path := range []string{
		"/buf.registry.module.v1.DownloadService/Download",
		"/buf.registry.module.v1beta1.DownloadService/Download",
	} {
		t.Run(path, func(t *testing.T) {
			logBuf.Reset()
			body := buildDownloadRequest(wantCommitID)
			resp, err := http.Post(server.URL+path, "application/proto", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, respBody)
			}
			respBody, _ := io.ReadAll(resp.Body)
			if !bytes.Contains(respBody, []byte("unknown commit id")) {
				t.Errorf("body %q does not mention 'unknown commit id'", respBody)
			}
			// Per D-12: the 400 message names the recovery action
			// explicitly so an operator reading the log can see whether the
			// failure is "client forgot GetCommits" or "client is on an
			// older buf.lock and needs to re-resolve".
			if !bytes.Contains(respBody, []byte("re-resolve via buf mod update / buf dep update")) {
				t.Errorf("body %q does not mention 're-resolve via buf mod update / buf dep update' (D-12 message)", respBody)
			}

			logLine := logBuf.String()
			if !strings.Contains(logLine, `"commit_id":"`+wantCommitID+`"`) {
				t.Errorf("log does not contain commit_id=%s\nlog: %s", wantCommitID, logLine)
			}
			if !strings.Contains(logLine, `"server":"buf.example.com"`) {
				t.Errorf("log does not contain server attr\nlog: %s", logLine)
			}
		})
	}
}

// TestServeDownload_ForeignCommitID_FallbackKnownModule pins the foreign
// commit_id fallback: when a client sends a commit_id the proxy never
// minted (e.g. cached from real buf.build in buf.lock) but the module is
// known (a prior CommitService/GetCommits populated infoCache with exactly
// one entry), ServeDownload must serve the content (200) rather than 400.
//
// This reproduces the production symptom from the download-foreign-commit-id
// debug session: the proxy serves googleapis/googleapis, GetGraph minted a
// proxy-local id, but the client's Download carried a cached foreign id.
func TestServeDownload_ForeignCommitID_FallbackKnownModule(t *testing.T) {
	const foreignCommitID = "2d1654c2cc02a6e7f3bbea2d06fc1c59"

	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	p := &mockProvider{
		meta: content.Meta{
			Commit:        "e57bae6efbd075a925978a79bb9b997beb4ecc19",
			DefaultBranch: "main",
		},
		files: []content.File{
			{Path: "google/api/http.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
	}
	mux := testMuxWithLogger(p, log)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Warm infoCache with exactly one module via CommitService/GetCommits.
	// We do NOT use the id it returns; instead we send a foreign id the
	// proxy never minted.
	commitResp, err := http.Post(
		server.URL+"/buf.registry.module.v1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(buildGetCommitsRequest("googleapis", "googleapis")),
	)
	if err != nil {
		t.Fatalf("warm-up GetCommits request failed: %v", err)
	}
	if commitResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(commitResp.Body)
		commitResp.Body.Close()
		t.Fatalf("warm-up GetCommits status = %d, want 200; body: %s", commitResp.StatusCode, b)
	}
	io.ReadAll(commitResp.Body)
	commitResp.Body.Close()

	for _, path := range []string{
		"/buf.registry.module.v1.DownloadService/Download",
		"/buf.registry.module.v1beta1.DownloadService/Download",
	} {
		t.Run(path, func(t *testing.T) {
			logBuf.Reset()
			body := buildDownloadRequest(foreignCommitID)
			resp, err := http.Post(server.URL+path, "application/proto", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200 (foreign id should fall back to known module); body: %s", resp.StatusCode, respBody)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/proto" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/proto")
			}
			respBody, _ := io.ReadAll(resp.Body)
			if len(respBody) == 0 {
				t.Fatal("empty response body")
			}

			// The foreign id was served either via the fallback branch (first
			// time) or via the commitMap alias registered by a prior fallback
			// (subtests share the handler, so the second subtest reuses the
			// alias the first one registered). Either way the request must
			// succeed; the log must reference the foreign id.
			logLine := logBuf.String()
			if !strings.Contains(logLine, `"commit_id":"`+foreignCommitID+`"`) {
				t.Errorf("log does not reference foreign commit_id=%s\nlog: %s", foreignCommitID, logLine)
			}
			fallbackFired := strings.Contains(logLine, `"branch":"foreign_commit_id_fallback"`)
			aliasHit := strings.Contains(logLine, `"ref_found":true`)
			if !fallbackFired && !aliasHit {
				t.Errorf("neither fallback nor alias path served the request\nlog: %s", logLine)
			}
		})
	}
}

// TestServeDownload_ForeignCommitID_FallbackAliasCachesMapping verifies the
// optimization where, after the first foreign-commit_id fallback succeeds,
// the foreign id is registered as an alias in commitMap so a second request
// for the same foreign id is served directly (without re-running the
// fallback). We assert this indirectly: the second request must still
// return 200, and the log must NOT contain a second
// foreign_commit_id_fallback line — proving the alias hit took the direct
// path.
func TestServeDownload_ForeignCommitID_FallbackAliasCachesMapping(t *testing.T) {
	const foreignCommitID = "2d1654c2cc02a6e7f3bbea2d06fc1c59"

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	p := &mockProvider{
		meta: content.Meta{Commit: "e57bae6efbd075a925978a79bb9b997beb4ecc19", DefaultBranch: "main"},
		files: []content.File{
			{Path: "google/api/http.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
	}
	mux := testMuxWithLogger(p, log)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Warm infoCache with exactly one module.
	commitResp, err := http.Post(
		server.URL+"/buf.registry.module.v1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(buildGetCommitsRequest("googleapis", "googleapis")),
	)
	if err != nil {
		t.Fatalf("warm-up GetCommits request failed: %v", err)
	}
	io.ReadAll(commitResp.Body)
	commitResp.Body.Close()

	body := buildDownloadRequest(foreignCommitID)
	path := "/buf.registry.module.v1.DownloadService/Download"

	// First request: fallback fires, registers alias.
	resp1, err := http.Post(server.URL+path, "application/proto", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if resp1.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp1.Body)
		resp1.Body.Close()
		t.Fatalf("first request status = %d, want 200; body: %s", resp1.StatusCode, b)
	}
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Second request for the same foreign id: alias hit, no fallback.
	resp2, err := http.Post(server.URL+path, "application/proto", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("second request status = %d, want 200 (alias should serve); body: %s", resp2.StatusCode, b)
	}
}

// TestServeDownload_ForeignCommitID_TrulyUnknownModule pins the safety net:
// when commit_id is foreign AND the proxy has no resolvable module identity
// (infoCache empty — nothing served in this session), the request must
// still surface as 400. This preserves the original contract for the case
// where the fallback legitimately cannot recover the module.
func TestServeDownload_ForeignCommitID_TrulyUnknownModule(t *testing.T) {
	const foreignCommitID = "2d1654c2cc02a6e7f3bbea2d06fc1c59"

	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	// No GetCommits call is made, so infoCache stays empty.
	p := &mockProvider{
		meta: content.Meta{Commit: "e57bae6efbd075a925978a79bb9b997beb4ecc19", DefaultBranch: "main"},
		files: []content.File{
			{Path: "google/api/http.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
	}
	mux := testMuxWithLogger(p, log)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := buildDownloadRequest(foreignCommitID)
	resp, err := http.Post(
		server.URL+"/buf.registry.module.v1.DownloadService/Download",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 400 (no resolvable module); body: %s", resp.StatusCode, respBody)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(respBody, []byte("unknown commit id")) {
		t.Errorf("body %q does not mention 'unknown commit id'", respBody)
	}
}

// buildGetModulesRequestByID builds a ModuleService/GetModules request that
// references a module by id (ModuleRef.id oneof), the form the buf CLI sends
// once it has cached a module id from a prior GetModules response.
//   - ModuleRef { oneof value { string id = 1; Name name = 2 } }
//   - GetModulesRequest { repeated ModuleRef module_refs = 1 }
func buildGetModulesRequestByID(id string) []byte {
	var ref []byte
	ref = protowire.AppendTag(ref, 1, protowire.BytesType) // id
	ref = protowire.AppendString(ref, id)
	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType) // module_refs
	req = append(req, protowire.AppendVarint(nil, uint64(len(ref)))...)
	req = append(req, ref...)
	return req
}

// TestServeGetModules_ForeignModuleID_FallbackKnownModule pins the
// foreign-module-id fallback: when a client sends a module id the proxy
// does not recognize (an old hashed id from a prior build, or an opaque id
// from real buf.build) but the deployment serves exactly one module,
// ServeGetModules must serve that module (200) rather than 400. Mirrors the
// Download foreign-commit_id fallback. The id below is the legacy
// deterministicID("googleapis/googleapis"); this build emits raw
// "googleapis/googleapis", so the id cannot match by lookup.
func TestServeGetModules_ForeignModuleID_FallbackKnownModule(t *testing.T) {
	const foreignModuleID = "34c82441eab7ea2fea659aae20495091" // legacy hash id

	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	p := &mockProvider{
		repos: []source.Source{
			&mockSource{owner: "googleapis", repoName: "googleapis"},
		},
	}
	mux := testMuxWithLogger(p, log)
	server := httptest.NewServer(mux)
	defer server.Close()

	for _, path := range []string{
		"/buf.registry.module.v1.ModuleService/GetModules",
		"/buf.registry.module.v1beta1.ModuleService/GetModules",
	} {
		t.Run(path, func(t *testing.T) {
			logBuf.Reset()
			body := buildGetModulesRequestByID(foreignModuleID)
			resp, err := http.Post(server.URL+path, "application/proto", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200 (fallback should serve single known module); body: %s", resp.StatusCode, respBody)
			}
			respBody, _ := io.ReadAll(resp.Body)
			if len(respBody) == 0 {
				t.Fatalf("empty response body; expected a Module message")
			}
			logLine := logBuf.String()
			if !strings.Contains(logLine, `"branch":"module_id_fallback"`) {
				t.Errorf("log does not contain module_id_fallback branch\nlog: %s", logLine)
			}
		})
	}
}

// TestServeGetModules_ForeignModuleID_TrulyUnknown pins the safety net: when
// module id is foreign AND the deployment does not serve exactly one module
// (singleModule nil — no repos configured), the request must still surface as
// 400 "no module refs". Preserves the strict contract when the fallback
// cannot recover the module.
func TestServeGetModules_ForeignModuleID_TrulyUnknown(t *testing.T) {
	const foreignModuleID = "34c82441eab7ea2fea659aae20495091"

	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	p := &mockProvider{} // no repos -> singleModule nil
	mux := testMuxWithLogger(p, log)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := buildGetModulesRequestByID(foreignModuleID)
	resp, err := http.Post(
		server.URL+"/buf.registry.module.v1.ModuleService/GetModules",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 400 (no single known module); body: %s", resp.StatusCode, respBody)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(respBody, []byte("no module refs")) {
		t.Errorf("body %q does not mention 'no module refs'", respBody)
	}
}

// TestBadRequest_OnMalformedRepoName pins the connect-go mapping for
// validation errors: a malformed repository name should produce a
// CodeInvalidArgument error, which connect-go surfaces as HTTP 400.
func TestBadRequest_OnMalformedRepoName(t *testing.T) {
	p := &mockProvider{} // mock never gets called — validation fails first
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := v1alpha1connect.NewRepositoryServiceClient(
		server.Client(),
		server.URL,
	)

	resp, err := client.GetRepositoryByFullName(
		context.Background(),
		connect.NewRequest(&registry.GetRepositoryByFullNameRequest{FullName: "nodelimiter"}),
	)
	if err == nil {
		t.Fatalf("expected error, got response: %v", resp)
	}
	var cErr *connect.Error
	if !errors.As(err, &cErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if cErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", cErr.Code(), connect.CodeInvalidArgument)
	}
}

// TestUpstreamError_OnProviderFailure pins the 502 mapping: when the
// back-end provider (artifactory/git/...) fails, the proxy must surface
// that as a 502 Bad Gateway, not 500 Internal Server Error, so clients
// can tell "we are broken" from "they are broken".
func TestUpstreamError_OnProviderFailure(t *testing.T) {
	p := &mockProvider{
		meta: content.Meta{Commit: "deadbeef", DefaultBranch: "main"},
		files: []content.File{
			{Path: "a.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
		err: errUpstream,
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := buildGetCommitsRequest("owner", "repo")
	resp, err := http.Post(
		server.URL+"/buf.registry.module.v1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 502; body: %s", resp.StatusCode, respBody)
	}
	respBody, _ := io.ReadAll(resp.Body)
	// Body should describe which owner/repo failed so an operator can
	// identify the request from client-side logs, but the raw upstream
	// error message must NOT leak into the body — it goes into the
	// structured server log only (as the "upstream_error" attribute).
	if !bytes.Contains(respBody, []byte("owner/repo")) {
		t.Errorf("body %q does not identify failing module", respBody)
	}
	if bytes.Contains(respBody, []byte(errUpstream.Error())) {
		t.Errorf("body %q leaks internal upstream error — should be in logs only", respBody)
	}
}

// TestHandlerError_IncludesServer pins the funnel-level addition of the
// "server" attribute. Every error log emitted via logHandlerError must
// carry the proxy domain so operators can identify which instance
// produced a log line when several are behind a load balancer.
func TestHandlerError_IncludesServer(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	p := &mockProvider{}
	mux := testMuxWithLogger(p, log)
	server := httptest.NewServer(mux)
	defer server.Close()

	// 405 (method not allowed) is the simplest path through the funnel
	// because the handler short-circuits before parsing anything.
	resp, err := http.Get(server.URL + "/buf.registry.module.v1.CommitService/GetCommits")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	logLine := logBuf.String()
	if !strings.Contains(logLine, `"server":"buf.example.com"`) {
		t.Errorf("log does not contain server attr\nlog: %s", logLine)
	}
	if !strings.Contains(logLine, `"error_class":"bad_request"`) {
		t.Errorf("log does not contain error_class=bad_request (405)\nlog: %s", logLine)
	}
}

// TestHandlerError_UpstreamError_UsesModuleKey pins the rename of the
// upstream-error attribute key from "repo" to "module". Breaking change
// for any dashboard that filters on the old key — see the commit message.
func TestHandlerError_UpstreamError_UsesModuleKey(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	p := &mockProvider{
		meta: content.Meta{Commit: "deadbeef", DefaultBranch: "main"},
		files: []content.File{
			{Path: "a.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
		err: errUpstream,
	}
	mux := testMuxWithLogger(p, log)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := buildGetCommitsRequest("acme", "widgets")
	resp, err := http.Post(
		server.URL+"/buf.registry.module.v1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}

	logLine := logBuf.String()
	if !strings.Contains(logLine, `"module":"widgets"`) {
		t.Errorf("log does not contain module=widgets\nlog: %s", logLine)
	}
	if !strings.Contains(logLine, `"owner":"acme"`) {
		t.Errorf("log does not contain owner=acme\nlog: %s", logLine)
	}
	if !strings.Contains(logLine, `"server":"buf.example.com"`) {
		t.Errorf("log does not contain server attr\nlog: %s", logLine)
	}
	// As of the "max debug info" pass, `repo` is also logged alongside `module`
	// for the user-facing per-request line. The canonical key for the
	// module-level lookup is still `module` (verified by the assertions above);
	// `repo` is supplementary.
}

// --- OwnerService tests ---

// buildGetOwnersRequestByID builds a GetOwnersRequest containing one
// OwnerRef with the given id (the buf-style deterministic id form).
func buildGetOwnersRequestByID(id string) []byte {
	// OwnerRef: id = field 1, string
	var ref []byte
	ref = protowire.AppendTag(ref, 1, protowire.BytesType)
	ref = protowire.AppendString(ref, id)
	// GetOwnersRequest: owner_refs = field 1, repeated message
	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType)
	req = append(req, protowire.AppendVarint(nil, uint64(len(ref)))...)
	req = append(req, ref...)
	return req
}

// buildGetOwnersRequestByName builds a GetOwnersRequest containing one
// OwnerRef with the given name (the owner name form like "googleapis").
func buildGetOwnersRequestByName(name string) []byte {
	// OwnerRef: name = field 2, string
	var ref []byte
	ref = protowire.AppendTag(ref, 2, protowire.BytesType)
	ref = protowire.AppendString(ref, name)
	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType)
	req = append(req, protowire.AppendVarint(nil, uint64(len(ref)))...)
	req = append(req, ref...)
	return req
}

// extractOwnerNameFromResponse walks a GetOwnersResponse body and
// returns the Organization.name of the first owner, or "" if no owners
// are present.
//
// GetOwnersResponse: repeated Owner owners = 1
// Owner: oneof { Organization organization = 2 }
// Organization: id=1, name=4
func extractOwnerNameFromResponse(body []byte) string {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return ""
		}
		body = body[n:]
		if num == 1 && typ == protowire.BytesType {
			owner, mLen := protowire.ConsumeBytes(body)
			body = body[mLen:]
			// owner is Owner: skip into the Organization submessage (field 2)
			for len(owner) > 0 {
				oNum, oTyp, oN := protowire.ConsumeTag(owner)
				if oN < 0 {
					return ""
				}
				owner = owner[oN:]
				if oNum == 2 && oTyp == protowire.BytesType {
					org, _ := protowire.ConsumeBytes(owner)
					// Organization: id=1, name=4
					for len(org) > 0 {
						fNum, fTyp, fN := protowire.ConsumeTag(org)
						if fN < 0 {
							return ""
						}
						org = org[fN:]
						if fNum == 4 && fTyp == protowire.BytesType {
							name, _ := protowire.ConsumeBytes(org)
							return string(name)
						}
						fN = protowire.ConsumeFieldValue(fNum, fTyp, org)
						if fN < 0 {
							return ""
						}
						org = org[fN:]
					}
				} else {
					oN = protowire.ConsumeFieldValue(oNum, oTyp, owner)
					if oN < 0 {
						return ""
					}
					owner = owner[oN:]
				}
			}
		} else {
			n = protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return ""
			}
			body = body[n:]
		}
	}
	return ""
}

// TestOwnerServiceV1RouteRegistered pins the bug fix: a POST to the v1
// OwnerService path must NOT fall through to the text/plain rootHandler.
// Returns 200 application/proto (or 400 if the request body is empty),
// never 200 with text/plain content-type.
func TestOwnerServiceV1RouteRegistered(t *testing.T) {
	p := &mockProvider{}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/buf.registry.owner.v1.OwnerService/GetOwners",
		"application/proto",
		bytes.NewReader(nil),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode == http.StatusOK && ct == "text/plain; charset=utf-8" {
		t.Errorf("path /buf.registry.owner.v1.OwnerService/ not registered — fell through to rootHandler (200 text/plain)")
	}
}

// TestOwnerServiceV1ReturnsProtobuf pins the fix at the protocol level:
// when a known owner is requested, the response must be application/proto
// with a non-empty body containing the owner name. This is the exact
// path that was failing in prod with the text/plain content-type error.
func TestOwnerServiceV1ReturnsProtobuf(t *testing.T) {
	p := &mockProvider{
		repos: []source.Source{
			&mockSource{owner: "googleapis", repoName: "googleapis", typ: "github"},
		},
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := buildGetOwnersRequestByID("googleapis")
	resp, err := http.Post(
		server.URL+"/buf.registry.owner.v1.OwnerService/GetOwners",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "application/proto" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/proto")
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, respBody)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if len(respBody) == 0 {
		t.Fatal("empty response body")
	}
	if got := extractOwnerNameFromResponse(respBody); got != "googleapis" {
		t.Errorf("owner name = %q, want %q; body: %x", got, "googleapis", respBody)
	}
}

// TestOwnerServiceV1ByName verifies the handler accepts an OwnerRef with
// a name (the form the buf CLI uses when it knows the owner name but not
// the cached id, e.g. on a fresh machine).
func TestOwnerServiceV1ByName(t *testing.T) {
	p := &mockProvider{
		repos: []source.Source{
			&mockSource{owner: "googleapis", repoName: "googleapis"},
		},
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := buildGetOwnersRequestByName("googleapis")
	resp, err := http.Post(
		server.URL+"/buf.registry.owner.v1.OwnerService/GetOwners",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, respBody)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/proto" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/proto")
	}
	respBody, _ := io.ReadAll(resp.Body)
	if got := extractOwnerNameFromResponse(respBody); got != "googleapis" {
		t.Errorf("owner name = %q, want %q; body: %x", got, "googleapis", respBody)
	}
}

// TestOwnerServiceV1UnknownOwner pins the "we don't fabricate owners we
// don't serve" rule: an owner that is not in the configured repository
// set must NOT be returned, even if its id matches the buf-style format.
// This protects against the proxy accidentally answering for owners it
// has no information about.
func TestOwnerServiceV1UnknownOwner(t *testing.T) {
	p := &mockProvider{
		repos: []source.Source{
			&mockSource{owner: "googleapis", repoName: "googleapis"},
		},
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Build a request for an owner the proxy does NOT serve.
	body := buildGetOwnersRequestByID("not-a-real-owner")
	resp, err := http.Post(
		server.URL+"/buf.registry.owner.v1.OwnerService/GetOwners",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (empty body is correct for unknown owner)", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/proto" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/proto")
	}
	respBody, _ := io.ReadAll(resp.Body)
	if got := extractOwnerNameFromResponse(respBody); got != "" {
		t.Errorf("owner name = %q, want empty (unknown owner must not be returned); body: %x", got, respBody)
	}
}

// TestOwnerServiceV1EmptyBody pins the 400 path: an empty body has no
// owner refs to look up, so the handler must return 400, not 200. The
// 400 body uses Go's standard http.Error text/plain (this is the same
// pattern used by every other bad-request path in this package).
func TestOwnerServiceV1EmptyBody(t *testing.T) {
	p := &mockProvider{}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/buf.registry.owner.v1.OwnerService/GetOwners",
		"application/proto",
		bytes.NewReader(nil),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (empty body has no owner refs)", resp.StatusCode)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(respBody, []byte("no owner refs")) {
		t.Errorf("body %q does not mention 'no owner refs'", respBody)
	}
}

// TestOwnerServiceV1MethodNotAllowed pins the 405 path: GET is rejected
// at the handler entry, never reaching the rootHandler.
func TestOwnerServiceV1MethodNotAllowed(t *testing.T) {
	p := &mockProvider{}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/buf.registry.owner.v1.OwnerService/GetOwners")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

// newTestCommitHandler builds a commitServiceHandler wired to a provider with
// minimal state, for direct unit testing of probeCommitID without the HTTP
// layer. Enhancements are off by default; tests flip the knobs they exercise.
func newTestCommitHandler(repo provider) *commitServiceHandler {
	return &commitServiceHandler{ //nolint:exhaustruct
		api: &api{ //nolint:exhaustruct
			log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
			repo: repo,
		},
		commitMap:       make(map[string]moduleRef),
		infoCache:       make(map[string]commitInfoCache),
		filesMap:        make(map[string][]content.File),
		cidSha:          make(map[string]string),
		missCache:       make(map[string]time.Time),
		probeTimeout:    time.Second,
		probeNegativeTTL: time.Minute,
		probeSem:        make(chan struct{}, maxConcurrentProbes),
	}
}

// TestProbeCommitID_HitResolvesAndCaches verifies that a sha owned by exactly
// one source is resolved by the probe and registered as an alias so later
// requests for it hit the commit map directly.
func TestProbeCommitID_HitResolvesAndCaches(t *testing.T) {
	var calls atomic.Int32
	repo := &mockProvider{repos: []source.Source{
		&mockSource{owner: "cyp", repoName: "cyp-apis", commit: "deadbeef00000000000000000000000000000000", getMetaCalls: &calls},
		&mockSource{owner: "googleapis", repoName: "googleapis", commit: "cafef00d00000000000000000000000000000000", getMetaCalls: &calls},
	}}
	h := newTestCommitHandler(repo)
	h.probeEnabled = true

	ref, ok := h.probeCommitID(context.Background(), "deadbeef00000000000000000000000000000000")
	if !ok || ref == nil || ref.owner != "cyp" || ref.module != "cyp-apis" {
		t.Fatalf("probe should resolve deadbeef... -> cyp/cyp-apis; ok=%v ref=%+v", ok, ref)
	}

	h.commitMu.RLock()
	_, present := h.commitMap["deadbeef00000000000000000000000000000000"]
	h.commitMu.RUnlock()
	if !present {
		t.Error("probe hit did not register deadbeef... as a commitMap alias")
	}
}

// TestProbeCommitID_MissNegativeCaches verifies a bogus sha is reported as a
// miss, recorded in the negative cache, and that a retry within TTL does NOT
// re-probe (no extra upstream GetMeta calls).
func TestProbeCommitID_MissNegativeCaches(t *testing.T) {
	var calls atomic.Int32
	// Use a 40-char SHA that does NOT match the source's commit. The
	// mockSource's GetMeta returns the error "not found in cyp/cyp-apis"
	// for any commit arg that is neither s.commit nor a prefix of
	// s.commit, so the probe sees a real fan-out miss.
	repo := &mockProvider{repos: []source.Source{
		&mockSource{owner: "cyp", repoName: "cyp-apis", commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", getMetaCalls: &calls},
	}}
	h := newTestCommitHandler(repo)
	h.probeEnabled = true

	ref, ok := h.probeCommitID(context.Background(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if ok || ref != nil {
		t.Fatalf("bogus sha should miss; got ok=%v ref=%+v", ok, ref)
	}
	if first := calls.Load(); first != 1 {
		t.Fatalf("probe should issue exactly 1 GetMeta call on miss, got %d", first)
	}
	if !h.missCached("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Fatal("bogus sha not negative-cached after miss")
	}

	// Second call within TTL: must be served from the negative cache.
	ref2, ok2 := h.probeCommitID(context.Background(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if ok2 || ref2 != nil {
		t.Fatalf("negative-cached sha should still miss; got ok=%v ref=%+v", ok2, ref2)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("negative-cache hit must not re-probe; GetMeta calls = %d, want 1", got)
	}
}

// TestProbeCommitID_TransientNotNegativeCached verifies Fix 3: when every
// source returns a transient error (timeout/cancel/network), the sha is NOT
// negative-cached, so the next request retries instead of being locked out
// for ProbeNegativeTTL.
func TestProbeCommitID_TransientNotNegativeCached(t *testing.T) {
	var calls atomic.Int32
	repo := &mockProvider{repos: []source.Source{
		// Even the owning source errors transiently (simulates upstream outage).
		&mockSource{owner: "cyp", repoName: "cyp-apis", commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", getMetaErr: context.DeadlineExceeded, getMetaCalls: &calls},
	}}
	h := newTestCommitHandler(repo)
	h.probeEnabled = true

	if ref, ok := h.probeCommitID(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); ok || ref != nil {
		t.Fatalf("transient failure should be a miss; got ok=%v ref=%+v", ok, ref)
	}
	if h.missCached("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatal("transient miss must not be negative-cached")
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 GetMeta call, got %d", calls.Load())
	}
	// Retry: not negative-cached, so it probes again.
	if _, ok := h.probeCommitID(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); ok {
		t.Fatal("retry should still miss")
	}
	if calls.Load() != 2 {
		t.Errorf("expected re-probe after transient (2 calls), got %d", calls.Load())
	}
}

// TestServeDownload_NonHeadShaServesThatCommit verifies Fix 1: when a
// Download resolves a non-HEAD sha (here via the probe), ServeDownload fetches
// THAT sha's content, not HEAD's. Pins the headline correctness fix — without
// it, the miss-branch fetched HEAD ("") and the client got the wrong commit.
func TestServeDownload_NonHeadShaServesThatCommit(t *testing.T) {
	const (
		headSha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		oldSha  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	p := &mockProvider{
		meta: content.Meta{Commit: headSha, DefaultBranch: "main"},
		byCommit: map[string]content.Meta{
			headSha: {Commit: headSha},
			oldSha:  {Commit: oldSha},
		},
		filesByCommit: map[string][]content.File{
			headSha: {{Path: "head.proto", Data: []byte("HEAD-CONTENT")}},
			oldSha:  {{Path: "old.proto", Data: []byte("OLD-CONTENT")}},
		},
		repos: []source.Source{
			// A source that owns oldSha, so the probe resolves it.
			&mockSource{owner: "cyp", repoName: "cyp-apis", commit: oldSha},
		},
	}
	cfg := CommitResolution{
		ProbeEnabled:     true,
		ProbeTimeout:     time.Second,
		ProbeNegativeTTL: time.Minute,
	}
	mux := testMuxWithConfig(p, log, cfg)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := buildDownloadRequest(oldSha)
	resp, err := http.Post(
		server.URL+"/buf.registry.module.v1.DownloadService/Download",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body: %s; log: %s", resp.StatusCode, b, logBuf.String())
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(respBody, []byte("OLD-CONTENT")) {
		t.Errorf("response should serve oldSha's content (OLD-CONTENT); got %x", respBody)
	}
	if bytes.Contains(respBody, []byte("HEAD-CONTENT")) {
		t.Errorf("response must NOT serve HEAD's content; got %x", respBody)
	}
}

// TestServeDownload_AfterRestart_ProbeResolvesUUID pins the post-restart
// probe path: with an empty commitMap (no in-session GetCommits has run),
// a Download request with a buf-issued 32-char UUID is resolved by
// probeCommitID via the commitUUIDInverse + prefix-match path. The
// probe derives the 28-char SHA prefix from the UUID, asks each
// configured source for the prefix, validates the returned SHA starts
// with the prefix, and registers the alias. This is the regression
// guard for the prewarm removal: without the inverse, the probe
// would 400 the request and a process restart would be observable
// to the client as a foreign-id failure.
func TestServeDownload_AfterRestart_ProbeResolvesUUID(t *testing.T) {
	const headSha = "81353411f7b010d5b9ebeb1899066aac18a36701"
	uuid, err := commitUUID(headSha)
	if err != nil {
		t.Fatalf("commitUUID(%q) unexpected error: %v", headSha, err)
	}
	prefix, err := commitUUIDInverse(uuid)
	if err != nil {
		t.Fatalf("commitUUIDInverse(%q) unexpected error: %v", uuid, err)
	}
	t.Logf("fixture: headSha=%s uuid=%s prefix=%s", headSha, uuid, prefix)

	var sourceCalls atomic.Int32
	// mockProvider is the connect-package provider. It serves the same
	// head SHA for any commit arg the post-probe ServeDownload might
	// try (the UUID, the prefix, the resolved SHA). Without byCommit
	// set, it returns m.meta regardless of arg, mirroring the
	// pre-restart production behavior of "HEAD lookup always returns
	// the resolved commit".
	repo := &mockProvider{
		meta: content.Meta{Commit: headSha, DefaultBranch: "main"},
		files: []content.File{
			{Path: "test.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
		repos: []source.Source{
			&mockSource{
				owner: "cyp", repoName: "cyp-apis",
				commit:      headSha,
				getMetaCalls: &sourceCalls,
			},
		},
	}
	h := newTestCommitHandler(repo)
	h.probeEnabled = true
	h.probeNegativeTTL = 5 * time.Minute
	h.probeTimeout = 2 * time.Second
	// commitMap is intentionally empty (post-restart state — no in-session GetCommits).

	mux := http.NewServeMux()
	mux.HandleFunc("/buf.registry.module.v1.DownloadService/", h.ServeDownload)
	mux.HandleFunc("/buf.registry.module.v1beta1.DownloadService/", h.ServeDownload)
	server := httptest.NewServer(mux)
	defer server.Close()

	for _, path := range []string{
		"/buf.registry.module.v1.DownloadService/Download",
		"/buf.registry.module.v1beta1.DownloadService/Download",
	} {
		t.Run(path, func(t *testing.T) {
			body := buildDownloadRequest(uuid)
			resp, err := http.Post(server.URL+path, "application/proto", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200 (post-restart probe should resolve via inverse); body: %s", resp.StatusCode, b)
			}
			respBody, _ := io.ReadAll(resp.Body)
			if len(respBody) == 0 {
				t.Fatal("empty response body")
			}
		})
	}
	if sourceCalls.Load() < 1 {
		t.Errorf("expected at least 1 source GetMeta call (probe fan-out), got %d", sourceCalls.Load())
	}
}

// TestServeDownload_AfterRestart_ProbeMissesOnUnknownUUID pins the
// safety net: when the 32-char UUID's first 14 bytes do NOT correspond
// to any configured source's SHA, the probe must miss (no false
// positive) and the request must 400. This is the prefix-match
// validation closing the review.md finding #6: a wrong-source match
// would silently alias a real commit id to a wrong module and serve
// wrong content. Failing closed is the right tradeoff.
func TestServeDownload_AfterRestart_ProbeMissesOnUnknownUUID(t *testing.T) {
	// A 32-char UUID whose first 14 bytes are all-0xff — no configured
	// source's HEAD SHA starts with "ff" * 14. The mockSource owns
	// "0000000000000000000000000000000000000000" (starts with "00" * 14),
	// so the prefix-match validation rejects the probe hit.
	const (
		unknownUUID = "ffffffffffffffffffffffffffffffff" // first 14 bytes = "ff" * 14
		headSha     = "0000000000000000000000000000000000000000"
	)
	_ = unknownUUID // referenced below; declared here for clarity

	repo := &mockProvider{
		repos: []source.Source{
			&mockSource{owner: "cyp", repoName: "cyp-apis", commit: headSha},
		},
	}
	h := newTestCommitHandler(repo)
	h.probeEnabled = true
	h.probeNegativeTTL = 5 * time.Minute
	h.probeTimeout = 2 * time.Second

	mux := http.NewServeMux()
	mux.HandleFunc("/buf.registry.module.v1beta1.DownloadService/", h.ServeDownload)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := buildDownloadRequest(unknownUUID)
	resp, err := http.Post(
		server.URL+"/buf.registry.module.v1beta1.DownloadService/Download",
		"application/proto",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 400 (unknown UUID, probe must miss); body: %s", resp.StatusCode, b)
	}
}
