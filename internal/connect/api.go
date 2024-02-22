package connect

import (
	"context"
	"net/http"

	"golang.org/x/exp/slog"

	connect "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect"
	"github.com/easyp-tech/server/internal/providers/content"
)

type provider interface {
	GetMeta(ctx context.Context, owner, repoName, commit string) (content.Meta, error)
	GetFiles(ctx context.Context, owner, repoName, commit string) ([]content.File, error)
}

type api struct {
	log *slog.Logger
	connect.UnimplementedRepositoryServiceHandler
	connect.UnimplementedResolveServiceHandler
	connect.UnimplementedDownloadServiceHandler
	repo   provider
	domain string
}

// New creates and returns gRPC server.
func New(
	log *slog.Logger,
	core provider,
	domain string,
) *http.ServeMux {
	a := &api{ //nolint:exhaustruct
		log:    log,
		repo:   core,
		domain: domain,
	}

	mux := http.NewServeMux()
	mux.Handle(connect.NewResolveServiceHandler(a))
	mux.Handle(connect.NewRepositoryServiceHandler(a))
	mux.Handle(connect.NewDownloadServiceHandler(a))

	return mux
}
