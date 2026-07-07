package connect

import (
	"encoding/hex"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

// TestCommitUUIDFormat locks in the wire format buf v1.69.0 requires.
// buf.util.FromDashless (private/pkg/uuidutil/uuidutil.go) parses the
// commit id by:
//  1. Asserting length == 32
//  2. Inserting dashes at the standard positions
//  3. Calling uuid.Parse, which validates the version and variant bits
//
// A regression that lets the raw 40-char git SHA leak through fails
// step 1 with the message "expected dashless uuid to be of length 32
// but was 40"; a regression that produces a 32-char hex string without
// the version/variant stamps fails step 3 with a parse error. Both
// shapes must be guarded.
func TestCommitUUIDFormat(t *testing.T) {
	const sha = "81353411f7b010d5b9ebeb1899066aac18a36701"
	const want = "81353411f7b0401080d5b9ebeb189906"
	got, err := commitUUID(sha)
	if err != nil {
		t.Fatalf("commitUUID(%q) unexpected error: %v", sha, err)
	}
	if got != want {
		t.Fatalf("commitUUID(%q) = %q, want %q", sha, got, want)
	}

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
	first, err := commitUUID(sha)
	if err != nil {
		t.Fatalf("commitUUID(%q) unexpected error: %v", sha, err)
	}
	for i := 0; i < 100; i++ {
		got, err := commitUUID(sha)
		if err != nil {
			t.Fatalf("commitUUID(%q) iter=%d unexpected error: %v", sha, i, err)
		}
		if got != first {
			t.Fatalf("commitUUID(%q) drift: first=%q iter=%d=%q", sha, first, i, got)
		}
	}
}

// TestCommitUUIDDistinct ensures distinct SHAs mint distinct UUIDs.
// A regression that narrowed the id to fewer bits (or a bug in the
// version/variant stamping) would surface as a collision here. The
// current scheme is injective on the first 14 bytes of the SHA — the
// last 6 bytes are not represented in the id — so any two SHAs that
// agree in their first 14 bytes will collide by design. The fixtures
// below are chosen so their first 14 bytes all differ.
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
		u, err := commitUUID(s)
		if err != nil {
			t.Fatalf("commitUUID(%q) unexpected error: %v", s, err)
		}
		if prev, ok := seen[u]; ok {
			t.Fatalf("UUID collision: %q and %q both minted %q", prev, s, u)
		}
		seen[u] = s
	}
}

// TestCommitUUID_KnownSHA locks in the exact UUID produced for several
// representative SHAs. The 14-byte SHA-prefix -> 16-byte UUID mapping
// must be byte-exact: any off-by-one in the position table (e.g.
// reading sha[6] into result[7] vs result[6]) would surface as a
// mismatch here.
func TestCommitUUID_KnownSHA(t *testing.T) {
	cases := []struct {
		name string
		sha  string
		want string
	}{
		{
			name: "non-zero SHA with all-14-prefix bytes unique",
			sha:  "81353411f7b010d5b9ebeb1899066aac18a36701",
			want: "81353411f7b0401080d5b9ebeb189906",
		},
		{
			name: "all-zero SHA exercises zero-fill path",
			sha:  "0000000000000000000000000000000000000000",
			want: "00000000000040008000000000000000",
		},
		{
			name: "all-ones SHA exercises max-fill path",
			sha:  "ffffffffffffffffffffffffffffffffffffffff",
			want: "ffffffffffff40ff80ffffffffffffff",
		},
		{
			name: "SHA starting with 0x01 exposes leading-zero handling",
			sha:  "0123456789abcdef0123456789abcdef01234567",
			want: "0123456789ab40cd80ef0123456789ab",
		},
		{
			name: "SHA with deadbeef prefix",
			sha:  "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			want: "deadbeefdead40be80efdeadbeefdead",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := commitUUID(tc.sha)
			if err != nil {
				t.Fatalf("commitUUID(%q) unexpected error: %v", tc.sha, err)
			}
			if got != tc.want {
				t.Fatalf("commitUUID(%q) = %q, want %q", tc.sha, got, tc.want)
			}
		})
	}
}

// TestCommitUUID_SHA256_KnownSHA locks in the SHA-256 path added to
// commitUUID so Bitbucket Server repos that return 64-char hex shas stop
// 500-ing. Each case is paired with the byte-table read positions 0..5,
// 6, 7..13: the function only consumes the first 14 decoded bytes, so a
// 64-char SHA-256 whose first 14 bytes match a known 40-char fixture
// must produce the same UUID. This is the regression-guard against any
// future change that re-introduced a length-string slice ("first 40
// chars of input") instead of reading the decoded buffer.
func TestCommitUUID_SHA256_KnownSHA(t *testing.T) {
	cases := []struct {
		name string
		sha  string
		want string
	}{
		{
			// Same UUID as the 40-char all-zero case — both inputs decode
			// to all-zero in the first 14 byte-table read positions.
			name: "all-zero 64-char SHA-256",
			sha:  strings.Repeat("0", 64),
			want: "00000000000040008000000000000000",
		},
		{
			// Same UUID as the 40-char all-ones case for the same reason
			// — first 14 bytes of decoded input are 0xff.
			name: "all-ones 64-char SHA-256",
			sha:  strings.Repeat("f", 64),
			want: "ffffffffffff40ff80ffffffffffffff",
		},
		{
			// Load-bearing regression-guard: a 64-char input whose first
			// 14 decoded bytes match the 40-char `0123...4567` fixture
			// must produce the same UUID. Proves the function actually
			// consumes bytes from a SHA-256 buffer, not just the first 40
			// chars of the string. The first 14 bytes here are
			// 01 23 45 67 89 ab cd ef 01 23 45 67 89 ab; the trailing 18
			// bytes (36 zero hex chars) are unused by the byte-table.
			name: "64-char SHA-256 with 14-byte prefix matching 40-char fixture",
			sha:  "0123456789abcdef0123456789ab" + strings.Repeat("0", 36),
			want: "0123456789ab40cd80ef0123456789ab",
		},
		{
			// Same idea for the deadbeef fixture: first 14 bytes match,
			// trailing 18 bytes are zero. Same UUID as the 40-char case.
			name: "all-deadbeef 64-char SHA-256",
			sha:  "deadbeefdeadbeefdeadbeefdead" + strings.Repeat("0", 36),
			want: "deadbeefdead40be80efdeadbeefdead",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := commitUUID(tc.sha)
			if err != nil {
				t.Fatalf("commitUUID(%q) unexpected error: %v", tc.sha, err)
			}
			if got != tc.want {
				t.Fatalf("commitUUID(%q) = %q, want %q", tc.sha, got, tc.want)
			}
		})
	}
}

// TestCommitUUID_InvalidInput locks in the strict input contract from
// D-03: commitUUID returns ("", error) for any input that is not
// exactly 40 valid hex characters. Production callers always pass full
// SHAs from upstream GetMeta, so any non-conforming input is a
// contract violation.
func TestCommitUUID_InvalidInput(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "39 chars", in: "81353411f7b010d5b9ebeb1899066aac18a3670"},
		{name: "41 chars", in: "81353411f7b010d5b9ebeb1899066aac18a367011"},
		{name: "40 chars non-hex", in: strings.Repeat("z", 40)},
		{name: "40 chars mixed non-hex", in: "81353411f7b010d5b9ebeb1899066aac18a3670!"},
		{name: "1 char", in: "a"},
		{name: "63 chars", in: strings.Repeat("a", 63)},
		{name: "65 chars", in: strings.Repeat("a", 65)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := commitUUID(tc.in)
			if err == nil {
				t.Fatalf("commitUUID(%q) = %q, want error", tc.in, got)
			}
			if got != "" {
				t.Fatalf("commitUUID(%q) on error path returned non-empty string %q", tc.in, got)
			}
		})
	}
}

// TestCommitUUID_InverseRecovery is the regression-guard against the
// "we accidentally re-hashed it" class of bug. For each SHA, we take
// the UUID produced by commitUUID, decode it back to 16 bytes, and
// verify that the 14 bytes at positions 0..5, 7, 9..15 of the UUID
// match the first 14 bytes of the original SHA. If a future change
// re-introduced a hash step (e.g. SHA-256), the inverse would NOT
// recover the original bytes and this test would fail.
func TestCommitUUID_InverseRecovery(t *testing.T) {
	shas := []string{
		"81353411f7b010d5b9ebeb1899066aac18a36701",
		"0000000000000000000000000000000000000000",
		"ffffffffffffffffffffffffffffffffffffffff",
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	}
	for _, sha := range shas {
		t.Run(sha, func(t *testing.T) {
			uuid, err := commitUUID(sha)
			if err != nil {
				t.Fatalf("commitUUID(%q) unexpected error: %v", sha, err)
			}
			uuidBytes, err := hex.DecodeString(uuid)
			if err != nil {
				t.Fatalf("hex.DecodeString(%q) unexpected error: %v", uuid, err)
			}
			if len(uuidBytes) != 16 {
				t.Fatalf("decoded UUID length = %d, want 16", len(uuidBytes))
			}
			shaBytes, err := hex.DecodeString(sha)
			if err != nil {
				t.Fatalf("hex.DecodeString(%q) unexpected error: %v", sha, err)
			}

			// Inverse: skip uuidBytes[6] (version) and uuidBytes[8] (variant);
			// concatenate the remaining 14 bytes and compare to shaBytes[0..13].
			var recovered [14]byte
			copy(recovered[0:6], uuidBytes[0:6])
			recovered[6] = uuidBytes[7]
			copy(recovered[7:14], uuidBytes[9:16])

			for i := 0; i < 14; i++ {
				if recovered[i] != shaBytes[i] {
					t.Fatalf("inverse recovery failed at byte %d: got 0x%02x, want 0x%02x (sha=%q uuid=%q)",
						i, recovered[i], shaBytes[i], sha, uuid)
				}
			}
		})
	}
}

// preResolveForTest is a test fixture that right-pads a short hex string
// with '0' until it is exactly 40 characters. If the input is already 40
// or more characters, it is returned unchanged. This lets unit tests
// exercise commitUUID with short SHA prefixes (7-byte, 14-byte) that
// production callers never see directly.
func preResolveForTest(short string) string {
	if len(short) >= 40 {
		return short[:40]
	}
	return short + strings.Repeat("0", 40-len(short))
}

// TestIsSHA locks in the SHA-shape check used by the providers to decide
// whether a GetMeta commit arg is a raw SHA (fast path) or a ref to
// resolve. The contract: 40 or 64 lowercase hex characters only — refs
// like "main/v2" and buf-issued UUIDs (32 chars) must both return false.
func TestIsSHA(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "40 lowercase hex", in: strings.Repeat("0", 40), want: true},
		{name: "64 lowercase hex", in: strings.Repeat("0", 64), want: true},
		{name: "32 hex (UUID shape)", in: strings.Repeat("0", 32), want: false},
		{name: "40 hex with trailing non-hex", in: strings.Repeat("0", 39) + "X", want: false},
		{name: "40 hex with one uppercase", in: "81353411f7b010d5b9ebeb1899066aac18a3670A", want: false},
		{name: "41 chars", in: strings.Repeat("0", 41), want: false},
		{name: "39 chars", in: strings.Repeat("0", 39), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSHA(tc.in); got != tc.want {
				t.Fatalf("isSHA(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestIsUUID locks in the 32-lowercase-hex shape check used by
// probeCommitID to detect buf-issued dashless UUID inputs and route them
// through commitUUIDInverse. The contract: exactly 32 lowercase hex
// characters. Empty, 40-char, and non-hex inputs must all return false.
func TestIsUUID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "32 lowercase hex", in: strings.Repeat("0", 32), want: true},
		{name: "40 lowercase hex", in: strings.Repeat("0", 40), want: false},
		{name: "32 hex with trailing non-hex", in: strings.Repeat("0", 31) + "X", want: false},
		{name: "31 chars", in: strings.Repeat("a", 31), want: false},
		{name: "33 chars", in: strings.Repeat("a", 33), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUUID(tc.in); got != tc.want {
				t.Fatalf("isUUID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseResourceRefName_ReadsRef verifies that parseResourceRefName
// captures the ref field (proto field 3) of the buf BSR Name message.
// The buf CLI sends a `ref` to disambiguate which branch/tag the client
// wants; the proxy must return the SHA at that ref, not HEAD. Without
// the field-3 arm, ref inputs would be silently dropped.
func TestParseResourceRefName_ReadsRef(t *testing.T) {
	// Build a Name { owner=1, module=2, ref=3 } message.
	var name []byte
	name = protowire.AppendTag(name, 1, protowire.BytesType)
	name = protowire.AppendString(name, "cyp")
	name = protowire.AppendTag(name, 2, protowire.BytesType)
	name = protowire.AppendString(name, "cyp-net-listeners")
	name = protowire.AppendTag(name, 3, protowire.BytesType)
	name = protowire.AppendString(name, "main/v2")

	ref := parseResourceRefName(name)
	if ref == nil {
		t.Fatal("parseResourceRefName returned nil for a valid Name with owner+module+ref")
	}
	if ref.owner != "cyp" || ref.module != "cyp-net-listeners" || ref.ref != "main/v2" {
		t.Fatalf("parseResourceRefName = %+v, want {owner:cyp module:cyp-net-listeners ref:main/v2}", *ref)
	}
}

// TestParseResourceRefName_NoRef verifies that the ref field is optional
// in the buf BSR Name proto: a Name with only owner+module parses to a
// moduleRef whose ref is the zero value. Older buf clients do not send a
// ref; the ref-aware code paths must tolerate the field being absent.
func TestParseResourceRefName_NoRef(t *testing.T) {
	var name []byte
	name = protowire.AppendTag(name, 1, protowire.BytesType)
	name = protowire.AppendString(name, "cyp")
	name = protowire.AppendTag(name, 2, protowire.BytesType)
	name = protowire.AppendString(name, "cyp-net-listeners")

	ref := parseResourceRefName(name)
	if ref == nil {
		t.Fatal("parseResourceRefName returned nil for a valid Name with owner+module (no ref)")
	}
	if ref.owner != "cyp" || ref.module != "cyp-net-listeners" || ref.ref != "" {
		t.Fatalf("parseResourceRefName = %+v, want {owner:cyp module:cyp-net-listeners ref:}", *ref)
	}
}

// TestPreResolveForTest exercises the test fixture that pads short
// hex strings out to 40 chars. The padding behavior must be exactly
// right-pad-with-'0' and truncate-or-pass-through for inputs of 40+
// characters; otherwise the TestCommitUUID_DistinctFromPaddedShortSHA
// subtests could be silently testing the wrong thing.
func TestPreResolveForTest(t *testing.T) {
	t.Run("empty pads to 40 zeros", func(t *testing.T) {
		got := preResolveForTest("")
		want := strings.Repeat("0", 40)
		if got != want {
			t.Fatalf("preResolveForTest(\"\") = %q, want %q", got, want)
		}
	})

	t.Run("short pads to 40 with trailing zeros", func(t *testing.T) {
		got := preResolveForTest("abc")
		want := "abc" + strings.Repeat("0", 37)
		if got != want {
			t.Fatalf("preResolveForTest(\"abc\") = %q, want %q", got, want)
		}
	})

	t.Run("exact 40 returns unchanged", func(t *testing.T) {
		in := strings.Repeat("a", 40)
		got := preResolveForTest(in)
		if got != in {
			t.Fatalf("preResolveForTest(40x\"a\") = %q, want %q", got, in)
		}
	})

	t.Run("over 40 truncates to 40", func(t *testing.T) {
		in := strings.Repeat("a", 50)
		got := preResolveForTest(in)
		want := strings.Repeat("a", 40)
		if got != want {
			t.Fatalf("preResolveForTest(50x\"a\") = %q, want %q", got, want)
		}
	})

	t.Run("padding does not collide with real SHA", func(t *testing.T) {
		// A short input (first 7 chars) padded with zeros must NOT
		// produce the same UUID as the full 40-char SHA. If it does,
		// the short-SHA pre-resolve path would corrupt the commitMap.
		const fullSHA = "81353411f7b010d5b9ebeb1899066aac18a36701"
		fullUUID, err := commitUUID(fullSHA)
		if err != nil {
			t.Fatalf("commitUUID(%q) unexpected error: %v", fullSHA, err)
		}
		padded := preResolveForTest(fullSHA[:7])
		paddedUUID, err := commitUUID(padded)
		if err != nil {
			t.Fatalf("commitUUID(%q) unexpected error: %v", padded, err)
		}
		if paddedUUID == fullUUID {
			t.Fatalf("padded short SHA %q (from %q) collides with full SHA %q: both minted %q",
				padded, fullSHA[:7], fullSHA, fullUUID)
		}
	})
}
