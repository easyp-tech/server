package multisource

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/exp/slog"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/source"
)

type Provider interface {
	Find(owner, repoName string) source.Source
	Repositories() []source.Source
}

type Cache interface {
	Get(ctx context.Context, owner, repoName, commit, configHash string) ([]content.File, error)
	Put(ctx context.Context, owner, repoName, commit, configHash string, in []content.File) error
	Ping(ctx context.Context) error // Новый метод
}

type Repo struct {
	log       *slog.Logger
	cache     Cache
	providers []Provider
}

func New(log *slog.Logger, cache Cache, providers ...Provider) Repo {
	return Repo{
		log:       log,
		cache:     cache,
		providers: providers,
	}
}

var ErrNotFound = errors.New("not found")

func (r Repo) Repositories() []source.Source {
	var repos []source.Source
	for _, p := range r.providers {
		repos = append(repos, p.Repositories()...)
	}
	return repos
}

func (r Repo) GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error) {
	r.log.Debug("looking for meta", "owner", owner, "repo", repoName)

	s := r.findSource(owner, repoName)
	if s == nil {
		return content.Meta{}, ErrNotFound
	}

	r.log.Debug("module found", "source", s.Name(), "config", s.ConfigHash(), "owner", owner, "repo", repoName)

	return s.GetMeta(ctx, commit) //nolint:wrapcheck
}

func (r Repo) GetFiles(ctx context.Context, owner, repoName, commit string) ([]content.File, error) {
	s := r.findSource(owner, repoName)
	if s == nil {
		return nil, ErrNotFound
	}

	r.log.Debug("module found", "source", s.Name(), "config", s.ConfigHash(), "owner", owner, "repo", repoName)

	if files := r.cacheGet(ctx, owner, repoName, commit, s.ConfigHash()); files != nil {
		return files, nil
	}

	files, err := s.GetFiles(ctx, commit)
	if err != nil {
		return files, fmt.Errorf("getting files: %w", err)
	}

	r.cachePut(ctx, owner, repoName, commit, s.ConfigHash(), files)

	return files, nil
}

func (r Repo) cacheGet(ctx context.Context, owner, repoName, commit, configHash string) []content.File {
	files, err := r.cache.Get(ctx, owner, repoName, commit, configHash)
	if err != nil {
		r.log.Error("cache get failed",
			"owner", owner,
			"repo", repoName,
			"commit", commit,
			"error", err)
		return nil
	}

	if len(files) > 0 {
		r.log.Debug("cache hit",
			"owner", owner,
			"repo", repoName,
			"commit", commit,
			"files", len(files))
	}
	return files
}

func (r Repo) cachePut(ctx context.Context, owner, repoName, commit, configHash string, files []content.File) {
	if err := r.cache.Put(ctx, owner, repoName, commit, configHash, files); err != nil {
		r.log.Error("cache put failed",
			"owner", owner,
			"repo", repoName,
			"commit", commit,
			"files", len(files),
			"error", err)
	} else {
		r.log.Debug("cache updated",
			"owner", owner,
			"repo", repoName,
			"commit", commit,
			"files", len(files))
	}
}

func (r Repo) findSource(owner, repoName string) source.Source { //nolint:ireturn
	for _, p := range r.providers {
		if repo := p.Find(owner, repoName); repo != nil {
			return repo
		}
	}

	return nil
}
