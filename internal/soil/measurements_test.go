package soil_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/soil"
)

func TestMeasurementsCapturedAndAtomicFailures(t *testing.T) {
	server := fixtureServer(t, nil)
	defer server.Close()
	values, err := soil.FetchInterpretations(context.Background(), server.Client(), server.URL, []string{"115328", "115330", "115341"})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, m := range values[0].Measurements {
		kinds[m.Kind]++
		if m.ComponentKey == "" || m.SourceQuery == "" || m.FetchedAt.IsZero() {
			t.Fatalf("unidentified observation: %+v", m)
		}
	}
	if kinds["flood_frequency"] != 24 || kinds["ponding_frequency"] != 24 || kinds["water_table_depth"] < 24 || kinds["linear_extensibility"] == 0 || kinds["restriction_depth"] != 3 {
		t.Fatalf("missing captured observations: %v", kinds)
	}
	for _, m := range values[0].Measurements {
		if m.Kind == "restriction_depth" && m.Value.Availability == soil.Rated && m.Value.Unit != "cm" {
			t.Fatalf("restriction unit: %+v", m)
		}
	}
	for _, tc := range []struct {
		name, fixture string
		mutate        func(*[][]any)
	}{
		{"partial month", "union-monthly-raw.json", func(r *[][]any) { *r = (*r)[1:] }},
		{"duplicate calendar month", "union-monthly-raw.json", func(r *[][]any) { (*r)[1][3] = (*r)[0][3] }},
		{"unknown component", "union-monthly-raw.json", func(r *[][]any) { (*r)[0][1] = "unknown" }},
		{"unknown horizon component", "union-horizons-raw.json", func(r *[][]any) { (*r)[0][1] = "unknown" }},
		{"unknown restriction component", "union-restrictions-raw.json", func(r *[][]any) { (*r)[0][1] = "unknown" }},
		{"unknown moisture component", "union-moisture-raw.json", func(r *[][]any) { (*r)[0][1] = "unknown" }},
		{"orphan moisture", "union-moisture-raw.json", func(r *[][]any) { (*r)[0][2] = "unknown" }},
		{"invalid horizon", "union-horizons-raw.json", func(r *[][]any) { (*r)[0][5] = "not numeric" }},
		{"invalid restriction", "union-restrictions-raw.json", func(r *[][]any) { (*r)[0][2] = "-1" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := fixtureServer(t, func(q string) (int, []byte) {
				match := map[string]string{"union-monthly-raw.json": "JOIN comonth", "union-moisture-raw.json": "JOIN cosoilmoist", "union-horizons-raw.json": "JOIN chorizon", "union-restrictions-raw.json": "JOIN corestrictions"}[tc.fixture]
				if !strings.Contains(q, match) || (tc.fixture == "union-monthly-raw.json" && strings.Contains(q, "JOIN cosoilmoist")) {
					return 0, nil
				}
				raw, e := os.ReadFile("testdata/" + tc.fixture)
				if e != nil {
					t.Fatal(e)
				}
				var body struct {
					Table [][]any `json:"Table"`
				}
				if e = json.Unmarshal(raw, &body); e != nil {
					t.Fatal(e)
				}
				tc.mutate(&body.Table)
				raw, e = json.Marshal(body)
				if e != nil {
					t.Fatal(e)
				}
				return http.StatusOK, raw
			})
			defer broken.Close()
			v, e := soil.FetchInterpretations(context.Background(), broken.Client(), broken.URL, []string{"115328", "115330", "115341"})
			if e == nil || v != nil {
				t.Fatalf("partial fetch published %d results: %v", len(v), e)
			}
		})
	}
}
