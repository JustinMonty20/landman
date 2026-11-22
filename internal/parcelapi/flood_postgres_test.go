//go:build postgres

package parcelapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFloodMapAndParcelEvidence(t *testing.T) {
	dsn := os.Getenv("FEMA_API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("FEMA_API_TEST_DATABASE_URL not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "landman_api_test_fema" || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port == 5432 || len(cfg.ConnConfig.Fallbacks) != 0 {
		t.Fatal("use the disposable FEMA API database")
	}
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const parcel = "a0000000-0000-4000-8000-000000000001"
	const release = "a0000000-0000-4000-8000-000000000002"
	seed := []string{
		`INSERT INTO parcels(id,county_id,county_code,source_name,source_parcel_id,shape) VALUES ('` + parcel + `','union','union','union_county_gis','fixture',ST_Transform(ST_MakeEnvelope(-80.6,34.99,-80.59,35,4269),3857))`,
		`INSERT INTO ingest_captures(id,source_name,observed_at,manifest,manifest_sha256,storage_mode) VALUES ('` + release + `','fema_union','2026-09-15','',decode(repeat('00',32),'hex'),'local')`,
		`INSERT INTO fema_releases(capture_id,study_id,product_id,issued_at,catalog_effective_at,archive_sha256,source_srid,observed_at) VALUES ('` + release + `','37179C',1,'2026-09-14','2018-11-16','fixture',4269,'2026-09-15')`,
		`INSERT INTO fema_zones SELECT '` + release + `',id,'AE',subtype,'T',ST_Multi(ST_MakeEnvelope(-80.6,34.989,east,35.001,4269)),false,0 FROM (VALUES ('ae', '', -80.594),('ae_duplicate', '', -80.594),('floodway','FLOODWAY',-80.598)) x(id,subtype,east)`,
		`INSERT INTO fema_panels VALUES ('` + release + `','panel','3710542500J','2008-10-16',ST_Multi(ST_MakeEnvelope(-80.61,34.98,-80.592,35.01,4269)),false,0)`,
		`INSERT INTO fema_coverage SELECT '` + release + `', p.shape, z.shape, ST_Multi(ST_Difference(p.shape,z.shape)) FROM fema_panels p CROSS JOIN fema_zones z WHERE z.source_id='ae'`,
		`INSERT INTO fema_release_head VALUES ('37179C','` + release + `','` + release + `','2026-09-15')`,
	}
	for _, stmt := range seed {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	h := New(pool)
	request := func(path string, out any) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
	var layer struct {
		Overview bool
		Release  floodRelease
		Features []floodFeature
	}
	request("/api/v1/flood/map?bbox=-80.61,34.98,-80.58,35.01&zoom=15", &layer)
	if layer.Release.ID != release || len(layer.Features) != 3 {
		t.Fatalf("unexpected map release or features: %s %d", layer.Release.ID, len(layer.Features))
	}
	var detail struct {
		Status  string
		Release floodRelease
		Acres   struct{ Parcel, Unknown, Floodway float64 }
		Zones   []floodZone
		Panels  []struct {
			Number    string
			Effective string `json:"effective_date"`
		}
	}
	request("/api/v1/parcels/"+parcel+"/flood", &detail)
	if detail.Status != "available" || detail.Release.ID != release || len(detail.Zones) != 2 || len(detail.Panels) != 1 || detail.Panels[0].Effective != "2008-10-16" {
		t.Fatalf("unexpected parcel evidence: %+v", detail)
	}
	a := detail.Acres
	if !(a.Unknown/a.Parcel > .39 && a.Unknown/a.Parcel < .41 && a.Floodway/a.Parcel > .19 && a.Floodway/a.Parcel < .21) {
		t.Fatalf("incorrect coverage: %+v", a)
	}
	if ratio := detail.Zones[0].Acres / a.Parcel; ratio < .59 || ratio > .61 {
		t.Fatalf("duplicate zone counted or wrong area: %f", ratio)
	}
	// Both Zone X subtypes remain distinct; boundary-only contact requires opt-in.
	for _, stmt := range []string{
		`INSERT INTO parcels(id,county_id,county_code,source_name,source_parcel_id,shape) VALUES ('a0000000-0000-4000-8000-000000000003','union','union','union_county_gis','neighbor',(SELECT ST_Translate(shape,ST_XMax(shape)-ST_XMin(shape),0) FROM parcels WHERE source_parcel_id='fixture'))`,
		`INSERT INTO fema_zones SELECT '` + release + `','touch','AE','','T',ST_Multi(ST_Transform(shape,4269)),false,0 FROM parcels WHERE source_parcel_id='fixture'`,
		`INSERT INTO fema_zones VALUES ('` + release + `','moderate','X','0.2 PCT ANNUAL CHANCE FLOOD HAZARD','F',ST_Multi(ST_MakeEnvelope(-80.585,34.991,-80.583,34.999,4269)),false,0)`,
		`INSERT INTO fema_zones VALUES ('` + release + `','minimal','X','AREA OF MINIMAL FLOOD HAZARD','F',ST_Multi(ST_MakeEnvelope(-80.583,34.991,-80.581,34.999,4269)),false,0)`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	var list struct {
		Count       int
		Items       []Parcel
		SourceStamp string `json:"source_stamp"`
	}
	var mapResult struct {
		MatchCount  int64  `json:"match_count"`
		SourceStamp string `json:"source_stamp"`
		Features    []Feature
	}
	for _, check := range []struct {
		category string
		count    int
	}{{"floodway", 1}, {"sfha", 1}, {"moderate", 1}, {"minimal", 1}, {"unclassified", 0}} {
		path := "/api/v1/parcels?flood_category=" + check.category
		request(path, &list)
		if list.Count != check.count || len(list.Items) != check.count {
			t.Fatalf("%s list: %+v", check.category, list)
		}
		request("/api/v1/parcels/map?bbox=-80.61,34.98,-80.575,35.01&zoom=15&flood_category="+check.category, &mapResult)
		if mapResult.MatchCount != int64(check.count) || len(mapResult.Features) != check.count || mapResult.SourceStamp != list.SourceStamp {
			t.Fatalf("%s map/list disagree: %+v %+v", check.category, mapResult, list)
		}
		var grid struct {
			MatchCount  int64  `json:"match_count"`
			SourceStamp string `json:"source_stamp"`
		}
		request("/api/v1/parcels/map?bbox=-80.61,34.98,-80.575,35.01&zoom=10&flood_category="+check.category, &grid)
		if grid.MatchCount != int64(check.count) || grid.SourceStamp != list.SourceStamp {
			t.Fatalf("%s county grid/list disagree: %d %d", check.category, grid.MatchCount, list.Count)
		}
		request("/api/v1/flood/map?bbox=-80.61,34.98,-80.575,35.01&zoom=15&flood_category="+check.category, &layer)
		for _, feature := range layer.Features {
			if feature.Properties.Category != check.category {
				t.Fatalf("wrong category: %+v", feature.Properties)
			}
		}
	}
	request("/api/v1/parcels?flood_category=sfha&flood_contact=intersects", &list)
	if list.Count != 2 {
		t.Fatalf("boundary contact missing: %+v", list)
	}
	request("/api/v1/parcels/map?bbox=-80.61,34.98,-80.575,35.01&zoom=15&flood_category=sfha&flood_contact=intersects", &mapResult)
	if mapResult.MatchCount != 2 || len(mapResult.Features) != 2 {
		t.Fatalf("boundary map mismatch: %+v", mapResult)
	}
	for _, path := range []string{"/api/v1/parcels?flood_category=bogus", "/api/v1/parcels?flood_contact=area", "/api/v1/parcels/map?bbox=-80.61,34.98,-80.575,35.01&zoom=15&flood_category=sfha&flood_contact=bad", "/api/v1/flood/map?overview=true&flood_category=bogus"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fema_zones SELECT $1,'000-'||seq,'AE','','T',
 ST_Multi(ST_MakeEnvelope(CASE WHEN seq=1 THEN ST_XMin(viewport)-0.01 ELSE -80.6 END,34.99,
 CASE WHEN seq=1 THEN ST_XMin(viewport) ELSE -80.59 END,35,4269)),false,0
 FROM generate_series(1,501) seq CROSS JOIN
 (SELECT ST_Transform(ST_MakeEnvelope(-80.61,34.98,-80.58,35.01,4326),4269) AS viewport) b`, release); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/flood/map?bbox=-80.61,34.98,-80.58,35.01&zoom=15", nil))
	if w.Code != 413 {
		t.Fatalf("over-limit flood request: %d", w.Code)
	}
	request("/api/v1/flood/map?overview=true", &layer)
	if !layer.Overview || layer.Release.ID != release || len(layer.Features) != 4 || layer.Features[3].Properties.Subtype != "FLOODWAY" {
		t.Fatalf("unexpected flood overview: %d features", len(layer.Features))
	}
	if _, err := pool.Exec(ctx, `DELETE FROM fema_release_head WHERE study_id='37179C'`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/parcels?flood_category=sfha", "/api/v1/parcels/map?bbox=-80.61,34.98,-80.575,35.01&zoom=15&flood_category=sfha"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 503 {
			t.Fatalf("missing release must be unavailable: %s %d", path, w.Code)
		}
	}
	var missing struct{ Status string }
	request("/api/v1/parcels/"+parcel+"/flood", &missing)
	if !strings.Contains(missing.Status, "no_release") {
		t.Fatalf("expected unavailable release, got %q", missing.Status)
	}
}
