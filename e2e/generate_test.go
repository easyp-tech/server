package e2e

import (
	"os"
	"strings"
	"testing"

	"github.com/easyp-tech/server/e2e/testutil"
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
