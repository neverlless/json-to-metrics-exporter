package collector

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricPrefix = "jsonmetric_"
	maxBodyBytes = 10 << 20
)

var invalidNameChars = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// JsonCollector collects metrics from JSON endpoints.
// It implements prometheus.Collector interface.
type JsonCollector struct {
	ctx      context.Context
	endpoint string
	healthy  *regexp.Regexp
	client   *http.Client
	seen     map[string]struct{}
}

// NewClient creates the HTTP client shared by all collectors, so connections
// are reused between scrapes. Redirects are refused unless allowed matches them.
func NewClient(insecureTLS bool, allowed *regexp.Regexp) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if allowed != nil && !allowed.MatchString(req.URL.String()) {
				return fmt.Errorf("redirect to %s is not allowed", req.URL)
			}
			return nil
		},
	}
}

// NewJsonCollector creates a JsonCollector for a single scrape of endpoint.
// The request to endpoint is cancelled when ctx is done.
func NewJsonCollector(ctx context.Context, endpoint string, healthy *regexp.Regexp, client *http.Client) *JsonCollector {
	return &JsonCollector{
		ctx:      ctx,
		endpoint: endpoint,
		healthy:  healthy,
		client:   client,
		seen:     make(map[string]struct{}),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected
// by this Collector and sends it to the provided channel.
func (jc *JsonCollector) Describe(ch chan<- *prometheus.Desc) {
	// Intentionally left empty because all metrics are dynamic
	// and are described during the Collect phase.
}

// Collect collects metrics from the JSON endpoint.
func (jc *JsonCollector) Collect(ch chan<- prometheus.Metric) {
	urlLabel := prometheus.Labels{"url": jc.endpoint}

	start := time.Now()
	response, err := jc.get()
	jc.emit(ch, "probe_http_duration_seconds", "Duration of HTTP request in seconds", urlLabel, time.Since(start).Seconds())
	if err != nil {
		jc.fail(ch, err)
		return
	}
	defer response.Body.Close()

	jc.emit(ch, "probe_http_status_code", "HTTP status code returned by the target", urlLabel, float64(response.StatusCode))
	if response.StatusCode != http.StatusOK {
		jc.fail(ch, fmt.Errorf("unexpected status code %d", response.StatusCode))
		return
	}

	jsonData, err := readJSON(response.Body)
	if err != nil {
		jc.fail(ch, err)
		return
	}

	jc.emit(ch, "probe_success", "Indicates whether the probe succeeded", urlLabel, 1)
	jc.parse(jsonData, ch, "")
}

func (jc *JsonCollector) get() (*http.Response, error) {
	request, err := http.NewRequestWithContext(jc.ctx, http.MethodGet, jc.endpoint, nil)
	if err != nil {
		return nil, err
	}
	return jc.client.Do(request)
}

func readJSON(body io.Reader) (any, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBodyBytes {
		return nil, fmt.Errorf("response is larger than %d bytes", maxBodyBytes)
	}
	var jsonData any
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, err
	}
	return jsonData, nil
}

// fail logs why the probe failed and emits probe_success=0.
func (jc *JsonCollector) fail(ch chan<- prometheus.Metric, err error) {
	slog.Warn("Probe failed", "target", jc.endpoint, "error", err)
	jc.emit(ch, "probe_success", "Indicates whether the probe succeeded", prometheus.Labels{"url": jc.endpoint}, 0)
}

// emit sends one gauge. JSON keys are not trusted: a metric that is invalid or
// that repeats an earlier one after name cleanup is skipped, because the
// registry would otherwise fail the whole scrape.
func (jc *JsonCollector) emit(ch chan<- prometheus.Metric, name, help string, labels prometheus.Labels, val float64) {
	desc := prometheus.NewDesc(name, help, nil, labels)
	if _, duplicate := jc.seen[desc.String()]; duplicate {
		return
	}
	metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, val)
	if err != nil {
		slog.Warn("Skipping invalid metric", "target", jc.endpoint, "metric", name, "error", err)
		return
	}
	jc.seen[desc.String()] = struct{}{}
	ch <- metric
}

func (jc *JsonCollector) parse(data any, ch chan<- prometheus.Metric, prefix string) {
	switch v := data.(type) {
	case map[string]any:
		for key, val := range v {
			name := joinName(prefix, key)
			if svcMap, isMap := val.(map[string]any); isMap {
				jc.extractServiceMetrics(name, svcMap, ch)
				continue
			}
			jc.parse(val, ch, name)
		}
	case string:
		jc.parseEntry(prefix, v, ch)
	case float64:
		jc.parseEntry(prefix, v, ch)
	case bool:
		jc.parseEntry(prefix, boolToFloat(v), ch)
	case []any:
		for i, item := range v {
			jc.parse(item, ch, joinName(prefix, strconv.Itoa(i)))
		}
	}
}

// extractServiceMetrics treats strings and booleans in serviceData as service
// health. Numbers and nested values are ordinary metrics, not health.
func (jc *JsonCollector) extractServiceMetrics(prefix string, serviceData map[string]any, ch chan<- prometheus.Metric) {
	for service, status := range serviceData {
		switch status.(type) {
		case string, bool:
			jc.parseServiceMetric(prefix, service, status, ch)
		default:
			jc.parse(map[string]any{service: status}, ch, prefix)
		}
	}
}

func (jc *JsonCollector) parseServiceMetric(prefix, service string, status any, ch chan<- prometheus.Metric) {
	var val float64
	switch s := status.(type) {
	case string:
		val = boolToFloat(jc.healthy.MatchString(s))
	case bool:
		val = boolToFloat(s)
	}

	labels := prometheus.Labels{"service": service, "category": prefix, "url": jc.endpoint}
	jc.emit(ch, metricPrefix+"services", "Service health status", labels, val)
}

func (jc *JsonCollector) parseEntry(metadata string, value any, ch chan<- prometheus.Metric) {
	var val float64
	labels := prometheus.Labels{"url": jc.endpoint}
	switch v := value.(type) {
	case string:
		// Only healthy values become a label: free text such as a timestamp
		// would create a new time series on every scrape.
		if jc.healthy.MatchString(v) {
			val = 1
			labels["text"] = v
		}
	case float64:
		val = v
	}

	jc.emit(ch, correctMetricName(metricPrefix+metadata), "Parsed JSON metric", labels, val)
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func joinName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "_" + name
}

func correctMetricName(name string) string {
	return strings.Trim(invalidNameChars.ReplaceAllString(name, "_"), "_")
}
