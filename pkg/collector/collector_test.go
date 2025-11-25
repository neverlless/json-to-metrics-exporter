package collector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewJsonCollector(t *testing.T) {
	collector := NewJsonCollector("http://example.com/metrics")

	if collector.endpoint != "http://example.com/metrics" {
		t.Errorf("Expected endpoint to be http://example.com/metrics, got %s", collector.endpoint)
	}

	if collector.prefix != "jsonmetric_" {
		t.Errorf("Expected prefix to be jsonmetric_, got %s", collector.prefix)
	}

	if collector.client == nil {
		t.Error("Expected client to be initialized")
	}

	if len(collector.healthyRegex) == 0 {
		t.Error("Expected healthyRegex to be initialized")
	}
}

func TestCollect_Success(t *testing.T) {
	testData := map[string]interface{}{
		"status": "OK",
		"count":  42.0,
	}
	jsonData, _ := json.Marshal(testData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	}))
	defer server.Close()

	collector := NewJsonCollector(server.URL)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	foundProbeSuccess := false
	foundProbeDuration := false
	foundProbeStatusCode := false

	for _, mf := range metricFamilies {
		switch mf.GetName() {
		case "probe_success":
			foundProbeSuccess = true
			if len(mf.GetMetric()) > 0 && mf.GetMetric()[0].GetGauge().GetValue() != 1 {
				t.Error("Expected probe_success to be 1")
			}
		case "probe_http_duration_seconds":
			foundProbeDuration = true
		case "probe_http_status_code":
			foundProbeStatusCode = true
			if len(mf.GetMetric()) > 0 && mf.GetMetric()[0].GetGauge().GetValue() != 200 {
				t.Error("Expected probe_http_status_code to be 200")
			}
		}
	}

	if !foundProbeSuccess {
		t.Error("probe_success metric not found")
	}
	if !foundProbeDuration {
		t.Error("probe_http_duration_seconds metric not found")
	}
	if !foundProbeStatusCode {
		t.Error("probe_http_status_code metric not found")
	}
}

func TestCollect_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	collector := NewJsonCollector(server.URL)

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	foundProbeSuccess := false
	foundProbeStatusCode := false

	for metric := range ch {
		desc := metric.Desc().String()
		if containsString(desc, "probe_success") {
			foundProbeSuccess = true
		}
		if containsString(desc, "probe_http_status_code") {
			foundProbeStatusCode = true
		}
	}

	if !foundProbeSuccess {
		t.Error("probe_success metric not found")
	}
	if !foundProbeStatusCode {
		t.Error("probe_http_status_code metric not found")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCorrectMetricName(t *testing.T) {
	collector := NewJsonCollector("http://example.com")

	tests := []struct {
		input    string
		expected string
	}{
		{"test-metric", "test_metric"},
		{"test.metric", "test_metric"},
		{"test metric", "test_metric"},
		{"test__metric_", "test_metric"},
		{"123-abc", "123_abc"},
	}

	for _, test := range tests {
		result := collector.correctMetricName(test.input)
		if result != test.expected {
			t.Errorf("For input %s, expected %s but got %s", test.input, test.expected, result)
		}
	}
}

func TestParseServiceMetric(t *testing.T) {
	collector := NewJsonCollector("http://example.com")

	tests := []struct {
		name     string
		status   interface{}
		expected float64
	}{
		{"healthy service", "OK", 1},
		{"success service", "success", 1},
		{"failed service", "ERROR", 0},
		{"unknown service", "unknown", 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ch := make(chan prometheus.Metric, 1)
			collector.parseServiceMetric("category", "test_service", test.status, ch)

			if len(ch) == 0 {
				t.Error("Expected metric to be sent to channel")
				return
			}

			metric := <-ch
			if metric == nil {
				t.Error("Expected non-nil metric")
			}
		})
	}
}

func TestCollect_WithJSONServices(t *testing.T) {
	testData := map[string]interface{}{
		"database": map[string]interface{}{
			"postgres": "OK",
			"redis":    "success",
		},
	}
	jsonData, _ := json.Marshal(testData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	}))
	defer server.Close()

	collector := NewJsonCollector(server.URL)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	foundServicesMetric := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "jsonmetric_services" {
			foundServicesMetric = true
			if len(mf.GetMetric()) < 2 {
				t.Error("Expected at least 2 service metrics")
			}
		}
	}

	if !foundServicesMetric {
		t.Error("jsonmetric_services metric not found")
	}
}
