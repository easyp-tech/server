package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/grpclog"
	"gopkg.in/yaml.v3"

	"github.com/easyp-tech/server/cmd/easyp/internal/api"
	"github.com/easyp-tech/server/cmd/easyp/internal/core"
	"github.com/easyp-tech/server/cmd/easyp/internal/store"
	"github.com/easyp-tech/server/internal/grpc_helper"
	"github.com/easyp-tech/server/internal/logkey"
	"github.com/easyp-tech/server/internal/metrics"
	"github.com/easyp-tech/server/internal/serve"
)

type (
	config struct {
		DevMode bool    `yaml:"dev_mode"`
		Server  server  `yaml:"server"`
		Store   storage `yaml:"storage"`
	}
	server struct {
		External external `yaml:"external"`
	}
	ports struct {
		Connect uint16 `yaml:"connect"`
		Metric  uint16 `yaml:"metric"`
	}
	external struct {
		Domain string `yaml:"domain"`
		Host   string `yaml:"host"`
		Port   ports  `yaml:"port"`
	}
	storage struct {
		Root string   `yaml:"root"`
		URLS []string `yaml:"urls"`
	}
)

var (
	cfgFile = flag.String("cfg", "./cmd/easyp/local.config.yml", "path to config file")
)

func main() {
	flag.Parse()

	cfgFile, err := os.Open(*cfgFile)
	if err != nil {
		panic(err)
	}

	cfg := config{}
	err = yaml.NewDecoder(cfgFile).Decode(&cfg)
	if err != nil {
		panic(err)
	}

	log, err := buildLogger()
	if err != nil {
		panic(err)
	}
	grpclog.SetLoggerV2(grpc_helper.NewLogger(log))

	appName := filepath.Base(os.Args[0])

	ctxParent := logkey.NewContext(context.Background(), log)
	ctx, cancel := signal.NotifyContext(ctxParent, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGABRT, syscall.SIGTERM)
	defer cancel()
	go forceShutdown(ctx)

	err = start(ctx, cfg, appName)
	if err != nil {
		log.Error("shutdown", slog.String(logkey.Error, err.Error()))
		os.Exit(1)
	}
}

func start(ctx context.Context, cfg config, appName string) error {
	reg := prometheus.NewPedanticRegistry()

	return run(ctx, cfg, reg, appName)
}

func run(ctx context.Context, cfg config, reg *prometheus.Registry, namespace string) error {
	log := logkey.FromContext(ctx)
	m := metrics.New(reg, namespace)

	s, err := store.New(ctx, cfg.Store.Root, cfg.Store.URLS)
	if err != nil {
		return fmt.Errorf("store.New: %w", err)
	}

	module := core.New(s)
	_, httpAPI := api.New(ctx, m, module, reg, namespace, cfg.Server.External.Domain)

	return serve.Start(
		ctx,
		serve.Metrics(log.With(slog.String(logkey.Module, "metric")), cfg.Server.External.Host, cfg.Server.External.Port.Metric, reg),
		serve.HTTP(log.With(slog.String(logkey.Module, "gRPC-Gateway")), cfg.Server.External.Host, cfg.Server.External.Port.Connect, httpAPI),
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

func buildLogger() (*slog.Logger, error) {
	th := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})

	log := slog.New(th)

	return log, nil
}
