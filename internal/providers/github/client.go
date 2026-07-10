package github

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/go-github/v59/github"
)

const (
	ProtoSuffix  = ".proto"
	MaxRedirects = 1024
)

//nolint:lll
type Repositories interface {
	GetCommit(ctx context.Context, owner, repo, sha string, opts *github.ListOptions) (*github.RepositoryCommit, *github.Response, error)
	Get(ctx context.Context, owner, repo string) (*github.Repository, *github.Response, error)
	GetBranch(ctx context.Context, owner, repo, branch string, maxRedirects int) (*github.Branch, *github.Response, error)
	DownloadContents(ctx context.Context, owner, repo, filepath string, opts *github.RepositoryContentGetOptions) (io.ReadCloser, *github.Response, error)
}

//nolint:lll
type Git interface {
	GetTree(ctx context.Context, owner string, repo string, sha string, recursive bool) (*github.Tree, *github.Response, error)
}

type client struct {
	log   *slog.Logger
	repos Repositories
	git   Git
}

func connect(log *slog.Logger, token string) client {
	// Wrap the default transport with bounded retry for transient
	// upstream failures (TLS handshake timeouts to api.github.com /
	// raw.githubusercontent.com, EOF, connection reset, 429, 5xx).
	// GitHub reads fan out into many per-file GETs, so a single
	// transient timeout must not fail the whole batch.
	httpClient := &http.Client{
		Transport: &retryTransport{base: http.DefaultTransport, log: log},
	}
	c := github.NewClient(httpClient)

	if token != "" {
		c = c.WithAuthToken(token)
	}

	return client{log: log, repos: c.Repositories, git: c.Git}
}
