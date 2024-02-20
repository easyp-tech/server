package content

import (
	"time"

	"github.com/easyp-tech/server/internal/shake256"
)

type Meta struct {
	Commit        string
	DefaultBranch string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type File struct {
	Path string
	Data []byte
	Hash shake256.Hash
}
