package content

// IsSHA reports whether s is a 40-char (SHA-1) or 64-char (SHA-256)
// lowercase hex string. This is the shared provider-layer definition
// extracted from the duplicated copies in github/getrepo.go and
// bitbucket/getrepo.go. The connect-layer isSHA in
// internal/connect/commits_helpers.go is NOT consolidated because it
// serves a different domain (UUID-derivation gating vs. provider-layer
// commit-vs-ref gating).
func IsSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// IsConventionalDefaultName reports whether s is a well-known default
// branch or label name ("main", "master", "develop", "trunk"). It was
// preserved for future use even though its original caller (the GetMeta
// carve-out in github/getrepo.go and bitbucket/getrepo.go) was removed
// in Phase 25.
func IsConventionalDefaultName(s string) bool {
	switch s {
	case "main", "master", "develop", "trunk":
		return true
	}
	return false
}
