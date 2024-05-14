package cache

import (
	"context"

	"github.com/easyp-tech/server/internal/providers/content"
)

type Noop struct{}

func (c Noop) Get(_ context.Context, _, _, _, _ string) ([]content.File, error) {
	return nil, nil
}

func (c Noop) Put(_ context.Context, _, _, _, _ string, _ []content.File) error {
	return nil
}
