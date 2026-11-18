package union

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestQualificationRequiredFeatureAttributes(t *testing.T) {
	for _, spec := range zoningLayers {
		t.Run(fmt.Sprint(spec.id), func(t *testing.T) {
			var sample struct {
				Features []json.RawMessage `json:"features"`
			}
			if err := json.Unmarshal(fixture(t, spec.id, "sample"), &sample); err != nil {
				t.Fatal(err)
			}
			fields := []string{spec.code, spec.description, spec.caseField, spec.dateField}
			if spec.kind == QualificationRoads {
				fields = append(fields, "FTR_CODE", "RoadClass", "STATUS")
			}
			for _, field := range fields {
				if field == "" {
					continue
				}
				t.Run(field, func(t *testing.T) {
					for _, variation := range []string{"missing", "wrong type", "null"} {
						t.Run(variation, func(t *testing.T) {
							var feature map[string]interface{}
							if err := json.Unmarshal(sample.Features[0], &feature); err != nil {
								t.Fatal(err)
							}
							attrs := feature["attributes"].(map[string]interface{})
							switch variation {
							case "missing":
								delete(attrs, field)
							case "wrong type":
								if spec.kind == QualificationRoads && field == spec.dateField {
									attrs[field] = "yesterday"
								} else {
									attrs[field] = 42
								}
							case "null":
								attrs[field] = nil
							}
							raw, err := json.Marshal(feature)
							if err != nil {
								t.Fatal(err)
							}
							_, err = parseQualificationFeature(spec, raw, 102100)
							if variation == "null" && err != nil {
								t.Fatalf("explicit unknown %s: %v", field, err)
							}
							if variation != "null" && (err == nil || !strings.Contains(err.Error(), field)) {
								t.Fatalf("%s %s: %v", variation, field, err)
							}
						})
					}
				})
			}
			for _, id := range []interface{}{nil, "1", 1.5, 1e30} {
				var feature map[string]interface{}
				if err := json.Unmarshal(sample.Features[0], &feature); err != nil {
					t.Fatal(err)
				}
				feature["attributes"].(map[string]interface{})[spec.oid] = id
				raw, _ := json.Marshal(feature)
				if _, err := parseQualificationFeature(spec, raw, 102100); err == nil {
					t.Fatalf("invalid OID %v accepted", id)
				}
			}
		})
	}
}
