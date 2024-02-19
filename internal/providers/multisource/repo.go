package multisource

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/exp/slog"

	"github.com/easyp-tech/server/internal/providers/content"
)

type Source interface {
	Name() string
	Check(owner, repoName string) bool
	GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error)
	GetFiles(ctx context.Context, owner, repoName, commit string) ([]content.File, error)
}

type fileCache interface {
	Get(owner, repoName, commit string) ([]content.File, error)
	Put(owner, repoName, commit string, in []content.File) error
}

type Repo struct {
	log     *slog.Logger
	cache   fileCache
	sources []Source
}

func New(log *slog.Logger, cache fileCache, sources ...Source) Repo {
	return Repo{
		log:     log,
		cache:   cache,
		sources: sources,
	}
}

var ErrNotFound = errors.New("not found")

func (r Repo) GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error) {
	r.log.Debug("looking for meta", "owner", owner, "repo", repoName)

	for _, s := range r.sources {
		if s.Check(owner, repoName) {
			r.log.Debug(
				"module found",
				"source", s.Name(),
				"owner", owner,
				"repo", repoName,
			)

			return s.GetMeta(ctx, owner, repoName, commit)
		}
	}

	return content.Meta{}, ErrNotFound
}

func (r Repo) GetFiles(ctx context.Context, owner, repoName, commit string) ([]content.File, error) {
	if files := r.cacheGet(owner, repoName, commit); files != nil {
		return files, nil
	}

	for _, s := range r.sources {
		if s.Check(owner, repoName) {
			r.log.Debug(
				"module found",
				"source", s.Name(),
				"owner", owner,
				"repo", repoName,
			)

			files, err := s.GetFiles(ctx, owner, repoName, commit)
			if err != nil {
				return files, fmt.Errorf("getting files: %w", err)
			}

			r.cachePut(owner, repoName, commit, files)

			return files, nil
		}
	}
	return nil, ErrNotFound
}

func (r Repo) cacheGet(owner, repoName, commit string) []content.File {
	files, err := r.cache.Get(owner, repoName, commit)
	if err != nil {
		r.log.Error("from cache", "owner", owner, "repo", repoName, "commit", commit, "error", err)

		return nil
	}

	r.log.Debug("from cache", "owner", owner, "repo", repoName, "commit", commit, "files", len(files))

	return files
}

func (r Repo) cachePut(owner, repoName, commit string, files []content.File) {
	if err := r.cache.Put(owner, repoName, commit, files); err != nil {
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
