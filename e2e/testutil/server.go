package testutil

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ServerResult holds the result of starting a test proxy server.
type ServerResult struct {
	// Port is the allocated TCP port number the server is listening on.
	Port int
	// Output contains the combined stdout/stderr output from the server subprocess.
	// Use this in test failure messages for diagnostics.
	Output *bytes.Buffer
}

// StartServer starts the TLS proxy as a subprocess, waits for it to accept
// TCP connections, and registers cleanup via t.Cleanup. Returns a ServerResult
// with the allocated port and a buffer capturing server output.
//
// The server is started via "go run ./cmd/easyp -cfg <path>" with a config
// generated from cfg. The subprocess runs in the project root directory.
// Tests that call StartServer should first call RequireEnvToken to ensure
// the GitHub token is available.
func StartServer(t *testing.T, cfg TestConfig) ServerResult {
	t.Helper()

	// Allocate a free TCP port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "allocating free port")

	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close(), "closing port listener")

	// Generate config YAML from TestConfig.
	cfgPath := generateConfigYAML(t, cfg, port)

	// Start server subprocess.
	projectRoot := findProjectRoot(t)
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/easyp", "-cfg", cfgPath)
	cmd.Dir = projectRoot

	var serverOutput bytes.Buffer
	cmd.Stdout = &serverOutput
	cmd.Stderr = &serverOutput

	require.NoError(t, cmd.Start(), "starting server subprocess")

	// Register cleanup to kill server.
	t.Cleanup(func() {
		cancel()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	})

	// TCP poll for readiness.
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return ServerResult{Port: port, Output: &serverOutput}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("server did not become ready on port %d within 30s. Output:\n%s", port, serverOutput.String())
	return ServerResult{}
}

// RunBufModUpdate creates a minimal buf module in a temp directory and runs
// "buf mod update" against the proxy at the given port. Returns the exit code
// and stderr output. This is exported for use by Phase 4 and 5 tests.
func RunBufModUpdate(t *testing.T, bufBinary string, port int) (int, string) {
	t.Helper()

	tmpDir := t.TempDir()

	// Write buf.yaml with a dependency referencing the proxy domain.
	bufYAML := fmt.Sprintf(`version: v1
deps:
  - 127.0.0.1:%d/googleapis/googleapis
`, port)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600), "writing buf.yaml")

	// Run buf mod update.
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

	// Verify buf.lock was created on success.
	if exitCode == 0 {
		require.FileExists(t, filepath.Join(tmpDir, "buf.lock"), "buf.lock not created after successful buf mod update")
	}

	return exitCode, stderr.String()
}

// RunBufDepUpdate creates a minimal buf module in a temp directory and runs
// "buf dep update" against the proxy at the given port. Returns the exit code
// and stderr output. This is exported for use by Phase 5 tests.
func RunBufDepUpdate(t *testing.T, bufBinary string, port int) (int, string) {
	t.Helper()

	tmpDir := t.TempDir()

	bufYAML := fmt.Sprintf(`version: v1
deps:
  - 127.0.0.1:%d/googleapis/googleapis
`, port)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600), "writing buf.yaml")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bufBinary, "dep", "update")
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

	if exitCode == 0 {
		require.FileExists(t, filepath.Join(tmpDir, "buf.lock"), "buf.lock not created after successful buf dep update")
	}

	return exitCode, stderr.String()
}
