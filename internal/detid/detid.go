// Package detid computes the buf-style deterministic id for a module
// (or for a resolved commit hash).
//
// It is shared by internal/connect (handler) and internal/providers/multisource
// (upstream trace), neither of which can import the other without creating
// a cycle. Keeping it in its own leaf package avoids the cycle.
package detid

import "fmt"

// DeterministicID hashes input into the 32-hex-char id buf uses to identify
// modules and commits in the wire protocol. It is not cryptographic; it is
// stable, deterministic, and matches buf's deterministicID function so that
// commit ids minted by this proxy round-trip through buf CLI invocations.
func DeterministicID(input string) string {
	var h uint64
	for _, c := range input {
		h = h*31 + uint64(c)
	}
	return fmt.Sprintf("%016x%016x", h, h^0xdeadbeefcafebabe)
}
