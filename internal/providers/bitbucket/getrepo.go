package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/easyp-tech/server/internal/providers/content"
)

// isSHA reports whether s is a 40-char (SHA-1) or 64-char (SHA-256)
// lowercase hex string. Mirrors the connect-package isSHA used by
// probeCommitID; duplicated here so the provider's commit-vs-ref gate
// does not require exporting a connect-package helper.
func isSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func (c client) getMeta(ctx context.Context, commit string) (content.Meta, error) {
	meta, err := c.getRepo(ctx)
	if err != nil {
		return meta, fmt.Errorf("investigating: %w", err)
	}

	// Three branches:
	//   - commit == "":        no ref was supplied; keep HEAD from getRepo.
	//   - isSHA(commit):       raw SHA fast path (40/64 lowercase hex).
	//   - non-SHA non-empty:   treat as a ref and resolve via the
	//                          provider's commit-fetch API. The previous
	//                          `commit != "main"` carve-out silently stamped
	//                          the ref into meta.Commit without resolving,
	//                          which both ignored refs and broke
	//                          40/64-char SHAs that weren't at HEAD.
	if commit != "" {
		if isSHA(commit) {
			meta.Commit = commit
		} else {
			resolved, err := c.getCommit(ctx, commit)
			if err != nil {
				return meta, fmt.Errorf("resolving ref %q: %w", commit, err)
			}
			meta.Commit = resolved
		}
	}

	return meta, nil
}

// commitInfo is the subset of the Bitbucket Server commits API response
// that getCommit needs. The /commits/{commitId} endpoint returns the
// resolved commit id, the displayId (short sha), and a few metadata
// fields. We only need id.
type commitInfo struct {
	ID          string `json:"id"`
	DisplayID   string `json:"displayId"`
}

// getCommit resolves a ref (branch name, tag, or short sha) to a full
// commit id by calling Bitbucket Server's /commits/{commitId} endpoint.
// Bitbucket accepts a ref or short sha in the path; the response's id
// field is the full 40-char SHA-1 (or 64-char SHA-256 on SHA-256-enabled
// repos). The endpoint returns 404 for unknown refs.
func (c client) getCommit(ctx context.Context, ref string) (string, error) {
	info, err := httpGetJSON[commitInfo](
		ctx,
		c.client,
		tmplGetCommit,
		paramsMap{"id": ref},
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("resolving commit %q: %w", ref, err)
	}
	if info.ID == "" {
		return "", fmt.Errorf("resolving commit %q: empty id in response", ref)
	}
	return info.ID, nil
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
