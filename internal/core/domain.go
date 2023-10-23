package core

import (
	"io/fs"
	"time"
)

type (
	// Repository represents file store.
	Repository struct {
		fs.FS
		Owner      string
		Repository string
		Branch     string
		Commit     string
		CreatedAt  time.Time
		UpdatedAt  time.Time
	}

	// GetRequest contains git info for getting repository.
	GetRequest struct {
		Owner      string
		Repository string
		// If empty use default.
		Branch string
	}
)
