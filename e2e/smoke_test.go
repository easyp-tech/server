package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSmokeBufModUpdate verifies that buf mod update works end-to-end against
// the TLS proxy for both old (v1.30.1) and modern (v1.69.0) buf CLI versions.
// This validates HAND-02: existing RPCs serve correctly with new generated types.
func TestSmokeBufModUpdate(t *testing.T) {
	token := os.Getenv("EASYP_GITHUB_TOKEN")
	if token == "" {
		t.Skip("EASYP_GITHUB_TOKEN not set")
	}

	home := os.Getenv("HOME")
	projectRoot := findProjectRoot(t)

	cases := []struct {
		name       string
		bufBinary  string
	}{
		{
			name:      "buf_v1.30.1",
			bufBinary: filepath.Join(home, "go", "bin", "buf"),
		},
		{
			name:      "buf_v1.69.0",
			bufBinary: "/usr/local/bin/buf",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Verify buf binary exists before running
			info, err := os.Stat(tc.bufBinary)
			require.NoError(t, err, "buf binary not found at %s", tc.bufBinary)
			require.False(t, info.IsDir(), "buf path is a directory, not a file: %s", tc.bufBinary)

			port, cleanup := startServer(t, projectRoot, token, home)
			defer cleanup()

			exitCode, stderr := runBufModUpdate(t, tc.bufBinary, port)
			require.Equal(t, 0, exitCode, "buf mod update failed: %s", stderr)
		})
	}
}

// startServer starts the TLS proxy as a subprocess and waits for it to accept connections.
// Returns the port number and a cleanup function.
func startServer(t *testing.T, projectRoot, token, home string) (int, func()) {
	t.Helper()

	// Allocate a free TCP port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "allocating free port")

	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close(), "closing port listener")

	// Create temp config file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")

	tlsCertPath := filepath.Join(home, "local-tls", "server", "server-cert.pem")
	tlsKeyPath := filepath.Join(home, "local-tls", "server", "server-key.pem")

	cfgContent := fmt.Sprintf(`listen: "127.0.0.1:%d"
domain: "127.0.0.1:%d"
log:
  level: "info"
cache:
  type: "none"
tls:
  cert: %s
  key:  %s
proxy:
  github:
    - token: %s
      repo:
        owner: googleapis
        name:  googleapis
        path:
          - google/type/
`, port, port, tlsCertPath, tlsKeyPath, token)

	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0600), "writing config file")

	// Start server subprocess
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/easyp", "-cfg", cfgPath)
	cmd.Dir = projectRoot

	var serverOutput bytes.Buffer
	cmd.Stdout = &serverOutput
	cmd.Stderr = &serverOutput

	require.NoError(t, cmd.Start(), "starting server subprocess")

	// Cleanup function to kill server
	cleanup := func() {
		cancel()
		// Give process time to terminate gracefully
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	}

	// Wait for server to be ready by polling the TCP port
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return port, cleanup
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Server didn't start in time
	t.Fatalf("server did not become ready on port %d within 30s. Output:\n%s", port, serverOutput.String())
	return port, cleanup
}

// runBufModUpdate creates a minimal buf module and runs buf mod update.
// Returns the exit code and stderr output.
func runBufModUpdate(t *testing.T, bufBinary string, port int) (int, string) {
	t.Helper()

	tmpDir := t.TempDir()

	// Write buf.yaml with a dependency referencing the proxy domain
	bufYAML := fmt.Sprintf(`version: v1
deps:
  - 127.0.0.1:%d/googleapis/googleapis
`, port)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600), "writing buf.yaml")

	// Run buf mod update
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bufBinary, "mod", "update")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	exitErr := cmd.Run()

	exitCode := 0
	if exitErr != nil {
		if exitCodeErr, ok := exitErr.(*exec.ExitError); ok {
			exitCode = exitCodeErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// Verify buf.lock was created
	if exitCode == 0 {
		require.FileExists(t, filepath.Join(tmpDir, "buf.lock"), "buf.lock not created after successful buf mod update")
	}

	return exitCode, stderr.String()
}

// findProjectRoot returns the project root directory by locating this test file
// via runtime.Caller and walking up to the module root.
func findProjectRoot(t *testing.T) string {
	t.Helper()

	// Use runtime.Caller to find this source file, then walk up
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	// This file is at <project-root>/e2e/smoke_test.go
	dir := filepath.Dir(filename)
	root := filepath.Dir(dir)

	// Verify it's the project root by checking go.mod
	goModPath := filepath.Join(root, "go.mod")
	info, err := os.Stat(goModPath)
	require.NoError(t, err, "go.mod not found at project root %s", root)
	require.False(t, info.IsDir(), "go.mod is a directory at %s", root)

	return root
}
