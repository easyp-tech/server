package content

import (
	"context"
	"fmt"
	"strings"

	"slices"

	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/easyp-tech/server/internal/shake256"
)

// FileEntry represents a filtered file entry.
// Orig is the original path used for API calls.
// Name is the filtered/rewritten path used for content.File.Path.
type FileEntry struct {
	Orig string
	Name string
}

// GetFiles downloads each entry using downloadFn, hashes the data, and returns
// a sorted content.File slice. The caller provides the downloadFn so this
// helper is provider-agnostic.
func GetFiles(
	ctx context.Context,
	entries []FileEntry,
	downloadFn func(ctx context.Context, orig string) ([]byte, error),
) ([]File, error) {
	out := make([]File, 0, len(entries))

	for _, entry := range entries {
		data, err := downloadFn(ctx, entry.Orig)
		if err != nil {
			return nil, fmt.Errorf("downloading %q: %w", entry.Orig, err)
		}

		hash, err := shake256.SHA3Shake256(data)
		if err != nil {
			return nil, fmt.Errorf("hashing %q: %w", entry.Orig, err)
		}

		out = append(out, File{Path: entry.Name, Data: data, Hash: hash})
	}

	return out, nil
}

// FilterEntries applies repo.Check() to each raw entry and returns a sorted
// FileEntry slice of entries that pass the filter. The getPath function extracts
// the file path string from each entry of type T.
func FilterEntries[T any](
	entries []T,
	getPath func(T) string,
	repo filter.Repo,
) []FileEntry {
	out := make([]FileEntry, 0, len(entries))

	for _, entry := range entries {
		path := getPath(entry)
		if name, ok := repo.Check(path); ok {
			out = append(out, FileEntry{Orig: path, Name: name})
		}
	}

	slices.SortFunc(out, func(a, b FileEntry) int {
		return strings.Compare(a.Name, b.Name)
	})

	return out
}