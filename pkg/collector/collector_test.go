package collector

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var healthyRegex = regexp.MustCompile("OK|success")

func newTestCollector(endpoint string, client *http.Client) *JsonCollector {
	return NewJsonCollector(context.Background(), endpoint, healthyRegex, client)
}

// scrape serves body with status from a test target and gathers through a real
// registry, because the registry is what rejects duplicate and invalid metrics.
func scrape(t *testing.T, status int, body string) map[string]*dto.MetricFamily {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer server.Close()

	registry := prometheus.NewRegistry()
	registry.MustRegister(newTestCollector(server.URL, NewClient(false, nil)))

	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	families := make(map[string]*dto.MetricFamily, len(metricFamilies))
	for _, mf := range metricFamilies {
		families[mf.GetName()] = mf
	}
	return families
}

func gaugeValue(t *testing.T, families map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()

	mf, found := families[name]
	if !found {
		t.Fatalf("%s metric not found in %v", name, metricNames(families))
	}
	return mf.GetMetric()[0].GetGauge().GetValue()
}

func metricNames(families map[string]*dto.MetricFamily) []string {
	names := make([]string, 0, len(families))
	for name := range families {
		names = append(names, name)
	}
	return names
}

func serviceValues(families map[string]*dto.MetricFamily) map[string]float64 {
	values := make(map[string]float64)
	for _, metric := range families["jsonmetric_services"].GetMetric() {
		for _, label := range metric.GetLabel() {
			if label.GetName() == "service" {
				values[label.GetValue()] = metric.GetGauge().GetValue()
			}
		}
	}
	return values
}

func labelValue(metric *dto.Metric, name string) (string, bool) {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue(), true
		}
	}
	return "", false
}

func TestCollect_Success(t *testing.T) {
	families := scrape(t, http.StatusOK, `{"status":"OK","count":42}`)

	if got := gaugeValue(t, families, "probe_success"); got != 1 {
		t.Errorf("Expected probe_success to be 1, got %v", got)
	}
	if got := gaugeValue(t, families, "probe_http_status_code"); got != 200 {
		t.Errorf("Expected probe_http_status_code to be 200, got %v", got)
	}
	if got := gaugeValue(t, families, "jsonmetric_status"); got != 1 {
		t.Errorf("Expected jsonmetric_status to be 1, got %v", got)
	}
	if got := gaugeValue(t, families, "jsonmetric_count"); got != 42 {
		t.Errorf("Expected jsonmetric_count to be 42, got %v", got)
	}
	gaugeValue(t, families, "probe_http_duration_seconds")
}

func TestCollect_ProbeFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusInternalServerError, "Internal Server Error"},
		{"not found", http.StatusNotFound, "nope"},
		{"not JSON", http.StatusOK, "<html>not json"},
		{"too large", http.StatusOK, `"` + strings.Repeat("a", maxBodyBytes) + `"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			families := scrape(t, test.status, test.body)

			if got := gaugeValue(t, families, "probe_success"); got != 0 {
				t.Errorf("Expected probe_success to be 0, got %v", got)
			}
			if got := gaugeValue(t, families, "probe_http_status_code"); got != float64(test.status) {
				t.Errorf("Expected probe_http_status_code to be %d, got %v", test.status, got)
			}
		})
	}
}

func TestCollect_UnreachableTarget(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	endpoint := server.URL
	server.Close()

	registry := prometheus.NewRegistry()
	registry.MustRegister(newTestCollector(endpoint, NewClient(false, nil)))

	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	families := make(map[string]*dto.MetricFamily, len(metricFamilies))
	for _, mf := range metricFamilies {
		families[mf.GetName()] = mf
	}
	if got := gaugeValue(t, families, "probe_success"); got != 0 {
		t.Errorf("Expected probe_success to be 0, got %v", got)
	}
}

func TestCollect_InvalidKeysDoNotPanic(t *testing.T) {
	families := scrape(t, http.StatusOK, `{"my-key":"OK","a.b":1,"with space":2,"пинг":3}`)

	if got := gaugeValue(t, families, "jsonmetric_my_key"); got != 1 {
		t.Errorf("Expected jsonmetric_my_key to be 1, got %v", got)
	}
	if got := gaugeValue(t, families, "jsonmetric_a_b"); got != 1 {
		t.Errorf("Expected jsonmetric_a_b to be 1, got %v", got)
	}
	gaugeValue(t, families, "jsonmetric_with_space")
}

func TestCollect_KeysThatCollideAfterCleanup(t *testing.T) {
	families := scrape(t, http.StatusOK, `{"a-b":1,"a.b":2,"a_b":3}`)

	if got := len(families["jsonmetric_a_b"].GetMetric()); got != 1 {
		t.Errorf("Expected one jsonmetric_a_b metric, got %d", got)
	}
}

func TestCollect_ServiceValues(t *testing.T) {
	families := scrape(t, http.StatusOK, `{"checks":{
		"postgres":"OK","redis":"down","up":true,"off":false,
		"latency":12,"deep":{"inner":{"x":"OK"}},"none":null
	}}`)

	services := serviceValues(families)
	expected := map[string]float64{"postgres": 1, "redis": 0, "up": 1, "off": 0}
	if len(services) != len(expected)+1 {
		t.Errorf("Unexpected services: %v", services)
	}
	for service, value := range expected {
		if got, found := services[service]; !found || got != value {
			t.Errorf("Expected %s to be %v, got %v (found=%v)", service, value, got, found)
		}
	}

	if got := gaugeValue(t, families, "jsonmetric_checks_latency"); got != 12 {
		t.Errorf("Expected jsonmetric_checks_latency to be 12, got %v", got)
	}
	if got, found := services["x"]; !found || got != 1 {
		t.Errorf("Expected nested service x to be 1, got %v (found=%v)", got, found)
	}
}

func TestCollect_Arrays(t *testing.T) {
	families := scrape(t, http.StatusOK, `{"items":[1,{"size":2}]}`)

	if got := gaugeValue(t, families, "jsonmetric_items_0"); got != 1 {
		t.Errorf("Expected jsonmetric_items_0 to be 1, got %v", got)
	}
	if got := gaugeValue(t, families, "jsonmetric_items_1_size"); got != 2 {
		t.Errorf("Expected jsonmetric_items_1_size to be 2, got %v", got)
	}
}

func TestCollect_TextLabelOnlyForHealthyValues(t *testing.T) {
	families := scrape(t, http.StatusOK, `{"status":"OK","now":"2026-10-30T10:00:00Z"}`)

	if text, _ := labelValue(families["jsonmetric_status"].GetMetric()[0], "text"); text != "OK" {
		t.Errorf("Expected text label OK, got %q", text)
	}
	if text, found := labelValue(families["jsonmetric_now"].GetMetric()[0], "text"); found {
		t.Errorf("Expected no text label on free text, got %q", text)
	}
}

func TestSharedClientReusesConnections(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"OK"}`))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	client := NewClient(false, nil)
	defer client.CloseIdleConnections()

	for range 5 {
		registry := prometheus.NewRegistry()
		registry.MustRegister(newTestCollector(server.URL, client))
		if _, err := registry.Gather(); err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}
	}

	if got := connections.Load(); got != 1 {
		t.Errorf("Expected 1 connection across scrapes, got %d", got)
	}
}

func TestCollect_RespectsContext(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	registry := prometheus.NewRegistry()
	registry.MustRegister(NewJsonCollector(ctx, server.URL, healthyRegex, NewClient(false, nil)))

	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}
	for _, mf := range metricFamilies {
		if mf.GetName() == "probe_http_status_code" {
			t.Error("Expected no request to be made with a cancelled context")
		}
	}
}

func TestNewClient_TLSVerification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if response, err := NewClient(false, nil).Get(server.URL); err == nil {
		response.Body.Close()
		t.Error("Expected self-signed certificate to be rejected by default")
	}

	response, err := NewClient(true, nil).Get(server.URL)
	if err != nil {
		t.Fatalf("Expected insecure client to accept the certificate: %v", err)
	}
	response.Body.Close()
}

func TestCorrectMetricName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test-metric", "test_metric"},
		{"test.metric", "test_metric"},
		{"test metric", "test_metric"},
		{"test__metric_", "test_metric"},
		{"123-abc", "123_abc"},
		{"-leading", "leading"},
	}

	for _, test := range tests {
		if result := correctMetricName(test.input); result != test.expected {
			t.Errorf("For input %s, expected %s but got %s", test.input, test.expected, result)
		}
	}
}
