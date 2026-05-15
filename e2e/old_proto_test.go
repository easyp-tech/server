package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/easyp-tech/server/e2e/testutil"
)

// TestOldProtocolBufModUpdateTwice validates OLD-02 (reinterpreted):
// buf v1.30.1 backward compatibility via two-step "buf mod update".
// Step 1: buf mod update creates buf.lock (fresh workspace).
// Step 2: buf mod update again on same workspace (update with existing lock).
// This exercises the same RPC path (GetModulePins + DownloadManifestAndBlobs)
// that "buf dep update" exercises in buf v1.32.0+.
func TestOldProtocolBufModUpdateTwice(t *testing.T) {
	token := testutil.RequireEnvToken(t, "EASYP_GITHUB_TOKEN")

	cfg := testutil.DefaultTestConfig()
	cfg.GithubToken = token

	bufPath := testutil.GetBuf(t, testutil.BufV130)
	srv := testutil.StartServer(t, cfg)

	// Create workspace inline (RunBufModUpdate creates new workspace each call --
	// cannot use it for two-step testing).
	tmpDir := t.TempDir()
	bufYAML := fmt.Sprintf(`version: v1
deps:
  - 127.0.0.1:%d/googleapis/googleapis
`, srv.Port)
	if err := os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600); err != nil {
		t.Fatalf("writing buf.yaml: %v", err)
	}

	// runBuf executes buf mod update in the workspace directory.
	runBuf := func() (int, string) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, bufPath, "mod", "update")
		cmd.Dir = tmpDir
		cmd.Env = os.Environ()

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), stderr.String()
			}
			return 1, stderr.String()
		}
		return 0, stderr.String()
	}

	// Step 1: buf mod update (creates buf.lock).
	exitCode, stderr := runBuf()
	if exitCode != 0 {
		t.Fatalf("first buf mod update failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			exitCode, srv.Output.String(), stderr)
	}

	// Verify buf.lock was created.
	lockPath := filepath.Join(tmpDir, "buf.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("buf.lock not created after first buf mod update: %v.\nServer output:\n%s",
			err, srv.Output.String())
	}

	// Step 2: buf mod update again on same workspace (updates existing buf.lock).
	exitCode, stderr = runBuf()
	if exitCode != 0 {
		t.Fatalf("second buf mod update failed (exit %d).\nServer output:\n%s\nBuf stderr:\n%s",
			exitCode, srv.Output.String(), stderr)
	}
}
