package metrics

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// SampleLine converts metric line details into a Prometheus compatible line.
func SampleLine(name string, labels map[string]string, value float64, timestamp *float64) string {
	var labelStr, timeStr string
	if len(labels) > 0 {
		var parts []string
		for k, v := range labels {
			parts = append(parts, fmt.Sprintf(`%s="%s"`, k, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(v, "\\", "\\\\"), "\n", "\\n"), `"`, `\\"`)))
		}
		sort.Strings(parts)
		labelStr = fmt.Sprintf(`{%s}`, strings.Join(parts, ","))
	}
	if timestamp != nil {
		timeStr = fmt.Sprintf(" %d", int64(*timestamp*1000))
	}
	// Use precision formatting for the float value
	return fmt.Sprintf(`%s%s %.3f%s\n`, name, labelStr, value, timeStr)
}

// FloatToGoString converts a float64 to a Go-style float64 string.
func FloatToGoString(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	case math.IsNaN(f):
		return "NaN"
	default:
		s := fmt.Sprintf("%f", f)
		dot := strings.Index(s, ".")
		if f > 0 && dot > 6 {
			mantissa := strings.TrimRight(fmt.Sprintf("%s.%s%s", s[0:1], s[1:dot], s[dot+1:]), "0.")
			return fmt.Sprintf("%se+0%d", mantissa, dot-1)
		}
		return s
	}
}