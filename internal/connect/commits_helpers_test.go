package connect

import (
	"strings"
	"testing"
)

// TestCommitUUIDFormat locks in the wire format buf v1.69.0 requires.
// buf.util.FromDashless (private/pkg/uuidutil/uuidutil.go) parses the
// commit id by:
//   1. Asserting length == 32
//   2. Inserting dashes at the standard positions
//   3. Calling uuid.Parse, which validates the version and variant bits
//
// A regression that lets the raw 40-char git SHA leak through fails
// step 1 with the message "expected dashless uuid to be of length 32
// but was 40"; a regression that produces a 32-char hex string without
// the version/variant stamps fails step 3 with a parse error. Both
// shapes must be guarded.
func TestCommitUUIDFormat(t *testing.T) {
	const sha = "81353411f7b010d5b9ebeb1899066aac18a36701"
	got := commitUUID(sha)

	if len(got) != 32 {
		t.Fatalf("commitUUID(%q) length = %d, want 32 (buf v1.69.0 expects a dashless UUID)", sha, len(got))
	}
	for _, r := range got {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("commitUUID(%q) = %q contains non-lowercase-hex character %q", sha, got, r)
		}
	}

	// Version nibble: 4xxx (random UUID). byte 6 of the 16-byte UUID
	// renders as chars 12-13 of the dashless hex string.
	version := string(got[12])
	if version != "4" {
		t.Fatalf("commitUUID(%q) version nibble = %q, want \"4\" (random UUID)", sha, version)
	}

	// Variant nibble: 8/9/a/b. byte 8 of the 16-byte UUID renders as
	// chars 16-17 of the dashless hex string; we only check the first
	// char (high two bits), which must be one of {8,9,a,b}.
	switch got[16] {
	case '8', '9', 'a', 'b':
	default:
		t.Fatalf("commitUUID(%q) variant high nibble = %q, want one of 8/9/a/b (RFC 4122)", sha, got[16])
	}
}

// TestCommitUUIDDeterminism ensures the same git SHA always maps to the
// same UUID. Without this, buf.lock would hold a UUID that is invalid
// the next time the proxy restarts, breaking incremental workflows that
// pin the lockfile across invocations.
func TestCommitUUIDDeterminism(t *testing.T) {
	const sha = "81353411f7b010d5b9ebeb1899066aac18a36701"
	first := commitUUID(sha)
	for i := 0; i < 100; i++ {
		if got := commitUUID(sha); got != first {
			t.Fatalf("commitUUID(%q) drift: first=%q iter=%d=%q", sha, first, i, got)
		}
	}
}

// TestCommitUUIDDistinct ensures distinct SHAs mint distinct UUIDs.
// SHA-256 has effectively zero collisions for realistic input; a
// regression that narrowed the hash to 64 bits (or a bug in the
// version/variant stamping) would surface as a collision here.
func TestCommitUUIDDistinct(t *testing.T) {
	shas := []string{
		"81353411f7b010d5b9ebeb1899066aac18a36701",
		"0000000000000000000000000000000000000000",
		"ffffffffffffffffffffffffffffffffffffffff",
		"0123456789abcdef0123456789abcdef01234567",
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	}
	seen := make(map[string]string, len(shas))
	for _, s := range shas {
		u := commitUUID(s)
		if prev, ok := seen[u]; ok {
			t.Fatalf("UUID collision: %q and %q both minted %q", prev, s, u)
		}
		seen[u] = s
	}
}

// TestCommitUUIDEmpty guards the empty-input branch. The buf client
// never sends an empty commit id, but the proxy might compute one when
// an upstream returns an empty SHA. Returning "" (rather than a
// zero-UUID) keeps the empty case distinguishable in logs and avoids
// poisoning the commitMap with a junk key.
func TestCommitUUIDEmpty(t *testing.T) {
	if got := commitUUID(""); got != "" {
		t.Fatalf("commitUUID(\"\") = %q, want \"\"", got)
	}
}

// TestCommitUUIDNoLeadingZeroStrip guards against a "feature" where
// hex.EncodeToString might be replaced with a printer that drops
// leading zeros. A SHA whose first byte is < 0x10 would otherwise
// mint a 31-char id and trip buf's length check.
func TestCommitUUIDNoLeadingZeroStrip(t *testing.T) {
	// SHA starting with a hex digit 0..9 is the most likely to expose
	// leading-zero stripping. Compute one via deterministic input.
	const sha = "0123456789abcdef0123456789abcdef01234567"
	got := commitUUID(sha)
	if len(got) != 32 {
		t.Fatalf("commitUUID(%q) length = %d, want 32 (leading zeros stripped?)", sha, len(got))
	}
	if !strings.HasPrefix(got, "0") {
		t.Logf("note: commitUUID(%q) = %q does not start with 0; verify this is the SHA-256 hash, not a stripped version", sha, got)
	}
}
