package connect

import (
	"net/http"

	connect "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect"
	"github.com/easyp-tech/server/internal/content"
)

type application interface {
	Get(owner, repoName, commit string) (content.Meta, error)
	GetWithFiles(owner, repoName, commit string) (content.Meta, []content.File, error)
}

type api struct {
	connect.UnimplementedRepositoryServiceHandler
	connect.UnimplementedResolveServiceHandler
	connect.UnimplementedDownloadServiceHandler
	repo   application
	domain string
}

// New creates and returns gRPC server.
func New(
	core application,
	domain string,
) *http.ServeMux {
	a := &api{ //nolint:exhaustruct
		repo:   core,
		domain: domain,
	}

	mux := http.NewServeMux()
	mux.Handle(connect.NewResolveServiceHandler(a))
	mux.Handle(connect.NewRepositoryServiceHandler(a))
	mux.Handle(connect.NewDownloadServiceHandler(a))

	return mux
}
