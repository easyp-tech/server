package grpc_helper

import (
	"time"

	"connectrpc.com/connect"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	grpc_validator "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/validator"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/easyp-tech/server/internal/metrics"
)

const (
	keepaliveTime    = 50 * time.Second
	keepaliveTimeout = 10 * time.Second
	keepaliveMinTime = 30 * time.Second
)

// NewServer creates and returns a gRPC server.
func NewServer(
	m metrics.Metrics,
	log *slog.Logger,
	serverMetrics *grpc_prometheus.ServerMetrics,
	converter GRPCCodesConverterHandler,
	extraUnary []grpc.UnaryServerInterceptor,
	extraStream []grpc.StreamServerInterceptor,
) (server *grpc.Server, healthServer *health.Server) {

	loggingOpts := []logging.Option{
		logging.WithLogOnEvents(
			logging.StartCall,
			logging.FinishCall,
			logging.PayloadReceived,
			logging.PayloadSent,
		),
	}

	unaryInterceptor := append([]grpc.UnaryServerInterceptor{
		serverMetrics.UnaryServerInterceptor(),
		logging.UnaryServerInterceptor(interceptorLogger(log), loggingOpts...),
		grpc_recovery.UnaryServerInterceptor(grpc_recovery.WithRecoveryHandlerContext(recoveryFunc(m, errInternal))),
		grpc_validator.UnaryServerInterceptor(),
		UnaryConvertCodesServerInterceptor(converter),
	}, extraUnary...)

	streamInterceptor := append([]grpc.StreamServerInterceptor{
		serverMetrics.StreamServerInterceptor(),
		logging.StreamServerInterceptor(interceptorLogger(log), loggingOpts...),
		grpc_recovery.StreamServerInterceptor(grpc_recovery.WithRecoveryHandlerContext(recoveryFunc(m, errInternal))),
		grpc_validator.StreamServerInterceptor(),
		StreamConvertCodesServerInterceptor(converter),
	}, extraStream...)

	server = grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()), // FIXME: add tls.
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    keepaliveTime,
			Timeout: keepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             keepaliveMinTime,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(
			unaryInterceptor...,
		),
		grpc.ChainStreamInterceptor(
			streamInterceptor...,
		),
	)

	reflection.Register(server)
	healthServer = health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)

	connect.WithInterceptors()

	return server, healthServer
}

// NewConnectServer creates and returns a connect server.
func NewConnectServer(
	m metrics.Metrics,
	log *slog.Logger,
	serverMetrics *grpc_prometheus.ServerMetrics,
	converter GRPCCodesConverterHandler,
	extraUnary []grpc.UnaryServerInterceptor,
	extraStream []grpc.StreamServerInterceptor,
) (server *grpc.Server, healthServer *health.Server) {

	loggingOpts := []logging.Option{
		logging.WithLogOnEvents(
			logging.StartCall,
			logging.FinishCall,
			logging.PayloadReceived,
			logging.PayloadSent,
		),
	}

	unaryInterceptor := append([]grpc.UnaryServerInterceptor{
		serverMetrics.UnaryServerInterceptor(),
		logging.UnaryServerInterceptor(interceptorLogger(log), loggingOpts...),
		grpc_recovery.UnaryServerInterceptor(grpc_recovery.WithRecoveryHandlerContext(recoveryFunc(m, errInternal))),
		grpc_validator.UnaryServerInterceptor(),
		UnaryConvertCodesServerInterceptor(converter),
	}, extraUnary...)

	streamInterceptor := append([]grpc.StreamServerInterceptor{
		serverMetrics.StreamServerInterceptor(),
		logging.StreamServerInterceptor(interceptorLogger(log), loggingOpts...),
		grpc_recovery.StreamServerInterceptor(grpc_recovery.WithRecoveryHandlerContext(recoveryFunc(m, errInternal))),
		grpc_validator.StreamServerInterceptor(),
		StreamConvertCodesServerInterceptor(converter),
	}, extraStream...)

	server = grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()), // FIXME: add tls.
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    keepaliveTime,
			Timeout: keepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             keepaliveMinTime,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(
			unaryInterceptor...,
		),
		grpc.ChainStreamInterceptor(
			streamInterceptor...,
		),
	)

	reflection.Register(server)
	healthServer = health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)

	return server, healthServer
}
