package union

import (
	"strings"

	"github.com/JustinMonty20/landman/internal/connector"
)

// ucZeroNumericFields are GIS attribute keys where a zero value indicates no structure.
var ucZeroNumericFields = []string{"YEARBLT", "SQFT"}

// IsLikelyVacantGISRecord returns true when the GIS attributes indicate
// a likely vacant-land parcel for Union County. Only numeric zero in
// YEARBLT or SQFT marks a parcel as likely vacant. Field lookup is case-insensitive.
func IsLikelyVacantGISRecord(record connector.RawRecord) bool {
	fields := extractGISAttributes(record.RawData)
	if len(fields) == 0 {
		return false
	}

	normalized := normalizeKeys(fields)
	for _, key := range ucZeroNumericFields {
		if value, ok := normalized[key]; ok && isZeroNumeric(value) {
			return true
		}
	}
	return false
}

func normalizeKeys(fields map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		out[strings.ToUpper(k)] = v
	}
	return out
}

func isZeroNumeric(value interface{}) bool {
	switch v := value.(type) {
	case int:
		return v == 0
	case int64:
		return v == 0
	case float64:
		return v == 0
	}
	return false
}

func extractGISAttributes(raw map[string]interface{}) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}

	if attrsRaw, ok := raw["attributes"]; ok {
		if attrsMap := toStringAnyMap(attrsRaw); len(attrsMap) > 0 {
			return attrsMap
		}
	}

	return raw
}

func toStringAnyMap(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, val := range typed {
			out[key] = val
		}
		return out
	}
	return nil
}
