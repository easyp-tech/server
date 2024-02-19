package github

import (
	"context"

	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/filter"
)

type Repo struct {
	Token string
	filter.Repo
}

type multiRepo struct {
	log   *slog.Logger
	repos []Repo
	token string
}

func (m multiRepo) Name() string {
	return "github proxy"
}

func (m multiRepo) Check(owner, name string) bool {
	return true
}

func (m multiRepo) find(owner, name string) Repo {
	i := slices.IndexFunc(m.repos, func(r Repo) bool {
		return r.Repo.Owner == owner && r.Repo.Name == name
	})
	if i < 0 {
		return Repo{Token: m.token, Repo: filter.Repo{Owner: owner, Name: name}}
	}

	return m.repos[i]
}

func (m multiRepo) GetMeta(ctx context.Context, owner, name, commit string) (content.Meta, error) {
	r := m.find(owner, name)

	return connect(m.log, r.Token).GetMeta(ctx, owner, name, commit)
}

func (m multiRepo) GetFiles(ctx context.Context, owner, name, commit string) ([]content.File, error) {
	r := m.find(owner, name)

	return connect(m.log, r.Token).GetFiles(ctx, owner, name, commit, r.Repo)
}

func NewMultiRepo(log *slog.Logger, repos []Repo, token string) multiRepo {
	return multiRepo{
		log:   log,
		repos: repos,
		token: token,
	}
}
