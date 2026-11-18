//go:build postgres

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewMultipartClearsOmittedValuesAndNumericZeroVacancy(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	a, b := feature(1, "A", "first", true, 1), feature(2, "A", "", true, 0)
	delete(b["attributes"].(map[string]interface{}), "CURR_NAME1")
	f := &parcelHTTPFixture{features: []map[string]interface{}{a, b}}
	source := f.source(t, 1)
	if _, _, e := runParcelFixture(ctx, pool, dsn, source, false, 0); e != nil {
		t.Fatal(e)
	}
	var owner string
	var sales int
	var vacant bool
	if e := pool.QueryRow(ctx, `SELECT coalesce(current_name_1,'NULL'),(SELECT count(*) FROM parcel_sales),likely_vacant FROM parcels`).Scan(&owner, &sales, &vacant); e != nil {
		t.Fatal(e)
	}
	if owner != "NULL" || sales != 0 || !vacant {
		t.Errorf("owner=%s sales=%d likely_vacant=%t; want NULL/0/true", owner, sales, vacant)
	}
}
func TestReviewUnsignedCaptureCannotReplacePopulatedStore(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "live", true, 1), feature(2, "B", "same", true, 0)}}
	source := f.source(t, 1)
	if _, _, e := runParcelFixture(ctx, pool, dsn, source, false, 0); e != nil {
		t.Fatal(e)
	}
	f.features[0] = feature(1, "A", "unpublished", true, 1)
	f.features[1]["attributes"].(map[string]interface{})["SOURCE_NOTE"] = "new"
	root := os.Getenv("PARCEL_EVIDENCE_DIR")
	s, e := parcelevidence.New(root, "full_scan")
	if e != nil {
		t.Fatal(e)
	}
	n, e := source.CaptureParcelPages(ctx, s.Save, func([]connector.RawRecord) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Complete(n); e != nil {
		t.Fatal(e)
	}
	runner, _ := jobs.NewRunner(pool)
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			_, _, _ = runner.Run(ctx, jobs.Definition{ID: parcelJobID, Run: func(context.Context) (jobs.Result, error) {
				return jobs.Result{}, errors.New("unrelated source failure")
			}})
		}
		out, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
			return (parceljob.Job{DSN: dsn, Root: root}).Replay(ctx, s.Manifest.ID)
		}})
		if err == nil || out != jobs.OutcomeFailed {
			t.Fatalf("unsigned populated replay after unrelated failure = %s, %v", out, err)
		}
	}
	var owner, sourceNote string
	var published int
	if e := pool.QueryRow(ctx, `SELECT p.current_name_1,coalesce(c.source_features#>>'{0,attributes,SOURCE_NOTE}',''),
		(SELECT count(*) FROM parcel_evidence_observations WHERE artifact_id=$1)
		FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) WHERE p.source_parcel_id='A'`, s.Manifest.ID).Scan(&owner, &sourceNote, &published); e != nil {
		t.Fatal(e)
	}
	if owner != "live" || sourceNote != "" || published != 0 {
		t.Fatalf("unsigned capture changed serving or source links: owner=%s note=%s observations=%d", owner, sourceNote, published)
	}
}
func TestOwnerOnly(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "first", true, 1)}}
	source := f.source(t, 1)
	var before, after string
	for i := 0; i < 2; i++ {
		if _, _, e := runParcelFixture(ctx, pool, dsn, source, false, 0); e != nil {
			t.Fatal(e)
		}
		var ts string
		if e := pool.QueryRow(ctx, `SELECT updated_at::text FROM parcel_sales`).Scan(&ts); e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			before = ts
		} else {
			after = ts
		}
		f.features[0] = feature(1, "A", "second", true, 1)
	}
	if before != after {
		t.Errorf("unchanged sale timestamp %s -> %s", before, after)
	}
}
func TestOfflineEmptyCLI(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "offline", true, 0)}}
	source := f.source(t, 1)
	root := os.Getenv("PARCEL_EVIDENCE_DIR")
	s, e := parcelevidence.New(root, "full_scan")
	if e != nil {
		t.Fatal(e)
	}
	n, e := source.CaptureParcelPages(ctx, s.Save, func([]connector.RawRecord) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Complete(n); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"soil_polygon_set_heads", "soil_screening_heads", "soil_screening_runs", "soil_screening_refresh_status", "soil_parcel_assessments"} {
		if _, e := pool.Exec(ctx, "CREATE TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("DATABASE_URL", dsn)
	args := os.Args
	defer func() { os.Args = args }()
	os.Args = []string{"landman", "replay", parcelJobID, "--snapshot", s.Manifest.ID}
	if err := run(ctx); err != nil {
		t.Errorf("first replay on empty db: %v", err)
	}
	if err := run(ctx); err != nil {
		t.Error(err)
	}
	var pending, qualify int
	if e := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE outcome='pending'),count(*) FILTER(WHERE job_id='union.qualification.assess') FROM job_attempts`).Scan(&pending, &qualify); e != nil {
		t.Fatal(e)
	}
	if pending != 0 || qualify != 0 {
		t.Fatalf("offline replay launched downstream jobs: pending=%d qualification=%d", pending, qualify)
	}
	var due bool
	if e := pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM soil_screening_heads WHERE county_id='union' AND source_name='union_county_gis')`).Scan(&due); e != nil || !due {
		t.Fatalf("missing stored enrichment was not left due: %v %v", due, e)
	}
	os.Args = []string{"landman", "run", screeningJobID}
	if e := run(ctx); e == nil || !errors.Is(e, jobs.ErrPending) {
		t.Fatalf("independent screening job did not retain missing-input pending state: %v", e)
	}
	if e := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE outcome='pending') FROM job_attempts WHERE job_id=$1`, screeningJobID).Scan(&pending); e != nil || pending != 1 {
		t.Fatalf("missing input pending attempt=%d err=%v", pending, e)
	}
}
func TestMixed(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	a, b := feature(1, "A", "correct", true, 0), feature(2, "B", "old", true, 0)
	f := &parcelHTTPFixture{features: []map[string]interface{}{a, b}}
	source := f.source(t, 1)
	if _, _, e := runParcelFixture(ctx, pool, dsn, source, false, 0); e != nil {
		t.Fatal(e)
	}
	var id string
	if e := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations`).Scan(&id); e != nil {
		t.Fatal(e)
	}
	f.features = []map[string]interface{}{feature(2, "B", "new", true, 0)}
	if _, _, e := runParcelFixture(ctx, pool, dsn, source, false, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `UPDATE parcels SET current_name_1='old projection' WHERE source_parcel_id='A'; UPDATE parcel_evidence_current SET normalizer_version='previous',normalized_hash=repeat('0',64) WHERE source_parcel_id='A'`); e != nil {
		t.Fatal(e)
	}
	runner, _ := jobs.NewRunner(pool)
	out, r, e := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return (parceljob.Job{DSN: dsn, Root: os.Getenv("PARCEL_EVIDENCE_DIR")}).Replay(ctx, id)
	}})
	if e != nil {
		t.Fatal(e)
	}
	var owner string
	if e := pool.QueryRow(ctx, `SELECT current_name_1 FROM parcels WHERE source_parcel_id='A'`).Scan(&owner); e != nil {
		t.Fatal(e)
	}
	var newerOwner string
	if e := pool.QueryRow(ctx, `SELECT current_name_1 FROM parcels WHERE source_parcel_id='B'`).Scan(&newerOwner); e != nil {
		t.Fatal(e)
	}
	if owner != "correct" || newerOwner != "new" || out != jobs.OutcomeSuccess || r.Processed != 1 {
		t.Fatalf("mixed replay corrected eligible PID but changed newer PID: outcome=%s processed=%d A=%s B=%s", out, r.Processed, owner, newerOwner)
	}
}
func TestExactScale(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	f.features[0]["attributes"].(map[string]interface{})["TOTVAL"] = json.Number("12.3449999999999999999")
	if _, _, e := runParcelFixture(ctx, pool, dsn, f.source(t, 1), false, 0); e != nil {
		t.Fatal(e)
	}
	var raw, rounded, stored string
	if e := pool.QueryRow(ctx, `SELECT c.source_features#>>'{0,attributes,TOTVAL}',((c.source_features#>>'{0,attributes,TOTVAL}')::numeric(14,2))::text,p.valuation_assessed::text FROM parcel_evidence_current c JOIN parcels p USING(source_parcel_id)`).Scan(&raw, &rounded, &stored); e != nil {
		t.Fatal(e)
	}
	var history string
	if e := pool.QueryRow(ctx, `SELECT new_fields->>'valuation_assessed' FROM parcel_evidence_history ORDER BY id DESC LIMIT 1`).Scan(&history); e != nil {
		t.Fatal(e)
	}
	if raw != "12.3449999999999999999" || rounded != "12.34" || stored != "12.34" || history != "12.34" {
		t.Fatalf("source=%s exact=%s serving=%s history=%s", raw, rounded, stored, history)
	}
	var hash, historyHash, updated string
	if e := pool.QueryRow(ctx, `SELECT c.normalized_hash,p.updated_at::text,(SELECT new_normalized_hash FROM parcel_evidence_history ORDER BY id DESC LIMIT 1) FROM parcel_evidence_current c JOIN parcels p USING(source_parcel_id)`).Scan(&hash, &updated, &historyHash); e != nil {
		t.Fatal(e)
	}
	f.features[0]["attributes"].(map[string]interface{})["TOTVAL"] = json.Number("12.34")
	if _, _, e := runParcelFixture(ctx, pool, dsn, f.source(t, 1), false, 0); e != nil {
		t.Fatal(e)
	}
	var currentHash, currentUpdated, currentValue string
	if e := pool.QueryRow(ctx, `SELECT c.normalized_hash,p.updated_at::text,p.valuation_assessed::text FROM parcel_evidence_current c JOIN parcels p USING(source_parcel_id)`).Scan(&currentHash, &currentUpdated, &currentValue); e != nil {
		t.Fatal(e)
	}
	if hash != historyHash || hash != currentHash || updated != currentUpdated || currentValue != stored || countRows(t, ctx, pool, "parcel_evidence_history") != 1 {
		t.Fatalf("quantized storage/history/hash disagree or rewrote unchanged content: original=%s history=%s current=%s value=%s", hash, historyHash, currentHash, currentValue)
	}
}
func TestLiveCause(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	source := f.source(t, 1)
	if _, _, e := runParcelFixture(ctx, pool, dsn, source, false, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `UPDATE parcels SET current_name_1='old projection';UPDATE parcel_evidence_current SET normalizer_version='previous',normalized_hash=repeat('0',64)`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := runParcelFixture(ctx, pool, dsn, source, false, 0); e != nil {
		t.Fatal(e)
	}
	var cause string
	var same bool
	if e := pool.QueryRow(ctx, `SELECT cause,old_source_hash=new_source_hash FROM parcel_evidence_history ORDER BY id DESC LIMIT 1`).Scan(&cause, &same); e != nil {
		t.Fatal(e)
	}
	if cause != "normalization_correction" {
		t.Errorf("old projection -> owner: identical source=%v, cause=%s", same, cause)
	}
}
func TestSizeAtLimit(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	bad := feature(1, "A", "a", false, 0)
	attrs := bad["attributes"].(map[string]interface{})
	attrs["note"] = ""
	bad["source_declared_crs"] = 3857
	data, _ := json.Marshal(bad)
	attrs["note"] = strings.Repeat("x", 65535-len(data))
	delete(bad, "source_declared_crs")
	f := &parcelHTTPFixture{features: []map[string]interface{}{bad, feature(2, "B", "b", true, 0)}}
	out, r, e := runParcelFixture(ctx, pool, dsn, f.source(t, 2), false, 0)
	if e != nil || out != jobs.OutcomeSuccessWithQuarantine || r.Processed != 1 || r.Quarantined != 1 {
		t.Fatalf("bounded invalid payload blocked independent parcel: outcome=%s accepted=%d quarantined=%d err=%v", out, r.Processed, r.Quarantined, e)
	}
	var payloadBytes int
	var reference string
	if e := pool.QueryRow(ctx, `SELECT octet_length(payload::text),payload_reference FROM quarantined_records`).Scan(&payloadBytes, &reference); e != nil || payloadBytes > 8*1024 || !strings.Contains(reference, "@") {
		t.Fatalf("quarantine payload=%d reference=%q err=%v", payloadBytes, reference, e)
	}
}
func TestLocalCRS(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	a := feature(1, "A", "owner", true, 0)
	a["geometry"].(map[string]interface{})["spatialReference"] = map[string]interface{}{"wkt": `GEOGCS["WGS 84",DATUM["WGS_1984",SPHEROID["WGS 84",6378137,298.257223563]],PRIMEM["Greenwich",0],UNIT["degree",0.0174532925199433]]`}
	f := &parcelHTTPFixture{features: []map[string]interface{}{a}}
	out, _, e := runParcelFixture(ctx, pool, dsn, f.source(t, 1), false, 0)
	if e == nil {
		var srid int
		if e := pool.QueryRow(ctx, `SELECT source_srid FROM parcels`).Scan(&srid); e != nil {
			t.Fatal(e)
		}
		t.Errorf("local WGS84 WKT + page 3857: outcome=%s source_srid=%d", out, srid)
	}
}
func TestFailedPageQuarantine(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "bad", false, 0), feature(2, "B", "b", true, 0)}, failStatus: 500, failOffset: 1}
	out, _, e := runParcelFixture(ctx, pool, dsn, f.source(t, 1), false, 0)
	if e == nil {
		t.Fatal("fixture failure missing")
	}
	var n int
	if e := pool.QueryRow(ctx, `SELECT count(*) FROM quarantined_records`).Scan(&n); e != nil {
		t.Fatal(e)
	}
	var ref string
	if e := pool.QueryRow(ctx, `SELECT payload_reference FROM quarantined_records WHERE reprocessed_at IS NULL`).Scan(&ref); e != nil {
		t.Fatal(e)
	}
	if n != 1 || out != jobs.OutcomeFailed || countRows(t, ctx, pool, "parcels") != 0 || !strings.Contains(ref, "/000001.json@") || !strings.HasSuffix(ref, "#0") {
		t.Fatalf("failed later page lost exact retained evidence or published partial PID: outcome=%s pending=%d ref=%s", out, n, ref)
	}
	path, digestAndIndex, ok := strings.Cut(ref, "@")
	if !ok {
		t.Fatal("missing receipt digest")
	}
	data, e := os.ReadFile(filepath.Join(os.Getenv("PARCEL_EVIDENCE_DIR"), path))
	if e != nil {
		t.Fatal(e)
	}
	digest, _, ok := strings.Cut(digestAndIndex, "#")
	if !ok || digest != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal("quarantine reference does not identify exact retained response bytes")
	}
	var page struct {
		Features []struct {
			Attributes struct {
				ObjectID int `json:"OBJECTID"`
			} `json:"attributes"`
		} `json:"features"`
	}
	if e := json.Unmarshal(data, &page); e != nil || len(page.Features) != 1 || page.Features[0].Attributes.ObjectID != 1 {
		t.Fatalf("quarantine byte reference points to wrong feature: %v", e)
	}
}

type noDial func(*http.Request) (*http.Response, error)

func (f noDial) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestOfflineHTTP(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "current", true, 0)}}
	if _, _, e := runParcelFixture(ctx, pool, dsn, f.source(t, 1), false, 0); e != nil {
		t.Fatal(e)
	}
	var id string
	if e := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations`).Scan(&id); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `UPDATE parcels SET shape=ST_Translate(shape,500,0); UPDATE parcel_evidence_current SET normalizer_version='previous',geometry_hash=repeat('0',64),normalized_hash=repeat('1',64)`); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"soil_screening_heads", "soil_screening_runs", "soil_screening_refresh_status", "soil_parcel_assessments", "soil_polygon_set_heads", "soil_polygon_sets", "soil_polygon_set_members", "soil_polygons", "soil_map_units"} {
		if _, e := pool.Exec(ctx, "CREATE TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := pool.Exec(ctx, `INSERT INTO soil_polygon_sets(county_id,source_name,fingerprint) VALUES ('union','union_county_gis','a'); INSERT INTO soil_polygon_set_heads SELECT county_id,source_name,id FROM soil_polygon_sets; INSERT INTO soil_map_units(map_unit_key,map_unit_symbol,area_symbol) VALUES ('115328','A','NC179'); INSERT INTO soil_polygons(county_id,source_name,object_id,map_unit_key,shape,fetched_at) VALUES ('union','union_county_gis','p','115328',ST_GeomFromText('POLYGON((0 0,0 2,2 2,2 0,0 0))',3857),now()); INSERT INTO soil_polygon_set_members(set_id,polygon_id,map_unit_key,shape) SELECT s.id,p.id,p.map_unit_key,p.shape FROM soil_polygon_sets s,soil_polygons p`); e != nil {
		t.Fatal(e)
	}
	prev := http.DefaultClient
	defer func() { http.DefaultClient = prev }()
	var dest string
	http.DefaultClient = &http.Client{Transport: noDial(func(r *http.Request) (*http.Response, error) {
		dest = r.Method + " " + r.URL.Host
		return nil, errors.New("fixture: offline boundary")
	})}
	t.Setenv("DATABASE_URL", dsn)
	args := os.Args
	defer func() { os.Args = args }()
	os.Args = []string{"landman", "replay", parcelJobID, "--snapshot", id}
	err := run(ctx)
	if dest != "" || err != nil {
		t.Fatalf("offline replay attempted %s or failed: %v", dest, err)
	}
	var derived, corrected int
	if e := pool.QueryRow(ctx, `SELECT count(*) FROM job_attempts WHERE job_id IN ($1,$2)`, screeningJobID, qualificationJobID).Scan(&derived); e != nil {
		t.Fatal(e)
	}
	if e := pool.QueryRow(ctx, `SELECT count(*) FROM parcel_evidence_history WHERE cause='normalization_correction'`).Scan(&corrected); e != nil {
		t.Fatal(e)
	}
	if derived != 0 || corrected != 1 {
		t.Fatalf("offline correction or deferred derived work: derived=%d correction=%d", derived, corrected)
	}
}
