package main

import (
	"flag"
	"net/http"
	"os"

	"golang.org/x/exp/slog"

	"github.com/easyp-tech/server/cmd/easyp/internal/config"
	"github.com/easyp-tech/server/internal/connect"
	"github.com/easyp-tech/server/internal/https"
	"github.com/easyp-tech/server/internal/logger"
	"github.com/easyp-tech/server/internal/providers/cache"
	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/easyp-tech/server/internal/providers/github"
	"github.com/easyp-tech/server/internal/providers/localgit"
	"github.com/easyp-tech/server/internal/providers/localgit/namedlocks"
	"github.com/easyp-tech/server/internal/providers/multisource"
)

//nolint:gochecknoglobals
var (
	cfgFile = flag.String("cfg", "./local.config.yml", "path to Config file")
	debug   = flag.Bool("debug", false, "enable debug logging")
)

const (
	minNumberOfRepos = 128
)

func main() {
	flag.Parse()

	var (
		cfg      = must(config.ReadYaml[config.Config](*cfgFile))
		log      = logger.New(*debug)
		nameLock = namedlocks.New(minNumberOfRepos)
		cache    = cache.FileCache{Dir: cfg.Cache}
		storage  = multisource.New(
			log,
			cache,
			localgit.New(cfg.Local.Storage, filterRepos(cfg.Local.Repos), nameLock),
			githubProxy(log, cfg.Proxy.Github),
		)
		handler = connect.New(log, storage, cfg.Domain)
		serve   = func() error { return http.ListenAndServe(cfg.Listen.String(), handler) } //nolint:gosec,wrapcheck
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

func githubProxy(log *slog.Logger, defs config.Github) multisource.Source {
	repos := make([]github.Repo, 0, len(defs.Repos))
	for _, def := range defs.Repos {
		repos = append(
			repos,
			github.Repo{
				Token: ternary(def.AccessToken != "", def.AccessToken, defs.AccessToken),
				Repo: filter.Repo{
					Owner:    def.Repo.Owner,
					Name:     def.Repo.Name,
					Prefixes: def.Repo.Prefixes,
					Paths:    def.Repo.Paths,
				},
			},
		)
	}

	return github.NewMultiRepo(log, repos, defs.AccessToken)
}

func filterRepos(defs []config.Repo) []filter.Repo {
	repos := make([]filter.Repo, 0, len(defs))
	for _, def := range defs {
		repos = append(
			repos,
			filter.Repo{
				Owner:    def.Owner,
				Name:     def.Name,
				Prefixes: def.Prefixes,
				Paths:    def.Paths,
			},
		)
	}

	return repos
}

func ternary[T any](cond bool, ifTrue, ifFalse T) T {
	if cond {
		return ifTrue
	}

	return ifFalse
}
