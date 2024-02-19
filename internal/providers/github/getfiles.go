package github

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/go-github/v59/github"
	"golang.org/x/exp/slices"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/easyp-tech/server/internal/shake256"
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

	files, err := c.getFiles(ctx, owner, repoName, commit, filterEntries(tree.Entries, repo))
	if err != nil {
		return files, fmt.Errorf("downloading %q/%q:%q: %w", owner, repoName, commit, err)
	}

	return files, nil
}

type fileFiltered struct {
	orig string
	name string
}

func filterEntries(entries []*github.TreeEntry, repo filter.Repo) []fileFiltered {
	out := make([]fileFiltered, 0, len(entries))

	for _, entry := range entries {
		if name, ok := repo.Check(entry.GetPath()); ok {
			out = append(out, fileFiltered{orig: entry.GetPath(), name: name})
		}
	}

	slices.SortFunc(out, func(a, b fileFiltered) int { return strings.Compare(a.name, b.name) })

	return out
}

func (c client) getFiles(
	ctx context.Context,
	owner string,
	repoName string,
	commit string,
	files []fileFiltered,
) ([]content.File, error) {
	out := make([]content.File, 0, len(files))

	for _, file := range files {
		data, err := c.getFile(ctx, owner, repoName, commit, file.orig)
		if err != nil {
			return nil, fmt.Errorf("downloading %q: %w", file, err)
		}

		hash, err := shake256.SHA3Shake256(data)
		if err != nil {
			return nil, fmt.Errorf("hashing %q: %w", file, err)
		}

		out = append(out, content.File{Path: file.name, Data: data, Hash: hash})
	}

	return out, nil
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
