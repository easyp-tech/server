package github

import (
	"context"
	"fmt"
	"io"
	"time"

	"log/slog"

	connectpkg "github.com/easyp-tech/server/internal/connect"
	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/google/go-github/v59/github"
)

func (c client) GetFiles(
	ctx context.Context,
	owner string,
	repoName string,
	commit string,
	repo filter.Repo,
) ([]content.File, error) {
	reqID := connectpkg.RequestIDFrom(ctx)

	start := time.Now()
	c.log.DebugContext(ctx, "github GetFiles start",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("commit", commit),
		slog.String("request_id", reqID),
	)
	tree, _, err := c.git.GetTree(ctx, owner, repoName, commit, true)
	dur := time.Since(start)
	if err != nil {
		c.log.LogAttrs(ctx, slog.LevelDebug, "github GetFiles tree failed",
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("commit", commit),
			slog.String("request_id", reqID),
			slog.Duration("duration", dur),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("listing %q/%q:%q: %w", owner, repoName, commit, err)
	}
	c.log.LogAttrs(ctx, slog.LevelDebug, "github GetFiles tree completed",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("commit", commit),
		slog.String("request_id", reqID),
		slog.Duration("duration", dur),
		slog.Int("entries", len(tree.Entries)),
	)

	entries := content.FilterEntries(tree.Entries, func(e *github.TreeEntry) string { return e.GetPath() }, repo)
	files, err := c.getFiles(ctx, owner, repoName, commit, entries)
	if err != nil {
		return nil, fmt.Errorf("downloading %q/%q:%q: %w", owner, repoName, commit, err)
	}

	return files, nil
}

func (c client) getFiles(
	ctx context.Context,
	owner string,
	repoName string,
	commit string,
	entries []content.FileEntry,
) ([]content.File, error) {
	return content.GetFiles(ctx, entries, func(ctx context.Context, orig string) ([]byte, error) {
		return c.getFile(ctx, owner, repoName, commit, orig)
	})
}

func (c client) getFile(
	ctx context.Context,
	owner string,
	repoName string,
	commit string,
	path string,
) ([]byte, error) {
	reqID := connectpkg.RequestIDFrom(ctx)

	start := time.Now()
	c.log.DebugContext(ctx, "github getFile start",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("path", path),
		slog.String("request_id", reqID),
	)
	r, _, err := c.repos.DownloadContents(
		ctx,
		owner,
		repoName,
		path,
		&github.RepositoryContentGetOptions{Ref: commit},
	)
	dur := time.Since(start)
	if err != nil {
		c.log.LogAttrs(ctx, slog.LevelDebug, "github getFile failed",
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("path", path),
			slog.String("request_id", reqID),
			slog.Duration("duration", dur),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("requesting: %w", err)
	}

	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return data, fmt.Errorf("downloading: %w", err)
	}

	c.log.LogAttrs(ctx, slog.LevelDebug, "github getFile completed",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("path", path),
		slog.String("request_id", reqID),
		slog.Duration("duration", dur),
		slog.Int("size", len(data)),
	)

	return data, nil
}