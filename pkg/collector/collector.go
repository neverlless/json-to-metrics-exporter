package collector

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// JsonCollector collects metrics from JSON endpoints.
// It implements prometheus.Collector interface.
type JsonCollector struct {
	endpoint     string
	healthyRegex []*regexp.Regexp
	prefix       string
	client       *http.Client
}

// NewJsonCollector creates a new JsonCollector with regex patterns from environment.
func NewJsonCollector(endpoint string) *JsonCollector {
	regexString := os.Getenv("HEALTHY_REGEX")
	if regexString == "" {
		regexString = "OK|success"
	}
	regex := regexp.MustCompile(regexString)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	return &JsonCollector{
		endpoint:     endpoint,
		healthyRegex: []*regexp.Regexp{regex},
		prefix:       "jsonmetric_",
		client:       client,
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
	start := time.Now()
	
	response, err := jc.client.Get(jc.endpoint)
	duration := time.Since(start).Seconds()

	// Emit probe_http_duration_seconds metric
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"probe_http_duration_seconds",
			"Duration of HTTP request in seconds",
			nil,
			prometheus.Labels{"url": jc.endpoint},
		),
		prometheus.GaugeValue,
		duration,
	)

	if err != nil {
		jc.emitFailureMetrics(ch, 0)
		return
	}
	defer response.Body.Close()

	// Emit probe_http_status_code metric
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"probe_http_status_code",
			"HTTP status code returned by the target",
			nil,
			prometheus.Labels{"url": jc.endpoint},
		),
		prometheus.GaugeValue,
		float64(response.StatusCode),
	)

	if response.StatusCode != http.StatusOK {
		jc.emitFailureMetrics(ch, float64(response.StatusCode))
		return
	}

	// Emit probe_success metric
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"probe_success",
			"Indicates whether the probe succeeded",
			nil,
			prometheus.Labels{"url": jc.endpoint},
		),
		prometheus.GaugeValue,
		1,
	)

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return
	}

	var jsonData interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return
	}

	jc.parse(jsonData, ch, "")
}

// emitFailureMetrics emits probe_success=0 when probe fails.
func (jc *JsonCollector) emitFailureMetrics(ch chan<- prometheus.Metric, statusCode float64) {
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"probe_success",
			"Indicates whether the probe succeeded",
			nil,
			prometheus.Labels{"url": jc.endpoint},
		),
		prometheus.GaugeValue,
		0,
	)
	if statusCode > 0 {
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				"probe_http_status_code",
				"HTTP status code returned by the target",
				nil,
				prometheus.Labels{"url": jc.endpoint},
			),
			prometheus.GaugeValue,
			statusCode,
		)
	}
}

func (jc *JsonCollector) parse(data interface{}, ch chan<- prometheus.Metric, prefix string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			newPrefix := prefix + key
			if svcMap, isMap := val.(map[string]interface{}); isMap {
				jc.extractServiceMetrics(newPrefix, svcMap, ch)
				continue
			}
			jc.parse(val, ch, newPrefix+"_")
		}
	case string:
		jc.parseEntry(prefix, v, ch)
	case float64:
		jc.parseEntry(prefix, v, ch)
	case bool:
		floatVal := 0.0
		if v {
			floatVal = 1.0
		}
		jc.parseEntry(prefix, floatVal, ch)
	case []interface{}:
		for i, item := range v {
			jc.parse(item, ch, jc.correctMetricName(prefix+strconv.Itoa(i))+"_")
		}
	}
}

func (jc *JsonCollector) extractServiceMetrics(prefix string, serviceData map[string]interface{}, ch chan<- prometheus.Metric) {
	for service, status := range serviceData {
		jc.parseServiceMetric(prefix, service, status, ch)
	}
}

func (jc *JsonCollector) parseServiceMetric(prefix, service string, status interface{}, ch chan<- prometheus.Metric) {
	var val float64
	if strVal, ok := status.(string); ok {
		for _, regex := range jc.healthyRegex {
			if regex.MatchString(strVal) {
				val = 1
				break
			}
		}
	}

	name := jc.prefix + "services"
	labels := prometheus.Labels{"service": service, "category": prefix, "url": jc.endpoint}
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(name, "Service health status", nil, labels),
		prometheus.GaugeValue,
		val,
	)
}

func (jc *JsonCollector) parseEntry(metadata string, value interface{}, ch chan<- prometheus.Metric) {
	var val float64
	var labels map[string]string
	if strVal, ok := value.(string); ok {
		labels = map[string]string{"text": strVal, "url": jc.endpoint}
		for _, regex := range jc.healthyRegex {
			if regex.MatchString(strVal) {
				val = 1
				break
			}
		}
	} else if numVal, ok := value.(float64); ok {
		val = numVal
	} else {
		val = 0
	}

	name := jc.prefix + metadata
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(name, "Parsed JSON metric", nil, labels),
		prometheus.GaugeValue,
		val,
	)
}

func (jc *JsonCollector) correctMetricName(name string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	cleanName := reg.ReplaceAllString(name, "_")
	return strings.TrimSuffix(cleanName, "_")
}
