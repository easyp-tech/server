package testutil

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// CommitLineRE matches the "commit: <value>" line in either buf.lock
// format. v1 and v2 use the same field name (just under different
// parent keys: `remote/owner/repository` vs `name`), so a single regex
// covers both. The pinned value is a buf-issued 32-char dashless UUID.
// Defined in testutil (not duplicated in e2e/ref_test.go) per Phase 25
// FIX-06 (PR #39 post-merge review finding).
var CommitLineRE = regexp.MustCompile(`(?m)^[ \t]+commit:[ \t]+(\S+)\s*$`)

// ExtractCommitFromLock returns the first "commit:" value in buf.lock,
// or an error if no such line is present. Used by e2e tests to assert
// the proxy pinned to the expected commit.
func ExtractCommitFromLock(lockContent []byte) (string, error) {
	m := CommitLineRE.FindSubmatch(lockContent)
	if m == nil {
		return "", fmt.Errorf("no commit: line found in buf.lock")
	}
	return string(m[1]), nil
}

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
//
// The dep reference is "<host>:<port>/googleapis/googleapis" (no ref) so the
// proxy falls back to HEAD. To exercise the Name.ref code path, use
// RunBufModUpdateWithRef instead.
func RunBufModUpdate(t *testing.T, bufBinary string, port int) (int, string) {
	t.Helper()

	exitCode, stderr, _ := runBufUpdate(t, bufBinary, port, "", "mod")
	return exitCode, stderr
}

// RunBufDepUpdate creates a minimal buf module in a temp directory and runs
// "buf dep update" against the proxy at the given port. Returns the exit code
// and stderr output. This is exported for use by Phase 5 tests.
//
// The dep reference is "<host>:<port>/googleapis/googleapis" (no ref) so the
// proxy falls back to HEAD. To exercise the Name.ref code path, use
// RunBufDepUpdateWithRef instead.
func RunBufDepUpdate(t *testing.T, bufBinary string, port int) (int, string) {
	t.Helper()

	exitCode, stderr, _ := runBufUpdate(t, bufBinary, port, "", "dep")
	return exitCode, stderr
}

// RunBufModUpdateWithRef is the ref-pinned variant of RunBufModUpdate. It
// writes a buf.yaml whose dep reference ends with ":ref" so the buf CLI
// sends the ref in the Name.ref field. Returns the exit code, stderr, and
// the contents of buf.lock on success (nil on failure).
//
// Use this to verify the proxy honors Name.ref end-to-end: the lock file
// produced by a ref-pinned update should pin to the SHA at that ref, not
// to HEAD.
func RunBufModUpdateWithRef(t *testing.T, bufBinary string, port int, ref string) (int, string, []byte) {
	t.Helper()
	return runBufUpdate(t, bufBinary, port, ref, "mod")
}

// RunBufDepUpdateWithRef is the ref-pinned variant of RunBufDepUpdate.
// Same contract as RunBufModUpdateWithRef but invokes "buf dep update".
// Only buf v1.32+ understands "buf dep update"; older binaries must use
// RunBufModUpdateWithRef.
func RunBufDepUpdateWithRef(t *testing.T, bufBinary string, port int, ref string) (int, string, []byte) {
	t.Helper()
	return runBufUpdate(t, bufBinary, port, ref, "dep")
}

// runBufUpdate is the shared implementation behind the four public
// RunBuf*Update helpers. The ref arg may be empty, in which case the dep
// is written without a ":ref" suffix and the proxy falls back to HEAD.
// The subcommand arg selects "mod" or "dep" (the buf subcommand prefix).
//
// Returns (exitCode, stderr, bufLockContent). bufLockContent is the raw
// bytes of buf.lock on success and nil on failure; the caller is
// responsible for any parsing.
func runBufUpdate(t *testing.T, bufBinary string, port int, ref, subcommand string) (int, string, []byte) {
	t.Helper()

	tmpDir := t.TempDir()

	// Write buf.yaml with a dependency referencing the proxy domain.
	// The dep string is "<host>:<port>/googleapis/googleapis" or that
	// with ":ref" appended. The buf CLI parses the optional suffix as
	// the ref to send in Name.ref.
	depRef := "127.0.0.1:" + strconv.Itoa(port) + "/googleapis/googleapis"
	if ref != "" {
		depRef = depRef + ":" + ref
	}
	bufYAML := fmt.Sprintf(`version: v1
deps:
  - %s
`, depRef)
	require.NoError(t,
		os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600),
		"writing buf.yaml")

	// Write a dummy proto file so modern buf CLI versions don't complain about empty workspace.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "dummy.proto"), []byte(`syntax = "proto3"; package dummy;`), 0600), "writing dummy.proto")

	// Run buf update.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bufBinary, subcommand, "update")
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

	// Read buf.lock on success so callers can inspect the pinned commit.
	var lockContent []byte
	if exitCode == 0 {
		lockPath := filepath.Join(tmpDir, "buf.lock")
		require.FileExists(t, lockPath, "buf.lock not created after successful buf update")
		var readErr error
		lockContent, readErr = os.ReadFile(lockPath)
		require.NoError(t, readErr, "reading buf.lock")
	}

	return exitCode, stderr.String(), lockContent
}

// RunBufGenerateWithPinnedLock creates a minimal buf module in a temp
// directory, runs "buf mod update" against the proxy at the given port,
// overwrites the resulting buf.lock's commit: line with pinnedCommit (a
// 32-char dashless buf-issued UUID), and then runs "buf generate" against
// the overwritten lock. Returns the exit code of "buf generate", the
// stderr it produced, and the list of generated gen/go/google/type/*.pb.go
// files on success (nil on failure).
//
// The temp workspace contains:
//   - buf.yaml: deps: [127.0.0.1:<port>/googleapis/googleapis]
//   - dummy.proto: syntax = "proto3"; package dummy;
//   - buf.gen.yaml: pinned remote plugin buf.build/protocolbuffers/go:v1.28.1
//     with out: gen/go
//
// This exercises the read-path that fires AFTER the client has loaded
// buf.lock: the proxy must resolve a pre-existing 32-char UUID via its
// infoCache/commitMap or the post-restart probeCommitID path, and
// return content for that commit (NOT HEAD, NOT a 400 on a "foreign" id).
func RunBufGenerateWithPinnedLock(t *testing.T, bufBinary string, port int, pinnedCommit string) (int, string, []string) {
	t.Helper()
	return runBufGenerate(t, bufBinary, port, pinnedCommit)
}

// runBufGenerate is the private implementation behind
// RunBufGenerateWithPinnedLock. See that function for the contract.
func runBufGenerate(t *testing.T, bufBinary string, port int, pinnedCommit string) (int, string, []string) {
	t.Helper()

	tmpDir := t.TempDir()

	// Write buf.yaml pointing at the proxy.
	depRef := "127.0.0.1:" + strconv.Itoa(port) + "/googleapis/googleapis"
	bufYAML := fmt.Sprintf(`version: v1
deps:
  - %s
`, depRef)
	require.NoError(t,
		os.WriteFile(filepath.Join(tmpDir, "buf.yaml"), []byte(bufYAML), 0600),
		"writing buf.yaml")

	// Write a dummy proto so modern buf CLI versions don't complain about
	// an empty workspace.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "dummy.proto"), []byte(`syntax = "proto3"; package dummy;`), 0600), "writing dummy.proto")

	// Write buf.gen.yaml. The remote plugin version is pinned; the v1
	// buf.gen.yaml format is understood by both v1.30.1 and v1.69.0.
	// Uses the "plugin:" field (not the deprecated "remote:" field) —
	// the "remote:" field was the alpha-remote-generation API and has
	// been removed in v1.69.0+ (and is flagged as deprecated by v1.30.1).
	bufGenYAML := `version: v1
plugins:
  - plugin: buf.build/protocolbuffers/go:v1.28.1
    out: gen/go
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "buf.gen.yaml"), []byte(bufGenYAML), 0600), "writing buf.gen.yaml")

	// Step 1: buf mod update to populate a real buf.lock.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, bufBinary, "mod", "update")
		cmd.Dir = tmpDir
		cmd.Env = os.Environ()

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return 1, "buf mod update failed: " + stderr.String(), nil
		}
	}

	// Step 2: read buf.lock, extract the original commit, overwrite it
	// with the pinned UUID. ExtractCommitFromLock is in the testutil
	// package (defined above), so we call it directly rather than
	// duplicating the regex.
	lockPath := filepath.Join(tmpDir, "buf.lock")
	lockContent, err := os.ReadFile(lockPath)
	require.NoError(t, err, "reading buf.lock")

	m := CommitLineRE.FindSubmatch(lockContent)
	require.NotNil(t, m, "no commit: line found in buf.lock:\n%s", lockContent)
	originalCommit := string(m[1])

	modified := strings.Replace(string(lockContent), originalCommit, pinnedCommit, 1)
	require.NotEqual(t, string(lockContent), modified,
		"overwrite did not change buf.lock: commit %q not found", originalCommit)
	require.NoError(t, os.WriteFile(lockPath, []byte(modified), 0600), "writing overwritten buf.lock")

	// Step 3: buf generate with the overwritten lock. Longer timeout
	// than the mod update step because the plugin fetch + codegen
	// takes longer.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, bufBinary, "generate")
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

		if exitCode != 0 {
			return exitCode, stderr.String(), nil
		}

		// On success, collect the generated files.
		matches, _ := filepath.Glob(filepath.Join(tmpDir, "gen", "go", "google", "type", "*.pb.go"))
		return 0, stderr.String(), matches
	}
}
