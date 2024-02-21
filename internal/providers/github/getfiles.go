package github

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/go-github/v59/github"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/shake256"
)

func (c client) GetFiles(
	ctx context.Context,
	owner string,
	repoName string,
	commit string,
	prefixes []string,
) ([]content.File, error) {
	tree, _, err := c.git.GetTree(ctx, owner, repoName, commit, true)
	if err != nil {
		return nil, fmt.Errorf("listing %q/%q:%q: %w", owner, repoName, commit, err)
	}

	files, err := c.getFiles(ctx, owner, repoName, commit, filterEntries(tree.Entries, prefixes))
	if err != nil {
		return files, fmt.Errorf("downloading %q/%q:%q: %w", owner, repoName, commit, err)
	}

	return files, nil
}

func filterEntries(entries []*github.TreeEntry, prefixes []string) []string {
	out := make([]string, 0, len(entries))

	for _, entry := range entries {
		if checkPath(entry.GetPath(), prefixes) {
			out = append(out, entry.GetPath())
		}
	}

	sort.Strings(out)

	return out
}

func checkPath(path string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return filepath.Ext(path) == ProtoSuffix
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return filepath.Ext(path) == ProtoSuffix
		}
	}

	return false
}

func (c client) getFiles(
	ctx context.Context,
	owner string,
	repoName string,
	commit string,
	paths []string,
) ([]content.File, error) {
	out := make([]content.File, 0, len(paths))

	for _, path := range paths {
		data, err := c.getFile(ctx, owner, repoName, commit, path)
		if err != nil {
			return nil, fmt.Errorf("downloading %q: %w", path, err)
		}

		hash, err := shake256.SHA3Shake256(data)
		if err != nil {
			return nil, fmt.Errorf("hashing %q: %w", path, err)
		}

		out = append(out, content.File{Path: path, Data: data, Hash: hash})
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
