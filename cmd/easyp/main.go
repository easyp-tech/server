package main

import (
	"flag"
	"net/http"
	"os"

	"golang.org/x/exp/slog"

	"github.com/easyp-tech/server/cmd/easyp/internal/config"
	"github.com/easyp-tech/server/internal/connect"
	"github.com/easyp-tech/server/internal/https"
	"github.com/easyp-tech/server/internal/localgit"
	"github.com/easyp-tech/server/internal/logger"
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
		log      = logger.New(*debug)
		nameLock = namedlocks.New(minNumberOfRepos)
		handler  = connect.New(localgit.New(cfg.Storage, nameLock), cfg.Domain)
		serve    = func() error { return http.ListenAndServe(cfg.Listen.String(), handler) } //nolint:gosec,wrapcheck
	)

	log.Debug("started", slog.Any("config", cfg))

	if cfg.TLS.CertFile != "" {
		serve = func() error {
			//nolint:wrapcheck
			return https.ListenAndServe(cfg.Listen, handler, cfg.TLS.CertFile, cfg.TLS.KeyFile, cfg.TLS.CACertFile)
		}
	}

	if err := serve(); err != nil {
		log.Error("shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}

	return v
}
