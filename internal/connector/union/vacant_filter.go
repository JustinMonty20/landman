package union

import (
	"strings"

	"github.com/JustinMonty20/landman/internal/connector"
)

// ucZeroNumericFields are GIS attribute keys where a zero value indicates no structure.
var ucZeroNumericFields = []string{"YEARBLT", "SQFT"}

// ucZeroCurrencyFields are GIS attribute keys where a zero currency value indicates no structure value.
var ucZeroCurrencyFields = []string{"FMV_IMPRV"}

// ucVacantLandTypes are LAND_TYPE substrings (uppercased) that indicate vacant land.
var ucVacantLandTypes = []string{"RURAL ACREAGE TRACT"}

// IsLikelyVacantGISRecord returns true when the GIS attributes indicate
// a likely vacant-land parcel for Union County.
//
// Rules (any match → vacant):
//   - YEARBLT or SQFT is zero
//   - FMV_IMPRV parses to zero currency
//   - LAND_TYPE contains a known vacant-land descriptor
//
// Field lookup is case-insensitive.
func IsLikelyVacantGISRecord(record connector.RawRecord) bool {
	fields := extractGISAttributes(record.RawData)
	if len(fields) == 0 {
		return false
	}

	// Normalize all keys to uppercase for case-insensitive lookup.
	normalized := normalizeKeys(fields)

	for _, key := range ucZeroNumericFields {
		if value, ok := normalized[key]; ok {
			if isZeroNumeric(value) {
				return true
			}
		}
	}

	for _, key := range ucZeroCurrencyFields {
		if value, ok := normalized[key]; ok {
			if isZeroCurrency(value) {
				return true
			}
		}
	}

	if value, ok := normalized["LAND_TYPE"]; ok {
		if str, ok := asString(value); ok {
			for _, indicator := range ucVacantLandTypes {
				if strings.Contains(str, indicator) {
					return true
				}
			}
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

func isZeroCurrency(value interface{}) bool {
	str, ok := asString(value)
	if !ok {
		return false
	}
	str = strings.TrimPrefix(str, "$")
	str = strings.TrimSpace(str)
	return str == "0"
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

func asString(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case string:
		return strings.ToUpper(typed), true
	case []byte:
		return strings.ToUpper(string(typed)), true
	}
	return "", false
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
