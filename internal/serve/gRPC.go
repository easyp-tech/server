package serve

import (
	"context"
	"fmt"
	"net"

	"golang.org/x/exp/slog"
	"google.golang.org/grpc"

	"github.com/easyp-tech/server/internal/logger"
)

// GRPC starts gRPC server on addr, logged as service.
// It runs until failed or ctx.Done.
func GRPC(log *slog.Logger, host string, port uint16, srv *grpc.Server) func(context.Context) error {
	return func(ctx context.Context) error {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
		if err != nil {
			return fmt.Errorf("net.Listen: %w", err)
		}

		errc := make(chan error, 1)
		go func() { errc <- srv.Serve(ln) }()
		log.Info("started", slog.String(logger.Host, host), slog.Uint64(logger.Port, uint64(port)))

		defer log.Info("shutdown")

		select {
		case err = <-errc:
		case <-ctx.Done():
			srv.GracefulStop() // It will not interrupt streaming.
		}

		if err != nil {
			return fmt.Errorf("srv.Serve: %w", err)
		}

		return nil
	}
}
