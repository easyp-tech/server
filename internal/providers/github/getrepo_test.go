package github

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-github/v59/github"
)

// TestGetMeta_Empty pins the HEAD path: when no ref is supplied, GetMeta
// returns the default branch's HEAD commit. This is the regression
// guard for the empty-input path so a future change to the ref-resolution
// branch cannot accidentally break the no-ref case.
func TestGetMeta_Empty(t *testing.T) {
	const headCommit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	mock := newMockRepos().
		WithGet("cyp", "cyp-net-listeners",
			&github.Repository{DefaultBranch: github.String("main")}, nil).
		WithGetBranch("cyp", "cyp-net-listeners", "main", &github.Branch{
			Commit: &github.RepositoryCommit{SHA: github.String(headCommit)},
		}, nil)

	c := client{log: testLogger(), repos: mock}
	meta, err := c.GetMeta(context.Background(), "cyp", "cyp-net-listeners", "")
	if err != nil {
		t.Fatalf("GetMeta(empty) unexpected error: %v", err)
	}
	if meta.Commit != headCommit {
		t.Fatalf("meta.Commit = %q, want %q (HEAD from default branch)", meta.Commit, headCommit)
	}
}

// TestGetMeta_RawSHA_40 pins the SHA fast path: a 40-char lowercase hex
// input is returned verbatim in meta.Commit without a call to repos.GetCommit.
// This guards against a future refactor that accidentally routes SHAs
// through the ref-resolution API.
func TestGetMeta_RawSHA_40(t *testing.T) {
	const sha = "81353411f7b010d5b9ebeb1899066aac18a36701"

	mock := newMockRepos().
		WithGet("cyp", "cyp-net-listeners",
			&github.Repository{DefaultBranch: github.String("main")}, nil).
		WithGetBranch("cyp", "cyp-net-listeners", "main", &github.Branch{
			Commit: &github.RepositoryCommit{SHA: github.String("head")},
		}, nil).
		// Expect NO call to GetCommit for the SHA fast path.
		WithGetCommitError("cyp", "cyp-net-listeners", sha,
			errors.New("unexpected GetCommit call on SHA fast path"))

	c := client{log: testLogger(), repos: mock}
	meta, err := c.GetMeta(context.Background(), "cyp", "cyp-net-listeners", sha)
	if err != nil {
		t.Fatalf("GetMeta(40-char sha) unexpected error: %v", err)
	}
	if meta.Commit != sha {
		t.Fatalf("meta.Commit = %q, want %q (SHA fast path)", meta.Commit, sha)
	}
}

// TestGetMeta_ResolvesRef pins the ref-resolution path: a non-SHA, non-
// empty input like "main/v2" triggers a call to repos.GetCommit(ctx,
// owner, repo, "main/v2", nil), the response's SHA is parsed, and
// meta.Commit is stamped with the resolved SHA. This is the headline
// fix for the `buf.yaml: main/v2` case that previously collapsed to
// HEAD.
func TestGetMeta_ResolvesRef(t *testing.T) {
	const (
		ref      = "main/v2"
		resolved = "abc1230000000000000000000000000000000000"
	)

	mock := newMockRepos().
		WithGet("cyp", "cyp-net-listeners",
			&github.Repository{DefaultBranch: github.String("main")}, nil).
		WithGetBranch("cyp", "cyp-net-listeners", "main", &github.Branch{
			Commit: &github.RepositoryCommit{SHA: github.String("head")},
		}, nil).
		WithGetCommit("cyp", "cyp-net-listeners", ref, &github.RepositoryCommit{
			SHA: github.String(resolved),
		}, nil)

	c := client{log: testLogger(), repos: mock}
	meta, err := c.GetMeta(context.Background(), "cyp", "cyp-net-listeners", ref)
	if err != nil {
		t.Fatalf("GetMeta(%q) unexpected error: %v", ref, err)
	}
	if mock.GetCommitCallCount("cyp", "cyp-net-listeners", ref) != 1 {
		t.Errorf("expected exactly 1 GetCommit call for ref=%q, got %d", ref,
			mock.GetCommitCallCount("cyp", "cyp-net-listeners", ref))
	}
	if meta.Commit != resolved {
		t.Fatalf("meta.Commit = %q, want %q (resolved SHA from repos.GetCommit)", meta.Commit, resolved)
	}
}

// TestGetMeta_DefaultBranchName_github is the regression guard for the
// v1.30.1 v1alpha1 case caught by Phase 19 e2e tests. v1.30.1 sends
// ModuleReference.reference="main" (the default label name) via
// v1alpha1.ResolveService/GetModulePins, and the proxy at
// modulepins.go:46 passes "main" to GetMeta. The pre-fix provider code
// routed "main" through repos.GetCommit(ctx, owner, repo, "main", nil),
// which hits GitHub's /commits/main endpoint — the endpoint expects a
// SHA, not a branch name, and rejects the request with 422 "No commit
// found for SHA: main". After the Phase 18 default-branch carve-out,
// "main" that matches the repo's actual default branch name is
// recognized and the HEAD already in meta.Commit is returned without a
// second round-trip.
//
// This test pins the contract: when commit equals meta.DefaultBranch,
// GetMeta returns HEAD and does NOT call repos.GetCommit. A regression
// that re-routes the default branch name through the ref-resolution API
// is caught by the WithGetCommitError trap and by the explicit
// GetCommitCallCount assertion below.
func TestGetMeta_DefaultBranchName_github(t *testing.T) {
	const headCommit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	mock := newMockRepos().
		WithGet("cyp", "cyp-net-listeners",
			&github.Repository{DefaultBranch: github.String("main")}, nil).
		WithGetBranch("cyp", "cyp-net-listeners", "main", &github.Branch{
			Commit: &github.RepositoryCommit{SHA: github.String(headCommit)},
		}, nil).
		// Any unexpected call to GetCommit for the default branch name
		// returns an error and fails the test. This is the trap: the
		// pre-fix code would have hit this and the test would have
		// failed with "unexpected GetCommit call: default-branch
		// carve-out should not call repos.GetCommit".
		WithGetCommitError("cyp", "cyp-net-listeners", "main",
			errors.New("unexpected GetCommit call: default-branch carve-out should not call repos.GetCommit"))

	c := client{log: testLogger(), repos: mock}
	meta, err := c.GetMeta(context.Background(), "cyp", "cyp-net-listeners", "main")
	if err != nil {
		t.Fatalf("GetMeta(\"main\") unexpected error: %v", err)
	}
	if mock.GetCommitCallCount("cyp", "cyp-net-listeners", "main") != 0 {
		t.Errorf("expected 0 GetCommit calls for default-branch name, got %d",
			mock.GetCommitCallCount("cyp", "cyp-net-listeners", "main"))
	}
	if meta.Commit != headCommit {
		t.Fatalf("meta.Commit = %q, want %q (HEAD from getRepo via repos.GetBranch)", meta.Commit, headCommit)
	}
}

// TestGetMeta_ConventionalDefaultName_github is the regression guard
// for the v1.30.1 v1alpha1 case where the buf CLI sends the buf
// default label name (e.g., "main") as the reference, even when it
// does NOT match the repo's actual default branch name. The live case
// caught by Phase 19 e2e tests: googleapis/googleapis has default
// branch "master" but the buf CLI v1.30.1 sends reference="main" (the
// buf default label name). The v1alpha1 ResolveService handler
// (modulepins.go GetModulePins) is NOT gated by parseResourceRefName,
// so the carve-out (content.IsConventionalDefaultName) must remain.
func TestGetMeta_ConventionalDefaultName_github(t *testing.T) {
	const headCommit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	mock := newMockRepos().
		WithGet("googleapis", "googleapis",
			&github.Repository{DefaultBranch: github.String("master")}, nil).
		WithGetBranch("googleapis", "googleapis", "master", &github.Branch{
			Commit: &github.RepositoryCommit{SHA: github.String(headCommit)},
		}, nil).
		WithGetCommitError("googleapis", "googleapis", "main",
			errors.New("unexpected GetCommit call: conventional-default-name carve-out should not call repos.GetCommit"))

	c := client{log: testLogger(), repos: mock}
	meta, err := c.GetMeta(context.Background(), "googleapis", "googleapis", "main")
	if err != nil {
		t.Fatalf("GetMeta(\"main\") unexpected error: %v", err)
	}
	if mock.GetCommitCallCount("googleapis", "googleapis", "main") != 0 {
		t.Errorf("expected 0 GetCommit calls for conventional default name, got %d",
			mock.GetCommitCallCount("googleapis", "googleapis", "main"))
	}
	if meta.Commit != headCommit {
		t.Fatalf("meta.Commit = %q, want %q (HEAD from getRepo via repos.GetBranch)", meta.Commit, headCommit)
	}
}
