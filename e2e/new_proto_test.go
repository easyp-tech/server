package e2e

import (
	"testing"

	"github.com/easyp-tech/server/e2e/testutil"
)

func TestNewProtocolBufModUpdate(t *testing.T) {
	t.Parallel()
	token := testutil.RequireEnvToken(t, "EASYP_GITHUB_TOKEN")

	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token
	cfg.LogLevel = "debug"

	bufPath := testutil.GetBuf(t, testutil.BufV169)
	srv := testutil.StartServer(t, cfg)

	exitCode, stderr := testutil.RunBufModUpdate(t, bufPath, srv.Port)
	if exitCode != 0 {
		t.Fatalf("buf mod update failed with v1.69.0 (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			exitCode, srv.Output.String(), stderr)
	}
}

func TestNewProtocolBufDepUpdate(t *testing.T) {
	t.Parallel()
	token := testutil.RequireEnvToken(t, "EASYP_GITHUB_TOKEN")

	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token
	cfg.LogLevel = "debug"

	bufPath := testutil.GetBuf(t, testutil.BufV169)
	srv := testutil.StartServer(t, cfg)

	exitCode, stderr := testutil.RunBufDepUpdate(t, bufPath, srv.Port)
	if exitCode != 0 {
		t.Fatalf("buf dep update failed with v1.69.0 (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			exitCode, srv.Output.String(), stderr)
	}
}
