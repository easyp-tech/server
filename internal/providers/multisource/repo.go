package multisource

import (
	"context"
	"errors"
	"fmt"
	"time"

	"log/slog"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/source"
	"github.com/easyp-tech/server/internal/reqid"
)

type Provider interface {
	Find(owner, repoName string) source.Source
	Repositories() []source.Source
}

type Cache interface {
	Get(ctx context.Context, owner, repoName, commit, configHash string) ([]content.File, error)
	Put(ctx context.Context, owner, repoName, commit, configHash string, in []content.File) error
	CheckWriteAccess(ctx context.Context) error
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

// reqLogger returns the package logger with the per-request correlation id
// attached, so every upstream call line carries the same request_id as the
// matching access log line.
func (r Repo) reqLogger(ctx context.Context) *slog.Logger {
	if id := reqid.From(ctx); id != "" {
		return r.log.With(slog.String("request_id", id))
	}
	return r.log
}

func (r Repo) GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error) {
	r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "upstream call",
		slog.String("target", "multisource.GetMeta"),
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("module", repoName),
		slog.String("commit", commit),
	)
	start := time.Now()

	s := r.findSource(owner, repoName)
	if s == nil {
		r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "upstream result",
			slog.String("target", "multisource.GetMeta"),
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("module", repoName),
			slog.String("commit", commit),
			slog.String("commit_id", commit),
			slog.String("outcome", "not_found"),
			slog.Duration("duration", time.Since(start)),
		)
		return content.Meta{}, ErrNotFound
	}

	r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "source selected",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("module", repoName),
		slog.String("commit", commit),
		slog.String("source", s.Name()),
		slog.String("source_config", s.ConfigHash()),
	)

	meta, err := s.GetMeta(ctx, commit)
	if err != nil {
		r.reqLogger(ctx).LogAttrs(ctx, slog.LevelWarn, "upstream result",
			slog.String("target", "multisource.GetMeta"),
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("module", repoName),
			slog.String("commit", commit),
			slog.String("commit_id", commit),
			slog.String("outcome", "error"),
			slog.String("source", s.Name()),
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()),
		)
		return content.Meta{}, err //nolint:wrapcheck
	}

	r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "upstream result",
		slog.String("target", "multisource.GetMeta"),
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("module", repoName),
		slog.String("commit", commit),
		slog.String("resolved_commit", meta.Commit),
		slog.String("commit_id", meta.Commit),
		slog.String("outcome", "ok"),
		slog.String("source", s.Name()),
		slog.Duration("duration", time.Since(start)),
	)
	return meta, nil
}

func (r Repo) GetFiles(ctx context.Context, owner, repoName, commit string) ([]content.File, error) {
	r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "upstream call",
		slog.String("target", "multisource.GetFiles"),
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("module", repoName),
		slog.String("commit", commit),
		slog.String("commit_id", commit),
	)
	start := time.Now()

	s := r.findSource(owner, repoName)
	if s == nil {
		r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "upstream result",
			slog.String("target", "multisource.GetFiles"),
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("module", repoName),
			slog.String("commit", commit),
			slog.String("commit_id", commit),
			slog.String("outcome", "not_found"),
			slog.Duration("duration", time.Since(start)),
		)
		return nil, ErrNotFound
	}

	r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "source selected",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("module", repoName),
		slog.String("commit", commit),
		slog.String("source", s.Name()),
		slog.String("source_config", s.ConfigHash()),
	)

	files, hit, srcLatency, cacheLatency := r.cacheGet(ctx, owner, repoName, commit, s.ConfigHash())
	if hit {
		r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "upstream result",
			slog.String("target", "multisource.GetFiles"),
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("module", repoName),
			slog.String("commit", commit),
			slog.String("commit_id", commit),
			slog.String("outcome", "cache_hit"),
			slog.String("source", s.Name()),
			slog.Int("files", len(files)),
			slog.Int("bytes", fileBytes(files)),
			slog.Duration("cache_latency", cacheLatency),
			slog.Duration("duration", time.Since(start)),
		)
		return files, nil
	}

	sourceStart := time.Now()
	srcFiles, err := s.GetFiles(ctx, commit)
	srcLatency = time.Since(sourceStart)
	if err != nil {
		r.reqLogger(ctx).LogAttrs(ctx, slog.LevelWarn, "upstream result",
			slog.String("target", "multisource.GetFiles"),
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("module", repoName),
			slog.String("commit", commit),
			slog.String("commit_id", commit),
			slog.String("outcome", "error"),
			slog.String("source", s.Name()),
			slog.Duration("source_latency", srcLatency),
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("getting files: %w", err)
	}

	r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "upstream result",
		slog.String("target", "multisource.GetFiles"),
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("module", repoName),
		slog.String("commit", commit),
		slog.String("commit_id", commit),
		slog.String("outcome", "cache_miss"),
		slog.String("source", s.Name()),
		slog.Int("files", len(srcFiles)),
		slog.Int("bytes", fileBytes(srcFiles)),
		slog.Duration("source_latency", srcLatency),
		slog.Duration("cache_latency", cacheLatency),
		slog.Duration("duration", time.Since(start)),
	)

	r.cachePut(ctx, owner, repoName, commit, s.ConfigHash(), srcFiles)

	return srcFiles, nil
}

// cacheGet returns (files, hit, srcLatency, cacheLatency).
// On error the function falls through as a miss; the error is logged at WARN
// with a per-request correlation id.
func (r Repo) cacheGet(ctx context.Context, owner, repoName, commit, configHash string) ([]content.File, bool, time.Duration, time.Duration) {
	var zero time.Duration
	start := time.Now()
	files, err := r.cache.Get(ctx, owner, repoName, commit, configHash)
	cacheLatency := time.Since(start)
	if err != nil {
		r.reqLogger(ctx).LogAttrs(ctx, slog.LevelWarn, "cache get failed",
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("module", repoName),
			slog.String("commit", commit),
			slog.String("commit_id", commit),
			slog.Duration("cache_latency", cacheLatency),
			slog.String("error", err.Error()),
		)
		return nil, false, zero, cacheLatency
	}

	if len(files) > 0 {
		return files, true, zero, cacheLatency
	}
	return nil, false, zero, cacheLatency
}

func (r Repo) cachePut(ctx context.Context, owner, repoName, commit, configHash string, files []content.File) {
	start := time.Now()
	if err := r.cache.Put(ctx, owner, repoName, commit, configHash, files); err != nil {
		r.reqLogger(ctx).LogAttrs(ctx, slog.LevelWarn, "cache put failed",
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("module", repoName),
			slog.String("commit", commit),
			slog.String("commit_id", commit),
			slog.Int("files", len(files)),
			slog.Int("bytes", fileBytes(files)),
			slog.Duration("cache_latency", time.Since(start)),
			slog.String("error", err.Error()),
		)
		return
	}
	r.reqLogger(ctx).LogAttrs(ctx, slog.LevelInfo, "cache put ok",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("module", repoName),
		slog.String("commit", commit),
		slog.String("commit_id", commit),
		slog.Int("files", len(files)),
		slog.Int("bytes", fileBytes(files)),
		slog.Duration("cache_latency", time.Since(start)),
	)
}

func (r Repo) findSource(owner, repoName string) source.Source { //nolint:ireturn
	for _, p := range r.providers {
		if repo := p.Find(owner, repoName); repo != nil {
			return repo
		}
	}

	return nil
}

// fileBytes sums the byte size of every file in the slice, for at-a-glance
// observability in upstream trace lines.
func fileBytes(files []content.File) int {
	n := 0
	for _, f := range files {
		n += len(f.Data)
	}
	return n
}
