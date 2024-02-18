package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/ghodss/yaml"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc/grpclog"

	"github.com/easyp-tech/server/cmd/easyp/internal/config"
	"github.com/easyp-tech/server/internal/api"
	"github.com/easyp-tech/server/internal/core"
	"github.com/easyp-tech/server/internal/grpchelper"
	"github.com/easyp-tech/server/internal/logger"
	"github.com/easyp-tech/server/internal/metrics"
	"github.com/easyp-tech/server/internal/serve"
	"github.com/easyp-tech/server/internal/store"
)

//nolint:gochecknoglobals
var (
	cfgFile = flag.String("cfg", "./local.config.yml", "path to Config file")
	debug   = flag.Bool("debug", false, "enable debug logging")
)

func main() {
	flag.Parse()

	cfg := must(readYaml[config.Config](*cfgFile))
	log := buildLogger(*debug)

	grpclog.SetLoggerV2(grpchelper.NewLogger(log))

	if err := start(context.Background(), cfg, os.Args[0], log); err != nil {
		log.Error("shutdown", slog.String(logger.Error, err.Error()))
		os.Exit(1)
	}
}

func start(
	ctx context.Context,
	cfg config.Config,
	namespace string,
	log *slog.Logger,
) error {
	p := prometheus.NewPedanticRegistry()
	m := metrics.New(p, namespace)

	s := store.New(ctx, cfg.Store.Root)
	module := core.New(s)
	_, httpAPI := api.New(ctx, m, module, p, namespace, cfg.Server.External.Domain)

	return serve.Start( //nolint:wrapcheck
		ctx,
		serve.Metrics(
			log.With(slog.String(logger.Module, "metric")),
			cfg.Server.External.Host,
			cfg.Server.External.Port.Metric,
			p,
		),
		serve.HTTP(
			log.With(slog.String(logger.Module, "connect")),
			cfg.Server.External.Host,
			cfg.Server.External.Port.Connect,
			httpAPI,
		),
	)
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

func ternary[T any](cond bool, ifTrue, ifFalse T) T {
	if cond {
		return ifTrue
	}

	return ifFalse
}
