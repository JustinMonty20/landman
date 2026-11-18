//go:build postgres

package soiljob

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSoilJobCapturedGeometryRepeatAndReplay(t *testing.T) {
	dsn := os.Getenv("SOIL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable PostGIS URL not set")
	}
	if !strings.Contains(dsn, "/soil_test?") {
		t.Fatal("soil integration requires disposable soil_test database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	fixture, err := os.ReadFile(filepath.Join("..", "testdata", "soil", "layer197-feature.json"))
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("returnCountOnly") == "true" {
			_, _ = w.Write([]byte(`{"count":1}`))
			return
		}
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/parcels", SoilQueryURL: server.URL + "/soil", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "retained"))
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Pool: pool, Source: source, Artifacts: store}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	run := func() {
		t.Helper()
		outcome, result, err := runner.Run(ctx, jobs.Definition{ID: "union.gis.soil", Run: job.Import})
		if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != 1 {
			t.Fatalf("soil job result=%+v outcome=%s error=%v", result, outcome, err)
		}
	}
	read := func(table string) int {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	run()
	if read("soil_polygons") != 1 || read("soil_polygon_sets") != 1 || read("soil_ingest_publications") != 1 {
		t.Fatal("incomplete first soil publication")
	}
	var firstSet, firstFetch string
	if err := pool.QueryRow(ctx, `SELECT h.set_id::text,p.fetched_at::text FROM soil_polygon_set_heads h JOIN soil_polygons p ON p.county_id=h.county_id AND p.source_name=h.source_name WHERE h.county_id='union'`).Scan(&firstSet, &firstFetch); err != nil {
		t.Fatal(err)
	}
	run()
	var secondSet, secondFetch string
	if err := pool.QueryRow(ctx, `SELECT h.set_id::text,p.fetched_at::text FROM soil_polygon_set_heads h JOIN soil_polygons p ON p.county_id=h.county_id AND p.source_name=h.source_name WHERE h.county_id='union'`).Scan(&secondSet, &secondFetch); err != nil {
		t.Fatal(err)
	}
	if read("soil_polygon_sets") != 1 || firstSet != secondSet || firstFetch != secondFetch || read("soil_ingest_publications") != 2 {
		t.Fatal("identical geometry rewrote soil business tables")
	}
	var captureID string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM soil_ingest_publications ORDER BY observed_at LIMIT 1`).Scan(&captureID); err != nil {
		t.Fatal(err)
	}
	before := requests
	if err := job.Replay(ctx, captureID); err != nil {
		t.Fatal(err)
	}
	if requests != before || read("soil_polygon_sets") != 1 {
		t.Fatal("replay fetched live pages or wrote a new soil set")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) SELECT 'union','179','union_county_gis','soil-fixture',shape FROM soil_polygons WHERE object_id='1'`); err != nil {
		t.Fatal(err)
	}
	rating := "Very limited"
	requestsBefore := 0
	client := &http.Client{Transport: testUSDATransport(func(req *http.Request) (*http.Response, error) {
		requestsBefore++
		if req.URL.Host != "sdmdataaccess.sc.egov.usda.gov" || req.Method != "POST" {
			return nil, fmt.Errorf("unexpected USDA request %s", req.URL)
		}
		if err := req.ParseForm(); err != nil {
			return nil, err
		}
		query := req.Form.Get("query")
		var data []byte
		if strings.Contains(query, "FROM mapunit AS m JOIN legend") {
			data = []byte(`{"Table":[["115341","ChA","Chewacla silt loam","NC179","Union County","9/3/2026 1:43:46 PM"]]}`)
		} else {
			name := soilFixtureFile(query)
			if name == "" {
				return nil, fmt.Errorf("unrecognized USDA query %s", query)
			}
			raw, e := os.ReadFile(filepath.Join("..", "..", "..", "soil", "testdata", name))
			if e != nil {
				return nil, e
			}
			var payload struct {
				Table [][]any `json:"Table"`
			}
			if e = json.Unmarshal(raw, &payload); e != nil {
				return nil, e
			}
			if !strings.Contains(query, "FROM sdvattribute") {
				filtered := [][]any{}
				for _, row := range payload.Table {
					if row[0] == "115341" {
						if strings.Contains(query, "JOIN muaggatt") {
							row[1] = rating
						}
						filtered = append(filtered, row)
					}
				}
				payload.Table = filtered
			}
			data, e = json.Marshal(payload)
			if e != nil {
				return nil, e
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}, Request: req}, nil
	})}
	screen := ScreeningJob{Pool: pool, Artifacts: store, Client: client, Interval: time.Hour}
	screenImport := func() {
		t.Helper()
		result, e := screen.Import(ctx)
		if e != nil || result.Processed != 1 {
			t.Fatalf("screen result=%+v error=%v", result, e)
		}
	}
	screenImport()
	if read("soil_screening_runs") != 1 || read("soil_screening_ratings") != 3 || read("soil_polygon_parcels") != 1 {
		t.Fatal("screening did not publish complete derived generation")
	}
	var overlapVersion, interpretationVersion string
	if e := pool.QueryRow(ctx, `SELECT xmin::text FROM soil_polygon_parcels`).Scan(&overlapVersion); e != nil {
		t.Fatal(e)
	}
	if e := pool.QueryRow(ctx, `SELECT string_agg(xmin::text, ',' ORDER BY id) FROM soil_interpretation_keys`).Scan(&interpretationVersion); e != nil {
		t.Fatal(e)
	}
	screenImport()
	if read("soil_screening_runs") != 1 {
		t.Fatal("identical USDA responses rewrote screening head")
	}
	rating = "Somewhat limited"
	screenImport()
	if read("soil_screening_runs") != 2 {
		t.Fatal("changed USDA rating did not publish generation")
	}
	var overlapVersionAfter string
	if e := pool.QueryRow(ctx, `SELECT xmin::text FROM soil_polygon_parcels`).Scan(&overlapVersionAfter); e != nil {
		t.Fatal(e)
	}
	if overlapVersionAfter != overlapVersion {
		t.Fatal("rating-only change recomputed stored overlaps")
	}
	var interpretationVersionAfter string
	if e := pool.QueryRow(ctx, `SELECT string_agg(xmin::text, ',' ORDER BY id) FROM soil_interpretation_keys`).Scan(&interpretationVersionAfter); e != nil {
		t.Fatal(e)
	}
	if interpretationVersionAfter != interpretationVersion {
		t.Fatal("rating-only change rewrote interpretation identity rows")
	}
	var screenCapture string
	if e := pool.QueryRow(ctx, `SELECT capture_id::text FROM soil_ingest_publications WHERE source_name=$1 ORDER BY observed_at LIMIT 1`, interpretationSource).Scan(&screenCapture); e != nil {
		t.Fatal(e)
	}
	noRequests := requestsBefore
	if e := screen.Replay(ctx, screenCapture); e != nil {
		t.Fatal(e)
	}
	if requestsBefore != noRequests || read("soil_screening_runs") != 2 {
		t.Fatal("USDA replay issued request or republished historical run")
	}
	// Seal a complete historical USDA capture before another county geometry generation takes the head.
	var manifestData []byte
	if e := pool.QueryRow(ctx, `SELECT manifest FROM ingest_captures WHERE id=$1::uuid`, screenCapture).Scan(&manifestData); e != nil {
		t.Fatal(e)
	}
	var manifest ingest.Manifest
	if e := json.Unmarshal(manifestData, &manifest); e != nil {
		t.Fatal(e)
	}
	recorder, e := ingest.NewRecorder(store, interpretationSource, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	for _, receipt := range manifest.Receipts {
		rawHash, e := hex.DecodeString(receipt.SHA256)
		if e != nil {
			t.Fatal(e)
		}
		var hash [32]byte
		copy(hash[:], rawHash)
		reference, e := taxartifact.Reference(receipt.ArtifactKey, receipt.Size, hash)
		if e != nil {
			t.Fatal(e)
		}
		reference, e = taxartifact.VerifiedReference(ctx, store, reference)
		if e != nil {
			t.Fatal(e)
		}
		reader, e := store.Get(ctx, reference)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := io.ReadAll(reader)
		closeErr := reader.Close()
		if e != nil || closeErr != nil {
			t.Fatal(e, closeErr)
		}
		if e = recorder.Record(ctx, receipt.Request, receipt.Revision, receipt.CRS, bytes.NewReader(raw)); e != nil {
			t.Fatal(e)
		}
	}
	pendingID, e := (ingest.Engine{Pool: pool, Store: store}).Seal(ctx, recorder)
	if e != nil {
		t.Fatal(e)
	}
	fixture = []byte(strings.Replace(string(fixture), `"SPATIALVER":5.0`, `"SPATIALVER":6.0`, 1))
	run()
	if read("soil_polygon_sets") != 2 {
		t.Fatal("changed geometry did not publish new set")
	}
	previousHead := ""
	if e := pool.QueryRow(ctx, `SELECT run_id::text FROM soil_screening_heads WHERE county_id='union' AND source_name=$1`, polygonSource).Scan(&previousHead); e != nil {
		t.Fatal(e)
	}
	noRequests = requestsBefore
	if e := screen.Replay(ctx, pendingID); e == nil {
		t.Fatal("replay published against replaced soil set")
	}
	if requestsBefore != noRequests {
		t.Fatal("replay requested USDA")
	}
	var currentHead string
	if e := pool.QueryRow(ctx, `SELECT run_id::text FROM soil_screening_heads WHERE county_id='union' AND source_name=$1`, polygonSource).Scan(&currentHead); e != nil {
		t.Fatal(e)
	}
	if currentHead != previousHead || read("soil_screening_runs") != 2 {
		t.Fatal("failed publication changed screening generation")
	}
	failedScreen := screen
	failedScreen.Client = &http.Client{Transport: testUSDATransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("required USDA response unavailable")
	})}
	if _, e := failedScreen.Import(ctx); e == nil {
		t.Fatal("unavailable USDA response accepted")
	}
	if e := pool.QueryRow(ctx, `SELECT run_id::text FROM soil_screening_heads WHERE county_id='union' AND source_name=$1`, polygonSource).Scan(&currentHead); e != nil {
		t.Fatal(e)
	}
	if currentHead != previousHead || read("soil_screening_runs") != 2 || read("soil_polygon_parcels") != 1 {
		t.Fatal("USDA failure changed published screening or overlaps")
	}
	if read("soil_screening_refresh_status") != 1 {
		t.Fatal("USDA failure not recorded as stale")
	}
	var partial int
	if e := pool.QueryRow(ctx, `SELECT count(*) FROM ingest_captures WHERE source_name=$1 AND published_at IS NULL`, interpretationSource).Scan(&partial); e != nil || partial != 2 {
		t.Fatalf("missing registered failed capture: count=%d error=%v", partial, e)
	}
	var artifactCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ingest_captures WHERE source_name=$1 AND published_at IS NOT NULL`, captureSource).Scan(&artifactCount); err != nil || artifactCount != 3 {
		t.Fatalf("missing source observations %d %v", artifactCount, err)
	}
}

type testUSDATransport func(*http.Request) (*http.Response, error)

func (fn testUSDATransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
func soilFixtureFile(q string) string {
	switch {
	case strings.Contains(q, "FROM sdvattribute"):
		return "union-rules-raw.json"
	case strings.Contains(q, "JOIN muaggatt"):
		return "union-muaggatt-raw.json"
	case strings.Contains(q, "JOIN sacatalog"):
		return "union-catalog-raw.json"
	case strings.Contains(q, "FROM component c WHERE"):
		return "union-ancillary-components-raw.json"
	case strings.Contains(q, "JOIN cointerp"):
		return "union-components-raw.json"
	case strings.Contains(q, "JOIN cosoilmoist"):
		return "union-moisture-raw.json"
	case strings.Contains(q, "JOIN comonth"):
		return "union-monthly-raw.json"
	case strings.Contains(q, "JOIN chorizon"):
		return "union-horizons-raw.json"
	case strings.Contains(q, "JOIN corestrictions"):
		return "union-restrictions-raw.json"
	}
	return ""
}
