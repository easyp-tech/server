package localgit

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"slices"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/easyp-tech/server/internal/detid"
	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/easyp-tech/server/internal/providers/localgit/namedlocks"
	"github.com/easyp-tech/server/internal/providers/source"
	"github.com/easyp-tech/server/internal/reqid"
	"github.com/easyp-tech/server/internal/shake256"
)

const (
	minNumberOfFiles = 1024
)

type namedLocks interface {
	Lock(name string) *namedlocks.Unlocker
}

type store struct {
	rootDir string
	repos   []filter.Repo
	l       namedLocks
}

func (s *store) Find(owner, repoName string) source.Source {
	if s.rootDir == "" {
		return nil
	}

	dirName := path.Join(s.rootDir, owner, repoName)

	fileStat, err := os.Stat(filepath.Join(s.rootDir, owner, repoName))
	if err != nil || !fileStat.IsDir() {
		return nil
	}

	repo := filter.FindRepo(owner, repoName, s.repos)
	if repo.Owner != owner {
		return nil
	}

	return sourceRepo{
		dirName: dirName,
		repo:    repo,
		l:       s.l,
	}
}

func (s *store) Repositories() []source.Source {
	repos := make([]source.Source, 0, len(s.repos))
	for _, r := range s.repos {
		dirName := path.Join(s.rootDir, r.Owner, r.Name)
		if fileStat, err := os.Stat(dirName); err == nil && fileStat.IsDir() {
			repos = append(repos, sourceRepo{
				dirName: dirName,
				repo:    r,
				l:       s.l,
			})
		}
	}
	return repos
}

func (s *store) Check(owner, repoName string) bool {
	if s.rootDir == "" {
		return false
	}

	fileStat, err := os.Stat(filepath.Join(s.rootDir, owner, repoName))
	if err != nil {
		return false
	}

	return fileStat.IsDir()
}

// New returns new instance of store.
func New(
	rootDir string,
	repos []filter.Repo,
	l namedLocks,
) *store {
	return &store{
		rootDir: rootDir,
		repos:   repos,
		l:       l,
	}
}

var _ source.Source = sourceRepo{} //nolint:exhaustruct

type sourceRepo struct {
	dirName string
	repo    filter.Repo
	l       namedLocks
}

func (r sourceRepo) ConfigHash() string {
	return r.repo.Hash()
}

func (r sourceRepo) Name() string     { return "local git" }
func (r sourceRepo) Owner() string    { return r.repo.Owner }
func (r sourceRepo) RepoName() string { return r.repo.Name }
func (r sourceRepo) Type() string     { return "local" }

func (r sourceRepo) GetMeta(ctx context.Context, commit string) (content.Meta, error) {
	l := r.l.Lock(r.dirName)
	defer l.Unlock()

	r.trace(ctx, slog.LevelInfo, "upstream call",
		slog.String("target", "localgit.GetMeta"),
		slog.String("owner", r.repo.Owner),
		slog.String("repo", r.repo.Name),
		slog.String("module", r.repo.Name),
		slog.String("commit", commit),
		slog.String("commit_id", detid.DeterministicID(commit)),
	)
	start := time.Now()

	defaultBranch, resolvedCommit, err := getRepoSwitchedCommit(r.dirName, commit)
	if err != nil {
		r.trace(ctx, slog.LevelWarn, "upstream result",
			slog.String("target", "localgit.GetMeta"),
			slog.String("owner", r.repo.Owner),
			slog.String("repo", r.repo.Name),
			slog.String("module", r.repo.Name),
			slog.String("commit", commit),
			slog.String("commit_id", detid.DeterministicID(commit)),
			slog.String("outcome", "error"),
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()),
		)
		return content.Meta{DefaultBranch: defaultBranch, Commit: resolvedCommit}, //nolint:exhaustruct
			fmt.Errorf("investigating %q/%q:%q: %w", r.repo.Owner, r.repo.Name, resolvedCommit, err)
	}

	r.trace(ctx, slog.LevelInfo, "upstream result",
		slog.String("target", "localgit.GetMeta"),
		slog.String("owner", r.repo.Owner),
		slog.String("repo", r.repo.Name),
		slog.String("module", r.repo.Name),
		slog.String("commit", commit),
		slog.String("resolved_commit", resolvedCommit),
		slog.String("commit_id", detid.DeterministicID(resolvedCommit)),
		slog.String("outcome", "ok"),
		slog.String("default_branch", defaultBranch),
		slog.Duration("duration", time.Since(start)),
	)
	return content.Meta{DefaultBranch: defaultBranch, Commit: resolvedCommit}, nil //nolint:exhaustruct
}

func (r sourceRepo) GetFiles(ctx context.Context, commit string) ([]content.File, error) {
	l := r.l.Lock(r.dirName)
	defer l.Unlock()

	r.trace(ctx, slog.LevelInfo, "upstream call",
		slog.String("target", "localgit.GetFiles"),
		slog.String("owner", r.repo.Owner),
		slog.String("repo", r.repo.Name),
		slog.String("module", r.repo.Name),
		slog.String("commit", commit),
		slog.String("commit_id", detid.DeterministicID(commit)),
	)
	start := time.Now()

	if _, _, err := getRepoSwitchedCommit(r.dirName, commit); err != nil {
		r.trace(ctx, slog.LevelWarn, "upstream result",
			slog.String("target", "localgit.GetFiles"),
			slog.String("owner", r.repo.Owner),
			slog.String("repo", r.repo.Name),
			slog.String("module", r.repo.Name),
			slog.String("commit", commit),
			slog.String("commit_id", detid.DeterministicID(commit)),
			slog.String("outcome", "error"),
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("investigating %q/%q:%q: %w", r.repo.Owner, r.repo.Name, commit, err)
	}

	enumStart := time.Now()
	files, err := enumerateProto(r.dirName, r.repo)
	enumLatency := time.Since(enumStart)
	if err != nil {
		r.trace(ctx, slog.LevelWarn, "upstream result",
			slog.String("target", "localgit.GetFiles"),
			slog.String("owner", r.repo.Owner),
			slog.String("repo", r.repo.Name),
			slog.String("module", r.repo.Name),
			slog.String("commit", commit),
			slog.String("commit_id", detid.DeterministicID(commit)),
			slog.String("outcome", "error"),
			slog.Duration("enum_latency", enumLatency),
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()),
		)
		return files, fmt.Errorf("enumerating %q/%q:%q: %w", r.repo.Owner, r.repo.Name, commit, err)
	}

	r.trace(ctx, slog.LevelInfo, "upstream result",
		slog.String("target", "localgit.GetFiles"),
		slog.String("owner", r.repo.Owner),
		slog.String("repo", r.repo.Name),
		slog.String("module", r.repo.Name),
		slog.String("commit", commit),
		slog.String("commit_id", detid.DeterministicID(commit)),
		slog.String("outcome", "ok"),
		slog.Int("files", len(files)),
		slog.Int("bytes", fileBytes(files)),
		slog.Duration("enum_latency", enumLatency),
		slog.Duration("duration", time.Since(start)),
	)
	return files, nil
}

// trace emits a structured log line carrying the per-request correlation id
// when one is present in the context. localgit does not own a logger because
// store is constructed without one; the only context available is the
// request context, so the line is a no-op when there is no request id. The
// point of these traces is to surface in prod logs (which include request_id)
// when a per-request log line names a slow localgit call.
func (r sourceRepo) trace(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	id := reqid.From(ctx)
	if id == "" {
		return
	}
	base := []slog.Attr{slog.String("request_id", id)}
	slog.Default().LogAttrs(ctx, level, msg, append(base, attrs...)...)
}

func fileBytes(files []content.File) int {
	n := 0
	for _, f := range files {
		n += len(f.Data)
	}
	return n
}

func getRepoSwitchedCommit(dirName, commit string) (string, string, error) {
	r, err := git.PlainOpen(dirName)
	if err != nil {
		return "", "", fmt.Errorf("opening git: %w", err)
	}

	defaultBranch, err := r.Reference(plumbing.NewRemoteHEADReferenceName("origin"), true)
	if err != nil {
		return "", "", fmt.Errorf("resolving default branch: %w", err)
	}

	if commit == "" || commit == "main" {
		commit = defaultBranch.Hash().String()
	}

	w, err := r.Worktree()
	if err != nil {
		return "", "", fmt.Errorf("getting work tree: %w", err)
	}

	if err = w.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(commit)}); err != nil { //nolint:exhaustruct
		return "", "", fmt.Errorf("checking out %q: %w", commit, err)
	}

	return defaultBranch.Name().Short(), commit, nil
}

func enumerateProto(dirName string, repo filter.Repo) ([]content.File, error) {
	res := make([]content.File, 0, minNumberOfFiles)

	fsys := os.DirFS(dirName)

	err := fs.WalkDir(
		fsys,
		".",
		func(path string, info fs.DirEntry, err error) error {
			if err != nil || info.IsDir() {
				return nil //nolint:nilerr
			}

			newPath, ok := repo.Check(path)
			if !ok {
				return nil
			}

			data, err := fs.ReadFile(fsys, path)
			if err != nil {
				return fmt.Errorf("reading %q: %w", path, err)
			}

			hash, err := shake256.SHA3Shake256(data)
			if err != nil {
				return fmt.Errorf("hashing %q: %w", path, err)
			}

			res = append(res, content.File{Path: newPath, Data: data, Hash: hash})

			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("walking %q: %w", dirName, err)
	}

	slices.SortFunc(res, func(a, b content.File) int { return strings.Compare(a.Path, b.Path) })

	return res, nil
}
