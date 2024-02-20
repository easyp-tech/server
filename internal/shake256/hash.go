package shake256

import (
	"fmt"

	"golang.org/x/crypto/sha3"
)

type Hash [64]byte

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
