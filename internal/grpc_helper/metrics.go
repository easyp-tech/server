package grpc_helper

import (
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

// NewServerMetrics returns gRPC server metrics.
// Do not forget to call .InitializeMetrics(server) on returned value.
func NewServerMetrics(reg *prometheus.Registry, namespace, subsystem string) *grpc_prometheus.ServerMetrics {
	serverMetrics := grpc_prometheus.NewServerMetrics(
		grpc_prometheus.WithServerCounterOptions(func(o *prometheus.CounterOpts) {
			o.Namespace = namespace
			o.Subsystem = subsystem
		}),
		grpc_prometheus.WithServerHandlingTimeHistogram(func(o *prometheus.HistogramOpts) {
			o.Namespace = namespace
			o.Subsystem = subsystem
		}),
	)

	reg.MustRegister(serverMetrics)

	return serverMetrics
}
