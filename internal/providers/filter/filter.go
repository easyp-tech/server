package filter

import (
	"fmt"
	"hash/crc32"
	"strings"

	"golang.org/x/exp/slices"
)

const ProtoSuffix = ".proto"

type Repo struct {
	Owner    string
	Name     string
	Prefixes []string
	Paths    []string
}

func FindRepo(owner, repoName string, repos []Repo) Repo {
	i := slices.IndexFunc(repos, func(repo Repo) bool { return repo.Owner == owner && repo.Name == repoName })
	if i < 0 {
		return Repo{} //nolint:exhaustruct
	}

	return repos[i]
}

func (r Repo) Hash() string {
	return fmt.Sprintf("%X", crc32.ChecksumIEEE([]byte(fmt.Sprintf("%+v", r))))
}

func (r Repo) Check(fileName string) (string, bool) {
	fileName = checkPrefix(fileName, r.Prefixes)
	if fileName == "" {
		return "", false
	}

	if !checkPath(fileName, r.Paths) {
		return "", false
	}

	if !strings.HasSuffix(fileName, ProtoSuffix) {
		return "", false
	}

	return fileName, true
}

func checkPrefix(fileName string, prefixes []string) string {
	if len(prefixes) == 0 {
		return fileName
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(fileName, prefix) {
			return strings.TrimPrefix(fileName, prefix)
		}
	}

	return ""
}

func checkPath(name string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}

	for _, path := range paths {
		if strings.HasPrefix(name, path) {
			return true
		}
	}

	return false
}
