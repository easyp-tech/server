package github

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/google/go-github/v59/github"
)

// testLogger returns a slog.Logger that discards output. Used by tests
// that exercise the github client without caring about log output.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// key3 is the composite lookup key for mock repos: (owner, repo, arg).
// The third element is sha for GetCommit, branch for GetBranch, and
// ignored for Get / DownloadContents.
type key3 struct{ owner, repo, arg string }

// mockRepos is a stub Repositories implementation for tests. It records
// calls and returns canned responses. Build with newMockRepos(); chain
// .WithGet / .WithGetBranch / .WithGetCommit / .WithDownloadContents to
// configure responses. By default a call with no configured response
// panics — this surfaces unexpected upstream calls in tests, instead of
// silently returning a zero value.
//
// A call that has been configured (via WithGetCommitError) to return
// an error lets tests assert the SHA fast path did NOT make the call.
type mockRepos struct {
	mu sync.Mutex

	gets          map[key3]*getResp
	getBranches   map[key3]*getResp
	getCommits    map[key3]*getResp
	downloads     map[key3]*getResp
	commitCalls   map[key3]*atomic.Int32
	branchCalls   map[key3]*atomic.Int32
	getCalls      map[key3]*atomic.Int32
	downloadCalls map[key3]*atomic.Int32
}

type getResp struct {
	repo     *github.Repository
	branch   *github.Branch
	commit   *github.RepositoryCommit
	rcBody   io.ReadCloser
	response *github.Response
	err      error
}

// newMockRepos returns an empty mockRepos. The mock will panic on any
// upstream call that has not been configured via WithXxx.
func newMockRepos() *mockRepos {
	return &mockRepos{
		gets:        make(map[key3]*getResp),
		getBranches: make(map[key3]*getResp),
		getCommits:  make(map[key3]*getResp),
		downloads:   make(map[key3]*getResp),
		commitCalls: make(map[key3]*atomic.Int32),
		branchCalls: make(map[key3]*atomic.Int32),
		getCalls:    make(map[key3]*atomic.Int32),
	}
}

// WithGet configures the response for repos.Get(ctx, owner, repo).
func (m *mockRepos) WithGet(owner, repo string, r *github.Repository, err error) *mockRepos {
	m.gets[key3{owner, repo, ""}] = &getResp{repo: r, err: err}
	return m
}

// WithGetBranch configures the response for repos.GetBranch(ctx, owner,
// repo, branch, maxRedirects).
func (m *mockRepos) WithGetBranch(owner, repo, branch string, b *github.Branch, err error) *mockRepos {
	m.getBranches[key3{owner, repo, branch}] = &getResp{branch: b, err: err}
	return m
}

// WithGetCommit configures the response for repos.GetCommit(ctx, owner,
// repo, sha, opts).
func (m *mockRepos) WithGetCommit(owner, repo, sha string, c *github.RepositoryCommit, err error) *mockRepos {
	m.getCommits[key3{owner, repo, sha}] = &getResp{commit: c, err: err}
	return m
}

// WithGetCommitError configures GetCommit to return err for the given
// (owner, repo, sha). Used to assert the SHA fast path did NOT make a
// GetCommit call: if the test reaches the configured call, the test
// fails with the supplied error.
func (m *mockRepos) WithGetCommitError(owner, repo, sha string, err error) *mockRepos {
	m.getCommits[key3{owner, repo, sha}] = &getResp{err: err}
	return m
}

// WithDownloadContents configures the response for repos.DownloadContents
// (not used by the current tests but required by the Repositories
// interface).
func (m *mockRepos) WithDownloadContents(owner, repo, path string, body io.ReadCloser, err error) *mockRepos {
	m.downloads[key3{owner, repo, path}] = &getResp{rcBody: body, err: err}
	return m
}

// GetCommitCallCount returns the number of times GetCommit was called
// for the given (owner, repo, sha). Used by tests that assert the
// ref-resolution path was taken (or, conversely, the SHA fast path
// was NOT taken).
func (m *mockRepos) GetCommitCallCount(owner, repo, sha string) int32 {
	k := key3{owner, repo, sha}
	m.mu.Lock()
	c, ok := m.commitCalls[k]
	m.mu.Unlock()
	if !ok {
		return 0
	}
	return c.Load()
}

func (m *mockRepos) Get(ctx context.Context, owner, repo string) (*github.Repository, *github.Response, error) {
	k := key3{owner, repo, ""}
	m.mu.Lock()
	c, ok := m.getCalls[k]
	if !ok {
		c = &atomic.Int32{}
		m.getCalls[k] = c
	}
	m.mu.Unlock()
	c.Add(1)

	m.mu.Lock()
	r, rok := m.gets[k]
	m.mu.Unlock()
	if !rok {
		panic("mockRepos: Get not configured for " + k.owner + "/" + k.repo)
	}
	return r.repo, r.response, r.err
}

func (m *mockRepos) GetBranch(ctx context.Context, owner, repo, branch string, maxRedirects int) (*github.Branch, *github.Response, error) {
	k := key3{owner, repo, branch}
	m.mu.Lock()
	c, ok := m.branchCalls[k]
	if !ok {
		c = &atomic.Int32{}
		m.branchCalls[k] = c
	}
	m.mu.Unlock()
	c.Add(1)

	m.mu.Lock()
	r, rok := m.getBranches[k]
	m.mu.Unlock()
	if !rok {
		panic("mockRepos: GetBranch not configured for " + k.owner + "/" + k.repo + "@" + branch)
	}
	return r.branch, r.response, r.err
}

func (m *mockRepos) GetCommit(ctx context.Context, owner, repo, sha string, opts *github.ListOptions) (*github.RepositoryCommit, *github.Response, error) {
	k := key3{owner, repo, sha}
	m.mu.Lock()
	c, ok := m.commitCalls[k]
	if !ok {
		c = &atomic.Int32{}
		m.commitCalls[k] = c
	}
	m.mu.Unlock()
	c.Add(1)

	m.mu.Lock()
	r, rok := m.getCommits[k]
	m.mu.Unlock()
	if !rok {
		panic("mockRepos: GetCommit not configured for " + k.owner + "/" + k.repo + "@" + sha)
	}
	return r.commit, r.response, r.err
}

func (m *mockRepos) DownloadContents(ctx context.Context, owner, repo, filepath string, opts *github.RepositoryContentGetOptions) (io.ReadCloser, *github.Response, error) {
	k := key3{owner, repo, filepath}
	m.mu.Lock()
	c, ok := m.downloadCalls[k]
	if !ok {
		c = &atomic.Int32{}
		m.downloadCalls[k] = c
	}
	m.mu.Unlock()
	c.Add(1)

	m.mu.Lock()
	r, rok := m.downloads[k]
	m.mu.Unlock()
	if !rok {
		panic("mockRepos: DownloadContents not configured for " + k.owner + "/" + k.repo + "/" + filepath)
	}
	return r.rcBody, r.response, r.err
}

// Compile-time check that mockRepos satisfies the Repositories interface.
var _ Repositories = (*mockRepos)(nil)
