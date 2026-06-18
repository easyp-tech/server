package connect

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

type requestIDKey struct{}

// WithRequestID stores a request ID in the context for log correlation.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom extracts the request ID from context, returning empty string if absent.
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// NewLoggingInterceptor returns a connect.Option that wraps unary handlers with
// structured request/response logging at debug level.
func NewLoggingInterceptor(log *slog.Logger) connect.Option {
	return connect.WithInterceptors(&loggingInterceptor{log: log})
}

type loggingInterceptor struct {
	log *slog.Logger
}

// WrapUnary implements connect.Interceptor.
func (i *loggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		start := time.Now()
		procedure := req.Spec().Procedure

		// Attach procedure and request_id to the logger for this RPC
		logger := i.log.With(slog.String("procedure", procedure))
		if reqID := RequestIDFrom(ctx); reqID != "" {
			logger = logger.With(slog.String("request_id", reqID))
		}

		if logger.Enabled(ctx, slog.LevelDebug) {
			var reqSize int
			if m := req.Any(); m != nil {
				if msg, ok := m.(proto.Message); ok {
					reqSize = proto.Size(msg)
				}
			}

			logger.LogAttrs(ctx, slog.LevelDebug, "rpc started",
				slog.String("peer", req.Peer().Addr),
				slog.Int("request_size", reqSize),
			)
		}

		resp, err := next(ctx, req)

		duration := time.Since(start)

		if err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "rpc failed",
				slog.Duration("duration", duration),
				slog.String("error", err.Error()),
				slog.String("code", connect.CodeOf(err).String()),
			)
		} else if logger.Enabled(ctx, slog.LevelDebug) {
			var respSize int
			if resp != nil && resp.Any() != nil {
				if msg, ok := resp.Any().(proto.Message); ok {
					respSize = proto.Size(msg)
				}
			}

			logger.LogAttrs(ctx, slog.LevelDebug, "rpc completed",
				slog.Duration("duration", duration),
				slog.Int("response_size", respSize),
			)
		}

		return resp, err
	}
}

// WrapStreamingClient is required by connect.Interceptor.
func (i *loggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler is required by connect.Interceptor.
func (i *loggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
