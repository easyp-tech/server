package core

import (
	"context"
)

// GetRepository returns repository by request.
func (c *Core) GetRepository(ctx context.Context, request GetRequest) (*Repository, error) {
	return c.store.Get(ctx, request)
}
