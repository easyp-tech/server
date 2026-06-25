package testutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Pinned buf binary versions for testing.
const (
	// BufV130 is the last buf version supporting the deprecated registry.v1alpha1 protocol.
	BufV130 = "v1.30.1"
	// BufV169 is a modern buf version using the current protocol.
	BufV169 = "v1.69.0"
)

// AvailableBufVersions returns the list of buf version strings that have a
// cached binary on disk under testdata/buf/. The list is discovered
// dynamically (one directory per version) so that adding a new version to
// the cache is enough to extend the matrix of E2E tests. Order is the
// natural directory sort (lexicographic), which puts older versions first.
//
// Returns an empty slice (without failing the test) when testdata/buf does
// not exist. This is the common case on CI, where the cached binaries are
// gitignored: callers should treat len(versions)==0 as "no binaries to test
// against" and t.Skip() rather than fataling. The directory being unreadable
// for a different reason (permission denied, etc.) is still a hard error.
func AvailableBufVersions(t *testing.T) []string {
	t.Helper()

	projectRoot := findProjectRoot(t)
	dir := filepath.Join(projectRoot, "testdata", "buf")

	// A missing testdata/buf is the expected state on CI (binaries are
	// gitignored) and on a fresh checkout. Treat it as "no versions
	// available" so matrix tests can skip rather than fail the whole run.
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("stat testdata/buf: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("testdata/buf is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading testdata/buf: %v", err)
	}

	versions := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		bin := filepath.Join(dir, e.Name(), "buf")
		if info, err := os.Stat(bin); err == nil && !info.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	return versions
}

// GetBuf returns the path to a pinned buf binary, downloading it from GitHub
// Releases on cache miss. Binaries are cached at testdata/buf/{version}/buf.
//
// Checksum verification is intentionally skipped: the download uses HTTPS from
// GitHub's CDN which provides transport integrity. The binaries are used only
// in tests, not in production.
func GetBuf(t *testing.T, version string) string {
	t.Helper()

	projectRoot := findProjectRoot(t)
	binDir := filepath.Join(projectRoot, "testdata", "buf", version)
	binPath := filepath.Join(binDir, "buf")

	// Check cache: if binary exists and is a regular file, return immediately.
	if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
		return binPath
	}

	// Cache miss: download from GitHub releases.
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("creating buf cache directory: %v", err)
	}

	assetURL := fmt.Sprintf(
		"https://github.com/bufbuild/buf/releases/download/%s/buf-%s-%s",
		version, capitalizeOS(), mapArch(),
	)

	// Download to temp file first, then rename for atomic placement.
	tmpPath := binPath + ".tmp"
	if err := downloadFile(tmpPath, assetURL); err != nil {
		os.Remove(tmpPath)
		t.Fatalf("downloading buf %s from %s: %v", version, assetURL, err)
	}

	// Set execute permission (pitfall: io.Copy preserves source mode, not
	// the execute bit; must chmod explicitly).
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		t.Fatalf("chmod buf binary: %v", err)
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		t.Fatalf("renaming buf binary: %v", err)
	}

	return binPath
}

// RequireEnvToken reads an environment variable and skips the test if it is
// empty. Returns the token value. Use this for required test secrets like
// EASYP_GH_TOKEN (current) or EASYP_GITHUB_TOKEN (legacy).
func RequireEnvToken(t *testing.T, envVar string) string {
	t.Helper()
	val := os.Getenv(envVar)
	if val != "" {
		return val
	}
	// Fall back to the next name in the canonical list, in order, until one
	// resolves. Lets a single token (the modern EASYP_GH_TOKEN) satisfy tests
	// that ask for either spelling.
	for _, name := range githubTokenEnvVars {
		if name == envVar {
			continue
		}
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	t.Skipf("%s (or EASYP_GH_TOKEN / EASYP_GITHUB_TOKEN) not set -- skipping test", envVar)
	return ""
}

// capitalizeOS maps runtime.GOOS to the casing used in buf release asset names.
func capitalizeOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// mapArch maps runtime.GOARCH to the architecture string used in buf release
// asset names.
func mapArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		if runtime.GOOS == "linux" {
			return "aarch64"
		}
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

// downloadFile downloads url to the given local path via HTTP GET.
// Go's net/http follows 302 redirects, which GitHub releases use.
func downloadFile(path, url string) error {
	resp, err := http.Get(url) //nolint:gosec // URL is constructed from known constants
	if err != nil {
		return fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	return nil
}
