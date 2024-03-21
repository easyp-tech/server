package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/easyp-tech/server/internal/providers/content"
)

type FileCache struct {
	Dir string
}

func (c FileCache) Get(owner, repoName, commit string) ([]content.File, error) {
	if c.Dir == "" {
		return nil, nil
	}

	fullName := path.Join(c.Dir, owner, repoName, commit+".json")

	data, err := os.ReadFile(fullName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("reading %q: %w", fullName, err)
	}

	var out []content.File

	if err = json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decoding %q: %w", fullName, err)
	}

	return out, nil
}

func (c FileCache) Put(owner, repoName, commit string, in []content.File) error {
	if c.Dir == "" {
		return nil
	}

	fullDir := path.Join(c.Dir, owner, repoName)

	err := os.MkdirAll(fullDir, 0750)
	if err != nil {
		return fmt.Errorf("creating dir %q: %w", fullDir, err)
	}

	var (
		fileName = path.Join(fullDir, commit+".json")
		tmpName  = fileName + ".tmp"
	)

	file, err := os.Create(tmpName)
	if err != nil {
		return fmt.Errorf("creating %q: %w", tmpName, err)
	}

	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err = encoder.Encode(in); err != nil {
		return fmt.Errorf("writing %q: %w", tmpName, err)
	}

	if err = os.Rename(tmpName, fileName); err != nil {
		return fmt.Errorf("renaming %q to %q: %w", tmpName, fileName, err)
	}

	return nil
}
