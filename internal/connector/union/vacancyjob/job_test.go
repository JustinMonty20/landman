package vacancyjob

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/taxartifact"
)

func TestStructureCaptureValidation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "https://example.test/server/rest/services/OperationalLayers/MapServer/171/query?"
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", `{"spatialReference":{"wkid":3857},"features":[{"attributes":{"OBJECTID":7,"outline_id":"shape-7","PIN_Num":"001","status":"existing"},"geometry":{"rings":[[[0,0],[1,0],[1,1],[0,0]]]}}]}`, true},
		{"invalid crs", `{"spatialReference":{"wkid":4326},"features":[{"attributes":{"OBJECTID":7},"geometry":{"rings":[]}}]}`, false},
		{"duplicate OID", `{"spatialReference":{"wkid":3857},"features":[{"attributes":{"OBJECTID":7},"geometry":{"rings":[[]]}},{"attributes":{"OBJECTID":7},"geometry":{"rings":[[]]}}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder, err := ingest.NewRecorder(store, "union_structures_171", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			count := `{"count":1}`
			if tc.name == "duplicate OID" {
				count = `{"count":2}`
			}
			for _, r := range []struct {
				url, body string
				crs       int
			}{{base + "f=json&returnCountOnly=true&where=1%3D1", count, 0}, {base + "f=json&outFields=%2A&outSR=3857&resultRecordCount=2000&returnGeometry=true&where=1%3D1", tc.body, 3857}} {
				if err := recorder.Record(ctx, r.url, "", r.crs, strings.NewReader(r.body)); err != nil {
					t.Fatal(err)
				}
			}
			if err := recorder.BindAttempt(StructuresID, "00000000-0000-0000-0000-000000000001", "full_scan", 0); err != nil {
				t.Fatal(err)
			}
			capture, err := recorder.Verify(ctx)
			if err != nil {
				t.Fatal(err)
			}
			pub := &publisher{layer: union.StructuresLayer}
			err = pub.Prepare(ctx, capture)
			if (err == nil) != tc.valid {
				t.Fatalf("prepare: %v", err)
			}
			if tc.valid && (len(pub.structures) != 1 || pub.structures[0].PIN != "001") {
				t.Fatalf("missing typed geometry: %+v", pub.structures)
			}
		})
	}
}
func TestNormalizedFingerprintRetainsMultiplicityWithoutPageOrder(t *testing.T) {
	row := assessment{ObjectID: "7", ParcelNumber: "001", Category: "AV"}
	other := assessment{ObjectID: "8", ParcelNumber: "002", Category: "PY"}
	fingerprint := func(rows ...assessment) string {
		p := publisher{layer: union.AssessedValueLayer, assessments: rows, count: len(rows)}
		h, err := p.normalizedFingerprint()
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if fingerprint(row, other, row) != fingerprint(other, row, row) {
		t.Fatal("page order changed normalized fingerprint")
	}
	if fingerprint(row, other, row) == fingerprint(row, other) {
		t.Fatal("duplicate multiplicity was lost")
	}
}

func TestAssessmentRepeatedOIDPreserved(t *testing.T) {
	raw := []byte(`{"attributes":{"OBJECTID":7,"parcel_number":"09336350","parcel_year":2027,"assess_category":"AV","market_building":0,"market_land":123.40,"market_total":123.40}}`)
	first, err := parseAssessment(raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := parseAssessment(raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.ObjectID != "7" || second.ObjectID != "7" || first.Year == nil || *first.Year != 2027 || *first.Building != "0.00" {
		t.Fatalf("rows lost: %+v %+v", first, second)
	}
	for _, bad := range []string{`{"attributes":{"OBJECTID":7,"parcel_number":"001","market_building":-1}}`, `{"attributes":{"parcel_number":"001","market_building":1.001}}`} {
		if _, err := parseAssessment(json.RawMessage(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
func TestPrepareRejectsPartialAndCorruptCapture(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "https://example.test/server/rest/services/Property_Tax_Live/Assessed_Value/MapServer/0/query?"
	count := base + "f=json&returnCountOnly=true&where=1%3D1"
	page := base + "f=json&outFields=%2A&resultRecordCount=2000&returnGeometry=false&where=1%3D1"
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"repeated object id", `{"features":[{"attributes":{"OBJECTID":7,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":0}},{"attributes":{"OBJECTID":7,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":0}}]}`, true},
		{"partial", `{"features":[{"attributes":{"OBJECTID":7,"parcel_number":"001"}}]}`, false},
		{"error", `{"error":{"message":"denied"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder, err := ingest.NewRecorder(store, "union_assessed_value_0", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range []struct{ url, body string }{{count, `{"count":2}`}, {page, tc.body}} {
				if err := recorder.Record(ctx, r.url, "", 0, strings.NewReader(r.body)); err != nil {
					t.Fatal(err)
				}
			}
			if err := recorder.BindAttempt(AssessedValueID, "00000000-0000-0000-0000-000000000001", "full_scan", 0); err != nil {
				t.Fatal(err)
			}
			capture, err := recorder.Verify(ctx)
			if err != nil {
				t.Fatal(err)
			}
			pub := &publisher{layer: union.AssessedValueLayer}
			err = pub.Prepare(ctx, capture)
			if (err == nil) != tc.valid {
				t.Fatalf("prepare: %v", err)
			}
			if tc.valid && (len(pub.assessments) != 2 || pub.assessments[0].ObjectID != pub.assessments[1].ObjectID) {
				t.Fatalf("lost repeated rows: %+v", pub.assessments)
			}
		})
	}
}
