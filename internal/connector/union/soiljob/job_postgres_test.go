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
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSoilJobCapturedGeometryRepeatAndReplay(t *testing.T) {
	dsn := os.Getenv("SOIL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable PostGIS URL not set")
	}
	if !strings.Contains(dsn, "/soil_test_") {
		t.Fatal("soil integration requires disposable soil_test database")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
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
	var bound int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE c.source_name=$1 AND a.job_id='union.gis.soil' AND a.outcome='success'`, captureSource).Scan(&bound); err != nil || bound != 1 {
		t.Fatalf("county soil capture missing origin attempt: %d %v", bound, err)
	}
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
	if _, _, err := runner.Run(ctx, jobs.Definition{ID: "union.gis.soil", Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return jobs.Result{}, job.Replay(ctx, captureID)
	}}); err != nil {
		t.Fatal(err)
	}
	if read("ingest_replay_attempts") != 1 {
		t.Fatal("county soil replay missing separate attempt")
	}
	if requests != before || read("soil_polygon_sets") != 1 {
		t.Fatal("replay fetched live pages or wrote a new soil set")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) SELECT 'union','179','union_county_gis','soil-fixture',shape FROM soil_polygons WHERE object_id='1'`); err != nil {
		t.Fatal(err)
	}
	// Count attempted overlap insertions, including ON CONFLICT updates with no business write.
	if _, err := pool.Exec(ctx, `CREATE TABLE soil_test_intersection_attempts (n int); CREATE FUNCTION soil_test_count_intersection() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO soil_test_intersection_attempts VALUES (1); RETURN NEW; END $$; CREATE TRIGGER count_soil_intersection BEFORE INSERT ON soil_polygon_parcels FOR EACH ROW EXECUTE FUNCTION soil_test_count_intersection()`); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	features := document["features"].([]any)
	geometry := features[0].(map[string]any)["geometry"].(map[string]any)
	rings := geometry["rings"].([]any)
	ring := rings[0].([]any)
	rings[0] = append(append([]any{}, ring[1:len(ring)-1]...), ring[0], ring[1])
	rotatedFixture, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	fixture = rotatedFixture
	run()
	if read("soil_polygon_sets") != 1 || read("soil_test_intersection_attempts") != 0 {
		t.Fatal("rotated ring triggered a geometry generation or intersection")
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
		_, result, e := runner.Run(ctx, jobs.Definition{ID: "union.soil.screening", Run: screen.Import})
		if e != nil || result.Processed != 1 {
			t.Fatalf("screen result=%+v error=%v", result, e)
		}
	}
	screenImport()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE c.source_name=$1 AND a.job_id='union.soil.screening' AND a.outcome='success'`, interpretationSource).Scan(&bound); err != nil || bound != 1 {
		t.Fatalf("USDA capture missing origin attempt: %d %v", bound, err)
	}
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
	var parcelID string
	if e := pool.QueryRow(ctx, `SELECT id::text FROM parcels WHERE source_parcel_id='soil-fixture'`).Scan(&parcelID); e != nil {
		t.Fatal(e)
	}
	assessment, e := soil.CurrentAssessment(ctx, pool, county.Union, polygonSource, parcelID)
	if e != nil {
		t.Fatal(e)
	}
	for _, use := range assessment.Uses {
		for _, unit := range use.MapUnits {
			for _, polygon := range unit.Polygons {
				if polygon.SurveySavedOn == nil || polygon.SurveySavedOn.Format("2006-01-02") != "2026-09-03" {
					t.Fatalf("assessment lost saved USDA survey date: %+v", polygon)
				}
			}
		}
	}
	var dated, undated int
	if e := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE survey_saved_on='2026-09-03'),count(*) FILTER (WHERE survey_saved_on IS NULL) FROM soil_screening_survey_dates`).Scan(&dated, &undated); e != nil || dated != 2 || undated != 0 {
		t.Fatalf("saved USDA survey date missing from screening generations: %d %d %v", dated, undated, e)
	}
	var currentScreenCapture string
	if e := pool.QueryRow(ctx, `SELECT capture_id::text FROM soil_ingest_publications WHERE source_name=$1 ORDER BY observed_at DESC LIMIT 1`, interpretationSource).Scan(&currentScreenCapture); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `UPDATE soil_ingest_publications SET geometry_version='soil-set-v1' WHERE capture_id=$1::uuid`, currentScreenCapture); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `UPDATE soil_screening_runs SET assessment_method='old-method' WHERE id IN (SELECT run_id FROM soil_screening_heads)`); e != nil {
		t.Fatal(e)
	}
	if due, e := soil.NeedsScreenStored(ctx, pool, county.Union, polygonSource); e != nil || !due {
		t.Fatalf("old soil method not due for refresh: %t %v", due, e)
	}
	if e := screen.Replay(ctx, currentScreenCapture); e != nil {
		t.Fatal(e)
	}
	updated, e := soil.CurrentAssessment(ctx, pool, county.Union, polygonSource, parcelID)
	if e != nil || updated == nil || updated.MethodVersion != soil.AssessmentMethod || updated.Freshness.Stale {
		t.Fatalf("replay did not publish current soil method: %+v %v", updated, e)
	}
	if read("soil_screening_runs") != 3 || read("soil_screening_survey_dates") != 3 {
		t.Fatal("current USDA correction failed to publish revised screening and preserve captured survey date")
	}
	var correctionRevision int
	if e := pool.QueryRow(ctx, `SELECT max(revision) FROM soil_ingest_publications WHERE capture_id=$1::uuid`, currentScreenCapture).Scan(&correctionRevision); e != nil || correctionRevision != 2 {
		t.Fatalf("USDA correction revision=%d error=%v", correctionRevision, e)
	}
	if e := screen.Replay(ctx, currentScreenCapture); e != nil {
		t.Fatal(e)
	}
	if read("soil_screening_runs") != 3 || read("soil_ingest_publications") != 7 {
		t.Fatal("unchanged USDA replay republished correction")
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
	if requestsBefore != noRequests || read("soil_screening_runs") != 3 {
		t.Fatal("USDA replay issued request or republished historical run")
	}
	if _, e := pool.Exec(ctx, `UPDATE parcels SET shape=ST_Scale(shape,ST_MakePoint(0.5,0.5),ST_Centroid(shape)) WHERE id=$1`, parcelID); e != nil {
		t.Fatal(e)
	}
	if e := screen.Replay(ctx, currentScreenCapture); e != nil {
		t.Fatal(e)
	}
	moved, e := soil.CurrentAssessment(ctx, pool, county.Union, polygonSource, parcelID)
	if e != nil || moved == nil || moved.Freshness.Stale || moved.TotalAcres == nil || updated.TotalAcres == nil || *moved.TotalAcres >= *updated.TotalAcres || !moved.Freshness.FreshUntil.Equal(updated.Freshness.FreshUntil) {
		t.Fatalf("replay did not refresh changed parcel geometry at the original observation time: %+v %v", moved, e)
	}
	if e := screen.Replay(ctx, currentScreenCapture); e != nil || requestsBefore != noRequests || read("soil_screening_runs") != 4 {
		t.Fatalf("unchanged replay after parcel refresh issued a request or published again: %v", e)
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
	if _, e := pool.Exec(ctx, `TRUNCATE soil_test_intersection_attempts`); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `UPDATE soil_ingest_publications SET geometry_version='soil-ewkb-v1' WHERE source_name=$1 AND capture_id=(SELECT capture_id FROM soil_ingest_publications WHERE source_name=$1 ORDER BY observed_at DESC LIMIT 1)`, captureSource); e != nil {
		t.Fatal(e)
	}
	fixture = bytes.Replace(fixture, []byte(`"SPATIALVER":5`), []byte(`"SPATIALVER":6`), 1)
	run()
	if read("soil_polygon_sets") != 2 || read("soil_test_intersection_attempts") != 0 {
		t.Fatalf("metadata-only revision: sets=%d intersection attempts=%d fixture-version-six=%t", read("soil_polygon_sets"), read("soil_test_intersection_attempts"), bytes.Contains(fixture, []byte(`"SPATIALVER":6`)))
	}
	var savedVersion float64
	if e := pool.QueryRow(ctx, `SELECT spatial_version FROM soil_polygon_set_members WHERE set_id=(SELECT set_id FROM soil_polygon_set_heads WHERE county_id='union' AND source_name=$1)`, polygonSource).Scan(&savedVersion); e != nil || savedVersion != 6 {
		t.Fatalf("current set lost metadata revision: %v %v", savedVersion, e)
	}
	if e := pool.QueryRow(ctx, `SELECT spatial_version FROM soil_polygon_set_members WHERE set_id=$1::uuid`, firstSet).Scan(&savedVersion); e != nil || savedVersion != 5 {
		t.Fatalf("historical set metadata mutated: %v %v", savedVersion, e)
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
	if currentHead != previousHead || read("soil_screening_runs") != 4 {
		t.Fatal("failed publication changed screening generation")
	}
	failedScreen := screen
	failedScreen.Client = &http.Client{Transport: testUSDATransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("required USDA response unavailable")
	})}
	if _, _, e := runner.Run(ctx, jobs.Definition{ID: "union.soil.screening", Run: failedScreen.Import}); e == nil {
		t.Fatal("unavailable USDA response accepted")
	}
	var failedCapture int
	if e := pool.QueryRow(ctx, `SELECT count(*) FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE c.source_name=$1 AND c.published_at IS NULL AND a.job_id='union.soil.screening' AND a.outcome='failed' AND c.attempt_kind='full_scan'`, interpretationSource).Scan(&failedCapture); e != nil || failedCapture != 1 {
		t.Fatalf("partial USDA capture missing failed attempt: %d %v", failedCapture, e)
	}
	if e := pool.QueryRow(ctx, `SELECT run_id::text FROM soil_screening_heads WHERE county_id='union' AND source_name=$1`, polygonSource).Scan(&currentHead); e != nil {
		t.Fatal(e)
	}
	if currentHead != previousHead || read("soil_screening_runs") != 4 || read("soil_polygon_parcels") != 1 {
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
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ingest_captures WHERE source_name=$1 AND published_at IS NOT NULL`, captureSource).Scan(&artifactCount); err != nil || artifactCount != 4 {
		t.Fatalf("missing source observations %d %v", artifactCount, err)
	}
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	geometry = document["features"].([]any)[0].(map[string]any)["geometry"].(map[string]any)
	positiveRing := geometry["rings"].([]any)[0].([]any)
	coordinate := positiveRing[0].([]any)
	coordinate[0] = coordinate[0].(float64) + 4
	positiveRing[len(positiveRing)-1].([]any)[0] = coordinate[0]
	fixture, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	run()
	if read("soil_polygon_sets") != 3 || read("soil_test_intersection_attempts") == 0 {
		t.Fatal("changed geometry failed positive-control intersection probe")
	}
	var currentCountyCapture string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM soil_ingest_publications WHERE source_name=$1 ORDER BY observed_at DESC LIMIT 1`, captureSource).Scan(&currentCountyCapture); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_ingest_publications SET normalized_version='soil-metadata-v1' WHERE capture_id=$1::uuid`, currentCountyCapture); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE soil_test_intersection_attempts`); err != nil {
		t.Fatal(err)
	}
	if err := job.Replay(ctx, currentCountyCapture); err != nil {
		t.Fatal(err)
	}
	if read("soil_polygon_sets") != 4 || read("soil_test_intersection_attempts") != 0 {
		t.Fatal("county correction replay did not publish metadata-only generation")
	}
	if err := pool.QueryRow(ctx, `SELECT max(revision) FROM soil_ingest_publications WHERE capture_id=$1::uuid`, currentCountyCapture).Scan(&correctionRevision); err != nil || correctionRevision != 2 {
		t.Fatalf("county correction revision=%d error=%v", correctionRevision, err)
	}
	var correctedCause string
	if err := pool.QueryRow(ctx, `SELECT change_cause FROM soil_ingest_publications WHERE capture_id=$1::uuid AND revision=2`, currentCountyCapture).Scan(&correctedCause); err != nil || correctedCause != "normalization_correction" {
		t.Fatalf("county correction cause=%s error=%v", correctedCause, err)
	}
	if err := job.Replay(ctx, currentCountyCapture); err != nil {
		t.Fatal(err)
	}
	if read("soil_polygon_sets") != 4 {
		t.Fatal("unchanged county correction replay published again")
	}
	screenImport()
	assessment, err = soil.CurrentAssessment(ctx, pool, county.Union, polygonSource, parcelID)
	if err != nil {
		t.Fatal(err)
	}
	for _, use := range assessment.Uses {
		for _, unit := range use.MapUnits {
			for _, polygon := range unit.Polygons {
				if polygon.SurveySavedOn == nil || polygon.SurveySavedOn.Format("2006-01-02") != "2026-09-03" {
					t.Fatalf("new generation lost retained USDA survey date: %+v", polygon)
				}
			}
		}
	}
	fixture = bytes.Replace(fixture, []byte(`"SPATIALVER":6`), []byte(`"SPATIALVER":7`), 1)
	if !bytes.Contains(fixture, []byte(`"SPATIALVER":7`)) {
		t.Fatal("metadata revision fixture not found")
	}
	run()
	if _, err := pool.Exec(ctx, `TRUNCATE soil_test_intersection_attempts`); err != nil {
		t.Fatal(err)
	}
	screenImport()
	if read("soil_test_intersection_attempts") != 0 {
		t.Fatal("metadata-only county to screening refresh recomputed intersections")
	}
	assessment, err = soil.CurrentAssessment(ctx, pool, county.Union, polygonSource, parcelID)
	if err != nil || !assessment.Available {
		t.Fatalf("metadata-only screening assessment unavailable: %+v %v", assessment, err)
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
