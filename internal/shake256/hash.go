package shake256

import (
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/sha3"
)

type Hash [64]byte

func (h *Hash) String() string               { return hex.EncodeToString(h[:]) }
func (h *Hash) MarshalText() ([]byte, error) { return []byte(hex.EncodeToString(h[:])), nil }

func (h *Hash) UnmarshalText(text []byte) error {
	_, err := hex.Decode(h[:], text)

	return err
}

func SHA3Shake256(data []byte) (Hash, error) {
	var hash Hash

	d := sha3.NewShake256()

	if _, err := d.Write(data); err != nil {
		return hash, fmt.Errorf("calculating hash: %w", err)
	}

	if _, err := d.Read(hash[:]); err != nil {
		return hash, fmt.Errorf("extracting hash: %w", err)
	}

	return hash, nil
}
