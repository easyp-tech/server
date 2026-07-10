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

	// Four branches:
	//   - commit == "":              no ref was supplied; keep HEAD from getRepo.
	//   - commit == DefaultBranch:   client asked for the default branch by name;
	//                                meta.Commit already holds HEAD.
	//   - content.IsSHA(commit):     raw SHA fast path (40/64 lowercase hex).
	//   - non-SHA non-empty:         treat as a ref and resolve via the provider's
	//                                /commits endpoint (accepts ref names, short
	//                                SHAs >= 7 chars, and full SHAs).
	//
	// Note: The isConventionalDefaultName carve-out (main/master/develop/trunk)
	// was REMOVED in Phase 25. It was originally added to handle the v1.30.1
	// v1alpha1 case where buf sent reference="main" via label_name=3. Phase 18
	// changed parseResourceRefName to read proto field 4 only, so label_name=3
	// is silently ignored and ref="" falls through to the HEAD path. The carve-out
	// is no longer reached. Its risk -- silently returning HEAD for a real branch
	// named one of the conventional labels -- outweighed its benefit.
	if commit != "" {
		if commit == meta.DefaultBranch {
			// Client asked for the default branch by name; meta.Commit already
			// holds HEAD (set by getRepo from repo.LatestCommit).
		} else if content.IsSHA(commit) {
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