package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ghodss/yaml"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc/grpclog"

	"github.com/easyp-tech/server/internal/api"
	"github.com/easyp-tech/server/internal/core"
	"github.com/easyp-tech/server/internal/grpchelper"
	"github.com/easyp-tech/server/internal/logkey"
	"github.com/easyp-tech/server/internal/metrics"
	"github.com/easyp-tech/server/internal/serve"
	"github.com/easyp-tech/server/internal/store"
)

type (
	//nolint:tagliatelle
	config struct {
		DevMode bool    `json:"dev_mode"`
		Server  server  `json:"server"`
		Store   storage `json:"storage"`
	}
	server struct {
		External external `json:"external"`
	}
	ports struct {
		Connect uint16 `json:"connect"`
		Metric  uint16 `json:"metric"`
	}
	external struct {
		Domain string `json:"domain"`
		Host   string `json:"host"`
		Port   ports  `json:"port"`
	}
	storage struct {
		Root string   `json:"root"`
		URLS []string `json:"urls"`
	}
)

//nolint:gochecknoglobals
var cfgFile = flag.String("cfg", "./cmd/easyp/local.config.yml", "path to config file")

func main() {
	flag.Parse()

	cfg := must(readYaml[config](*cfgFile))
	log := buildLogger()

	grpclog.SetLoggerV2(grpchelper.NewLogger(log))

	appName := filepath.Base(os.Args[0])

	ctxParent := logkey.NewContext(context.Background(), log)

	ctx, cancel := signal.NotifyContext(
		ctxParent,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGABRT,
		syscall.SIGTERM,
	)
	defer cancel()

	go forceShutdown(ctx)

	if err := start(ctx, cfg, appName); err != nil {
		log.Error("shutdown", slog.String(logkey.Error, err.Error()))
		os.Exit(1)
	}
}

func start(ctx context.Context, cfg config, namespace string) error {
	log := logkey.FromContext(ctx)
	reg := prometheus.NewPedanticRegistry()
	m := metrics.New(reg, namespace)

	s, err := store.New(ctx, cfg.Store.Root, cfg.Store.URLS)
	if err != nil {
		return fmt.Errorf("store.New: %w", err)
	}

	module := core.New(s)
	_, httpAPI := api.New(ctx, m, module, reg, namespace, cfg.Server.External.Domain)

	return serve.Start( //nolint:wrapcheck
		ctx,
		serve.Metrics(
			log.With(slog.String(logkey.Module, "metric")),
			cfg.Server.External.Host,
			cfg.Server.External.Port.Metric,
			reg,
		),
		serve.HTTP(
			log.With(slog.String(logkey.Module, "gRPC-Gateway")),
			cfg.Server.External.Host,
			cfg.Server.External.Port.Connect,
			httpAPI,
		),
	)
}

func forceShutdown(ctx context.Context) {
	log := logkey.FromContext(ctx)

	const shutdownDelay = 15 * time.Second

	<-ctx.Done()
	time.Sleep(shutdownDelay)

	log.Error("failed to graceful shutdown")
	os.Exit(1)
}

func buildLogger() *slog.Logger {
	return slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{ //nolint:exhaustruct
				AddSource: true,
				Level:     slog.LevelDebug,
			},
		),
	)
}

func readYaml[T any](fileName string) (T, error) {
	dst := new(T)

	data, err := os.ReadFile(fileName)
	if err != nil {
		return *dst, fmt.Errorf("reading %q: %w", fileName, err)
	}

	if err = yaml.Unmarshal(data, dst); err != nil {
		return *dst, fmt.Errorf("parsing %q: %w", fileName, err)
	}

	return *dst, nil
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}

	return v
}
