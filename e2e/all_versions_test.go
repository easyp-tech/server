package e2e

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/easyp-tech/server/e2e/testutil"
)

// supportsDepUpdate reports whether the buf binary at bufPath supports the
// "buf dep update" subcommand. Older buf versions (v1.30.x and earlier) only
// understand "buf mod update"; v1.32.0 introduced "buf dep update" and it is
// the canonical command from v1.40 onward.
//
// Detection: run `buf dep update --help` and look for the "Usage:" line.
// The flag exists on supported versions and is rejected with a non-zero
// exit on unsupported ones.
func supportsDepUpdate(t *testing.T, bufPath string) bool {
	t.Helper()

	cmd := exec.Command(bufPath, "dep", "update", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	// "dep update" is a recognized subcommand iff help text mentions it.
	// Older buf versions either print a different top-level command list
	// (no "dep update" line) or refuse with exit != 0.
	return strings.Contains(string(out), "dep update")
}

// TestAllBufVersionsModUpdate is the matrix test: for every buf version
// cached under testdata/buf/, run "buf mod update" against a freshly
// started proxy. The proxy must accept the request and produce a buf.lock
// for every version we ship, so a regression that breaks any single
// version is caught at CI time.
//
// "buf mod update" is supported by every buf version we test (including
// v1.30.1 — the last v1alpha1-protocol client). v1.69.0 still accepts it
// as a deprecated alias for `buf dep update`, so this single test covers
// the entire supported version range without per-version branching.
func TestAllBufVersionsModUpdate(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")

	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token
	cfg.LogLevel = "debug"

	versions := testutil.AvailableBufVersions(t)
	if len(versions) == 0 {
		t.Skip("no buf binaries cached under testdata/buf/")
	}

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			bufPath := testutil.GetBuf(t, version)
			srv := testutil.StartServer(t, cfg)

			exitCode, stderr := testutil.RunBufModUpdate(t, bufPath, srv.Port)
			if exitCode != 0 {
				t.Fatalf("buf mod update failed for %s (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
					version, exitCode, srv.Output.String(), stderr)
			}
		})
	}
}

// TestAllBufVersionsDepUpdate runs "buf dep update" for every cached buf
// version. Unlike `mod update`, this command does NOT exist on v1.30.x —
// buf 1.31 deprecated mod update and 1.32 introduced dep update. The
// matrix skips versions that don't support the command, with a clear
// "skipped" subtest, so adding a new older binary never breaks the build.
func TestAllBufVersionsDepUpdate(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GH_TOKEN")

	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token
	cfg.LogLevel = "debug"

	versions := testutil.AvailableBufVersions(t)
	if len(versions) == 0 {
		t.Skip("no buf binaries cached under testdata/buf/")
	}

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			bufPath := testutil.GetBuf(t, version)
			srv := testutil.StartServer(t, cfg)

			if !supportsDepUpdate(t, bufPath) {
				t.Skipf("buf %s does not support 'buf dep update'", version)
			}

			exitCode, stderr := testutil.RunBufDepUpdate(t, bufPath, srv.Port)
			if exitCode != 0 {
				t.Fatalf("buf dep update failed for %s (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
					version, exitCode, srv.Output.String(), stderr)
			}
		})
	}
}

// TestAllBufVersionsHelp is a sanity test that runs every cached buf binary
// with --version. It does not start a proxy. The point is to detect a
// corrupt or missing binary (e.g. truncated download) before the more
// expensive proxy tests run on it. Failures here typically mean the
// testdata/buf cache is broken and need re-population, not a proxy bug.
func TestAllBufVersionsHelp(t *testing.T) {
	versions := testutil.AvailableBufVersions(t)
	if len(versions) == 0 {
		t.Skip("no buf binaries cached under testdata/buf/")
	}

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			bufPath := testutil.GetBuf(t, version)
			cmd := exec.Command(bufPath, "--version")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("buf %s --version failed: %v\noutput: %s", version, err, out)
			}
			got := strings.TrimSpace(string(out))
			// The cached binary's --version should match its directory name
			// (modulo the "v" prefix in the path; buf prints just "1.30.1").
			want := strings.TrimPrefix(version, "v")
			if got != want {
				t.Fatalf("buf %s --version = %q, want %q (cache directory is mismatched with binary)",
					version, got, want)
			}
		})
	}
}
