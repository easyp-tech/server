// Package detid computes the buf-style deterministic id for the resource
// types buf addresses by an opaque server-minted id — namely module and
// owner resources. It is consumed only by internal/connect.
//
// Commit ids are NOT minted here: a commit id is the raw git commit sha
// resolved for the module (see internal/connect ServeCommits/ServeGraph/
// ServeDownload and the commit_id log fields in internal/providers/*).
// Earlier revisions hashed the sha into a 32-hex toy id; that fake
// opaqueness added no value (the sha is already logged in the clear) and
// broke when clients cached ids minted by a different registry, so commit
// ids are now the sha itself.
package detid

import "fmt"

// DeterministicID hashes input into the 32-hex-char id buf uses to identify
// module and owner resources in the wire protocol. It is not cryptographic;
// it is stable and deterministic. It is intentionally NOT used for commit
// ids — those are the raw git commit sha.
func DeterministicID(input string) string {
	var h uint64
	for _, c := range input {
		h = h*31 + uint64(c)
	}
	return fmt.Sprintf("%016x%016x", h, h^0xdeadbeefcafebabe)
}
