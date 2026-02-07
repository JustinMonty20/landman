package union

import (
	"strconv"
	"strings"

	"github.com/JustinMonty20/landman/internal/connector"
)

var landTypeIndicators = []string{
	"RURAL",
	"ACRES",
	"VACANT",
}

// IsLikelyVacantGISRecord returns true when the GIS attributes indicate
// a likely vacant-land parcel for Union County.
//
// Rule:
// - YEARBLT == 0 OR SQFT == 0 OR FMV_IMPRV == 0
// - Optional include: LAND_TYPE contains RURAL, ACRES, or VACANT
func IsLikelyVacantGISRecord(record connector.RawRecord) bool {
	fields := extractGISAttributes(record.RawData)
	if len(fields) == 0 {
		return false
	}

	for _, key := range []string{"YEARBLT", "SQFT", "FMV_IMPRV"} {
		value, ok := getField(fields, key)
		if !ok {
			continue
		}
		number, ok := asNumber(value)
		if ok && number == 0 {
			return true
		}
	}

	landTypeValue, ok := getField(fields, "LAND_TYPE")
	if !ok {
		return false
	}

	landType, ok := asString(landTypeValue)
	if !ok {
		return false
	}

	upperLandType := strings.ToUpper(landType)
	for _, indicator := range landTypeIndicators {
		if strings.Contains(upperLandType, indicator) {
			return true
		}
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

func getField(fields map[string]interface{}, key string) (interface{}, bool) {
	upperKey := strings.ToUpper(key)
	for fieldKey, value := range fields {
		if strings.ToUpper(fieldKey) == upperKey {
			return value, true
		}
	}
	return nil, false
}

func asString(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	}
	return "", false
}

func asNumber(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case string:
		clean := strings.TrimSpace(typed)
		clean = strings.ReplaceAll(clean, "$", "")
		clean = strings.ReplaceAll(clean, ",", "")
		if clean == "" {
			return 0, false
		}
		n, err := strconv.ParseFloat(clean, 64)
		if err == nil {
			return n, true
		}
	}
	return 0, false
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
