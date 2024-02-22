package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/easyp-tech/server/internal/providers/content"
)

func (c client) getMeta(ctx context.Context, commit string) (content.Meta, error) {
	meta, err := c.getRepo(ctx)
	if err != nil {
		return meta, fmt.Errorf("investigating: %w", err)
	}

	if commit != "" && commit != "main" {
		meta.Commit = commit
	}

	return meta, nil
}

var ErrEmpty = errors.New("empty")

func (c client) getRepo(ctx context.Context) (content.Meta, error) {
	var out content.Meta

	repo, err := c.searchRepo(ctx)
	if err != nil {
		return out, fmt.Errorf("searching repo: %w", err)
	}

	if repo.DisplayID == "" {
		return out, fmt.Errorf("error getting default branch: %w", ErrEmpty)
	}

	out.DefaultBranch = repo.DisplayID
	out.Commit = repo.LatestCommit
	out.CreatedAt = time.Now()
	out.UpdatedAt = out.CreatedAt

	return out, nil
}

type repoInfo struct {
	ID              string `json:"id"`
	DisplayID       string `json:"displayId"`
	Type            string `json:"type"`
	LatestCommit    string `json:"latestCommit"`
	LatestChangeset string `json:"latestChangeset"`
	IsDefault       bool   `json:"isDefault"`
}

func (c client) searchRepo(ctx context.Context) (repoInfo, error) {
	branchInfo, err := httpGetJSON[repoInfo](
		ctx,
		c.client,
		tmplGetDefaultBranch,
		nil,
		nil,
	)
	if err != nil {
		return branchInfo, fmt.Errorf("getting default branch: %w", err)
	}

	return branchInfo, nil
}
