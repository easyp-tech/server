package github

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/easyp-tech/server/internal/providers/content"
)

func (c client) GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error) {
	meta, err := c.getRepo(ctx, owner, repoName)
	if err != nil {
		return meta, fmt.Errorf("investigating %q/%q: %w", owner, repoName, err)
	}

	if commit != "" {
		meta.Commit = commit
	}

	if _, _, err = c.repos.GetCommit(ctx, owner, repoName, meta.Commit, nil); err != nil {
		return meta, fmt.Errorf("investigating %q/%q:%q: %w", owner, repoName, meta.Commit, err)
	}

	return meta, nil
}

var ErrEmpty = errors.New("empty")

func (c client) getRepo(ctx context.Context, owner, repoName string) (content.Meta, error) {
	out := content.Meta{}

	repo, _, err := c.repos.Get(ctx, owner, repoName)
	if err != nil {
		return out, fmt.Errorf("resolving default branch: %w", err)
	}

	out.CreatedAt = safeTime(repo.CreatedAt.GetTime())
	out.UpdatedAt = safeTime(repo.UpdatedAt.GetTime())

	out.DefaultBranch = repo.GetDefaultBranch()
	if out.DefaultBranch == "" {
		return out, fmt.Errorf("error getting default branch: %w", ErrEmpty)
	}

	branch, _, err := c.repos.GetBranch(ctx, owner, repoName, out.DefaultBranch, MaxRedirects)
	if err != nil {
		return out, fmt.Errorf("investigating branch %q: %w", out.DefaultBranch, err)
	}

	out.Commit = branch.GetCommit().GetSHA()

	return out, nil
}

func safeTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}

	return *v
}
