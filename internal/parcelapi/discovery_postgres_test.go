//go:build postgres

package parcelapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This fixture occupies a separate viewport and uses unique source keys; only the
// two singleton soil publication heads are shared, and are restored on cleanup.
func TestDiscoveryPublishedFiltersPostGIS(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "landman_api_test_") {
		t.Fatal("refusing non-disposable database", err)
	}
	exec := func(q string, a ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, q, a...); e != nil {
			t.Fatalf("%s: %v", q, e)
		}
	}
	stamp := fmt.Sprintf("discovery-%d", time.Now().UnixNano())
	ids := make([]string, 8)
	for i := range ids {
		x := -79.3 + float64(i)*.003
		if err := db.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,mapped_acres,likely_vacant,shape) VALUES('union','union','union_county_gis',$1,$1,2,true,ST_Transform(ST_MakeEnvelope($2,34.3,$3,34.301,4326),3857)) RETURNING id::text`, fmt.Sprintf("%s-%d", stamp, i), x, x+.001).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	// Cleanup is registered before any remaining inserts, including when an assertion fails.
	var oldSet, activeSet, run, polygon string
	t.Cleanup(func() {
		c := context.Background()
		for _, q := range []string{
			`DELETE FROM soil_assessment_map_units WHERE run_id=$1`, `DELETE FROM soil_parcel_assessments WHERE run_id=$1`, `DELETE FROM soil_screening_ratings WHERE run_id=$1`,
			`DELETE FROM soil_screening_heads WHERE run_id=$1`, `DELETE FROM soil_screening_runs WHERE id=$1`,
		} {
			db.Exec(c, q, run)
		}
		db.Exec(c, `DELETE FROM soil_polygon_set_heads WHERE set_id=$1 OR set_id=$2`, oldSet, activeSet)
		db.Exec(c, `DELETE FROM soil_polygon_set_members WHERE set_id=$1 OR set_id=$2`, oldSet, activeSet)
		db.Exec(c, `DELETE FROM soil_polygon_sets WHERE id=$1 OR id=$2`, oldSet, activeSet)
		db.Exec(c, `DELETE FROM soil_polygons WHERE id=$1`, polygon)
		db.Exec(c, `DELETE FROM soil_map_units WHERE map_unit_key=$1 OR map_unit_key=$2`, stamp+"-active", stamp+"-retired")
		db.Exec(c, `DELETE FROM tax_foreclosure_pipeline WHERE parcel_id=ANY($1::uuid[])`, ids)
		db.Exec(c, `DELETE FROM tax_foreclosure_cases WHERE source_name=$1`, stamp)
		db.Exec(c, `DELETE FROM tax_file_imports WHERE source_name=$1 OR source_name=$2`, stamp, stamp+"_old")
		db.Exec(c, `DELETE FROM parcels WHERE id=ANY($1::uuid[])`, ids)
	})
	for _, source := range []string{stamp, stamp + "_old"} {
		hash := fmt.Sprintf("%064x", len(source)+len(stamp))
		exec(`INSERT INTO tax_file_imports(county_id,source_name,source_url,fetched_at,byte_size,file_sha256,artifact_key,storage_mode,validation_result,active) VALUES('union',$1,$2,now(),0,decode($3,'hex'),'tax/'||$1||'/sha256/'||$3,'local','valid',$4)`, source, "https://example.test/"+source, hash, source == stamp)
	}
	exec(`INSERT INTO tax_foreclosure_pipeline(county_id,source_name,parcel_id,source_url,file_sha256,fetched_at) VALUES('union',$1,$2,'fixture',decode(repeat('01',32),'hex'),now()),('union',$3,$4,'fixture',decode(repeat('02',32),'hex'),now())`, stamp, ids[0], stamp+"_old", ids[2])
	var caseID string
	if err := db.QueryRow(ctx, `INSERT INTO tax_foreclosure_cases(county_id,source_name,case_number,parcel_set_sha256,legal_sha256,source_url,file_sha256,fetched_at,active) VALUES('union',$1,$2,decode(repeat('03',32),'hex'),decode(repeat('04',32),'hex'),'fixture',decode(repeat('05',32),'hex'),now(),true) RETURNING id::text`, stamp, stamp).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO tax_foreclosure_parcels(case_id,parcel_id) VALUES($1,$2),($1,$3)`, caseID, ids[1], ids[0])
	var retiredCase string
	if err := db.QueryRow(ctx, `INSERT INTO tax_foreclosure_cases(county_id,source_name,case_number,parcel_set_sha256,legal_sha256,source_url,file_sha256,fetched_at,active) VALUES('union',$1,$2,decode(repeat('06',32),'hex'),decode(repeat('07',32),'hex'),'fixture',decode(repeat('08',32),'hex'),now(),true) RETURNING id::text`, stamp+"_old", stamp).Scan(&retiredCase); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO tax_foreclosure_parcels(case_id,parcel_id) VALUES($1,$2)`, retiredCase, ids[2])
	for _, key := range []string{stamp + "-active", stamp + "-retired"} {
		exec(`INSERT INTO soil_map_units(map_unit_key,map_unit_name) VALUES($1,$2)`, key, key)
	}
	if err := db.QueryRow(ctx, `INSERT INTO soil_polygons(county_id,source_name,object_id,map_unit_key,shape,fetched_at) VALUES('union','union_county_gis',$1,$2,ST_Transform(ST_MakeEnvelope(-79.3,34.3,-79.299,34.301,4326),3857),now()) RETURNING id::text`, stamp, stamp+"-active").Scan(&polygon); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		dest *string
		key  string
	}{{&oldSet, "retired"}, {&activeSet, "active"}} {
		if err := db.QueryRow(ctx, `INSERT INTO soil_polygon_sets(county_id,source_name,fingerprint) VALUES('union','union_county_gis',$1) RETURNING id::text`, stamp+entry.key).Scan(entry.dest); err != nil {
			t.Fatal(err)
		}
	}
	// Active type covers parcel 0 and reaches parcel 3 only along its boundary.
	exec(`INSERT INTO soil_polygon_set_members(set_id,polygon_id,map_unit_key,shape) VALUES($1,$2,$3,ST_Transform(ST_MakeEnvelope(-79.3,34.3,-79.291,34.301,4326),3857))`, activeSet, polygon, stamp+"-active")
	exec(`INSERT INTO soil_polygon_set_members(set_id,polygon_id,map_unit_key,shape) VALUES($1,$2,$3,ST_Transform(ST_MakeEnvelope(-79.294,34.3,-79.293,34.301,4326),3857))`, oldSet, polygon, stamp+"-retired")
	exec(`INSERT INTO soil_polygon_set_heads(county_id,source_name,set_id) VALUES('union','union_county_gis',$1)`, activeSet)
	if err := db.QueryRow(ctx, `INSERT INTO soil_screening_runs(county_id,source_name,source_url,method,soil_set_id,assessment_method,fresh_until) VALUES('union','union_county_gis','fixture','fixture',$1,$2,now()+interval '1 day') RETURNING id::text`, activeSet, soil.AssessmentMethod).Scan(&run); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO soil_screening_heads(county_id,source_name,run_id) VALUES('union','union_county_gis',$1)`, run)
	exec(`INSERT INTO soil_screening_ratings(run_id,map_unit_key,rule_key,aggregation,rating,availability,components,evidence,source_query,fetched_at) VALUES($1,$2,'208','fixture','Not limited','rated','{}','{}','fixture',now())`, run, stamp+"-active")
	for _, i := range []int{0, 4, 5, 6} {
		fingerprint := "stale"
		if i != 6 {
			if err := db.QueryRow(ctx, `SELECT md5(ST_AsEWKB(shape)) FROM parcels WHERE id=$1`, ids[i]).Scan(&fingerprint); err != nil {
				t.Fatal(err)
			}
		}
		exec(`INSERT INTO soil_parcel_assessments(run_id,parcel_id,geometry_fingerprint,total_acres,uncovered_acres,ambiguous_acres) VALUES($1,$2,$3,1,0,$4)`, run, ids[i], fingerprint, map[bool]float64{true: 1, false: 0}[i == 5])
		exclusive := 1.
		if i == 5 {
			exclusive = 0
		}
		exec(`INSERT INTO soil_assessment_map_units(run_id,parcel_id,map_unit_key,exclusive_acres,mapped_acres,polygon_ids) VALUES($1,$2,$3,$4,1,ARRAY[$5::uuid])`, run, ids[i], stamp+"-active", exclusive, polygon)
	}
	h := New(db)
	get := func(path string) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var body map[string]any
		if w.Code == 200 {
			if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
				t.Fatal(e)
			}
		}
		return w.Code, body
	}
	bbox := "-79.301,34.299,-79.277,34.302"
	match := func(query string, want ...int) map[string]any {
		t.Helper()
		code, b := get("/api/v1/parcels?bbox=" + bbox + "&" + query)
		if code != 200 {
			t.Fatalf("list %s: %d", query, code)
		}
		got := map[string]bool{}
		for _, item := range b["items"].([]any) {
			got[item.(map[string]any)["id"].(string)] = true
		}
		if len(got) != len(want) || int(b["count"].(float64)) != len(want) {
			t.Fatalf("list %s got %v count %v want %v", query, got, b["count"], want)
		}
		for _, i := range want {
			if !got[ids[i]] {
				t.Fatalf("list %s missing %d: %v", query, i, got)
			}
		}
		for _, zoom := range []int{8, 12, 14} {
			code, m := get(fmt.Sprintf("/api/v1/parcels/map?bbox=%s&zoom=%d&%s", bbox, zoom, query))
			if code != 200 || int(m["match_count"].(float64)) != len(want) {
				t.Fatalf("map %d %s: %d %v", zoom, query, code, m)
			}
			expectOutlines := zoom == 14 || (zoom == 12 && strings.Contains(query, "discovery=browse&"))
			if expectOutlines && m["mode"] != "features" {
				t.Fatalf("expected outlines at zoom %d for %s: %v", zoom, query, m["mode"])
			}
			if !expectOutlines && m["mode"] != "grid" {
				t.Fatalf("expected grid at zoom %d for %s: %v", zoom, query, m["mode"])
			}
			if expectOutlines {
				seen := map[string]bool{}
				for _, raw := range m["features"].([]any) {
					f := raw.(map[string]any)
					if f["match"] == true {
						seen[f["id"].(string)] = true
					}
				}
				if len(seen) != len(got) {
					t.Fatalf("map identities %v vs list %v", seen, got)
				}
				for id := range got {
					if !seen[id] {
						t.Fatalf("map missing %s", id)
					}
				}
			}
		}
		return b
	}
	match("discovery=browse", 0, 1, 2, 3, 4, 5, 6, 7)
	match("foreclosure=true", 0, 1)
	match("discovery=browse&foreclosure=true", 0, 1)
	match("foreclosure=false", 2, 3, 4, 5, 6, 7)
	match("soil_type="+url.QueryEscape(stamp+"-active"), 0, 1, 2)
	match("soil_type=" + url.QueryEscape(stamp+"-retired"))
	rated := "soil_use=conventional-septic&soil_rating=Not+limited"
	match(rated, 0, 4)
	match("foreclosure=true&"+rated, 0)
	pageCode, page := get("/api/v1/parcels?bbox=" + bbox + "&discovery=browse&page_size=1")
	if pageCode != 200 || page["count"] != float64(8) || len(page["items"].([]any)) != 1 {
		t.Fatalf("page: %d %v", pageCode, page)
	}
	cursor := page["next_cursor"].(string)
	exec(`UPDATE soil_screening_runs SET fresh_until=now()-interval '1 day' WHERE id=$1`, run)
	match(rated)
	code, _ := get("/api/v1/parcels?bbox=" + bbox + "&discovery=browse&page_size=1&cursor=" + url.QueryEscape(cursor))
	if code != 400 {
		t.Fatalf("soil publication change did not invalidate cursor: %d", code)
	}
	_, page = get("/api/v1/parcels?bbox=" + bbox + "&discovery=browse&page_size=1")
	cursor = page["next_cursor"].(string)
	exec(`UPDATE tax_file_imports SET active=false WHERE county_id='union' AND source_name=$1 AND active`, stamp)
	match("foreclosure=true")
	code, _ = get("/api/v1/parcels?bbox=" + bbox + "&discovery=browse&page_size=1&cursor=" + url.QueryEscape(cursor))
	if code != 400 {
		t.Fatalf("tax publication change did not invalidate cursor: %d", code)
	}
}
