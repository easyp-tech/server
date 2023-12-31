package api

import (
	"context"
	"errors"
	"net/http"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	registryv1alpha1connect "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1/v1alpha1connect"
	"github.com/easyp-tech/server/internal/core"
	"github.com/easyp-tech/server/internal/grpchelper"
	"github.com/easyp-tech/server/internal/logger"
	"github.com/easyp-tech/server/internal/metrics"
)

type application interface {
	GetRepository(context.Context, core.GetRequest) (*core.Repository, error)
}

type api struct {
	registryv1alpha1connect.UnimplementedRepositoryServiceHandler
	registryv1alpha1connect.UnimplementedResolveServiceHandler
	registryv1alpha1connect.UnimplementedDownloadServiceHandler
	core   application
	domain string
}

// New creates and returns gRPC server.
func New(
	ctx context.Context,
	m metrics.Metrics,
	core application,
	reg *prometheus.Registry,
	namespace,
	domain string,
) (*grpc.Server, *http.ServeMux) {
	log := logger.FromContext(ctx)
	subsystem := "api"

	grpcMetrics := grpchelper.NewServerMetrics(reg, namespace, subsystem)
	srvExternal, _ := grpchelper.NewServer(m, log, grpcMetrics, apiError,
		[]grpc.UnaryServerInterceptor{grpc_auth.UnaryServerInterceptor(nil)},   // Nil because we are using override.
		[]grpc.StreamServerInterceptor{grpc_auth.StreamServerInterceptor(nil)}, // Nil because we are using override.
	)

	a := &api{ //nolint:exhaustruct
		core:   core,
		domain: domain,
	}

	mux := http.NewServeMux()
	path, handler := registryv1alpha1connect.NewResolveServiceHandler(a)
	mux.Handle(path, handler)

	path, handler = registryv1alpha1connect.NewRepositoryServiceHandler(a)
	mux.Handle(path, handler)

	path, handler = registryv1alpha1connect.NewDownloadServiceHandler(a)
	mux.Handle(path, handler)

	return srvExternal, mux
}

func apiError(err error) *status.Status {
	if err == nil {
		return nil
	}

	code := codes.Internal

	switch {
	case errors.Is(err, core.ErrInvalidArgument):
		code = codes.InvalidArgument
	case errors.Is(err, core.ErrNotFound):
		code = codes.NotFound
	case errors.Is(err, context.DeadlineExceeded):
		code = codes.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		code = codes.Canceled
	}

	return status.New(code, err.Error())
}
