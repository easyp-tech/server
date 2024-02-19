package localgit

import (
	"context"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/exp/slices"

	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/easyp-tech/server/internal/providers/localgit/namedlocks"
	"github.com/easyp-tech/server/internal/providers/source"
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

//nolint:ireturn
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
	return fmt.Sprintf("%X", crc32.ChecksumIEEE([]byte(fmt.Sprintf("%+v", r.repo))))
}

func (r sourceRepo) Name() string { return "local git" }

func (r sourceRepo) GetMeta(_ context.Context, commit string) (content.Meta, error) {
	l := r.l.Lock(r.dirName)
	defer l.Unlock()

	defaultBranch, commit, err := getRepoSwitchedCommit(r.dirName, commit)
	if err != nil {
		return content.Meta{DefaultBranch: defaultBranch, Commit: commit}, //nolint:exhaustruct
			fmt.Errorf("investigating %q/%q:%q: %w", r.repo.Owner, r.repo.Name, commit, err)
	}

	return content.Meta{DefaultBranch: defaultBranch, Commit: commit}, nil //nolint:exhaustruct
}

func (r sourceRepo) GetFiles(_ context.Context, commit string) ([]content.File, error) {
	l := r.l.Lock(r.dirName)
	defer l.Unlock()

	if _, _, err := getRepoSwitchedCommit(r.dirName, commit); err != nil {
		return nil, fmt.Errorf("investigating %q/%q:%q: %w", r.repo.Owner, r.repo.Name, commit, err)
	}

	files, err := enumerateProto(r.dirName, r.repo)
	if err != nil {
		return files, fmt.Errorf("enumerating %q/%q:%q: %w", r.repo.Owner, r.repo.Name, commit, err)
	}

	return files, nil
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
