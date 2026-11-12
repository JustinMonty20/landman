package union

import (
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
)

func TestIsLikelyVacantGISRecord(t *testing.T) {
	tests := []struct {
		name       string
		attributes map[string]interface{}
		want       bool
	}{
		{name: "YEARBLT zero", attributes: map[string]interface{}{"YEARBLT": 0}, want: true},
		{name: "SQFT zero", attributes: map[string]interface{}{"SQFT": 0}, want: true},
		{name: "float YEARBLT zero", attributes: map[string]interface{}{"YEARBLT": float64(0)}, want: true},
		{name: "numeric FMV_IMPRV zero on a house", attributes: map[string]interface{}{
			"FMV_IMPRV": 0, "YEARBLT": 2025, "SQFT": 3391,
		}, want: false},
		{name: "RURAL ACRES house", attributes: map[string]interface{}{
			"LAND_TYPE": "RURAL ACRES", "YEARBLT": 1997, "SQFT": 22797,
		}, want: false},
		{name: "RURAL ACRES with zero year", attributes: map[string]interface{}{
			"LAND_TYPE": "RURAL ACRES", "YEARBLT": 0, "SQFT": 22797,
		}, want: true},
		{name: "RURAL ACRES with zero year and area", attributes: map[string]interface{}{
			"LAND_TYPE": "RURAL ACRES", "YEARBLT": 0, "SQFT": 0,
		}, want: true},
		{name: "currency string alone", attributes: map[string]interface{}{"FMV_IMPRV": "$0"}, want: false},
		{name: "old land type phrase alone", attributes: map[string]interface{}{"LAND_TYPE": "Rural acreage tract"}, want: false},
		{name: "non-vacant values", attributes: map[string]interface{}{
			"YEARBLT": 1998, "SQFT": 1800, "FMV_IMPRV": "$150,000", "LAND_TYPE": "Residential",
		}, want: false},
		{name: "missing attributes", want: false},
		{name: "null values", attributes: map[string]interface{}{"YEARBLT": nil, "SQFT": nil}, want: false},
		{name: "numeric-looking string", attributes: map[string]interface{}{"YEARBLT": "0"}, want: false},
		{name: "case-insensitive fields", attributes: map[string]interface{}{"yearblt": 0}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := connector.RawRecord{RawData: map[string]interface{}{"attributes": tt.attributes}}
			if got := IsLikelyVacantGISRecord(record); got != tt.want {
				t.Fatalf("IsLikelyVacantGISRecord() = %v, want %v", got, tt.want)
			}
		})
	}
}
