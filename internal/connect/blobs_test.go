package connect

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	v1alpha1connect "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect"
	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/source"
	"github.com/easyp-tech/server/internal/shake256"
)

// countingProvider wraps a provider and counts GetFiles calls, so the
// resolution-error test (TestDownloadManifestAndBlobs_ResolveError) can prove
// the handler never reached the provider on an unresolvable UUID — the
// hallmark of PR-22-3 (no silent HEAD fallback / no wrong content served).
type countingProvider struct {
	inner         provider
	getFilesCalls *atomic.Int32
}

func (c *countingProvider) GetMeta(ctx context.Context, owner, repo, commit string) (content.Meta, error) {
	return c.inner.GetMeta(ctx, owner, repo, commit)
}

func (c *countingProvider) GetFiles(ctx context.Context, owner, repo, commit string) ([]content.File, error) {
	c.getFilesCalls.Add(1)
	return c.inner.GetFiles(ctx, owner, repo, commit)
}

func (c *countingProvider) Repositories() []source.Source { return c.inner.Repositories() }

// TestDownloadManifestAndBlobs_ResolveUUID (PR-22-1): warm-cache case.
// Pre-seed the commitMap by issuing a real v1 CommitService/GetCommits POST
// against a testMux-backed httptest.Server. Then issue a v1alpha1
// DownloadServiceClient.DownloadManifestAndBlobs with Reference = the 32-char
// UUID returned in the GetCommits response. The handler MUST resolve the UUID
// to the SHA and pass the SHA to provider.GetFiles.
//
// The mockProvider is keyed by the 40-char SHA in filesByCommit, so a UUID-keyed
// GetFiles lookup fails loudly — if the handler forgets to resolve, the RPC
// errors instead of returning content.
func TestDownloadManifestAndBlobs_ResolveUUID(t *testing.T) {
	const headSha = "f00dcafe00000000000000000000000000000000"
	uuid, err := commitUUID(headSha)
	if err != nil {
		t.Fatalf("commitUUID(%q): %v", headSha, err)
	}
	t.Logf("fixture: headSha=%s uuid=%s", headSha, uuid)

	headMeta := content.Meta{Commit: headSha, DefaultBranch: "main"}
	p := &mockProvider{
		byCommit: map[string]content.Meta{
			"":      headMeta, // GetMeta(HEAD) during warm-seed
			headSha: headMeta, // safety net
		},
		filesByCommit: map[string][]content.File{
			headSha: {
				{Path: "test.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
			},
		},
	}
	mux := testMux(p)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Pre-seed commit cache via CommitService/GetCommits (raw HTTP POST).
	commitResp, err := http.Post(
		server.URL+"/buf.registry.module.v1.CommitService/GetCommits",
		"application/proto",
		bytes.NewReader(buildGetCommitsRequest("owner", "repo")),
	)
	if err != nil {
		t.Fatalf("pre-seed GetCommits request failed: %v", err)
	}
	commitBody, _ := io.ReadAll(commitResp.Body)
	commitResp.Body.Close()

	commitID := extractCommitID(commitBody)
	if commitID == "" {
		t.Fatal("failed to extract commit ID from GetCommits response")
	}
	if commitID != uuid {
		t.Fatalf("extracted commit ID %q does not match commitUUID(headSha) %q", commitID, uuid)
	}

	// Issue DownloadManifestAndBlobs via the v1alpha1 Connect client.
	client := v1alpha1connect.NewDownloadServiceClient(server.Client(), server.URL)
	resp, err := client.DownloadManifestAndBlobs(
		context.Background(),
		connect.NewRequest(&registry.DownloadManifestAndBlobsRequest{
			Owner:      "owner",
			Repository: "repo",
			Reference:  uuid,
		}),
	)
	if err != nil {
		t.Fatalf("DownloadManifestAndBlobs(uuid=%s) returned error: %v", uuid, err)
	}
	if resp == nil || resp.Msg == nil {
		t.Fatal("nil response or message")
	}
	if resp.Msg.GetManifest() == nil {
		t.Fatal("manifest is nil — handler did not serve files for the resolved SHA")
	}
	if len(resp.Msg.GetManifest().GetContent()) == 0 {
		t.Fatal("manifest content is empty — handler did not resolve UUID to SHA before GetFiles")
	}
	if len(resp.Msg.GetBlobs()) == 0 {
		t.Fatal("no blobs returned — handler did not resolve UUID to SHA before GetFiles")
	}
}

// TestDownloadManifestAndBlobs_ProbeFallback (PR-22-2): cold-cache case.
// Start with an empty commitMap (no pre-seed). The mockProvider has a
// mockSource owning the headSha. Issue DownloadManifestAndBlobs with the UUID.
// The handler must reach probeCommitID (which uses commitUUIDInverse +
// prefix-match) to resolve the UUID. Assert sourceCalls > 0 to prove the
// probe fan-out ran, not just the in-session cache.
func TestDownloadManifestAndBlobs_ProbeFallback(t *testing.T) {
	const headSha = "81353411f7b010d5b9ebeb1899066aac18a36701"
	uuid, err := commitUUID(headSha)
	if err != nil {
		t.Fatalf("commitUUID(%q): %v", headSha, err)
	}
	t.Logf("fixture: headSha=%s uuid=%s", headSha, uuid)

	var sourceCalls atomic.Int32
	p := &mockProvider{
		meta: content.Meta{Commit: headSha, DefaultBranch: "main"},
		files: []content.File{
			{Path: "test.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
		repos: []source.Source{
			&mockSource{
				owner:        "cyp",
				repoName:     "cyp-apis",
				commit:       headSha,
				getMetaCalls: &sourceCalls,
			},
		},
	}
	mux := testMuxWithConfig(p, slog.New(slog.NewTextHandler(io.Discard, nil)), CommitResolution{
		ProbeEnabled:     true,
		ProbeNegativeTTL: 5 * time.Minute,
		ProbeTimeout:     2 * time.Second,
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := v1alpha1connect.NewDownloadServiceClient(server.Client(), server.URL)
	resp, err := client.DownloadManifestAndBlobs(
		context.Background(),
		connect.NewRequest(&registry.DownloadManifestAndBlobsRequest{
			Owner:      "cyp",
			Repository: "cyp-apis",
			Reference:  uuid,
		}),
	)
	if err != nil {
		t.Fatalf("DownloadManifestAndBlobs(uuid=%s) returned error: %v", uuid, err)
	}
	if resp == nil || resp.Msg == nil || resp.Msg.GetManifest() == nil {
		t.Fatal("nil response / message / manifest — handler did not serve content")
	}
	if len(resp.Msg.GetManifest().GetContent()) == 0 {
		t.Fatal("manifest content is empty")
	}
	if sourceCalls.Load() < 1 {
		t.Errorf("expected at least 1 source GetMeta call (probe fan-out), got %d — "+
			"handler did not reach probeCommitID for cold-cache UUID", sourceCalls.Load())
	}
}

// TestDownloadManifestAndBlobs_ResolveError (PR-22-3): resolution-failure case.
// Issue DownloadManifestAndBlobs with a UUID whose 14-byte prefix matches no
// configured source. The handler MUST return a Connect error and MUST NOT call
// provider.GetFiles (no silent HEAD fallback serving wrong content).
func TestDownloadManifestAndBlobs_ResolveError(t *testing.T) {
	const (
		unknownUUID = "ffffffffffffffffffffffffffffffff" // prefix = "ff" * 14
		headSha     = "0000000000000000000000000000000000000000"
	)

	var getFilesCalls atomic.Int32
	inner := &mockProvider{
		meta: content.Meta{Commit: headSha, DefaultBranch: "main"},
		files: []content.File{
			{Path: "test.proto", Data: []byte("syntax = \"proto3\";"), Hash: shake256.Hash{}},
		},
		repos: []source.Source{
			&mockSource{owner: "cyp", repoName: "cyp-apis", commit: headSha},
		},
	}
	p := &countingProvider{inner: inner, getFilesCalls: &getFilesCalls}

	mux := testMuxWithConfig(p, slog.New(slog.NewTextHandler(io.Discard, nil)), CommitResolution{
		ProbeEnabled:     true,
		ProbeNegativeTTL: 5 * time.Minute,
		ProbeTimeout:     2 * time.Second,
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := v1alpha1connect.NewDownloadServiceClient(server.Client(), server.URL)
	resp, err := client.DownloadManifestAndBlobs(
		context.Background(),
		connect.NewRequest(&registry.DownloadManifestAndBlobsRequest{
			Owner:      "cyp",
			Repository: "cyp-apis",
			Reference:  unknownUUID,
		}),
	)
	if err == nil {
		t.Fatalf("expected non-OK Connect error for unresolvable UUID, got success: %v", resp)
	}
	// connect-go has no CodeOK sentinel; CodeOf returns CodeUnknown for non-connect
	// errors. The load-bearing assertion is that the handler surfaced a real error
	// (above) and never touched the provider (below).
	if getFilesCalls.Load() != 0 {
		t.Errorf("provider.GetFiles must not be called on resolution failure; got %d calls "+
			"(silent HEAD fallback / wrong content risk)", getFilesCalls.Load())
	}
}
