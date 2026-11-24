package union_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
)

func TestNormalizeParcelClassification(t *testing.T) {
	for _, tc := range []struct {
		name, attrs, shape string
		invalid, fatal     bool
	}{
		{"valid", `{"OBJECTID":1,"PID":"A","YEARBLT":0}`, `{"rings":[[[0,0],[0,1],[1,1],[1,0],[0,0]]]}`, false, false},
		{"fallback", `{"OBJECTID":1,"PID":null,"YEARBLT":0}`, `{"rings":[[[0,0],[0,1],[1,1],[1,0],[0,0]]]}`, false, false},
		{"bad shape", `{"OBJECTID":1,"PID":"A"}`, `{"rings":[]}`, true, false},
		{"bad year", `{"OBJECTID":1,"PID":"A","YEARBLT":1200}`, `{}`, true, false},
		{"bad amount", `{"OBJECTID":1,"PID":"A","s1_SALESAMT":-1}`, `{}`, true, false},
		{"no identity", `{"PID":"A"}`, `{}`, false, true},
		{"fraction oid", `{"OBJECTID":1.5,"PID":"A"}`, `{}`, false, true},
		{"numeric pid", `{"OBJECTID":1,"PID":123}`, `{}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(`{"attributes":`+tc.attrs+`,"geometry":`+tc.shape+`}`), &data); err != nil {
				t.Fatal(err)
			}
			parcel, err := union.NormalizeParcel(connector.RawRecord{SourceName: "union_county_gis", RawData: data})
			var invalid *union.InvalidParcelRecord
			if errors.As(err, &invalid) != tc.invalid || (err != nil && invalid == nil) != tc.fatal {
				t.Fatalf("classification %T %v", err, err)
			}
			if err == nil && parcel.Identity.SourceParcelID == "" {
				t.Fatalf("missing parcel identity: %+v", parcel.Identity)
			}
		})
	}
}

func TestNormalizeParcelNumericBoundaries(t *testing.T) {
	for _, field := range []struct {
		name           string
		upperExclusive float64
	}{
		{"mapped_acres", 1e8 - 0.0000005}, {"gross_acres", 1e8 - 0.0000005}, {"VAL_AC", 1e8 - 0.0000005},
		{"VAL_SQFT", 1e14}, {"SQFT", 1e12 - 0.005},
		{"FMV_LAND", 1e12 - 0.005}, {"FMV_IMPRV", 1e12 - 0.005}, {"FMV_TOTAL", 1e12 - 0.005}, {"TOTVAL", 1e12 - 0.005},
		{"SHAPE.STArea()", 1e14}, {"SHAPE.STLength()", 1e14},
		{"s1_SALESAMT", 1e12 - 0.005}, {"s2_SALESAMT", 1e12 - 0.005}, {"s3_SALESAMT", 1e12 - 0.005},
	} {
		t.Run(field.name, func(t *testing.T) {
			for _, tc := range []struct {
				name  string
				value interface{}
				valid bool
			}{
				{"zero", 0, true},
				{"rounds to maximum", math.Nextafter(field.upperExclusive, 0), true},
				{"rounds to overflow", field.upperExclusive, false},
				{"above overflow", math.Nextafter(field.upperExclusive, math.Inf(1)), false},
				{"negative", -0.000001, false},
				{"nan", math.NaN(), false},
				{"positive infinity", math.Inf(1), false},
				{"negative infinity", math.Inf(-1), false},
				{"unreadable", "not numeric", false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var data map[string]interface{}
					if err := json.Unmarshal([]byte(`{"attributes":{"OBJECTID":1,"PID":"A"},"geometry":{"rings":[[[0,0],[0,1],[1,1],[1,0],[0,0]]]}}`), &data); err != nil {
						t.Fatal(err)
					}
					data["attributes"].(map[string]interface{})[field.name] = tc.value
					_, err := union.NormalizeParcel(connector.RawRecord{SourceName: "union_county_gis", RawData: data})
					var invalid *union.InvalidParcelRecord
					if tc.valid {
						if err != nil {
							t.Fatal(err)
						}
					} else if !errors.As(err, &invalid) || invalid.Reason != "invalid "+field.name {
						t.Fatalf("expected field quarantine, got %T %v", err, err)
					}
				})
			}
		})
	}
}
