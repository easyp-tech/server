package github

import (
	"context"
	"fmt"
	"io"

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
	tree, _, err := c.git.GetTree(ctx, owner, repoName, commit, true)
	if err != nil {
		return nil, fmt.Errorf("listing %q/%q:%q: %w", owner, repoName, commit, err)
	}

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
	r, _, err := c.repos.DownloadContents(
		ctx,
		owner,
		repoName,
		path,
		&github.RepositoryContentGetOptions{Ref: commit},
	)
	if err != nil {
		return nil, fmt.Errorf("requesting: %w", err)
	}

	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return data, fmt.Errorf("downloading: %w", err)
	}

	return data, nil
}