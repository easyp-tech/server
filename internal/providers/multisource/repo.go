package multisource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/source"
)

type Source interface {
	Find(owner, repoName string) source.Source
}

type Cache interface {
	Get(ctx context.Context, owner, repoName, commit, configHash string) ([]content.File, error)
	Put(ctx context.Context, owner, repoName, commit, configHash string, in []content.File) error
}

type Repo struct {
	log     *slog.Logger
	cache   Cache
	sources []Source
}

func New(log *slog.Logger, cache Cache, sources ...Source) Repo {
	return Repo{
		log:     log,
		cache:   cache,
		sources: sources,
	}
}

var ErrNotFound = errors.New("not found")

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
		r.log.Error("from cache", "owner", owner, "repo", repoName, "commit", commit, "error", err)

		return nil
	}

	r.log.Debug("from cache", "owner", owner, "repo", repoName, "commit", commit, "files", len(files))

	return files
}

func (r Repo) cachePut(ctx context.Context, owner, repoName, commit, configHash string, files []content.File) {
	if err := r.cache.Put(ctx, owner, repoName, commit, configHash, files); err != nil {
		r.log.Error(
			"to cache",
			"owner", owner,
			"repo", repoName,
			"commit", commit,
			"files", len(files),
			"error", err,
		)

		return
	}

	r.log.Debug("to cache", "owner", owner, "repo", repoName, "commit", commit, "files", len(files))
}

func (r Repo) findSource(owner, repoName string) source.Source {
	for _, s := range r.sources {
		if repo := s.Find(owner, repoName); repo != nil {
			return repo
		}
	}

	return nil
}
