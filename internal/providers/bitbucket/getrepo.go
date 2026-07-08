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

// isConventionalDefaultName reports whether s is a well-known default
// branch or label name. Used by the getMeta carve-out to handle the
// v1.30.1 v1alpha1 case where the buf CLI sends the buf default label
// name as the reference, even when it doesn't match the repo's actual
// default branch name (e.g., googleapis/googleapis has default branch
// "master" but the buf default label is "main", so the v1.30.1 client
// sends reference="main" via modulepins.go:46). The set covers the
// conventional default names in use across the git ecosystem since
// ~2010: "main" (modern), "master" (legacy), "develop" (git-flow),
// "trunk" (subversion-style). Returning HEAD for any of these is a
// reasonable heuristic for the v1.30.1 case (the client wants the
// default label's HEAD; the proxy doesn't have label resolution, so
// the default branch's HEAD is the best approximation). The risk —
// a repo with a non-conventional default (e.g., "production") and a
// branch named "main" — returns HEAD instead of the branch's commit,
// but this is the pre-Phase-18 behavior and the v1.30.1 case is the
// common one.
func isConventionalDefaultName(s string) bool {
	switch s {
	case "main", "master", "develop", "trunk":
		return true
	}
	return false
}

func (c client) getMeta(ctx context.Context, commit string) (content.Meta, error) {
	meta, err := c.getRepo(ctx)
	if err != nil {
		return meta, fmt.Errorf("investigating: %w", err)
	}

	// Four branches:
	//   - commit == "":        no ref was supplied; keep HEAD from getRepo.
	//   - commit == DefaultBranch || isConventionalDefaultName(commit):
	//                          client asked for a well-known default-
	//                          branch/label name (the v1.30.1 v1alpha1
	//                          case: client sends reference="main" even
	//                          when the repo's default is "master");
	//                          keep HEAD from getRepo, no /commits/ call.
	//   - isSHA(commit):       raw SHA fast path (40/64 lowercase hex).
	//   - non-SHA non-empty:   treat as a ref and resolve via the
	//                          provider's commit-fetch API. The previous
	//                          `commit != "main"` carve-out silently stamped
	//                          the ref into meta.Commit without resolving,
	//                          which both ignored refs and broke
	//                          40/64-char SHAs that weren't at HEAD.
	if commit != "" {
		if commit == meta.DefaultBranch || isConventionalDefaultName(commit) {
			// Client asked for the default branch (or a well-known
			// default-name like "main" / "master" / "develop" /
			// "trunk") by name. meta.Commit already holds the default
			// branch's HEAD SHA (set by getRepo from
			// repo.LatestCommit), so we can return without a second
			// round-trip. Without this carve-out, c.getCommit(ctx,
			// "main") hits Bitbucket's /commits/main endpoint, which
			// expects a SHA and rejects branch names with 404 (this is
			// the v1.30.1 v1alpha1 path: the client sends
			// reference="main" via modulepins.go:46, even when the
			// repo's default branch is "master").
		} else if isSHA(commit) {
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
