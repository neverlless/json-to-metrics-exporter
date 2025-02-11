package exporter

import (
	"fmt"
	"github.com/neverlless/json-to-metrics-exporter/pkg/converter"
)

// ExportMetrics exports the converted metrics in Prometheus format
func ExportMetrics(metrics []converter.Metric) string {
	metricsOutput := ""
	for _, metric := range metrics {
		metricsOutput += fmt.Sprintf("%s %f\n", metric.Name, metric.Value)
	}
	return metricsOutput
}