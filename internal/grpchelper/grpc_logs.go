package grpchelper

import (
	"context"
	"fmt"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc/codes"

	"github.com/easyp-tech/server/internal/logger"
	"github.com/easyp-tech/server/internal/metrics"
)

func recoveryFunc(m metrics.Metrics, err error) grpc_recovery.RecoveryHandlerFuncContext {
	return func(ctx context.Context, p interface{}) error {
		m.PanicsTotal.Inc()

		l := logger.FromContext(ctx)
		l.Error("panic",
			slog.String(logger.GRPCCode, codes.Internal.String()),
			slog.Any(logger.PanicReason, p),
		)

		return err
	}
}

func interceptorLogger(l *slog.Logger) logging.Logger { //nolint:ireturn
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		switch lvl {
		case logging.LevelDebug:
			l.Debug(msg, fields...)
		case logging.LevelInfo:
			l.Info(msg, fields...)
		case logging.LevelWarn:
			l.Warn(msg, fields...)
		case logging.LevelError:
			l.Error(msg, fields...)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}
