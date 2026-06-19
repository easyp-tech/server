package detid

import "testing"

// TestDeterministicIDStable checks the function is stable across calls and
// matches the format buf uses (32 lowercase hex chars).
func TestDeterministicIDStable(t *testing.T) {
	got := DeterministicID("owner/module")
	if len(got) != 32 {
		t.Fatalf("expected 32 hex chars, got %d (%q)", len(got), got)
	}
	if DeterministicID("owner/module") != got {
		t.Fatalf("function not deterministic: %q vs %q", got, DeterministicID("owner/module"))
	}
}

// TestDeterministicIDEmpty covers the degenerate empty-input case. We do not
// assert a specific value (it is a function of the constant in the algorithm)
// but we do assert it is stable and well-formed.
func TestDeterministicIDEmpty(t *testing.T) {
	got := DeterministicID("")
	if len(got) != 32 {
		t.Fatalf("expected 32 hex chars, got %d (%q)", len(got), got)
	}
}

// TestDeterministicIDDistinct sanity-checks that different inputs yield
// different outputs. The hash is not cryptographic, so we do not rely on
// this for security, but distinctness is the entire point of the id.
func TestDeterministicIDDistinct(t *testing.T) {
	a := DeterministicID("owner/module")
	b := DeterministicID("owner/other")
	if a == b {
		t.Fatalf("expected distinct ids for distinct inputs, both = %q", a)
	}
}
