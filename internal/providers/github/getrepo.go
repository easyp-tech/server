package github

import (
	"context"
	"errors"
	"fmt"
	"time"

	"log/slog"

	connectpkg "github.com/easyp-tech/server/internal/connect"
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
// branch or label name. Used by the GetMeta carve-out to handle the
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

func (c client) GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error) {
	meta, err := c.getRepo(ctx, owner, repoName)
	if err != nil {
		return meta, fmt.Errorf("investigating %q/%q: %w", owner, repoName, err)
	}

	// Four branches:
	//   - commit == "":        no ref was supplied; keep HEAD from getRepo.
	//   - commit == DefaultBranch || isConventionalDefaultName(commit):
	//                          client asked for a well-known default-
	//                          branch/label name (the v1.30.1 v1alpha1
	//                          case: client sends reference="main" even
	//                          when the repo's default is "master");
	//                          keep HEAD from getRepo, no repos.GetCommit.
	//   - isSHA(commit):       raw SHA fast path (40/64 lowercase hex).
	//   - non-SHA non-empty:   treat as a ref and resolve via GitHub's
	//                          repos.GetCommit (accepts ref names, short
	//                          SHAs >= 7 chars, and full SHAs). The
	//                          previous `commit != "main"` carve-out
	//                          silently stamped the ref into meta.Commit
	//                          without resolving, which both ignored refs
	//                          and broke 40/64-char SHAs that weren't at
	//                          HEAD.
	if commit != "" {
		if commit == meta.DefaultBranch || isConventionalDefaultName(commit) {
			// Client asked for the default branch (or a well-known
			// default-name like "main" / "master" / "develop" /
			// "trunk") by name. meta.Commit already holds the default
			// branch's HEAD SHA (set by getRepo via repos.GetBranch),
			// so we can return without a second round-trip. Without
			// this carve-out, repos.GetCommit(ctx, owner, repoName,
			// "main", nil) hits GitHub's /commits/main endpoint, which
			// expects a SHA and rejects branch names with 422 "No
			// commit found for SHA: main" (this is the v1.30.1
			// v1alpha1 path: the client sends reference="main" via
			// modulepins.go:46, even when the repo's default branch
			// is "master").
		} else if isSHA(commit) {
			meta.Commit = commit
		} else {
			rc, _, err := c.repos.GetCommit(ctx, owner, repoName, commit, nil)
			if err != nil {
				return meta, fmt.Errorf("resolving ref %q: %w", commit, err)
			}
			meta.Commit = rc.GetSHA()
		}
	}

	return meta, nil
}

var ErrEmpty = errors.New("empty")

func (c client) getRepo(ctx context.Context, owner, repoName string) (content.Meta, error) {
	var out content.Meta
	reqID := connectpkg.RequestIDFrom(ctx)

	start := time.Now()
	c.log.DebugContext(ctx, "github getRepo start",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("request_id", reqID),
	)
	repo, _, err := c.repos.Get(ctx, owner, repoName)
	dur := time.Since(start)
	if err != nil {
		c.log.LogAttrs(ctx, slog.LevelDebug, "github getRepo failed",
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("request_id", reqID),
			slog.Duration("duration", dur),
			slog.String("error", err.Error()),
		)
		return out, fmt.Errorf("resolving default branch: %w", err)
	}
	c.log.LogAttrs(ctx, slog.LevelDebug, "github getRepo completed",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("request_id", reqID),
		slog.Duration("duration", dur),
		slog.String("default_branch", repo.GetDefaultBranch()),
	)

	out.CreatedAt = safeTime(repo.CreatedAt.GetTime())
	out.UpdatedAt = safeTime(repo.UpdatedAt.GetTime())

	out.DefaultBranch = repo.GetDefaultBranch()
	if out.DefaultBranch == "" {
		return out, fmt.Errorf("error getting default branch: %w", ErrEmpty)
	}

	start = time.Now()
	c.log.DebugContext(ctx, "github getBranch start",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("branch", out.DefaultBranch),
		slog.String("request_id", reqID),
	)
	branch, _, err := c.repos.GetBranch(ctx, owner, repoName, out.DefaultBranch, MaxRedirects)
	dur = time.Since(start)
	if err != nil {
		c.log.LogAttrs(ctx, slog.LevelDebug, "github getBranch failed",
			slog.String("owner", owner),
			slog.String("repo", repoName),
			slog.String("branch", out.DefaultBranch),
			slog.String("request_id", reqID),
			slog.Duration("duration", dur),
			slog.String("error", err.Error()),
		)
		return out, fmt.Errorf("investigating branch %q: %w", out.DefaultBranch, err)
	}
	c.log.LogAttrs(ctx, slog.LevelDebug, "github getBranch completed",
		slog.String("owner", owner),
		slog.String("repo", repoName),
		slog.String("branch", out.DefaultBranch),
		slog.String("sha", branch.GetCommit().GetSHA()),
		slog.String("request_id", reqID),
		slog.Duration("duration", dur),
	)

	out.Commit = branch.GetCommit().GetSHA()

	return out, nil
}

func safeTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}

	return *v
}
