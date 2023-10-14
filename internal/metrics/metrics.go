package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics shared by all packages.
type Metrics struct {
	PanicsTotal prometheus.Counter
}

// New registers and returns metrics shared by all packages.
func New(reg *prometheus.Registry, namespace string) Metrics {
	const subsystem = `standard`

	var metrics Metrics
	metrics.PanicsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "panics_total",
			Help:        "Amount of recovered panics.",
			ConstLabels: nil,
		},
	)
	reg.MustRegister(metrics.PanicsTotal)

	return metrics
}
