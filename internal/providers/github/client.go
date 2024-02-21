package github

import (
	"context"
	"io"

	"github.com/google/go-github/v59/github"
)

const (
	ProtoSuffix  = ".proto"
	MaxRedirects = 1024
)

type Repositories interface {
	GetCommit(ctx context.Context, owner, repo, sha string, opts *github.ListOptions) (*github.RepositoryCommit, *github.Response, error)
	Get(ctx context.Context, owner, repo string) (*github.Repository, *github.Response, error)
	GetBranch(ctx context.Context, owner, repo, branch string, maxRedirects int) (*github.Branch, *github.Response, error)
	DownloadContents(ctx context.Context, owner, repo, filepath string, opts *github.RepositoryContentGetOptions) (io.ReadCloser, *github.Response, error)
}

type Git interface {
	GetTree(ctx context.Context, owner string, repo string, sha string, recursive bool) (*github.Tree, *github.Response, error)
}

type client struct {
	repos Repositories
	git   Git
}

func connect(token string) client {
	c := github.NewClient(nil)

	if token != "" {
		c = c.WithAuthToken(token)
	}

	return client{repos: c.Repositories, git: c.Git}
}
