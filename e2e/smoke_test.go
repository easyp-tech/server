package e2e

import (
	"testing"

	"github.com/easyp-tech/server/e2e/testutil"
)

// TestSmokeBufModUpdate verifies that buf mod update works end-to-end against
// the TLS proxy for both old (v1.30.1) and modern (v1.69.0) buf CLI versions.
// This validates HAND-02: existing RPCs serve correctly with new generated types.
func TestSmokeBufModUpdate(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GITHUB_TOKEN")

	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token

	cases := []struct {
		name    string
		version string
	}{
		{
			name:    "buf_v1.30.1",
			version: testutil.BufV130,
		},
		{
			name:    "buf_v1.69.0",
			version: testutil.BufV169,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bufPath := testutil.GetBuf(t, tc.version)
			srv := testutil.StartServer(t, cfg)

			exitCode, stderr := testutil.RunBufModUpdate(t, bufPath, srv.Port)
			if exitCode != 0 {
				t.Fatalf("buf mod update failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
					exitCode, srv.Output.String(), stderr)
			}
		})
	}
}
