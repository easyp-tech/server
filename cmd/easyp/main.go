package main

import (
	"flag"
	"net/http"
	"os"

	"golang.org/x/exp/slog"

	"github.com/easyp-tech/server/cmd/easyp/internal/config"
	"github.com/easyp-tech/server/internal/connect"
	"github.com/easyp-tech/server/internal/localgit"
	"github.com/easyp-tech/server/internal/namedlocks"
)

//nolint:gochecknoglobals
var (
	cfgFile = flag.String("cfg", "./local.config.yml", "path to Config file")
	debug   = flag.Bool("debug", false, "enable debug logging")
)

const (
	minNumberOfRepos = 1024
)

func main() {
	flag.Parse()

	var (
		cfg      = must(config.ReadYaml[config.Config](*cfgFile))
		log      = buildLogger(*debug)
		nameLock = namedlocks.New(minNumberOfRepos)
		handler  = connect.New(localgit.New(cfg.Storage, nameLock), cfg.Domain)
	)

	if err := http.ListenAndServe(cfg.Listen.String(), handler); err != nil { //nolint:gosec
		log.Error("shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func buildLogger(debug bool) *slog.Logger {
	return slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{ //nolint:exhaustruct
				AddSource: true,
				Level:     ternary(debug, slog.LevelDebug, slog.LevelInfo),
			},
		),
	)
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}

	return v
}

func ternary[T any](cond bool, ifTrue, ifFalse T) T {
	if cond {
		return ifTrue
	}

	return ifFalse
}
