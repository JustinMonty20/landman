package union

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestQualificationOmitsOnlyRoadWithoutGeometry(t *testing.T) {
	for _, tc := range []struct {
		name, geometry string
		wantError      string
	}{
		{"absent", "absent", ""},
		{"null", "null", ""},
		{"wrong geometry type", "wrong", "invalid road geometry"},
		{"empty paths", "empty", "missing road paths"},
		{"invalid point", "invalid", "invalid road point"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, close := qualificationServer(t, 170, fixtureHandler(t, 170, func(r *http.Request, _ int, body string) (int, string) {
				if r.URL.Query().Get("returnGeometry") != "true" {
					return 200, body
				}
				var page struct {
					Features         []map[string]json.RawMessage `json:"features"`
					SpatialReference json.RawMessage              `json:"spatialReference"`
				}
				if err := json.Unmarshal([]byte(body), &page); err != nil {
					t.Fatal(err)
				}
				if tc.geometry == "absent" {
					delete(page.Features[0], "geometry")
				} else {
					value := map[string]string{"null": "null", "wrong": "[]", "empty": `{"paths":[]}`, "invalid": `{"paths":[[0,1]]}`}[tc.geometry]
					page.Features[0]["geometry"] = json.RawMessage(value)
				}
				encoded, err := json.Marshal(page)
				if err != nil {
					t.Fatal(err)
				}
				return 200, string(encoded)
			}))
			defer close()
			var seen []int64
			receipt, err := src.FetchQualificationLayer(context.Background(), 170, func(f QualificationFeature) error {
				seen = append(seen, f.(RoadFeature).SourceID)
				return nil
			})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil || receipt.Count != 2 || len(seen) != 1 || seen[0] != 2 {
				t.Fatalf("receipt=%+v seen=%v error=%v", receipt, seen, err)
			}
			if len(receipt.OmittedRoadIDs) != 1 || receipt.OmittedRoadIDs[0] != 1 {
				t.Fatalf("omitted=%v", receipt.OmittedRoadIDs)
			}
		})
	}
}
