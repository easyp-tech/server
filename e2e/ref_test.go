package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/easyp-tech/server/e2e/testutil"
)

// pinnedRef is the ref used by the ref-respecting tests. It is a real,
// stable tag in googleapis/googleapis whose SHA differs from HEAD, so
// the proxy's ref-honoring behavior is observable in the resulting
// buf.lock file.
//
//	common-protos-1_3_1 → 27156597fdf4fb77004434d4409154a230dc9a32
//	HEAD              → af2513fa2dc3b1fb9992faaf900807f856d35990
const pinnedRef = "common-protos-1_3_1"

// branchRef is the branch-name fixture for the branch-ref test. It is a
// stable, long-lived non-default branch on googleapis/googleapis (whose
// default branch is "master"). It is NOT in the provider's
// isConventionalDefaultName set (main/master/develop/trunk), so a dep
// pinned to ":gh-pages" exercises the repos.GetCommit fall-through in
// GetMeta (getrepo.go:96) rather than the HEAD carve-out. Its tip differs
// from master's tip, which the tests assert at runtime.
//
//	gh-pages tip (runtime) → branch tip SHA
//	master     (default)   → master tip SHA
const branchRef = "gh-pages"

// commitLineRE matches the "commit: <value>" line in either buf.lock
// format. v1 and v2 use the same field name (just under different
// parent keys: `remote/owner/repository` vs `name`), so a single regex
// covers both. The pinned value is a buf-issued 32-char dashless UUID.
var commitLineRE = regexp.MustCompile(`(?m)^[ \t]+commit:[ \t]+(\S+)\s*$`)

// extractCommitFromLock returns the first "commit:" value in buf.lock,
// or an error if no such line is present. Used to assert the proxy
// pinned to the expected commit (or, for the diff tests, that two
// updates pinned to different commits).
func extractCommitFromLock(lockContent []byte) (string, error) {
	m := commitLineRE.FindSubmatch(lockContent)
	if m == nil {
		return "", errors.New("no commit: line found in buf.lock")
	}
	return string(m[1]), nil
}

// TestRefRespected_ModUpdate_DiffersFromHead is the ref-honoring
// matrix test. For every cached buf version it runs "buf mod update"
// twice: once with no ref (proxy falls back to HEAD) and once with
// the test fixture ref (proxy should pin to the SHA at that ref). The
// two buf.lock files must pin different commits — proving the proxy
// did not return HEAD in the ref-pinned run.
//
// This test does not depend on any specific SHA being at the ref; it
// only requires that the ref's SHA differs from HEAD's SHA. The chosen
// ref (common-protos-1_3_1) is a stable googleapis tag, so this
// property is expected to hold for a long time.
func TestRefRespected_ModUpdate_DiffersFromHead(t *testing.T) {
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

			// Run 1: no ref. Proxy falls back to HEAD.
			srvHead := testutil.StartServer(t, cfg)
			headExit, headStderr, headLock := testutil.RunBufModUpdateWithRef(t, bufPath, srvHead.Port, "")
			if headExit != 0 {
				t.Fatalf("buf mod update (HEAD) failed for %s (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
					version, headExit, srvHead.Output.String(), headStderr)
			}
			headCommit, err := extractCommitFromLock(headLock)
			if err != nil {
				t.Fatalf("extract commit from HEAD lock for %s: %v\nlock:\n%s", version, err, headLock)
			}

			// Run 2: with the pinned ref. Proxy must pin to the SHA at that ref.
			srvRef := testutil.StartServer(t, cfg)
			refExit, refStderr, refLock := testutil.RunBufModUpdateWithRef(t, bufPath, srvRef.Port, pinnedRef)
			if refExit != 0 {
				t.Fatalf("buf mod update (ref=%s) failed for %s (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
					pinnedRef, version, refExit, srvRef.Output.String(), refStderr)
			}
			refCommit, err := extractCommitFromLock(refLock)
			if err != nil {
				t.Fatalf("extract commit from ref lock for %s: %v\nlock:\n%s", version, err, refLock)
			}

			if headCommit == refCommit {
				t.Fatalf("buf mod update for %s pinned the same commit %q for both HEAD and ref=%s; proxy did not honor Name.ref.\nHEAD lock:\n%s\nRef lock:\n%s",
					version, headCommit, pinnedRef, headLock, refLock)
			}
		})
	}
}

// TestRefRespected_ModUpdate_MatchesUpstreamSHA is the stronger
// ref-honoring test. It uses git ls-remote to query the SHA at the
// pinned ref on googleapis/googleapis, derives the buf-issued UUID
// via the same commitUUID algorithm the proxy uses, and asserts the
// lock file's pinned commit matches. This proves the proxy pinned to
// the SHA at the ref, not just to "some other commit than HEAD".
//
// Only v1.69.0 is exercised because the UUID derivation is sensitive
// to the SHA -> UUID byte table; this test is intended as a strict
// regression guard for the current minting contract and the v1.30.1
// test above already covers the broader "differ from HEAD" guarantee.
func TestRefRespected_ModUpdate_MatchesUpstreamSHA(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")
	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token
	cfg.LogLevel = "debug"

	bufPath := testutil.GetBuf(t, testutil.BufV169)
	srv := testutil.StartServer(t, cfg)

	// Look up the SHA at the ref in googleapis/googleapis. The result
	// is the source of truth the proxy is expected to honor.
	sha := gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/tags/"+pinnedRef)
	if !isLowerHex(sha, 40) {
		t.Fatalf("git ls-remote for ref %q returned %q, expected a 40-char lowercase hex SHA", pinnedRef, sha)
	}

	// Derive the expected UUID via the same byte table the proxy uses
	// (see internal/connect/commits_helpers.go commitUUID). The proxy
	// mints the UUID from the SHA before stamping it into the lock
	// file, so this is the value the lock should contain.
	expectedUUID := commitUUIDForTest(sha)

	exitCode, stderr, lock := testutil.RunBufModUpdateWithRef(t, bufPath, srv.Port, pinnedRef)
	if exitCode != 0 {
		t.Fatalf("buf mod update (ref=%s) failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			pinnedRef, exitCode, srv.Output.String(), stderr)
	}

	gotCommit, err := extractCommitFromLock(lock)
	if err != nil {
		t.Fatalf("extract commit from lock: %v\nlock:\n%s", err, lock)
	}
	if gotCommit != expectedUUID {
		t.Fatalf("buf.lock commit = %q, want %q (derived from upstream SHA %s at ref %s).\nbuf.lock:\n%s",
			gotCommit, expectedUUID, sha, pinnedRef, lock)
	}
}

// TestRefRespected_DepUpdate_DiffersFromHead is the buf dep update
// counterpart to TestRefRespected_ModUpdate_DiffersFromHead. It
// exercises the same ref-honoring path through the modern "buf dep
// update" command (v1.32+). Only v1.69.0 is tested because older buf
// versions do not support "buf dep update" at all.
func TestRefRespected_DepUpdate_DiffersFromHead(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")
	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token

	bufPath := testutil.GetBuf(t, testutil.BufV169)

	// Run 1: no ref. Proxy falls back to HEAD.
	srvHead := testutil.StartServer(t, cfg)
	headExit, headStderr, headLock := testutil.RunBufDepUpdateWithRef(t, bufPath, srvHead.Port, "")
	if headExit != 0 {
		t.Fatalf("buf dep update (HEAD) failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			headExit, srvHead.Output.String(), headStderr)
	}
	headCommit, err := extractCommitFromLock(headLock)
	if err != nil {
		t.Fatalf("extract commit from HEAD lock: %v\nlock:\n%s", err, headLock)
	}

	// Run 2: with the pinned ref.
	srvRef := testutil.StartServer(t, cfg)
	refExit, refStderr, refLock := testutil.RunBufDepUpdateWithRef(t, bufPath, srvRef.Port, pinnedRef)
	if refExit != 0 {
		t.Fatalf("buf dep update (ref=%s) failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			pinnedRef, refExit, srvRef.Output.String(), refStderr)
	}
	refCommit, err := extractCommitFromLock(refLock)
	if err != nil {
		t.Fatalf("extract commit from ref lock: %v\nlock:\n%s", err, refLock)
	}

	if headCommit == refCommit {
		t.Fatalf("buf dep update pinned the same commit %q for both HEAD and ref=%s; proxy did not honor Name.ref.\nHEAD lock:\n%s\nRef lock:\n%s",
			headCommit, pinnedRef, headLock, refLock)
	}
}

// TestRefRespected_BranchName_PinsBranchTip proves that a buf.yaml dep
// whose ":ref" is a branch name (not a tag, not a SHA) pins the resulting
// buf.lock to that branch's tip SHA — not to HEAD. The fixture is the
// "gh-pages" branch on googleapis/googleapis, a stable non-default branch
// (the repo's default is "master") that is NOT in the provider's
// isConventionalDefaultName set, so it exercises the repos.GetCommit
// fall-through in GetMeta (getrepo.go:96) rather than the HEAD carve-out.
//
// The expected lock commit is derived at runtime: git ls-remote resolves
// the branch tip, then commitUUIDForTest mints the buf-issued UUID the
// proxy should stamp. The test also asserts the branch tip differs from
// master's tip, confirming the fixture is genuinely a non-default branch.
//
// Only v1.69.0 is exercised, matching TestRefRespected_ModUpdate_MatchesUpstreamSHA:
// the strict UUID assertion is sensitive to the commitUUID byte table and
// is intended as a regression guard for the current minting contract.
func TestRefRespected_BranchName_PinsBranchTip(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")
	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token

	bufPath := testutil.GetBuf(t, testutil.BufV169)
	srv := testutil.StartServer(t, cfg)

	// Resolve the branch tip and the default-branch tip. The branch tip is
	// the source of truth the proxy is expected to honor; the master tip is
	// a sanity check that the fixture is genuinely non-default.
	branchTip := gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/heads/"+branchRef)
	if !isLowerHex(branchTip, 40) {
		t.Fatalf("git ls-remote for branch %q returned %q, expected a 40-char lowercase hex SHA", branchRef, branchTip)
	}
	masterTip := gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/heads/master")
	if branchTip == masterTip {
		t.Fatalf("%s tip == master tip %q; fixture is no longer a non-default branch", branchRef, branchTip)
	}

	// Derive the expected UUID via the same byte table the proxy uses.
	expectedUUID := commitUUIDForTest(branchTip)

	exitCode, stderr, lock := testutil.RunBufModUpdateWithRef(t, bufPath, srv.Port, branchRef)
	if exitCode != 0 {
		t.Fatalf("buf mod update (ref=%s) failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			branchRef, exitCode, srv.Output.String(), stderr)
	}

	gotCommit, err := extractCommitFromLock(lock)
	if err != nil {
		t.Fatalf("extract commit from lock: %v\nlock:\n%s", err, lock)
	}
	if gotCommit != expectedUUID {
		t.Fatalf("buf.lock commit = %q, want %q (derived from %s branch tip %s).\nbuf.lock:\n%s",
			gotCommit, expectedUUID, branchRef, branchTip, lock)
	}
}

// TestRefRespected_NonDefaultBranchCommitSHA proves that a buf.yaml dep
// whose ":ref" is a raw 40-char commit SHA — specifically a commit that
// is NOT on the default branch — pins the resulting buf.lock to that
// commit. This exercises the isSHA fast path in GetMeta (getrepo.go:93),
// which stamps the SHA directly regardless of which branch it lives on.
//
// The fixture SHA is the tip of the "gh-pages" branch (a non-default
// branch on googleapis/googleapis), so the commit is provably off the
// default branch "master". The test asserts the SHA differs from master's
// tip, then sends the SHA itself as the ref.
//
// This test also answers the empirical question "does the buf CLI accept
// a raw 40-char SHA as the ':ref' suffix?" If buf rejects the syntax
// client-side, the failure surfaces the captured buf stderr — which is
// itself the answer to the user's question.
//
// Only v1.69.0 is exercised, matching the matches-upstream test.
func TestRefRespected_NonDefaultBranchCommitSHA(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")
	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token

	bufPath := testutil.GetBuf(t, testutil.BufV169)
	srv := testutil.StartServer(t, cfg)

	// The fixture SHA is the tip of the gh-pages branch — a commit that
	// lives on a non-default branch.
	branchTip := gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/heads/"+branchRef)
	if !isLowerHex(branchTip, 40) {
		t.Fatalf("git ls-remote for branch %q returned %q, expected a 40-char lowercase hex SHA", branchRef, branchTip)
	}
	masterTip := gitLsRemote(t, "https://github.com/googleapis/googleapis", "refs/heads/master")
	if branchTip == masterTip {
		t.Fatalf("%s tip == master tip %q; fixture commit is no longer off the default branch", branchRef, branchTip)
	}

	// Derive the expected UUID the proxy should stamp for this SHA.
	expectedUUID := commitUUIDForTest(branchTip)

	// The ref IS the raw 40-char SHA. The dep string becomes
	// host:port/owner/repo:<40hex>.
	exitCode, stderr, lock := testutil.RunBufModUpdateWithRef(t, bufPath, srv.Port, branchTip)
	if exitCode != 0 {
		t.Fatalf("buf mod update (ref=<sha %s>) failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			branchTip, exitCode, srv.Output.String(), stderr)
	}

	gotCommit, err := extractCommitFromLock(lock)
	if err != nil {
		t.Fatalf("extract commit from lock: %v\nlock:\n%s", err, lock)
	}
	if gotCommit != expectedUUID {
		t.Fatalf("buf.lock commit = %q, want %q (derived from non-default-branch commit %s).\nbuf.lock:\n%s",
			gotCommit, expectedUUID, branchTip, lock)
	}
}

// gitLsRemote runs `git ls-remote <repo> <ref>` and returns the SHA
// (or whatever the ref resolves to). Fails the test on error.
//
// This is a self-contained helper rather than going through the
// proxy's source chain because the upstream SHA is the ground truth
// we compare the lock file against. If this command fails, the
// network is down or the test is being run in an isolated
// environment that blocks outbound git; either way, the test should
// fail loudly rather than skip silently.
func gitLsRemote(t *testing.T, repo, ref string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", repo, ref)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("git ls-remote %s %s: %v\nstderr:\n%s", repo, ref, err, stderr.String())
	}

	// Output is "<sha>\t<refname>"; we want the first column.
	line := strings.TrimSpace(stdout.String())
	if line == "" {
		t.Fatalf("git ls-remote %s %s returned no output", repo, ref)
	}
	parts := strings.SplitN(line, "\t", 2)
	return strings.TrimSpace(parts[0])
}

// isLowerHex reports whether s is a string of exactly n lowercase hex
// characters. Used to validate the SHA returned by git ls-remote.
func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// commitUUIDForTest is the test-side mirror of the proxy's commitUUID
// helper. The byte table is identical to internal/connect/commits_helpers.go
// commitUUID; keeping it in the test avoids an import cycle and lets
// the test be self-contained.
//
// If the proxy changes its UUID minting algorithm, this function must
// be updated to match. The test that uses it
// (TestRefRespected_ModUpdate_MatchesUpstreamSHA) will fail loudly in
// that case, which is the intended regression signal.
func commitUUIDForTest(gitSHA string) string {
	if len(gitSHA) != 40 && len(gitSHA) != 64 {
		panic("commitUUIDForTest: input is not 40 or 64 hex characters")
	}
	sha, err := hex.DecodeString(gitSHA)
	if err != nil {
		panic("commitUUIDForTest: input is not hex: " + err.Error())
	}
	var result [16]byte
	// SHA bytes 0..5 → result bytes 0..5.
	copy(result[0:6], sha[0:6])
	// Result byte 6 = UUID version-4 nibble (high nibble = 4, low nibble = 0).
	result[6] = 0x40
	// SHA byte 6 → result byte 7.
	result[7] = sha[6]
	// Result byte 8 = RFC 4122 variant bits (high two bits = 10, low six bits = 0).
	result[8] = 0x80
	// SHA bytes 7..13 → result bytes 9..15.
	copy(result[9:16], sha[7:14])
	return hex.EncodeToString(result[:])
}
