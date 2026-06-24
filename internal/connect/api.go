package connect

import (
	"context"
	"net/http"

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

type api struct {
	log *slog.Logger
	v1alpha1connect.UnimplementedRepositoryServiceHandler
	v1alpha1connect.UnimplementedResolveServiceHandler
	v1alpha1connect.UnimplementedDownloadServiceHandler
	repo   provider
	domain string
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello! This is Buf Proxy Service and this is its Health Check!"))
}

// New creates and returns gRPC server.
func New(
	log *slog.Logger,
	core provider,
	domain string,
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
		api: a, commitMap: make(map[string]moduleRef),
		infoCache:   make(map[string]commitInfoCache),
		filesMap:    make(map[string][]content.File),
		knownOwners: knownOwners,
		singleModule: singleModule,
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
