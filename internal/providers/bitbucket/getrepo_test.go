package bitbucket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testBasePath = "/rest/api/1.0/projects/CYP/repos/cyp-net-listeners"

// TestGetMeta_Empty pins the HEAD path: when no ref is supplied, getMeta
// returns the default branch's HEAD commit as resolved by /branches/default.
// This is the regression guard for the empty-input path so a future change
// to the ref-resolution branch cannot accidentally break the no-ref case.
func TestGetMeta_Empty(t *testing.T) {
	const headCommit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /branches/default is what searchRepo calls. The basePath is
		// already prepended to the URL by httpClient.get.
		if r.URL.Path != testBasePath+"/branches/default" {
			t.Errorf("unexpected path %q on HEAD call", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + headCommit + `","displayId":"main","type":"BRANCH","latestCommit":"` + headCommit + `","latestChangeset":"` + headCommit + `","isDefault":true}`))
	}))
	defer srv.Close()

	c := connect(nil, "", "", srv.URL+testBasePath)
	meta, err := c.getMeta(context.Background(), "")
	if err != nil {
		t.Fatalf("getMeta(empty) unexpected error: %v", err)
	}
	if !hit {
		t.Fatal("HEAD endpoint was not called")
	}
	if meta.Commit != headCommit {
		t.Fatalf("meta.Commit = %q, want %q (HEAD from /branches/default)", meta.Commit, headCommit)
	}
}

// TestGetMeta_RawSHA_40 pins the SHA fast path: a 40-char lowercase hex
// input is returned verbatim in meta.Commit without an extra round-trip
// to /commits/{sha}. This guards against a future refactor that
// accidentally routes SHAs through the ref-resolution API.
func TestGetMeta_RawSHA_40(t *testing.T) {
	const sha = "81353411f7b010d5b9ebeb1899066aac18a36701"

	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		if r.URL.Path != testBasePath+"/branches/default" {
			t.Errorf("SHA fast path should ONLY call /branches/default; got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"head","displayId":"main","type":"BRANCH","latestCommit":"head","latestChangeset":"head","isDefault":true}`))
	}))
	defer srv.Close()

	c := connect(nil, "", "", srv.URL+testBasePath)
	meta, err := c.getMeta(context.Background(), sha)
	if err != nil {
		t.Fatalf("getMeta(40-char sha) unexpected error: %v", err)
	}
	if hit != 1 {
		t.Errorf("expected exactly 1 upstream call (HEAD), got %d", hit)
	}
	if meta.Commit != sha {
		t.Fatalf("meta.Commit = %q, want %q (SHA fast path)", meta.Commit, sha)
	}
}

// TestGetMeta_RawSHA_64 pins the SHA-256 fast path: a 64-char lowercase
// hex input (Bitbucket Server on SHA-256-enabled repos) is returned
// verbatim in meta.Commit. Mirrors the 40-char case; the gate must accept
// both shapes.
func TestGetMeta_RawSHA_64(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		if r.URL.Path != testBasePath+"/branches/default" {
			t.Errorf("SHA-256 fast path should ONLY call /branches/default; got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"head","displayId":"main","type":"BRANCH","latestCommit":"head","latestChangeset":"head","isDefault":true}`))
	}))
	defer srv.Close()

	c := connect(nil, "", "", srv.URL+testBasePath)
	meta, err := c.getMeta(context.Background(), sha)
	if err != nil {
		t.Fatalf("getMeta(64-char sha) unexpected error: %v", err)
	}
	if hit != 1 {
		t.Errorf("expected exactly 1 upstream call (HEAD), got %d", hit)
	}
	if meta.Commit != sha {
		t.Fatalf("meta.Commit = %q, want %q (SHA-256 fast path)", meta.Commit, sha)
	}
}

// TestGetMeta_ResolvesRef pins the ref-resolution path: a non-SHA, non-
// empty input like "main/v2" triggers a GET to /commits/main/v2, the
// response's id is parsed, and meta.Commit is stamped with the resolved
// SHA. This is the headline fix for the `buf.yaml: main/v2` case that
// previously collapsed to HEAD.
func TestGetMeta_ResolvesRef(t *testing.T) {
	const ref = "main/v2"
	const resolved = "abc1230000000000000000000000000000000000"

	commitsHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == testBasePath+"/branches/default":
			// HEAD lookup
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"head","displayId":"main","type":"BRANCH","latestCommit":"head","latestChangeset":"head","isDefault":true}`))
		case r.URL.Path == testBasePath+"/commits/"+ref:
			commitsHits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"` + resolved + `","displayId":"abc1230"}`))
		default:
			t.Errorf("unexpected upstream call to %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := connect(nil, "", "", srv.URL+testBasePath)
	meta, err := c.getMeta(context.Background(), ref)
	if err != nil {
		t.Fatalf("getMeta(%q) unexpected error: %v", ref, err)
	}
	if commitsHits != 1 {
		t.Errorf("expected exactly 1 /commits/ call, got %d", commitsHits)
	}
	if meta.Commit != resolved {
		t.Fatalf("meta.Commit = %q, want %q (resolved SHA from /commits/%s)", meta.Commit, resolved, ref)
	}
}

// _ = strings.HasPrefix keeps the import stable if a future test needs it.
var _ = strings.HasPrefix
