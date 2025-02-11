package collector

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// JsonCollector collects metrics from JSON endpoints
// Implements prometheus.Collector

type JsonCollector struct {
	Endpoint     string
	HealthyRegex []*regexp.Regexp
	Prefix       string
	ShowType     bool
}

// NewJsonCollector initializes JsonCollector with regex from environment
func NewJsonCollector(endpoint string) *JsonCollector {
	// Default regex values
	regexString := os.Getenv("HEALTHY_REGEX")
	if regexString == "" {
		regexString = "OK|success"
	}
	regex := regexp.MustCompile(regexString)

	return &JsonCollector{
		Endpoint:     endpoint,
		HealthyRegex: []*regexp.Regexp{regex},
		Prefix:       "jsonmetric_",
		ShowType:     false,
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected
// by this Collector and sends it to the provided channel.
func (jc *JsonCollector) Describe(ch chan<- *prometheus.Desc) {
	// This function is intentionally left empty because all metrics are dynamic
	// and are described during the Collect phase.
}

// Collect collects metrics from the JSON endpoint
func (jc *JsonCollector) Collect(ch chan<- prometheus.Metric) {
	// Создаем кастомный HTTP клиент с отключенной проверкой сертификатов
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transport}

	response, err := client.Get(jc.Endpoint)
	if err != nil || response.StatusCode != http.StatusOK {
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("scrape_success", "", nil, prometheus.Labels{"url": jc.Endpoint}),
			prometheus.GaugeValue, 0,
		)
		return
	}
	defer response.Body.Close()

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("response_code", "", nil, prometheus.Labels{"url": jc.Endpoint}),
		prometheus.GaugeValue, float64(response.StatusCode),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("scrape_success", "", nil, prometheus.Labels{"url": jc.Endpoint}),
		prometheus.GaugeValue, 1,
	)

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}

	var jsonData interface{}
	err = json.Unmarshal(body, &jsonData)
	if err != nil {
		return
	}

	jc.parse(jsonData, ch, "")
}

func (jc *JsonCollector) parse(data interface{}, ch chan<- prometheus.Metric, prefix string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			newPrefix := jc.correctMetricName(prefix+key) + "_"
			jc.parse(val, ch, newPrefix)
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

func (jc *JsonCollector) parseEntry(metadata string, value interface{}, ch chan<- prometheus.Metric) {
	var val float64
	var labels map[string]string
	if strVal, ok := value.(string); ok {
		labels = map[string]string{"text": strVal, "url": jc.Endpoint}
		for _, regex := range jc.HealthyRegex {
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

	name := jc.Prefix + metadata
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(name, "", nil, labels),
		prometheus.GaugeValue, val,
	)
}

func (jc *JsonCollector) correctMetricName(name string) string {
	// Replace non-standard characters with '_' and avoid trailing '_'
	reg := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	cleanName := reg.ReplaceAllString(name, "_")
	return strings.TrimSuffix(cleanName, "_")
}
