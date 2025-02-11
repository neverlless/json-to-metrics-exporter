package metrics_test

import (
	"testing"
	"strings"
	"github.com/neverlless/json-to-metrics-exporter/pkg/metrics"
	"math"
)

func TestSampleLine(t *testing.T) {
	line := metrics.SampleLine(
		"test_metric",
		map[string]string{"label1": "value1", "label2": "value2"},
		123.456,
		nil, // no timestamp for comparison
	)
	expectedParts := []string{
		"test_metric{label1=\"value1\",label2=\"value2\"}",
		"123.456",
	}

	for _, part := range expectedParts {
		if !strings.Contains(line, part) {
			t.Errorf("Expected line to contain %s, but got %s", part, line)
		}
	}
}

func TestFloatToGoString(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
		{math.NaN(), "NaN"},
		{1.0, "1.000000"},
	}

	for _, test := range tests {
		output := metrics.FloatToGoString(test.input)
		if output != test.expected {
			t.Errorf("For input %f, expected %s but got %s", test.input, test.expected, output)
		}
	}
}