package logkey

import (
	"context"
	"log/slog"
)

type ctxMarker struct{}

// NewContext returns context with slog.Logger.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxMarker{}, logger)
}

// FromContext returns slog.Logger from context.
func FromContext(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value(ctxMarker{}).(*slog.Logger)
	if !ok {
		return slog.Default()
	}

	return l
}
