package metrics_test

import (
	"testing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRegisterMetrics(t *testing.T) {
	// Используем тестовый реестр и создаем метрики в контексте теста
	requestCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_request_count",
			Help: "Total number of requests received",
		},
		[]string{"endpoint"},
	)

	// Используем дефолтный реестр
	prometheus.DefaultRegisterer.(*prometheus.Registry).Unregister(requestCount)
	err := prometheus.Register(requestCount)
	if err != nil {
		t.Fatalf("Failed to register metric: %v", err)
	}

	requestCount.WithLabelValues("unit_test").Add(1) // Инкремент для проверки
	if count := testutil.ToFloat64(requestCount.WithLabelValues("unit_test")); count != 1 {
		t.Errorf("Unexpected count value: %f", count)
	}
}