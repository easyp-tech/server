package shake256

import (
	"fmt"

	"golang.org/x/crypto/sha3"
)

const HashLen = 64

func SHA3Shake256(data []byte) ([HashLen]byte, error) {
	var hash [HashLen]byte

	d := sha3.NewShake256()

	if _, err := d.Write(data); err != nil {
		return hash, fmt.Errorf("calculating hash: %w", err)
	}

	if _, err := d.Read(hash[:]); err != nil {
		return hash, fmt.Errorf("extracting hash: %w", err)
	}

	return hash, nil
}
