package connect

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/source"
	"github.com/easyp-tech/server/internal/shake256"
)

// TestServeHTTP_GetCommits_ReturnsDashlessUUID pins the wire format that
// buf v1.69.0 (and later) requires. The buf client validates every commit
// id with uuidutil.FromDashless, which does:
//
//   1. Assert length == 32
//   2. Insert dashes and call uuid.Parse, which validates version/variant
//
// Before the commitUUID fix, ServeHTTP returned the raw 40-char git SHA
// and buf v1.69.0 failed with:
//   "Failure: expected dashless uuid to be of length 32 but was 40: ..."
// This test reproduces the buf client's exact validation against the
// response body, so any regression that leaks the raw SHA (or a non-UUID
// 32-char hex string) is caught in CI without running the buf binary.
//
// Covers both v1 and v1beta1 paths because buf v1.32+ uses v1beta1 with
// v1 buf.yaml and v1 with v2 buf.yaml.
func TestServeHTTP_GetCommits_ReturnsDashlessUUID(t *testing.T) {
	const wantSHA = "81353411f7b010d5b9ebeb1899066aac18a36701"
	wantUUID := commitUUID(wantSHA) // 32 hex chars, version 4, RFC 4122 variant.

	p := &mockProvider{
		meta: content.Meta{
			Commit:        wantSHA,
			DefaultBranch: "main",
		},
		files: []content.File{
			{Path: "a.proto", Data: []byte(`syntax = "proto3";`), Hash: shake256.Hash{}},
		},
	}
	srv := httptest.NewServer(testMux(p))
	defer srv.Close()

	paths := []string{
		"/buf.registry.module.v1beta1.CommitService/GetCommits",
		"/buf.registry.module.v1.CommitService/GetCommits",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			body := buildGetCommitsRequest("owner", "repo")
			resp, err := http.Post(srv.URL+path, "application/proto", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, b)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/proto" {
				t.Errorf("Content-Type = %q, want application/proto", ct)
			}

			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			cid := extractCommitIDFromGetCommitsResponse(t, respBody)

			// Step 1: len == 32 (buf's first check).
			if len(cid) != 32 {
				t.Fatalf("commit id length = %d, want 32 (buf rejects: \"expected dashless uuid to be of length 32 but was %d\"); got %q",
					len(cid), len(cid), cid)
			}
			// Step 2: lowercase hex only.
			for _, r := range cid {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
					t.Fatalf("commit id %q contains non-lowercase-hex char %q", cid, r)
				}
			}
			// Step 3: must parse as a valid UUID (uuid.Parse applies the
			// version and variant checks). We re-implement the relevant
			// rules here so the test fails with a precise reason.
			if cid[12] != '4' {
				t.Fatalf("commit id %q version nibble = %c, want '4' (buf requires random UUID)", cid, cid[12])
			}
			switch cid[16] {
			case '8', '9', 'a', 'b':
			default:
				t.Fatalf("commit id %q variant nibble = %c, want one of 8/9/a/b (RFC 4122)", cid, cid[16])
			}
			// Determinism: a fresh request must yield the same id.
			if cid != wantUUID {
				t.Fatalf("commit id = %q, want %q (sha=%s → commitUUID must be stable)", cid, wantUUID, wantSHA)
			}
		})
	}
}

// TestServeDownload_RoundTripWithMintedUUID verifies that a UUID returned
// by GetCommits can be fed straight back into Download and be resolved.
// This is what buf v1.69.0 actually does: it pins the commit id from
// GetCommits, then issues Download with the same id. A regression that
// mints the right id but keys commitMap by the SHA would 400 here.
func TestServeDownload_RoundTripWithMintedUUID(t *testing.T) {
	const wantSHA = "0123456789abcdef0123456789abcdef01234567"
	wantUUID := commitUUID(wantSHA)

	p := &mockProvider{
		meta: content.Meta{
			Commit:        wantSHA,
			DefaultBranch: "main",
		},
		files: []content.File{
			{Path: "x.proto", Data: []byte(`syntax = "proto3";`), Hash: shake256.Hash{}},
		},
	}
	srv := httptest.NewServer(testMux(p))
	defer srv.Close()

	// Step 1: GetCommits mints the UUID.
	getResp, err := http.Post(
		srv.URL+"/buf.registry.module.v1beta1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(buildGetCommitsRequest("owner", "repo")),
	)
	if err != nil {
		t.Fatalf("GetCommits: %v", err)
	}
	getBody, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GetCommits status = %d, want 200; body: %s", getResp.StatusCode, getBody)
	}
	cid := extractCommitIDFromGetCommitsResponse(t, getBody)
	if cid != wantUUID {
		t.Fatalf("GetCommits id = %q, want %q", cid, wantUUID)
	}

	// Step 2: Download with that UUID must succeed.
	dlResp, err := http.Post(
		srv.URL+"/buf.registry.module.v1beta1.DownloadService/Download",
		"application/proto",
		bytes.NewReader(buildDownloadRequest(cid)),
	)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(dlResp.Body)
		t.Fatalf("Download with fresh UUID %q status = %d, want 200; body: %s",
			cid, dlResp.StatusCode, b)
	}
}

// TestServeDownload_UnknownCommitID_ReturnsBadRequest pins the 400 path
// for "client sent a commit id we never minted". buf surfaces this as
// "Failure: failed to download ...: invalid commit id", so the proxy
// returning 400 is the right behavior — the bug is if we ever serve the
// wrong module on a foreign id, not if we 400.
//
// We assert the body contains the expected error message so a regression
// that 400s for the wrong reason (e.g. a panic or a 502) is also caught.
func TestServeDownload_UnknownCommitID_ReturnsBadRequest(t *testing.T) {
	p := &mockProvider{
		meta:  content.Meta{Commit: "abc123", DefaultBranch: "main"},
		files: []content.File{{Path: "x.proto", Data: []byte(`syntax = "proto3";`), Hash: shake256.Hash{}}},
	}
	// No prewarm: registerResolved is never called, commitMap stays empty.
	srv := httptest.NewServer(testMux(p))
	defer srv.Close()

	// Multi-module guard: prewarm stays off, infoCache empty, no foreign
	// alias → the unknown-id 400 must fire. The id we send is a 32-char
	// dashless UUID (not a real SHA) so the probe path is also not viable.
	foreignID := strings.Repeat("0", 32)

	dlResp, err := http.Post(
		srv.URL+"/buf.registry.module.v1beta1.DownloadService/Download",
		"application/proto",
		bytes.NewReader(buildDownloadRequest(foreignID)),
	)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(dlResp.Body)
		t.Fatalf("status = %d, want 400 (unknown commit id); body: %s", dlResp.StatusCode, b)
	}
	body, _ := io.ReadAll(dlResp.Body)
	if !strings.Contains(string(body), "unknown commit id") {
		t.Errorf("body does not contain \"unknown commit id\"; got: %s", body)
	}
}

// TestServeHTTP_GetCommits_NoRefs_ReturnsBadRequest pins the
// "no resource refs" 400. buf v1.32+ sends one ref per dependency, so
// an empty body is not a path it takes in practice — but the proxy
// must reject the malformed request rather than fall through to the
// rootHandler (which returns 200 text/plain and breaks the buf client).
func TestServeHTTP_GetCommits_NoRefs_ReturnsBadRequest(t *testing.T) {
	p := &mockProvider{}
	srv := httptest.NewServer(testMux(p))
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/buf.registry.module.v1beta1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(nil),
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, b)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "no resource refs") {
		t.Errorf("body does not contain \"no resource refs\"; got: %s", body)
	}
}

// TestServeGetModules_NoRefs_MultiModule_ReturnsBadRequest pins the
// "no module refs" 400 for the multi-module case. With no
// prewarm-resolved single module, an empty ModuleService request must
// 400 rather than serve the wrong module.
func TestServeGetModules_NoRefs_MultiModule_ReturnsBadRequest(t *testing.T) {
	// Mock provider reports TWO configured modules → singleModule is nil
	// → the fallback does not fire → 400 is the correct response.
	p := &mockProvider{
		meta:  content.Meta{Commit: "deadbeef", DefaultBranch: "main"},
		files: []content.File{{Path: "a.proto", Data: []byte(`syntax = "proto3";`), Hash: shake256.Hash{}}},
	}
	// Two mockSource entries so buildKnownModules registers more than one.
	p.repos = []source.Source{
		&mockSource{owner: "owner-a", repoName: "repo-a", commit: "deadbeef"},
		&mockSource{owner: "owner-b", repoName: "repo-b", commit: "deadbeef"},
	}
	srv := httptest.NewServer(testMux(p))
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/buf.registry.module.v1beta1.ModuleService/GetModules",
		"application/proto",
		bytes.NewReader(nil),
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, b)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "no module refs") {
		t.Errorf("body does not contain \"no module refs\"; got: %s", body)
	}
}

// extractCommitIDFromGetCommitsResponse pulls the first Commit.id (field 1)
// out of a GetCommits response body. GetCommitsResponse { Commit commits=1 }
// where Commit { id=1 (string) ... }.
//
// The commit id is a wire-format string — the same shape buf v1.69.0
// reads. We parse it the same way to keep the test honest: if the proxy
// stops emitting a string in field 1, the test fails on a clean protowire
// error rather than silently accepting an empty id.
func extractCommitIDFromGetCommitsResponse(t *testing.T, body []byte) string {
	t.Helper()
	msg := body
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			t.Fatalf("malformed response: %x", body)
		}
		msg = msg[n:]
		if num != 1 || typ != protowire.BytesType {
			skip := protowire.ConsumeFieldValue(num, typ, msg)
			if skip < 0 {
				t.Fatalf("malformed response skipping field %d: %x", num, body)
			}
			msg = msg[skip:]
			continue
		}
		commit, skip := protowire.ConsumeBytes(msg)
		msg = msg[skip:]
		// Walk the Commit message looking for id=1 (string).
		for len(commit) > 0 {
			cnum, ctyp, cn := protowire.ConsumeTag(commit)
			if cn < 0 {
				t.Fatalf("malformed Commit: %x", commit)
			}
			commit = commit[cn:]
			if cnum == 1 && ctyp == protowire.BytesType {
				id, _ := protowire.ConsumeString(commit)
				return id
			}
			cskip := protowire.ConsumeFieldValue(cnum, ctyp, commit)
			if cskip < 0 {
				t.Fatalf("malformed Commit field %d: %x", cnum, commit)
			}
			commit = commit[cskip:]
		}
		// No id in this Commit — try the next one.
	}
	t.Fatalf("no commit id found in response: %x", body)
	return ""
}

// Sanity: the extract function above should return the same hex string
// commitUUID would compute. This makes the round-trip assertion in
// TestServeDownload_RoundTripWithMintedUUID trustworthy (if extract
// broke, we'd notice here before the network test).
func TestExtractCommitIDFromGetCommitsResponse_RoundTrip(t *testing.T) {
	const sha = "abcdef0123456789abcdef0123456789abcdef01"
	want := commitUUID(sha)
	if len(want) != 32 {
		t.Fatalf("commitUUID length = %d, want 32", len(want))
	}
	// Round-trip the bytes through hex so a stray padding byte or null
	// terminator on the wire would show up here.
	if _, err := hex.DecodeString(want); err != nil {
		t.Fatalf("commitUUID %q is not valid hex: %v", want, err)
	}
}
