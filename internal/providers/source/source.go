package source

import (
	"context"

	"github.com/easyp-tech/server/internal/providers/content"
)

type Source interface {
	GetMeta(ctx context.Context, commit string) (content.Meta, error)
	GetFiles(ctx context.Context, commit string) ([]content.File, error)
	ConfigHash() string
	Name() string
	Owner() string
	RepoName() string
	Type() string
}
