package github

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/easyp-tech/server/internal/providers/content"
)

type Repo struct {
	Owner string
	Name  string
	Token string
	Paths []string
}

type multiRepo struct {
	repos []Repo
}

func (m multiRepo) Name() string {
	return "github proxy"
}

func (m multiRepo) Check(owner, name string) bool {
	_, ok := m.find(owner, name)

	return ok
}

func (m multiRepo) find(owner, name string) (Repo, bool) {
	i := slices.IndexFunc(m.repos, func(r Repo) bool { return r.Owner == owner && r.Name == name })
	if i < 0 {
		return Repo{}, false
	}

	return m.repos[i], true
}

var ErrNotFound = errors.New("not found")

func (m multiRepo) GetMeta(ctx context.Context, owner, name, commit string) (content.Meta, error) {
	r, ok := m.find(owner, name)
	if !ok {
		return content.Meta{}, fmt.Errorf("github %q/%q: %w", ErrNotFound)
	}

	return connect(r.Token).GetMeta(ctx, owner, name, commit)
}

func (m multiRepo) GetFiles(ctx context.Context, owner, name, commit string) ([]content.File, error) {
	r, ok := m.find(owner, name)
	if !ok {
		return nil, fmt.Errorf("github %q/%q: %w", ErrNotFound)
	}

	return connect(r.Token).GetFiles(ctx, owner, name, commit, r.Paths)
}

func NewMultiRepo(repos []Repo) multiRepo {
	return multiRepo{repos: repos}
}
