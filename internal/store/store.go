package store

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/go-git/go-git/v5"

	"github.com/easyp-tech/server/internal/core"
)

var _ core.Store = (*store)(nil)

const expectedURIPathSize = 2

type store struct {
	rootDir string
}

// New returns new instance of store.
func New(_ context.Context, rootDir string) *store {
	return &store{
		rootDir: rootDir,
	}
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
