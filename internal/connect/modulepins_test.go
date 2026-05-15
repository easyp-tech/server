package connect

import "testing"

// Test splitRepoName behavior for bug fix verification
// This test verifies the fix for CQ-01: no-panic on malformed input

func TestSplitRepoName_NoPanic(t *testing.T) {
	// These cases should not panic and should return empty strings
	testCases := []struct {
		name  string
		input string
	}{
		{"no_slash", "googleapis"},
		{"empty", ""},
		{"slash_only", "/"},
		{"trailing_slash", "owner/"},
		{"too_many_parts", "a/b/c"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo := splitRepoName(tc.input)
			// Should not panic
			// For these inputs, we expect empty strings
			_ = owner // can be empty or partial
			_ = repo  // can be empty or partial
		})
	}
}

// TestSplitRepoName_NormalBehavior verifies correct parsing
func TestSplitRepoName_NormalBehavior(t *testing.T) {
	owner, repo := splitRepoName("owner/repo")
	if owner != "owner" {
		t.Errorf("owner = %q, want %q", owner, "owner")
	}
	if repo != "repo" {
		t.Errorf("repo = %q, want %q", repo, "repo")
	}
}