package github

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	"log/slog"

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
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext(ctx, "tcp4", addr)
	}
	httpClient := &http.Client{Transport: transport}
	c := github.NewClient(httpClient)

	if token != "" {
		c = c.WithAuthToken(token)
	}

	return client{log: log, repos: c.Repositories, git: c.Git}
}
