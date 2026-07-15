package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	connectpkg "github.com/easyp-tech/server/internal/connect"
	"github.com/easyp-tech/server/internal/providers/content"
)

func (c client) GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error) {
	meta, err := c.getRepo(ctx, owner, repoName)
	if err != nil {
		return meta, fmt.Errorf("investigating %q/%q: %w", owner, repoName, err)
	}

	// Four branches:
	//   - commit == "":              no ref was supplied; keep HEAD from getRepo.
	//   - commit == DefaultBranch || content.IsConventionalDefaultName(commit):
	//                              client asked for the default branch by name,
	//                              or the v1.30.1 v1alpha1 path sent a well-known
	//                              default label (main/master/develop/trunk);
	//                              meta.Commit already holds HEAD.
	//   - content.IsSHA(commit):     raw SHA fast path (40/64 lowercase hex).
	//   - non-SHA non-empty:         treat as a ref and resolve via the provider's
	//                                repos.GetCommit (accepts ref names, short
	//                                SHAs >= 7 chars, and full SHAs).
	//
	// Note: The isConventionalDefaultName carve-out was extracted to
	// internal/providers/content/helpers.go in Phase 25 but the condition
	// was RETAINED because the v1alpha1 ResolveService handler (modulepins.go:
	// GetModulePins) is NOT gated by parseResourceRefName and still passes
	// label_name="main" (proto field 3) as commit="main" to GetMeta. Only the
	// v1beta1 CommitService path is guarded by parseResourceRefName (Phase 18).
	// If the v1alpha1 ResolveService path is also updated to ignore label_name,
	// this carve-out can be removed.
	if commit != "" {
		if commit == meta.DefaultBranch || content.IsConventionalDefaultName(commit) {
			// commit is either the actual default branch name or a well-known
			// default label (main/master/develop/trunk) that the v1.30.1
			// v1alpha1 path passes. meta.Commit already holds HEAD (set by
			// getRepo via repos.GetBranch).
		} else if content.IsSHA(commit) {
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
