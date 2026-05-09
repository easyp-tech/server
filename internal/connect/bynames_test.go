package connect

import "testing"

func TestSplitRepoName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
	}{
		{"normal_case", "googleapis/googleapis", "googleapis", "googleapis"},
		{"owner_only", "googleapis", "", ""},
		{"empty_string", "", "", ""},
		{"too_many_parts", "a/b/c", "", ""},
		{"slash_only", "/", "", ""},
		{"trailing_slash", "owner/", "owner", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo := splitRepoName(tc.input)
			if owner != tc.wantOwner || repo != tc.wantRepo {
				t.Errorf("splitRepoName(%q) = (%q, %q), want (%q, %q)",
					tc.input, owner, repo, tc.wantOwner, tc.wantRepo)
			}
		})
	}
}