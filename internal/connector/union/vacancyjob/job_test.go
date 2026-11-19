package vacancyjob

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
			}{{base + "f=json&returnCountOnly=true&where=1%3D1", count, 0}, {base + "f=json&returnIdsOnly=true&where=1%3D1", strings.ReplaceAll(`{"objectIdFieldName":"OBJECTID","objectIds":[7]}`, "[7]", map[bool]string{true: "[7,8]", false: "[7]"}[tc.name == "duplicate OID"]), 0}, {base + "f=json&outFields=%2A&orderByFields=OBJECTID+ASC&outSR=3857&resultOffset=0&resultRecordCount=2000&returnGeometry=true&where=1%3D1", tc.body, 3857}, {base + "f=json&returnIdsOnly=true&where=1%3D1", strings.ReplaceAll(`{"objectIdFieldName":"OBJECTID","objectIds":[7]}`, "[7]", map[bool]string{true: "[7,8]", false: "[7]"}[tc.name == "duplicate OID"]), 0}, {base + "f=json&returnCountOnly=true&where=1%3D1", count, 0}} {
				if err := recorder.RecordHTTP(ctx, r.url, "", r.crs, 200, true, strings.NewReader(r.body)); err != nil {
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
func TestPrepareRejectsUntrustedHTTPOutcome(t *testing.T) {
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
	for _, tc := range []struct {
		name     string
		status   int
		complete bool
		rejected bool
	}{
		{"success", 200, true, false},
		{"unknown", 0, true, true},
		{"refusal", 403, true, true},
		{"truncated", 200, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ingest.NewRecorder(store, "union_assessed_value_0", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			for i, request := range []string{base + "f=json&returnCountOnly=true&where=1%3D1", base + "f=json&outFields=%2A&resultRecordCount=2000&returnGeometry=false&where=1%3D1"} {
				body := `{"count":0}`
				if i == 1 {
					body = `{"features":[]}`
				}
				if err := r.RecordHTTP(ctx, request, "", 0, tc.status, tc.complete, strings.NewReader(body)); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.BindAttempt(AssessedValueID, "00000000-0000-0000-0000-000000000001", "full_scan", 0); err != nil {
				t.Fatal(err)
			}
			v, err := r.Verify(ctx)
			if err != nil {
				t.Fatal(err)
			}
			err = (&publisher{layer: union.AssessedValueLayer}).Prepare(ctx, v)
			if (err != nil) != tc.rejected {
				t.Fatalf("prepare outcome: %v", err)
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
				if err := recorder.RecordHTTP(ctx, r.url, "", 0, 200, true, strings.NewReader(r.body)); err != nil {
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

func TestStructureMultiPageCaptureAndReplayValidation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	var version, calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		switch {
		case q.Get("returnCountOnly") == "true":
			if version == 6 && calls > 3 {
				fmt.Fprint(w, `{"count":2002}`)
			} else {
				fmt.Fprint(w, `{"count":2001}`)
			}
		case q.Get("returnIdsOnly") == "true":
			fmt.Fprint(w, `{"objectIdFieldName":"OBJECTID","objectIds":[`)
			for i := 1; i <= 2001; i++ {
				if version == 4 && i == 2001 {
					break
				}
				if i > 1 {
					fmt.Fprint(w, ",")
				}
				v := i
				if version == 3 && calls > 3 && i == 2001 {
					v = 2002
				}
				fmt.Fprint(w, v)
			}
			fmt.Fprint(w, `]}`)
		default:
			offset, _ := strconv.Atoi(q.Get("resultOffset"))
			if version == 1 && offset == 2000 {
				fmt.Fprint(w, `{"features":[],"exceededTransferLimit":true}`)
				return
			}
			if version == 2 && offset == 2000 {
				fmt.Fprint(w, `{bad`)
				return
			}
			fmt.Fprint(w, `{"spatialReference":{"wkid":3857},"features":[`)
			end := offset + 2000
			if end > 2001 {
				end = 2001
			}
			for i := offset; i < end; i++ {
				if i > offset {
					fmt.Fprint(w, ",")
				}
				id := i + 1
				if version == 5 && offset == 2000 {
					id = 2000
				}
				fmt.Fprintf(w, `{"attributes":{"OBJECTID":%d,"PIN_Num":"001"},"geometry":{"rings":[[[0,0],[1,0],[0,0]]]}}`, id)
			}
			fmt.Fprint(w, `]}`)
		}
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	capture := func() (*publisher, error) {
		recorder, e := ingest.NewRecorder(store, "union_structures_171", time.Now())
		if e != nil {
			t.Fatal(e)
		}
		n, e := source.CaptureVacancyLayer(ctx, union.StructuresLayer, func(u string, b []byte, crs, status int, complete bool) error {
			return recorder.RecordHTTP(ctx, u, "", crs, status, complete, strings.NewReader(string(b)))
		})
		if e != nil {
			return nil, e
		}
		if n != 2001 || len(recorder.Manifest().Receipts) != 6 {
			t.Fatalf("count=%d receipts=%d", n, len(recorder.Manifest().Receipts))
		}
		if e = recorder.BindAttempt(StructuresID, "00000000-0000-0000-0000-000000000001", "full_scan", 0); e != nil {
			t.Fatal(e)
		}
		verified, e := recorder.Verify(ctx)
		if e != nil {
			t.Fatal(e)
		}
		pub := &publisher{layer: union.StructuresLayer}
		return pub, pub.Prepare(ctx, verified)
	}
	first, err := capture()
	if err != nil || len(first.structures) != 2001 {
		t.Fatalf("first capture %v", err)
	}
	second, err := capture()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := first.normalizedFingerprint()
	b, _ := second.normalizedFingerprint()
	if a != b {
		t.Fatal("repeat changed normalized rows")
	}
	for left, right := 0, len(second.structures)-1; left < right; left, right = left+1, right-1 {
		second.structures[left], second.structures[right] = second.structures[right], second.structures[left]
	}
	b, err = second.normalizedFingerprint()
	if err != nil || a != b {
		t.Fatal("page split or row ordering changed normalized rows")
	}
	for _, v := range []int{1, 2, 3, 4, 5, 6} {
		version = v
		calls = 0
		if _, err = capture(); err == nil {
			t.Fatalf("failed later page or membership accepted version %d", v)
		}
	}
}
