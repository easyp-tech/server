package filter

import "testing"

func TestRepoHash_Consistent(t *testing.T) {
	repo := Repo{Owner: "test", Name: "test"}
	h1 := repo.Hash()
	h2 := repo.Hash()
	if h1 != h2 {
		t.Errorf("Hash() inconsistent: %q != %q", h1, h2)
	}
}

func TestRepoHash_DifferentForDifferentRepos(t *testing.T) {
	repo1 := Repo{Owner: "a", Name: "b"}
	repo2 := Repo{Owner: "a", Name: "c"}
	h1 := repo1.Hash()
	h2 := repo2.Hash()
	if h1 == h2 {
		t.Errorf("Different repos should have different hashes: %q == %q", h1, h2)
	}
}

func TestRepoHash_UsesCrc32Format(t *testing.T) {
	repo := Repo{Owner: "x", Name: "y"}
	h := repo.Hash()
	if len(h) != 8 {
		t.Errorf("Hash() = %q, want 8 hex chars", h)
	}
}

func TestRepoCheck_Basic(t *testing.T) {
	repo := Repo{
		Owner:    "test",
		Name:     "test",
		Prefixes: []string{"proto/"},
		Paths:    []string{"api/"},
	}

	// Should pass: has .proto suffix, has prefix, has path
	if _, ok := repo.Check("proto/api/service.proto"); !ok {
		t.Errorf("Expected Check to pass for proto/api/service.proto")
	}

	// Should fail: no proto suffix
	if _, ok := repo.Check("proto/api/service.txt"); ok {
		t.Errorf("Expected Check to fail for .txt file")
	}

	// Should fail: wrong path
	if _, ok := repo.Check("proto/other/service.proto"); ok {
		t.Errorf("Expected Check to fail for wrong path")
	}
}