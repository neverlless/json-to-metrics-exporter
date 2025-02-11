package converter

import (
	"encoding/json"
	"errors"
)

// Metric represents a single extracted metric
// Name represents the key and Value is translated value
// where typical progression can be OK -> 1 and other states -> 0
// Date is an optional field

type Metric struct {
	Name  string
	Value float64
}

// ConvertJSONToMetrics converts JSON data to metrics
func ConvertJSONToMetrics(jsonData []byte) ([]Metric, error) {
	var data map[string]interface{}

	err := json.Unmarshal(jsonData, &data)
	if err != nil {
		return nil, err
	}

	checks, found := data["checks"].(map[string]interface{})
	if !found {
		return nil, errors.New("missing 'checks' key in JSON")
	}

	metrics := make([]Metric, 0)
	for key, value := range checks {
		strValue, ok := value.(string)
		if !ok {
			return nil, errors.New("unsupported data type for check values")
		}
		var floatVal float64
		if strValue == "OK" {
			floatVal = 1
		} else {
			floatVal = 0
		}
		metrics = append(metrics, Metric{Name: key, Value: floatVal})
	}

	return metrics, nil
}