// Package testutil provides reusable test helpers for E2E proxy tests.
//
// It extracts the inline test infrastructure from e2e/smoke_test.go into
// a proper, importable package for use by Phases 4 and 5 integration tests.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfig holds the configuration for starting a test proxy server.
type TestConfig struct {
	// TLSCertPath is the path to the TLS certificate file.
	TLSCertPath string
	// TLSKeyPath is the path to the TLS private key file.
	TLSKeyPath string
	// GithubToken is the GitHub API token for authenticating requests.
	GithubToken string
	// RepoOwner is the GitHub repository owner (e.g., "googleapis").
	RepoOwner string
	// RepoName is the GitHub repository name (e.g., "googleapis").
	RepoName string
	// RepoPaths is the list of proto paths to serve from the repository.
	RepoPaths []string
	// LogLevel is the proxy log level. Defaults to "info" if empty.
	LogLevel string
}

// DefaultTestConfig returns a TestConfig populated from environment variables
// and sensible defaults. Tests should call RequireEnvToken to ensure the
// GitHub token is present before using this config with StartServer.
func DefaultTestConfig() TestConfig {
	home := os.Getenv("HOME")
	return TestConfig{
		TLSCertPath: filepath.Join(home, "local-tls", "server", "server-cert.pem"),
		TLSKeyPath:  filepath.Join(home, "local-tls", "server", "server-key.pem"),
		GithubToken: os.Getenv("EASYP_GITHUB_TOKEN"),
		RepoOwner:   "googleapis",
		RepoName:    "googleapis",
		RepoPaths:   []string{"google/type/"},
		LogLevel:    "info",
	}
}

// generateConfigYAML writes a proxy YAML config file into a temp directory
// and returns the file path. The config file is written with mode 0600 to
// prevent world-readable access to the GitHub token (T-03-02).
func generateConfigYAML(t *testing.T, cfg TestConfig, port int) string {
	t.Helper()

	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")

	content := fmt.Sprintf(`listen: "127.0.0.1:%d"
domain: "127.0.0.1:%d"
log:
  level: %q
cache:
  type: "none"
tls:
  cert: %s
  key:  %s
proxy:
  github:
    - token: %s
      repo:
        owner: %s
        name:  %s
        path:
%s
`,
		port, port,
		logLevel,
		cfg.TLSCertPath, cfg.TLSKeyPath,
		cfg.GithubToken,
		cfg.RepoOwner, cfg.RepoName,
		formatYAMLPaths(cfg.RepoPaths),
	)

	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	return cfgPath
}

// formatYAMLPaths formats a slice of path strings as YAML list items.
func formatYAMLPaths(paths []string) string {
	result := ""
	for _, p := range paths {
		result += fmt.Sprintf("          - %q\n", p)
	}
	return result
}

// findProjectRoot returns the project root directory by locating this source
// file via runtime.Caller and walking up to the module root.
// Since this file lives at e2e/testutil/config.go, it takes three Dir calls:
// config.go -> testutil/ -> e2e/ -> project-root
func findProjectRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	// This file is at <project-root>/e2e/testutil/config.go
	dir := filepath.Dir(filename)     // e2e/testutil/
	dir = filepath.Dir(dir)            // e2e/
	root := filepath.Dir(dir)          // project root

	// Verify it's the project root by checking go.mod
	goModPath := filepath.Join(root, "go.mod")
	info, err := os.Stat(goModPath)
	require.NoError(t, err, "go.mod not found at project root %s", root)
	require.False(t, info.IsDir(), "go.mod is a directory at %s", root)

	return root
}
