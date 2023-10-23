package store

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"

	"github.com/easyp-tech/server/cmd/easyp/internal/core"
)

var _ core.Store = (*store)(nil)

const expectedURIPathSize = 2

type store struct {
	rootDir string
}

// New returns new instance of store.
func New(ctx context.Context, rootDir string, urls []string) (*store, error) {
	for i := range urls {
		u, err := url.Parse(urls[i])
		if err != nil {
			return nil, fmt.Errorf("url.Parse: %w", err)
		}

		uri := strings.Split(strings.TrimLeft(u.Path, "/"), "/")

		if len(uri) != expectedURIPathSize {
			return nil, fmt.Errorf("%w: %s", core.ErrInvalidArgument, urls[i])
		}

		owner, repository := uri[0], uri[1]
		ext := filepath.Ext(repository)
		repository = strings.TrimRight(repository, ext)

		_, err = git.PlainOpen(filepath.Join(rootDir, owner, repository))
		if err == nil {
			continue
		}

		_, err = git.PlainCloneContext(
			ctx,
			filepath.Join(rootDir, owner, repository),
			false,
			&git.CloneOptions{ //nolint:exhaustruct
				URL:      urls[i],
				Progress: os.Stderr,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("git.PlainClone: %w", err)
		}
	}

	return &store{
		rootDir: rootDir,
	}, nil
}

// Get implements core.Store.
func (s *store) Get(_ context.Context, request core.GetRequest) (*core.Repository, error) {
	directory := path.Join(s.rootDir, request.Owner, request.Repository)

	r, err := git.PlainOpen(directory)
	if err != nil {
		return nil, fmt.Errorf("git.PlainOpen: %w", err)
	}

	ref, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("r.Head: %w", err)
	}

	fs := os.DirFS(directory)

	return &core.Repository{
		FS:         fs,
		Owner:      request.Owner,
		Repository: request.Repository,
		Branch:     ref.Name().Short(),
		Commit:     ref.Hash().String(),
		CreatedAt:  time.Time{},
		UpdatedAt:  time.Time{},
	}, nil
}
