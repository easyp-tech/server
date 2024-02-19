package localgit

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/exp/slices"

	"github.com/easyp-tech/server/internal/content"
	"github.com/easyp-tech/server/internal/namedlocks"
	"github.com/easyp-tech/server/internal/shake256"
)

type Lock interface {
	Lock(name string) namedlocks.Unlocker
}

type store struct {
	rootDir string
	l       Lock
}

// New returns new instance of store.
func New(rootDir string, l Lock) *store {
	return &store{
		rootDir: rootDir,
		l:       l,
	}
}

// Get implements storage.Store.
func (s *store) Get(owner, repoName, commit string) (content.Meta, error) {
	dirName := path.Join(s.rootDir, owner, repoName)

	l := s.l.Lock(dirName)
	defer l.Unlock()

	defaultBranch, commit, err := getRepo(dirName, commit)
	if err != nil {
		return content.Meta{DefaultBranch: defaultBranch, Commit: commit},
			fmt.Errorf("investigating %q/%q:%q: %w", owner, repoName, commit, err)
	}

	return content.Meta{DefaultBranch: defaultBranch, Commit: commit}, nil
}

// Get implements storage.Store.
func (s *store) GetWithFiles(owner, repoName, commit string) (content.Meta, []content.File, error) {
	dirName := path.Join(s.rootDir, owner, repoName)

	l := s.l.Lock(dirName)
	defer l.Unlock()

	defaultBranch, commit, err := getRepo(dirName, commit)
	if err != nil {
		return content.Meta{DefaultBranch: defaultBranch, Commit: commit}, nil,
			fmt.Errorf("investigating %q/%q:%q: %w", owner, repoName, commit, err)
	}

	files, err := enumerateProto(dirName)
	if err != nil {
		return content.Meta{DefaultBranch: defaultBranch, Commit: commit}, files,
			fmt.Errorf("enumerating %q/%q:%q: %w", owner, repoName, commit, err)
	}

	return content.Meta{DefaultBranch: defaultBranch, Commit: commit}, files, nil
}

func getRepo(dirName, commit string) (string, string, error) {
	r, err := git.PlainOpen(dirName)
	if err != nil {
		return "", "", fmt.Errorf("opening git: %w", err)
	}

	defaultBranch, err := r.Reference(plumbing.NewRemoteHEADReferenceName("origin"), true)
	if err != nil {
		return "", "", fmt.Errorf("resolving default branch: %w", err)
	}

	if commit == "" {
		commit = defaultBranch.Hash().String()
	}

	w, err := r.Worktree()
	if err != nil {
		return "", "", fmt.Errorf("getting work tree: %w", err)
	}

	if err = w.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(commit)}); err != nil {
		return "", "", fmt.Errorf("checking out %q: %w", commit, err)
	}

	return defaultBranch.Name().Short(), commit, nil

}

func enumerateProto(dirName string) ([]content.File, error) {
	res := make([]content.File, 0, 1024)

	fsys := os.DirFS(dirName)

	err := fs.WalkDir(
		fsys,
		".",
		func(path string, info fs.DirEntry, err error) error {
			if err != nil || info.IsDir() || filepath.Ext(path) != ".proto" {
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

			res = append(res, content.File{Path: path, Data: data, Hash: hash})

			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("walking %q: %w", dirName, err)
	}

	slices.SortFunc(res, func(a, b content.File) int { return strings.Compare(a.Path, b.Path) })

	return res, nil
}
