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

// TestGetMeta_DefaultBranchName_bitbucket is the regression guard for
// the v1.30.1 v1alpha1 case caught by Phase 19 e2e tests. v1.30.1
// sends ModuleReference.reference="main" (the default label name) via
// v1alpha1.ResolveService/GetModulePins, and the proxy at
// modulepins.go:46 passes "main" to getMeta. The pre-fix provider code
// routed "main" through c.getCommit (Bitbucket's /commits/main
// endpoint), which expects a SHA and rejects branch names with 404.
// After the Phase 20-02 default-branch carve-out, "main" is recognized
// as a synonym for "the default branch" and the HEAD already in
// meta.Commit (set by getRepo from repo.LatestCommit) is returned
// without a second round-trip to /commits/main.
//
// This test pins the contract: when commit equals meta.DefaultBranch
// OR is a well-known default name ("main", "master", "develop",
// "trunk"), getMeta returns HEAD and does NOT call /commits/{commit}.
// A regression that re-routes the default branch name through the
// ref-resolution API is caught by the explicit t.Errorf on the
// /commits/main path (any call to /commits/main fails the test).
func TestGetMeta_DefaultBranchName_bitbucket(t *testing.T) {
	const headCommit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	commitsHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == testBasePath+"/branches/default":
			// HEAD lookup — getRepo calls this. Returns the default
			// branch ("main") with its latest commit.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"` + headCommit + `","displayId":"main","type":"BRANCH","latestCommit":"` + headCommit + `","latestChangeset":"` + headCommit + `","isDefault":true}`))
		case r.URL.Path == testBasePath+"/commits/main":
			// The pre-fix bug path: the carve-out must prevent this
			// call. If we see it, the test fails.
			commitsHits++
			t.Errorf("unexpected /commits/main call: default-branch carve-out should not call /commits/{commit}")
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected upstream call to %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := connect(nil, "", "", srv.URL+testBasePath)
	meta, err := c.getMeta(context.Background(), "main")
	if err != nil {
		t.Fatalf("getMeta(\"main\") unexpected error: %v", err)
	}
	if commitsHits != 0 {
		t.Errorf("expected 0 /commits/ calls for default-branch name, got %d", commitsHits)
	}
	if meta.Commit != headCommit {
		t.Fatalf("meta.Commit = %q, want %q (HEAD from getRepo via /branches/default)", meta.Commit, headCommit)
	}
}

// TestGetMeta_ConventionalDefaultName_bitbucket is the regression
// guard for the v1.30.1 v1alpha1 case where the buf CLI sends the buf
// default label name (e.g., "main") as the reference, even when it
// does NOT match the repo's actual default branch name. The live case
// caught by Phase 19 e2e tests: googleapis/googleapis has default
// branch "master" but the buf CLI v1.30.1 sends reference="main" (the
// buf default label name). The pre-fix provider code routed "main"
// through c.getCommit, which Bitbucket's /commits/main endpoint
// rejected with 404 because "main" is not a valid SHA. The
// Phase 20-02 fix extends the carve-out to a small set of
// conventional default names ("main", "master", "develop", "trunk")
// via isConventionalDefaultName, so "main" is recognized as a synonym
// for "the default branch" and the HEAD from getRepo is returned
// without a second round-trip.
func TestGetMeta_ConventionalDefaultName_bitbucket(t *testing.T) {
	const headCommit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	commitsHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == testBasePath+"/branches/default":
			// HEAD lookup — getRepo calls this. Returns the default
			// branch ("master") with its latest commit. Note: the buf
			// CLI sends "main" but the repo's default is "master", so
			// the proxy must recognize "main" as a conventional
			// default name and use the default branch's HEAD.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"` + headCommit + `","displayId":"master","type":"BRANCH","latestCommit":"` + headCommit + `","latestChangeset":"` + headCommit + `","isDefault":true}`))
		case r.URL.Path == testBasePath+"/commits/main":
			// The pre-fix bug path: the carve-out must prevent this
			// call. If we see it, the test fails.
			commitsHits++
			t.Errorf("unexpected /commits/main call: conventional-default-name carve-out should not call /commits/{commit}")
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected upstream call to %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := connect(nil, "", "", srv.URL+testBasePath)
	// Repo default is "master", but the client sends "main" (the buf
	// default label name). The carve-out must match the conventional
	// name and return HEAD without a second round-trip.
	meta, err := c.getMeta(context.Background(), "main")
	if err != nil {
		t.Fatalf("getMeta(\"main\") unexpected error: %v", err)
	}
	if commitsHits != 0 {
		t.Errorf("expected 0 /commits/ calls for conventional default name, got %d", commitsHits)
	}
	if meta.Commit != headCommit {
		t.Fatalf("meta.Commit = %q, want %q (HEAD from getRepo via /branches/default)", meta.Commit, headCommit)
	}
}
