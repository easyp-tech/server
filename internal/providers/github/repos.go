package github

import (
	"context"
	"fmt"
	"hash/crc32"

	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/easyp-tech/server/internal/providers/source"
)

type Repo struct {
	Token string
	filter.Repo
}

type multiRepo struct {
	log   *slog.Logger
	repos []Repo
}

func (m multiRepo) Find(owner, name string) source.Source { //nolint:ireturn
	s, ok := m.find(owner, name)
	if !ok {
		return nil
	}

	return s
}

func (m multiRepo) Repositories() []source.Source { //nolint:ireturn
	repos := make([]source.Source, len(m.repos))
	for i, r := range m.repos {
		repos[i] = sourceRepo{log: m.log, repo: r}
	}
	return repos
}

func (m multiRepo) find(owner, name string) (sourceRepo, bool) {
	i := slices.IndexFunc(m.repos, func(r Repo) bool {
		return r.Repo.Owner == owner && r.Repo.Name == name
	})
	if i < 0 {
		return sourceRepo{}, false //nolint:exhaustruct
	}

	return sourceRepo{log: m.log, repo: m.repos[i]}, true
}

func NewMultiRepo(log *slog.Logger, repos []Repo) multiRepo {
	return multiRepo{
		log:   log,
		repos: repos,
	}
}

var _ source.Source = sourceRepo{} //nolint:exhaustruct

type sourceRepo struct {
	log  *slog.Logger
	repo Repo
}

func (r sourceRepo) ConfigHash() string {
	return fmt.Sprintf("%X", crc32.ChecksumIEEE([]byte(fmt.Sprintf("%+v", r.repo.Repo))))
}

func (r sourceRepo) Name() string     { return "github proxy" }
func (r sourceRepo) Owner() string    { return r.repo.Owner }
func (r sourceRepo) RepoName() string { return r.repo.Name }
func (r sourceRepo) Type() string     { return "github" }

func (r sourceRepo) GetMeta(ctx context.Context, commit string) (content.Meta, error) {
	return connect(r.log, r.repo.Token).GetMeta(ctx, r.repo.Owner, r.repo.Name, commit)
}

func (r sourceRepo) GetFiles(ctx context.Context, commit string) ([]content.File, error) {
	return connect(r.log, r.repo.Token).GetFiles(ctx, r.repo.Owner, r.repo.Name, commit, r.repo.Repo)
}
