package core

import (
	"context"
)

type (
	// Store is provider to storage system.
	Store interface {
		// Get returns repository information.
		// Errors: ErrNotFound, ErrInvalidArgument, unknown.
		Get(context.Context, GetRequest) (*Repository, error)
	}
)
