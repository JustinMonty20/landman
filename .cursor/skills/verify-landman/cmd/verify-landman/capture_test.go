package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
)

func TestParcelLikelyVacantEmptyPIDIsFalse(t *testing.T) {
	empty := connector.RawRecord{RawData: map[string]any{
		"attributes": map[string]any{"PID": "", "YEARBLT": float64(0)},
	}}
	if parcelLikelyVacant(empty) {
		t.Fatal("empty PID")
	}
	present := connector.RawRecord{RawData: map[string]any{
		"attributes": map[string]any{"PID": "07102010", "YEARBLT": float64(0)},
	}}
	if !parcelLikelyVacant(present) {
		t.Fatal("zero year")
	}
}

func TestPublishCapturedSeedLeavesSeedWhenIDsDiffer(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "seed.sql")
	original := []byte(captureSeedSQL)
	if err := os.WriteFile(seedPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := seedPath + ".capture-tmp"
	other := strings.Replace(captureSeedSQL, "07102010", "99999999", 1)
	if err := os.WriteFile(tmp, []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	err := publishCapturedSeed(tmp, seedPath, []string{"07102010"})
	if err == nil {
		t.Fatal("publish")
	}
	if _, statErr := os.Stat(tmp); !os.IsNotExist(statErr) {
		t.Fatal("temp file remains")
	}
	got, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatal("seed bytes changed")
	}
}

func TestCaptureTimeoutLeavesSeed(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "seed.sql")
	original := []byte(captureSeedSQL)
	if err := os.WriteFile(seedPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := seedPath + ".capture-tmp"
	if err := os.WriteFile(tmp, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, code := runCapture(verifyContainerName, capturePostgresURL, seedPath, captureDeps{
		get: func(context.Context, string) (int, []byte, error) {
			return 0, nil, timeoutNetError{}
		},
		exec: func(context.Context, string, string, string) error {
			t.Fatal("exec")
			return nil
		},
		query: func(context.Context, string, string, string) (string, error) {
			t.Fatal("query")
			return "", nil
		},
	})
	if code != 2 || text != "capture: left seed.sql in place (county slow)\n" {
		t.Fatalf("code %d text %q", code, text)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp file remains")
	}
	got, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatal("seed bytes changed")
	}
}

func TestCaptureRefusesOtherContainerBeforeHTTP(t *testing.T) {
	text, code := runCapture("landman-postgis", capturePostgresURL, "missing.sql", captureDeps{
		get: func(context.Context, string) (int, []byte, error) {
			t.Fatal("http")
			return 0, nil, nil
		},
	})
	if code != 1 || text != "" {
		t.Fatalf("code %d text %q", code, text)
	}
}

func TestCaptureSuccessWritesSeedAndDropsCaptureDB(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "seed.sql")
	if err := os.WriteFile(seedPath, []byte(captureSeedSQL), 0o644); err != nil {
		t.Fatal(err)
	}
	var scripts []string
	var urls []string
	when := time.Date(2026, 11, 14, 22, 0, 0, 0, time.UTC)
	text, code := runCapture(verifyContainerName, capturePostgresURL, seedPath, captureDeps{
		now: when,
		get: captureCounty,
		exec: func(_ context.Context, _, databaseURL, sql string) error {
			scripts = append(scripts, sql)
			urls = append(urls, databaseURL)
			return nil
		},
		query: func(_ context.Context, _, databaseURL, sql string) (string, error) {
			if !strings.Contains(databaseURL, "/landman_capture") {
				t.Fatalf("query url %s", databaseURL)
			}
			if !strings.Contains(sql, overlapAcresExpr) {
				t.Fatalf("overlap sql %s", sql)
			}
			urls = append(urls, databaseURL)
			return captureOverlapRow(t, scripts), nil
		},
	})
	if code != 0 || text != "" {
		t.Fatalf("code %d text %q", code, text)
	}
	if _, err := os.Stat(seedPath + ".capture-tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file remains")
	}
	body, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	seed := string(body)
	assertCaptureSeed(t, seed, when)
	if !strings.Contains(strings.Join(scripts, "\n"), "DROP DATABASE IF EXISTS landman_capture") {
		t.Fatal("missing drop")
	}
	if dropsLandmanDatabase(strings.Join(scripts, "\n")) {
		t.Fatal("dropped landman")
	}
	joined := strings.Join(urls, "\n")
	if !strings.Contains(joined, "/landman_capture") {
		t.Fatal("capture database")
	}
}

func TestRenderSeedAcceptsRealSeedColumns(t *testing.T) {
	text, err := os.ReadFile("../../seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	previous, err := parseSeed(string(text))
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := previous.parcel("07102010")
	if !ok {
		t.Fatal("sample parcel")
	}
	parcel, err := union.NewGISTranslator().Translate(connector.RawRecord{
		SourceName: "union_county_gis",
		ParcelID:   "07102010",
		RawData:    captureParcelFeature(),
	})
	if err != nil {
		t.Fatal(err)
	}
	polygons, units, err := union.ParseSoilFeatures([]byte(captureSoilPage))
	if err != nil {
		t.Fatal(err)
	}
	soils, err := shapeSoils(polygons, units)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 14, 22, 0, 0, 0, time.UTC)
	body, err := renderSeed(previous, []renderedParcel{{
		ID: stored.ID, Vacant: parcelLikelyVacant(connector.RawRecord{RawData: captureParcelFeature()}), Parcel: parcel,
	}}, soils, []renderedOverlap{{
		ID: "55555555-5555-4555-8555-555555555555", SoilID: soils[0].ID, ParcelID: stored.ID, Acres: "0.25",
	}}, when)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseSeed(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.parcelIDs()[0] != "07102010" {
		t.Fatalf("ids %v", parsed.parcelIDs())
	}
	if !strings.Contains(body, "'0.25'") {
		t.Fatal("overlap acres")
	}
}

func assertCaptureSeed(t *testing.T, seed string, when time.Time) {
	t.Helper()
	for _, phrase := range []string{
		"does not write parcel_sales",
		"does not call SaveSales",
		"parcel_sales is still",
		"and 79 sales",
	} {
		if strings.Contains(seed, phrase) {
			t.Fatalf("stale %q", phrase)
		}
	}
	for _, line := range []string{
		"-- Captured " + when.UTC().Format(time.RFC3339) + ".",
		"-- Parcel query " + parcelLayer + "/query orderByFields=OBJECTID",
		"-- Soil query " + soilLayer + "/query orderByFields=OBJECTID_1",
		"-- SavePart calls SaveSales and then deletes omitted sequences.",
		"-- SaveSales alone does not delete.",
	} {
		if !strings.Contains(seed, line) {
			t.Fatalf("missing %s", line)
		}
	}
	if strings.Contains(seed, "county-wide") {
		t.Fatal("county-wide sales")
	}
	snap, err := parseSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	parcel, ok := snap.parcel("07102010")
	if !ok || parcel.ID != "11111111-1111-1111-1111-111111111111" || !parcel.LikelyVacant || parcel.Situs != "105 CUPPED OAK DR" {
		t.Fatalf("parcel %+v", parcel)
	}
	known := snap.mapUnits["115330"]
	if !known.HasName || known.Name != "Appling sandy loam" {
		t.Fatalf("known unit %+v", known)
	}
	fresh := snap.mapUnits["999999"]
	if fresh.HasName || fresh.HasArea {
		t.Fatalf("new unit %+v", fresh)
	}
	if !strings.Contains(seed, "'9472'") {
		t.Fatal("sale book")
	}
	if !strings.Contains(seed, "'1.5'") {
		t.Fatal("overlap acres")
	}
}

func captureCounty(_ context.Context, rawURL string) (int, []byte, error) {
	switch {
	case strings.Contains(rawURL, "/215?f=json"):
		return 200, []byte(`{"maxRecordCount":10}`), nil
	case strings.Contains(rawURL, "/215/query"):
		if !strings.Contains(rawURL, "orderByFields=OBJECTID") || !strings.Contains(rawURL, "07102010") || !strings.Contains(rawURL, "returnGeometry=true") {
			return 0, nil, errCaptureURL(rawURL)
		}
		if strings.Contains(rawURL, "resultRecordCount=11") || strings.Contains(rawURL, "resultRecordCount=2000") {
			return 0, nil, errCaptureURL(rawURL)
		}
		return 200, []byte(`{"features":[` + captureFeatureJSON(captureParcelFeature()) + `]}`), nil
	case strings.Contains(rawURL, "/197?f=json"):
		return 200, []byte(`{"maxRecordCount":10}`), nil
	case strings.Contains(rawURL, "/197/query"):
		if !strings.Contains(rawURL, "spatialRel=esriSpatialRelIntersects") || !strings.Contains(rawURL, "orderByFields=OBJECTID_1") {
			return 0, nil, errCaptureURL(rawURL)
		}
		return 200, []byte(captureSoilPage), nil
	default:
		return 0, nil, errCaptureURL(rawURL)
	}
}

func captureOverlapRow(t *testing.T, scripts []string) string {
	t.Helper()
	var parcelID, soilID string
	for _, script := range scripts {
		if id := firstUUID(script, "INSERT INTO parcels (id, shape)"); id != "" {
			parcelID = id
		}
		if id := firstUUID(script, "INSERT INTO soil_polygons (id, shape)"); id != "" && soilID == "" {
			soilID = id
		}
	}
	if parcelID == "" || soilID == "" {
		t.Fatal("capture inserts")
	}
	return soilID + "\t" + parcelID + "\t1.5\n"
}

func firstUUID(script, marker string) string {
	at := strings.Index(script, marker)
	if at < 0 {
		return ""
	}
	rest := script[at:]
	start := strings.Index(rest, "'")
	if start < 0 || start+37 > len(rest) {
		return ""
	}
	return rest[start+1 : start+37]
}

func dropsLandmanDatabase(script string) bool {
	for _, line := range strings.Split(script, "\n") {
		fields := strings.Fields(line)
		for i := 0; i < len(fields); i++ {
			if fields[i] != "DATABASE" || i+1 >= len(fields) {
				continue
			}
			name := strings.TrimSuffix(fields[i+1], ";")
			if name == "landman" {
				return true
			}
		}
	}
	return false
}

func captureParcelFeature() map[string]any {
	return map[string]any{
		"attributes": map[string]any{
			"PID":          "07102010",
			"PHYSSTRADD":   "105 CUPPED OAK DR",
			"YEARBLT":      float64(0),
			"s1_DEED_BOOK": "9472",
			"s1_DEED_PAGE": "0680",
			"s1_SALEDATE":  float64(1790726400000),
			"s1_SALESAMT":  float64(0),
		},
		"geometry": map[string]any{
			"rings": []any{
				[]any{[]any{0.0, 0.0}, []any{0.0, 1.0}, []any{1.0, 1.0}, []any{1.0, 0.0}, []any{0.0, 0.0}},
			},
		},
	}
}

func captureFeatureJSON(feature map[string]any) string {
	body, err := json.Marshal(feature)
	if err != nil {
		panic(err)
	}
	return string(body)
}

const captureSoilPage = `{"features":[
{"attributes":{"OBJECTID_1":1,"AREASYMBOL":"NC179","SPATIALVER":5,"MUSYM":"ApB","MUKEY":"115330"},"geometry":{"rings":[[[0,0],[0,1],[1,1],[1,0],[0,0]]]}},
{"attributes":{"OBJECTID_1":2,"AREASYMBOL":"NC179","SPATIALVER":5,"MUSYM":"ZzZ","MUKEY":"999999"},"geometry":{"rings":[[[2,2],[2,3],[3,3],[3,2],[2,2]]]}}
]}`

const capturePostgresURL = "postgres://landman:landman@localhost:5432/postgres?sslmode=disable"

const captureSeedSQL = `INSERT INTO parcels (id, source_parcel_id, situs_line_1, likely_vacant) VALUES ('11111111-1111-1111-1111-111111111111'::uuid, '07102010', 'OLD SITUS', false);
INSERT INTO soil_map_units (map_unit_key, map_unit_symbol, map_unit_name, area_symbol, area_name, fetched_at) VALUES ('115330', 'ApB', 'Appling sandy loam', 'NC179', 'Union County, North Carolina', '2026-01-01 00:00:00+00'::timestamptz);
INSERT INTO soil_polygons (id, county_id, source_name, object_id, map_unit_key, spatial_version, survey_saved_on, shape, fetched_at) VALUES ('33333333-3333-3333-3333-333333333333'::uuid, 'union', 'union_county_gis', '1', '115330', 5, NULL, ST_GeomFromEWKT('SRID=3857;POLYGON((0 0, 0 1, 1 1, 1 0, 0 0))'), '2026-01-01 00:00:00+00'::timestamptz);
INSERT INTO soil_polygon_parcels (id, soil_polygon_id, parcel_id, overlap_acres, created_at, updated_at) VALUES ('44444444-4444-4444-4444-444444444444'::uuid, '33333333-3333-3333-3333-333333333333'::uuid, '11111111-1111-1111-1111-111111111111'::uuid, '0.25', '2026-01-01 00:00:00+00'::timestamptz, '2026-01-01 00:00:00+00'::timestamptz);
INSERT INTO parcel_sales (parcel_id, sequence, sale_date, og_sale_date, amount, deed_book, deed_page, deed_type, deed_quality, grantor, grantor_2) SELECT id, 1, DATE '2020-01-01', 1, 1, '1', '1', NULL, NULL, NULL, NULL FROM parcels WHERE county_id = 'union' AND source_name = 'union_county_gis' AND source_parcel_id = '07102010';
`

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

type captureURLError string

func (e captureURLError) Error() string { return string(e) }

func errCaptureURL(rawURL string) error { return captureURLError(rawURL) }
