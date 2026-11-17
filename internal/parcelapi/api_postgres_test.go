//go:build postgres

package parcelapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHTTPPostGIS(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL not set")
	}
	pool, e := pgxpool.New(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	var db string
	if e = pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&db); e != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("refusing non-disposable API database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	// Fixtures stay in one transaction; this test never truncates a shared or retained database.
	var id string
	e = tx.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,situs_line_1,mapped_acres,likely_vacant,shape)
 VALUES('union','union','union_county_gis','api-fixture-one','TEST100','TEST ROAD',2.5,true,ST_Transform(ST_MakeEnvelope(-80.6,34.9,-80.599,34.901,4326),3857)) RETURNING id::text`).Scan(&id)
	if e != nil {
		t.Fatal(e)
	}
	// A separate pool connection sees only committed rows; rollback cleanup removes fixtures.
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	defer pool.Exec(context.Background(), "DELETE FROM parcels WHERE id=$1", id)
	h := New(pool)
	request := func(path string) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	if code, body := request("/api/v1/parcels?likely_vacant=true"); code != 200 || body["count"].(float64) < 1 {
		t.Fatalf("list %d %v", code, body)
	}
	if code, body := request("/api/v1/parcels"); code != 200 || body["count"].(float64) != 0 {
		t.Fatalf("no-filter %d %v", code, body)
	}
	if code, body := request("/api/v1/parcels/lookup?q=" + url.QueryEscape("TEST100")); code != 200 || len(body["items"].([]any)) == 0 {
		t.Fatalf("lookup %d %v", code, body)
	}
	if code, body := request("/api/v1/parcels/map?bbox=-80.61,34.89,-80.59,34.91&zoom=14&likely_vacant=true"); code != 200 || body["match_count"].(float64) < 1 {
		t.Fatalf("map %d %v", code, body)
	}
	for _, path := range []string{"/api/v1/parcels/" + id, "/api/v1/parcels/" + id + "/tax", "/api/v1/parcels/" + id + "/soil", "/api/v1/sources"} {
		if code, body := request(path); code != 200 {
			t.Errorf("%s: %d %v", path, code, body)
		}
	}
}
