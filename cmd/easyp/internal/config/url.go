package config

import (
	"fmt"
	"net/url"
)

type URL struct {
	url.URL
}

func (u *URL) UnmarshalText(text []byte) error {
	p, err := url.Parse(string(text))
	if err != nil {
		return fmt.Errorf("parsing URL: %w", err)
	}

	u.URL = *p

	return nil
}
