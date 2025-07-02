package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/easyp-tech/server/cmd/easyp/internal/config"
	"github.com/easyp-tech/server/cmd/easyp/internal/config/cachetype"
	"github.com/easyp-tech/server/internal/connect"
	"github.com/easyp-tech/server/internal/https"
	"github.com/easyp-tech/server/internal/providers/bitbucket"
	"github.com/easyp-tech/server/internal/providers/cache"
	"github.com/easyp-tech/server/internal/providers/cache/artifactory"
	"github.com/easyp-tech/server/internal/providers/filter"
	"github.com/easyp-tech/server/internal/providers/github"
	"github.com/easyp-tech/server/internal/providers/localgit"
	"github.com/easyp-tech/server/internal/providers/localgit/namedlocks"
	"github.com/easyp-tech/server/internal/providers/multisource"
	"golang.org/x/exp/slog"
)

//nolint:gochecknoglobals
var (
	cfgFile = flag.String("cfg", "./local.config.yml", "path to Config file")
)

const (
	minNumberOfRepos  = 128
	connectionTimeout = 5 * time.Second
)

func main() {
	flag.Parse()
	var (
		cfg      = must(config.ReadYaml[config.Config](*cfgFile))
		log      = newLogger(cfg.Log.Level)
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
		serve   = func() error { return http.ListenAndServe(cfg.Listen.String(), loggingMiddleware(log, handler)) } //nolint:gosec
	)

	// log.Info("Service started successfully.")

	// 1. Check repository connections
	checkRepositoryConnections(log, storage)

	// 2. Check cache connection if applicable
	if cache != nil {
		checkCacheConnection(log, cache)
	}

	if cfg.TLS.CertFile != "" {
		serve = func() error {
			return https.ListenAndServe(cfg.Listen, loggingMiddleware(log, handler), cfg.TLS.CertFile, cfg.TLS.KeyFile, cfg.TLS.CACertFile)
		}
	}

	if err := serve(); err != nil {
		log.Error("shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// Check cache connection (Artifactory or Local)
func checkCacheConnection(log *slog.Logger, cache multisource.Cache) {
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	if err := cache.Ping(ctx); err != nil {
		log.Error("cache connection failed",
			slog.String("type", fmt.Sprintf("%T", cache)),
			slog.String("error", err.Error()))
	} else {
		log.Info("cache connected",
			slog.String("type", fmt.Sprintf("%T", cache)))
	}
}

// Check connections to all repositories
func checkRepositoryConnections(log *slog.Logger, storage multisource.Repo) {
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	for _, repo := range storage.Repositories() {
		_, err := repo.GetMeta(ctx, "")
		if err != nil {
			log.Error("repository connection failed",
				slog.String("owner", repo.Owner()),
				slog.String("name", repo.RepoName()),
				slog.String("type", repo.Type()),
				slog.String("error", err.Error()))
		} else {
			log.Info("repository connected",
				slog.String("owner", repo.Owner()),
				slog.String("name", repo.RepoName()),
				slog.String("type", repo.Type()))
		}
	}
}

func newLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: false,
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

// Enhanced HTTP logging with security and optimization
func loggingMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		start := time.Now()
		requestID := r.Header.Get("X-Request-Id")
		w.Header().Set("X-Request-Id", requestID)
		clientIP := getClientIP(r)

		// Mask sensitive headers in debug logs
		if log.Enabled(ctx, slog.LevelDebug) {
			headers := r.Header.Clone()
			maskSensitiveHeaders(headers)
			log.DebugContext(ctx, "request details",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("request_id", requestID),
				slog.String("client_ip", clientIP),
				slog.Any("headers", headers),
			)
		}

		lrw := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		status := lrw.status

		// Log errors with appropriate levels
		if status >= 400 {
			logLevel := slog.LevelWarn
			if status >= 500 {
				logLevel = slog.LevelError
			}

			log.LogAttrs(ctx, logLevel, "request completed",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("request_id", requestID),
				slog.String("client_ip", clientIP),
				slog.Int("status", status),
				slog.Int("size", lrw.size),
				slog.Duration("duration", duration),
			)
		} else if log.Enabled(ctx, slog.LevelDebug) {
			// Log successful requests only in debug mode
			log.DebugContext(ctx, "request completed",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("request_id", requestID),
				slog.String("client_ip", clientIP),
				slog.Int("status", status),
				slog.Int("size", lrw.size),
				slog.Duration("duration", duration),
			)
		}
	})
}

// Helper functions
func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	return r.RemoteAddr
}

// Security: Mask sensitive headers
func maskSensitiveHeaders(headers http.Header) { //nolint:ireturn
	for key := range headers {
		if isSensitiveHeader(key) {
			headers.Set(key, "***")
		}
	}
}

func isSensitiveHeader(key string) bool { //nolint:ireturn
	key = strings.ToLower(key)
	return key == "authorization" ||
		key == "cookie" ||
		key == "x-api-key" ||
		key == "token"
}

type loggingResponseWriter struct { //nolint:ireturn
	http.ResponseWriter
	status int
	size   int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) { //nolint:ireturn
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) { //nolint:ireturn
	size, err := lrw.ResponseWriter.Write(b)
	lrw.size += size
	return size, err
}

func must[T any](v T, err error) T { //nolint:ireturn
	if err != nil {
		panic(err)
	}
	return v
}

// Provider initialization
func githubProxy(log *slog.Logger, defs []config.GithubRepo) multisource.Provider { //nolint:ireturn
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

func bbProxy(log *slog.Logger, defs []config.BitBucketRepo) multisource.Provider { //nolint:ireturn
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

func filterRepos(defs []config.Repo) []filter.Repo { //nolint:ireturn
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

// Cache initialization with connection check
func buildCache(log *slog.Logger, cfg config.Cache) multisource.Cache { //nolint:ireturn
	switch cfg.Type {
	case cachetype.None:
		return cache.Noop{}
	case cachetype.Local:
		c := cache.Local{Dir: cfg.Local.Dir}

		// Check local cache directory
		if _, err := os.Stat(cfg.Local.Dir); err != nil {
			log.Error("local cache directory inaccessible",
				slog.String("path", cfg.Local.Dir),
				slog.String("error", err.Error()))
		} else {
			log.Debug("local cache directory ready",
				slog.String("path", cfg.Local.Dir))
		}
		return c

	case cachetype.Artifactory:
		c := artifactory.New(
			log,
			cfg.Artifactory.BaseURL.String(),
			cfg.Artifactory.User,
			cfg.Artifactory.AccessToken,
		)

		// Check Artifactory connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Ping(ctx); err != nil {
			log.Error("Artifactory connection failed",
				slog.String("url", cfg.Artifactory.BaseURL.String()),
				slog.String("error", err.Error()))
		} else {
			log.Info("Artifactory connected",
				slog.String("url", cfg.Artifactory.BaseURL.String()))
		}
		return c

	default:
		panic("unreachable reached")
	}
}
