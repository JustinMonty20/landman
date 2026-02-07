package union

import (
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
)

func TestIsLikelyVacantGISRecord(t *testing.T) {
	tests := []struct {
		name   string
		record connector.RawRecord
		want   bool
	}{
		{
			name: "YEARBLT zero",
			record: connector.RawRecord{
				RawData: map[string]interface{}{
					"attributes": map[string]interface{}{
						"YEARBLT": 0,
					},
				},
			},
			want: true,
		},
		{
			name: "SQFT zero",
			record: connector.RawRecord{
				RawData: map[string]interface{}{
					"attributes": map[string]interface{}{
						"SQFT": 0,
					},
				},
			},
			want: true,
		},
		{
			name: "FMV_IMPRV zero as currency string",
			record: connector.RawRecord{
				RawData: map[string]interface{}{
					"attributes": map[string]interface{}{
						"FMV_IMPRV": "$0",
					},
				},
			},
			want: true,
		},
		{
			name: "LAND_TYPE indicator match",
			record: connector.RawRecord{
				RawData: map[string]interface{}{
					"attributes": map[string]interface{}{
						"LAND_TYPE": "Rural acreage tract",
					},
				},
			},
			want: true,
		},
		{
			name: "non-vacant values",
			record: connector.RawRecord{
				RawData: map[string]interface{}{
					"attributes": map[string]interface{}{
						"YEARBLT":   1998,
						"SQFT":      1800,
						"FMV_IMPRV": "$150,000",
						"LAND_TYPE": "Residential",
					},
				},
			},
			want: false,
		},
		{
			name: "missing attributes",
			record: connector.RawRecord{
				RawData: map[string]interface{}{},
			},
			want: false,
		},
		{
			name: "case-insensitive fields",
			record: connector.RawRecord{
				RawData: map[string]interface{}{
					"attributes": map[string]interface{}{
						"yearblt": 0,
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsLikelyVacantGISRecord(tt.record)
			if got != tt.want {
				t.Fatalf("IsLikelyVacantGISRecord() = %v, want %v", got, tt.want)
			}
		})
	}
}
