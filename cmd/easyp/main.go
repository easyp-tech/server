package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/easyp-tech/server/cmd/easyp/internal/config"
	"github.com/easyp-tech/server/cmd/easyp/internal/config/cachetype"
	"github.com/easyp-tech/server/internal/connect"
	"github.com/easyp-tech/server/internal/https"
	"github.com/easyp-tech/server/internal/logger"
	"github.com/easyp-tech/server/internal/providers/bitbucket"
	"github.com/easyp-tech/server/internal/providers/cache"
	"github.com/easyp-tech/server/internal/providers/cache/artifactory"
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
		cache    = buildCache(log, cfg.Cache)
		storage  = multisource.New(
			log,
			cache,
			localgit.New(cfg.Local.Storage, filterRepos(cfg.Local.Repos), nameLock),
			bbProxy(log, cfg.Proxy.BitBucket),
			githubProxy(log, cfg.Proxy.Github),
		)
		handler = connect.New(log, storage, cfg.Domain)
		serve   = func() error { return http.ListenAndServe(cfg.Listen.String(), handler) } //nolint:gosec
	)

	log.Debug("started", slog.Any("config", cfg))

	if cfg.TLS.CertFile != "" {
		serve = func() error {
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

func githubProxy(log *slog.Logger, defs []config.GithubRepo) multisource.Source { //nolint:ireturn
	repos := make([]github.Repo, 0, len(defs))
	for _, def := range defs {
		repos = append(
			repos,
			github.Repo{
				Token: def.AccessToken,
				Repo: filter.Repo{
					Owner:    def.Repo.Owner,
					Name:     def.Repo.Name,
					Prefixes: def.Repo.Prefixes,
					Paths:    def.Repo.Paths,
				},
			},
		)
	}

	return github.NewMultiRepo(log, repos)
}

func bbProxy(log *slog.Logger, defs []config.BitBucketRepo) multisource.Source { //nolint:ireturn
	repos := make([]bitbucket.Repo, 0, len(defs))
	for _, def := range defs {
		repos = append(
			repos,
			bitbucket.Repo{
				User:     bitbucket.User(def.User),
				Password: bitbucket.Password(def.AccessToken),
				URL:      def.BaseURL.URL,
				Repo: filter.Repo{
					Owner:    def.Repo.Owner,
					Name:     def.Repo.Name,
					Prefixes: def.Repo.Prefixes,
					Paths:    def.Repo.Paths,
				},
			},
		)
	}

	return bitbucket.NewMultiRepo(log, repos)
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

func buildCache(log *slog.Logger, cfg config.Cache) multisource.Cache { //nolint:ireturn
	switch cfg.Type {
	case cachetype.None:
		return cache.Noop{}
	case cachetype.Local:
		return cache.Local{Dir: cfg.Local.Dir}
	case cachetype.Artifactory:
		return artifactory.New(
			log,
			cfg.Artifactory.BaseURL.String(),
			cfg.Artifactory.User,
			cfg.Artifactory.AccessToken,
		)
	default:
		panic("unreachable reached")
	}
}
