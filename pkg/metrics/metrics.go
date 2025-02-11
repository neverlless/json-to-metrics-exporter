package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// AppMetrics contains the metrics to be exposed to Prometheus
var (
	RequestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_request_count",
			Help: "Total number of requests received",
		},
		[]string{"endpoint"},
	)
)

// RegisterMetrics registers all application metrics with the provided registry
func RegisterMetrics(registry *prometheus.Registry) {
	registry.MustRegister(RequestCount)
}