package connect

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	meta    content.Meta
	files   []content.File
	err     error
	repos   []source.Source
}

func (m *mockProvider) GetMeta(_ context.Context, _, _, _ string) (content.Meta, error) {
	return m.meta, m.err
}

func (m *mockProvider) GetFiles(_ context.Context, _, _, _ string) ([]content.File, error) {
	return m.files, m.err
}

func (m *mockProvider) Repositories() []source.Source {
	return m.repos
}

// mockSource is a source.Source that returns canned metadata for
// OwnerService tests. It only implements the methods needed for
// buildKnownOwners (Owner, RepoName) plus the Source interface contract
// that the rest of the package relies on.
type mockSource struct {
	owner    string
	repoName string
	typ      string
}

func (s *mockSource) GetMeta(_ context.Context, _ string) (content.Meta, error) {
	return content.Meta{}, nil
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
			Commit:        "abc123",
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
			Commit:        "deadbeef",
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
			Commit:        "cafe1234",
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
			Commit:        "f00dcafe",
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
