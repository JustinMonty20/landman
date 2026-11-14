package soil_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/soil"
)

func fixtureServer(t *testing.T, alter func(string) (int, []byte)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.FormValue("format") != "JSON" {
			t.Errorf("unexpected USDA request: %s %s", r.Method, r.FormValue("format"))
			w.WriteHeader(400)
			return
		}
		q := r.FormValue("query")
		if alter != nil {
			if code, body := alter(q); code != 0 {
				w.WriteHeader(code)
				_, _ = w.Write(body)
				return
			}
		}
		var file string
		switch {
		case strings.Contains(q, "FROM sdvattribute"):
			file = "union-rules-raw.json"
		case strings.Contains(q, "JOIN muaggatt"):
			file = "union-muaggatt-raw.json"
		case strings.Contains(q, "JOIN sacatalog"):
			file = "union-catalog-raw.json"
		case strings.Contains(q, "FROM component c WHERE"):
			file = "union-ancillary-components-raw.json"
		case strings.Contains(q, "JOIN cointerp"):
			file = "union-components-raw.json"
		case strings.Contains(q, "JOIN cosoilmoist"):
			file = "union-moisture-raw.json"
		case strings.Contains(q, "JOIN comonth"):
			file = "union-monthly-raw.json"
		case strings.Contains(q, "JOIN chorizon"):
			file = "union-horizons-raw.json"
		case strings.Contains(q, "JOIN corestrictions"):
			file = "union-restrictions-raw.json"
		default:
			t.Errorf("unexpected query %q", q)
			w.WriteHeader(400)
			return
		}
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write(b)
	}))
}
func TestFetchInterpretationsCapturedUSDA(t *testing.T) {
	server := fixtureServer(t, nil)
	defer server.Close()
	values, err := soil.FetchInterpretations(context.Background(), server.Client(), server.URL, []string{"115341", "115328", "115330", "115328"})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 9 {
		t.Fatalf("got %d uses, want 9", len(values))
	}
	var septic, dwelling *soil.Interpretation
	for i := range values {
		v := &values[i]
		if v.MapUnitKey == "115328" {
			if v.Use == soil.ConventionalSeptic {
				septic = v
			}
			if v.Use == soil.DwellingWithoutBasement {
				dwelling = v
			}
		}
	}
	if septic == nil || septic.Rating != "Somewhat limited" || septic.UnratedComponentPercent == nil || *septic.UnratedComponentPercent < 14.99 || len(septic.Components) < 2 {
		t.Fatalf("lost mixed component evidence: %+v", septic)
	}
	if dwelling == nil || dwelling.Rating != "Very limited" {
		t.Fatalf("lost distinct dwelling rating: %+v", dwelling)
	}
	if septic.Evidence["slope_percent"] == nil || *septic.Evidence["slope_percent"] != "5" {
		t.Fatalf("missing slope provenance: %+v", septic.Evidence)
	}
	fact := septic.Facts["slope_percent"]
	if fact.Availability != soil.Unavailable || fact.Value != nil || fact.RawValue == nil || fact.Unit != "" {
		t.Fatalf("unverified ancillary units became measured evidence: %+v", fact)
	}
	var appling *soil.ComponentResult
	for i := range septic.Components {
		if septic.Components[i].Key == "28382578" && septic.Components[i].Sequence == 0 {
			appling = &septic.Components[i]
		}
	}
	if appling == nil || appling.Percent == nil || *appling.Percent != 25 || appling.Facts["agricultural_capability_class"].Availability != soil.Rated || *appling.Facts["agricultural_capability_class"].Value != "2" || *appling.Facts["agricultural_capability_subclass"].Value != "e" || appling.Facts["slope_percent"].Unit != "percent" {
		t.Fatalf("missing validated, component-scoped USDA facts: %+v", appling)
	}
	if len(septic.Measurements) == 0 {
		t.Fatal("missing component observations")
	}
	if septic.Revision.SurveyVersion == nil || *septic.Revision.SurveyVersion != "28" || septic.Revision.SurveySavedOn == nil || septic.Revision.SurveySavedOn.Hour() != 0 {
		t.Fatalf("lost revision or invented survey timezone: %+v", septic.Revision)
	}
}
func TestFetchInterpretationsRejectsIncompleteOrUnsupportedEvidence(t *testing.T) {
	cases := map[string]func(string) (int, []byte){
		"http error": func(q string) (int, []byte) {
			if strings.Contains(q, "muaggatt") {
				return 503, nil
			}
			return 0, nil
		},
		"partial": func(q string) (int, []byte) {
			if strings.Contains(q, "muaggatt") {
				return 200, []byte(`{"Table":[]}`)
			}
			return 0, nil
		},
		"bad schema": func(q string) (int, []byte) {
			if strings.Contains(q, "muaggatt") {
				return 200, []byte(`{"Table":[["115328"]]}`)
			}
			return 0, nil
		},
		"unknown domain": func(q string) (int, []byte) {
			if strings.Contains(q, "muaggatt") {
				return 200, []byte(`{"Table":[["115328","Approved","Not limited","Not limited",null,null,null,null,null,null]]}`)
			}
			return 0, nil
		},
		"rule drift": func(q string) (int, []byte) {
			if strings.Contains(q, "sdvattribute") {
				return 200, []byte(`{"Table":[]}`)
			}
			return 0, nil
		},
		"unsupported ancillary domain": func(q string) (int, []byte) {
			if strings.Contains(q, "FROM component c WHERE") {
				return 200, []byte(`{"Table":[["115328","28382578","Appling","25","9","e","Well drained","5"]]}`)
			}
			return 0, nil
		},
		"component failure": func(q string) (int, []byte) {
			if strings.Contains(q, "cointerp") {
				return 503, nil
			}
			return 0, nil
		},
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			server := fixtureServer(t, alter)
			defer server.Close()
			values, err := soil.FetchInterpretations(context.Background(), server.Client(), server.URL, []string{"115328", "115330", "115341"})
			if err == nil || values != nil {
				t.Fatalf("published partial evidence: %+v, %v", values, err)
			}
		})
	}
	t.Run("canceled", func(t *testing.T) {
		server := fixtureServer(t, nil)
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		values, err := soil.FetchInterpretations(ctx, server.Client(), server.URL, []string{"115328"})
		if err == nil || values != nil {
			t.Fatalf("published canceled evidence: %+v %v", values, err)
		}
	})
}
func TestFetchInterpretationsNotRatedAndMissingEvidence(t *testing.T) {
	alter := func(query string) (int, []byte) {
		if !strings.Contains(query, "muaggatt") {
			return 0, nil
		}
		body, err := os.ReadFile("testdata/union-muaggatt-raw.json")
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Table [][]any `json:"Table"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			t.Fatal(err)
		}
		data.Table[0][1] = "Not rated"
		data.Table[0][7] = nil
		body, err = json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return 200, body
	}
	server := fixtureServer(t, alter)
	defer server.Close()
	values, err := soil.FetchInterpretations(context.Background(), server.Client(), server.URL, []string{"115328", "115330", "115341"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range values {
		if v.MapUnitKey == "115328" && v.Use == soil.ConventionalSeptic {
			if !v.NotRated || v.Rating != "" || v.Evidence["water_table_depth_cm"] != nil {
				t.Fatalf("missing data became favorable: %+v", v)
			}
			return
		}
	}
	t.Fatal("missing septic result")
}

func TestCapturedFixtureIsRawTable(t *testing.T) {
	for _, file := range []string{"union-rules-raw.json", "union-muaggatt-raw.json", "union-components-raw.json"} {
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]json.RawMessage
		if err = json.Unmarshal(b, &result); err != nil || result["Table"] == nil {
			t.Fatalf("fixture %s: %v", file, err)
		}
	}
}

type deadlineTransport struct {
	t        *testing.T
	deadline time.Time
}

func (d deadlineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	actual, ok := r.Context().Deadline()
	if !ok || !actual.Equal(d.deadline) {
		d.t.Errorf("required USDA query changed caller deadline: %v, want %v", actual, d.deadline)
	}
	return &http.Response{StatusCode: 503, Body: http.NoBody, Header: make(http.Header), Request: r}, nil
}

func TestRequiredInterpretationsDoNotUseOptionalNameTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	deadline, _ := ctx.Deadline()
	client := &http.Client{Transport: deadlineTransport{t: t, deadline: deadline}}
	if _, err := soil.FetchInterpretations(ctx, client, "https://fixture.invalid", []string{"115328"}); err == nil {
		t.Fatal("expected source failure")
	}
}
