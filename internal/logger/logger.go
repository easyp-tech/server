package logger

import (
	"os"

	"golang.org/x/exp/slog"
)

func New(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	return slog.New(
		slog.NewTextHandler(
			os.Stdout,
			&slog.HandlerOptions{ //nolint:exhaustruct
				// AddSource: true,
				Level: level,
			},
		),
	)
}
