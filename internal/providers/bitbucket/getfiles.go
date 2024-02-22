package bitbucket

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/easyp-tech/server/internal/shake256"
)

func (c client) GetFiles(
	ctx context.Context,
	commit string,
	repo filter.Repo,
) ([]content.File, error) {
	tree, err := c.listFiles(ctx, commit)
	if err != nil {
		return nil, fmt.Errorf("listing %q: %w", commit, err)
	}

	files, err := c.getFiles(ctx, commit, filterEntries(tree, repo))
	if err != nil {
		return files, fmt.Errorf("downloading %q: %w", commit, err)
	}

	return files, nil
}

type fileFiltered struct {
	orig string
	name string
}

func filterEntries(entries []string, repo filter.Repo) []fileFiltered {
	out := make([]fileFiltered, 0, len(entries))

	for _, entry := range entries {
		if name, ok := repo.Check(entry); ok {
			out = append(out, fileFiltered{orig: entry, name: name})
		}
	}

	slices.SortFunc(out, func(a, b fileFiltered) int { return strings.Compare(a.name, b.name) })

	return out
}

func (c client) getFiles(
	ctx context.Context,
	commit string,
	files []fileFiltered,
) ([]content.File, error) {
	out := make([]content.File, 0, len(files))

	for _, file := range files {
		data, err := c.getFile(ctx, c.client, commit, file.orig)
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
	cln httpClient,
	commit string,
	name string,
) ([]byte, error) {
	data, err := cln.get(
		ctx,
		tmplGetFileContent,
		paramsMap{"name": name},
		qeryMap{"at": commit},
	)
	if err != nil {
		return data, fmt.Errorf("downloading: %w", err)
	}

	return data, nil
}

const filesListUnlimited = "1000000"

func (c client) listFiles(
	ctx context.Context,
	commit string,
) ([]string, error) {
	type filesList struct {
		Values        []string `json:"values"`
		Size          int      `json:"size"`
		IsLastPage    bool     `json:"isLastPage"`
		Start         int      `json:"start"`
		Limit         int      `json:"limit"`
		NextPageStart int      `json:"nextPageStart"`
	}

	list, err := httpGetJSON[filesList](
		ctx,
		c.client,
		tmplGetFilesList,
		nil,
		qeryMap{
			"at":    commit,
			"limit": filesListUnlimited,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("getting files list: %w", err)
	}

	return list.Values, nil
}
