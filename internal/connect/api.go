package connect

import (
	"context"
	"net/http"
	"time"

	"log/slog"

	"connectrpc.com/connect"

	v1alpha1connect "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect"
	"github.com/easyp-tech/server/internal/providers/content"
	"github.com/easyp-tech/server/internal/providers/source"
)

type provider interface {
	GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error)
	GetFiles(ctx context.Context, owner, repoName, commit string) ([]content.File, error)
	Repositories() []source.Source
}

// CommitResolver resolves a buf-issued commit id (a 32-char dashless UUID
// minted by commitUUID) to the 40- or 64-char git SHA that the upstream
// source actually points at. It reuses the same commitMap /
// resolveForeignCommitID / probeCommitID ladder as ServeDownload so the
// v1alpha1 DownloadManifestAndBlobs path inherits the Phase 18 prefix-match
// validation and negative caching. Returns an error when the id cannot be
// resolved by any configured source — callers MUST surface the error and
// MUST NOT fall back to HEAD (RESEARCH.md anti-pattern "Returning HEAD when
// resolution fails").
type CommitResolver interface {
	resolveCommitForRead(ctx context.Context, owner, module, id string) (sha string, err error)
}

// CommitResolution configures the buf v1 commit-id resolution enhancements in
// commitServiceHandler: the upstream sha probe used on a Download cache miss.
// It is the connect-package mirror of the user-facing connect config — kept
// here (not imported from cmd/easyp/internal/config) to avoid an
// internal→cmd layering violation. A zero value disables the enhancement
// (the historical behavior), so callers that construct the mux via New
// (tests) are unaffected; production threads it via NewWithConfig.
type CommitResolution struct {
	ProbeEnabled     bool
	ProbeNegativeTTL time.Duration
	ProbeTimeout     time.Duration
}

type api struct {
	log *slog.Logger
	v1alpha1connect.UnimplementedRepositoryServiceHandler
	v1alpha1connect.UnimplementedResolveServiceHandler
	v1alpha1connect.UnimplementedDownloadServiceHandler
	repo           provider
	domain         string
	commitResolver CommitResolver // assigned in NewWithConfig; nil-guarded at call sites
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello! This is Buf Proxy Service and this is its Health Check!"))
}

// New creates and returns gRPC server. Commit-resolution enhancements (pre-warm,
// sha probe) are disabled; production should use NewWithConfig to enable them.
func New(
	log *slog.Logger,
	core provider,
	domain string,
	opts ...connect.HandlerOption,
) *http.ServeMux {
	return NewWithConfig(log, core, domain, CommitResolution{}, opts...)
}

// NewWithConfig is like New but enables commit-resolution enhancements per cfg.
// ProbeEnabled turns on the upstream sha probe for Download cache misses.
func NewWithConfig(
	log *slog.Logger,
	core provider,
	domain string,
	cfg CommitResolution,
	opts ...connect.HandlerOption,
) *http.ServeMux {
	a := &api{ //nolint:exhaustruct
		log:    log,
		repo:   core,
		domain: domain,
	}

	mux := http.NewServeMux()

	mux.Handle(v1alpha1connect.NewResolveServiceHandler(a, opts...))
	mux.Handle(v1alpha1connect.NewRepositoryServiceHandler(a, opts...))
	mux.Handle(v1alpha1connect.NewDownloadServiceHandler(a, opts...))

	// CommitService/GraphService/DownloadService handlers for buf CLI v1.69.0+.
	// Both v1 and v1beta1 paths are registered because buf CLI uses v1beta1
	// paths with v1 buf.yaml config and v1 paths with v2 buf.yaml config.
	// OwnerService is also part of the v1 API surface that buf CLI calls
	// during `buf dep update`; without it the request falls through to the
	// catch-all rootHandler and the client sees a text/plain response,
	// which the buf CLI rejects with "invalid content-type: ... expecting
	// application/proto".
	knownOwners := buildKnownOwners(core.Repositories())
	singleModule := buildKnownModules(core.Repositories())
	commitHandler := &commitServiceHandler{
		api:             a,
		commitMap:       make(map[string]moduleRef),
		infoCache:       make(map[string]commitInfoCache),
		filesMap:        make(map[string][]content.File),
		knownOwners:     knownOwners,
		singleModule:    singleModule,
		missCache:       make(map[string]time.Time),
		probeEnabled:    cfg.ProbeEnabled,
		probeNegativeTTL: cfg.ProbeNegativeTTL,
		probeTimeout:    cfg.ProbeTimeout,
		probeSem:        make(chan struct{}, maxConcurrentProbes),
	}
	// Wire the commit handler back to *api so v1alpha1 Connect-RPC handlers
	// (DownloadManifestAndBlobs) can resolve UUIDs through the Phase 18 ladder.
	// Safe post-construction mutation: NewWithConfig runs single-threaded at
	// startup and all handler invocations happen after this function returns.
	a.commitResolver = commitHandler
	if commitHandler.probeEnabled && commitHandler.probeNegativeTTL > 0 {
		go commitHandler.sweepMisses(context.Background())
	}
	mux.HandleFunc("/buf.registry.module.v1.CommitService/", commitHandler.ServeHTTP)
	mux.HandleFunc("/buf.registry.module.v1beta1.CommitService/", commitHandler.ServeHTTP)
	mux.HandleFunc("/buf.registry.module.v1.GraphService/", commitHandler.ServeGraph)
	mux.HandleFunc("/buf.registry.module.v1beta1.GraphService/", commitHandler.ServeGraph)
	mux.HandleFunc("/buf.registry.module.v1.DownloadService/", commitHandler.ServeDownload)
	mux.HandleFunc("/buf.registry.module.v1beta1.DownloadService/", commitHandler.ServeDownload)
	mux.HandleFunc("/buf.registry.module.v1.ModuleService/", commitHandler.ServeGetModules)
	mux.HandleFunc("/buf.registry.module.v1beta1.ModuleService/", commitHandler.ServeGetModules)
	mux.HandleFunc("/buf.registry.owner.v1.OwnerService/", commitHandler.ServeGetOwners)

	mux.HandleFunc("/", rootHandler)

	return mux
}
