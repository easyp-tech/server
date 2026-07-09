package e2e

import (
	"os"
	"strings"
	"testing"

	"github.com/easyp-tech/server/e2e/testutil"
	"github.com/stretchr/testify/require"
)

// generatePinnedRef is the ref used by TestGenerateWithPinnedBufLock. It
// is the same googleapis tag used by the Phase 19 ref-honoring tests; the
// test derives the expected UUID at runtime via commitUUIDForTest(sha)
// (not embedded as a literal) so a future byte-table drift in production
// fails the test with a clear got-vs-expected diff.
const generatePinnedRef = "common-protos-1_3_1"

// TestGenerateWithPinnedBufLock exercises a scenario distinct from the
// Phase 19 ref-honoring tests: the proxy must be able to serve a
// "buf generate" request when buf.lock already pins a valid (but
// non-HEAD) commit. The proxy's read-path (the handlers invoked by
// "buf generate" after the lock is loaded by the client) must resolve
// a pre-existing 32-char buf-issued UUID via the provider's
// GetMeta / Download chain and return content for that commit — NOT
// return HEAD and NOT 400 on a "foreign" id.
//
// Matrix: runs for every cached buf version (v1.30.1 v1alpha1 +
// v1.69.0 v1beta1). For v1.69.0 the pinned UUID is the commit the
// proxy must serve (via the post-restart probeCommitID path). For
// v1.30.1 the v1alpha1 client re-resolves via GetModulePins and
// ignores the pinned UUID, so the v1.30.1 subtest verifies the
// v1alpha1 read-path's robustness to a stale buf.lock (a regression
// that 400s on a stale lock is caught).
//
// Token gate: EASYP_GH_TOKEN. With the token unset, the test skips
// cleanly.
func TestGenerateWithPinnedBufLock(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")

	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token

	versions := testutil.AvailableBufVersions(t)
	if len(versions) == 0 {
		t.Skip("no buf binaries cached under testdata/buf/")
	}

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			bufPath := testutil.GetBuf(t, version)
			srv := testutil.StartServer(t, cfg)

			// Look up the upstream SHA at the pinned ref. This is the
			// ground truth we derive the pinned UUID from.
			sha := gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/tags/"+generatePinnedRef)
			if !isLowerHex(sha, 40) {
				t.Fatalf("git ls-remote for ref %q returned %q, expected a 40-char lowercase hex SHA", generatePinnedRef, sha)
			}

			// Derive the buf-issued UUID via the same byte table the
			// proxy uses (see internal/connect/commits_helpers.go
			// commitUUID).
			pinnedUUID := commitUUIDForTest(sha)

			exitCode, stderr, generated := testutil.RunBufGenerateWithPinnedLock(t, bufPath, srv.Port, pinnedUUID)
			if exitCode != 0 {
				t.Fatalf("buf generate failed for %s (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
					version, exitCode, srv.Output.String(), stderr)
			}

			if len(generated) == 0 {
				t.Fatalf("buf generate succeeded for %s but produced no files in gen/go/google/type/. Server output:\n%s",
					version, srv.Output.String())
			}

			for _, f := range generated {
				info, err := os.Stat(f)
				if err != nil {
					t.Fatalf("generated file %s not found: %v", f, err)
				}
				if info.Size() == 0 {
					t.Fatalf("generated file %s is empty (0 bytes)", f)
				}

				content, err := os.ReadFile(f)
				if err != nil {
					t.Fatalf("reading generated file %s: %v", f, err)
				}
				if !strings.Contains(string(content), "package google.type") {
					t.Fatalf("generated file %s does not contain 'package google.type' (regression: proxy served wrong content or plugin produced wrong output)", f)
				}
			}
		})
	}
}

// TestGeneratePinnedCommit_NotHEAD is the Phase 24 e2e gate. It extends
// TestGenerateWithPinnedBufLock with a stronger assertion: not only must
// `buf generate` succeed with a buf.lock-pinned cid, but the proxy must
// actually serve the PINNED commit's content — not HEAD's. This catches
// both prod defects fixed in Phase 24:
//
//  1. ServeGraph forwarding the 32-hex cid to GitHub (422 -> 502) — the
//     generate would fail outright.
//  2. infoCache keyed by owner/module serving HEAD's content under the
//     pinned cid — the generate would "succeed" but emit HEAD's code.
//
// Strategy: pin buf.lock to the cid derived from a known-old tag
// (common-protos-1_3_1) whose SHA differs from master HEAD. After the
// generate, assert the proxy's server log contains the PINNED commit's
// real git SHA — which only appears if the cid->sha resolution (cidSha map
// or commitUUIDInverse prefix probe) actually ran. In the bug state, only
// HEAD's SHA appears (or the request fails outright).
//
// Token gate: EASYP_GH_TOKEN. Without it, the test skips cleanly.
func TestGeneratePinnedCommit_NotHEAD(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")

	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token

	versions := testutil.AvailableBufVersions(t)
	if len(versions) == 0 {
		t.Skip("no buf binaries cached under testdata/buf/")
	}

	// Ground truth: the real git SHAs at the pinned tag and at HEAD.
	pinnedSHA := gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/tags/"+generatePinnedRef)
	require.True(t, isLowerHex(pinnedSHA, 40), "pinned SHA %q is not 40-char lowercase hex", pinnedSHA)
	headSHA := gitLsRemote(t, "https://github.com/googleapis/googleapis", "HEAD")
	require.True(t, isLowerHex(headSHA, 40), "head SHA %q is not 40-char lowercase hex", headSHA)
	require.NotEqual(t, headSHA, pinnedSHA,
		"test fixture invariant: HEAD (%s) must differ from pinned tag %s (%s) for the not-HEAD assertion to be meaningful",
		headSHA, generatePinnedRef, pinnedSHA)

	pinnedUUID := commitUUIDForTest(pinnedSHA)

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			bufPath := testutil.GetBuf(t, version)
			srv := testutil.StartServer(t, cfg)

			exitCode, stderr, generated := testutil.RunBufGenerateWithPinnedLock(t, bufPath, srv.Port, pinnedUUID)
			if exitCode != 0 {
				t.Fatalf("buf generate failed for %s (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
					version, exitCode, srv.Output.String(), stderr)
			}
			if len(generated) == 0 {
				t.Fatalf("buf generate produced no files for %s.\nServer output:\n%s",
					version, srv.Output.String())
			}

			// Content sanity: every generated file is non-empty. (The
			// `package google.type` marker is intentionally NOT asserted
			// here — WR-04: it exists at both the pinned tag and HEAD, so
			// it cannot distinguish the two. The decisive not-HEAD check
			// below inspects the proxy's decision log instead.)
			for _, f := range generated {
				info, err := os.Stat(f)
				require.NoError(t, err, "generated file %s not found", f)
				require.NotZero(t, info.Size(), "generated file %s is empty", f)
			}

			// The decisive not-HEAD assertion (WR-04): the proxy's server
			// log must carry the PINNED commit's real git SHA as the value
			// of a structured `commit=` attribute AND on a line tagged with
			// a serving-decision branch. Only the cid->sha resolution paths
			// (uuid_ref_resolved, info_cache_writeback, files_cache_hit, or
			// the digest_* branches) emit `commit=<pinnedSHA>`, and each of
			// those branches only runs after the proxy actually fetched and
			// processed the pinned commit's content. A bare substring match
			// on pinnedSHA is not enough — it could appear in unrelated
			// debug text — and `package google.type` cannot distinguish the
			// pinned tag from HEAD. In the bug state (cid forwarded upstream
			// 422, or infoCache serving HEAD), only HEAD's SHA appears in a
			// serving branch (or the request fails outright).
			srvOut := srv.Output.String()
			const commitAttr = "commit="
			servingBranches := []string{
				"branch=uuid_ref_resolved",
				"branch=info_cache_writeback",
				"branch=files_cache_hit",
				"branch=digest_b5_wrap",
				"branch=digest_b4_keep",
				"branch=commit_id_probe_hit",
			}
			pinnedInServingBranch := false
			for _, line := range strings.Split(srvOut, "\n") {
				if !strings.Contains(line, commitAttr+pinnedSHA) {
					continue
				}
				for _, b := range servingBranches {
					if strings.Contains(line, b) {
						pinnedInServingBranch = true
						break
					}
				}
				if pinnedInServingBranch {
					break
				}
			}
			if !pinnedInServingBranch {
				t.Errorf("proxy did not serve the pinned commit %s (%s).\n"+
					"Server log has no serving-decision line with %q and a %s* branch.\n"+
					"This means the cid->sha resolution (Phase 24) did not produce content for the pinned commit — "+
					"the proxy either forwarded the cid upstream (422) or served HEAD.\n"+
					"Server output:\n%s",
					generatePinnedRef, pinnedUUID, commitAttr+pinnedSHA, "branch=", srvOut)
			}
			// And the log must reference the pinned cid (the buf.lock value)
			// alongside the SHA — confirming the cid was the input to the
			// resolution, not an accident.
			if !strings.Contains(srvOut, pinnedUUID) {
				t.Errorf("proxy log does not reference the pinned cid %q at all.\n"+
					"Server output:\n%s", pinnedUUID, srvOut)
			}
		})
	}
}
