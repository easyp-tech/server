package bitbucket

import (
	"context"
	"fmt"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/filter"
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

	entries := content.FilterEntries(tree, func(s string) string { return s }, repo)
	files, err := c.getFiles(ctx, commit, entries)
	if err != nil {
		return nil, fmt.Errorf("downloading %q: %w", commit, err)
	}

	return files, nil
}

func (c client) getFiles(
	ctx context.Context,
	commit string,
	entries []content.FileEntry,
) ([]content.File, error) {
	return content.GetFiles(ctx, entries, func(ctx context.Context, orig string) ([]byte, error) {
		return c.getFile(ctx, c.client, commit, orig)
	})
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