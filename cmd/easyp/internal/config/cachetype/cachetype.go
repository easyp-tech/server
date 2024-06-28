package cachetype

import (
	"errors"
	"fmt"
)

type Type string

const (
	None        Type = "none"
	Local       Type = "local"
	Artifactory Type = "artifactory"
)

var ErrInvalidType = errors.New("invalid type")

func (v *Type) UnmarshalText(text []byte) error {
	switch Type(text) {
	case None:
		*v = None
	case Local:
		*v = Local
	case Artifactory:
		*v = Artifactory
	default:
		return fmt.Errorf("%q: %w", text, ErrInvalidType)
	}

	return nil
}
