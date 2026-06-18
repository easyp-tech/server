package connect

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	v1alpha1connect "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect"
	"github.com/easyp-tech/server/internal/providers/content"
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
}

func (m *mockProvider) GetMeta(_ context.Context, _, _, _ string) (content.Meta, error) {
	return m.meta, m.err
}

func (m *mockProvider) GetFiles(_ context.Context, _, _, _ string) ([]content.File, error) {
	return m.files, m.err
}

func testMux(p provider) *http.ServeMux {
	return New(slog.Default(), p, "buf.example.com")
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

// TestBadRequest_OnUnknownCommitID pins the stateful "commit id not seen"
// behavior of DownloadService/Download. The Download handler requires the
// caller to have already called CommitService/GetCommits (which populates
// the commitMap). An unknown id is a client protocol error, not an upstream
// failure — surface it as 400 with an explicit message.
func TestBadRequest_OnUnknownCommitID(t *testing.T) {
	p := &mockProvider{}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	for _, path := range []string{
		"/buf.registry.module.v1.DownloadService/Download",
		"/buf.registry.module.v1beta1.DownloadService/Download",
	} {
		t.Run(path, func(t *testing.T) {
			body := buildDownloadRequest("000000000000000000000000deadbeef")
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
		})
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
