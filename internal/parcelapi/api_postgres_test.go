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

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHTTPPostGIS(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err := pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("refusing non-disposable API database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ids := make([]string, 3)
	for i := range ids {
		err = pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,situs_line_1,mapped_acres,likely_vacant,shape) VALUES('union','union','union_county_gis',$1,$2,'API TEST ROAD',$3,$4,ST_Transform(ST_MakeEnvelope($5,33.1,$6,33.101,4326),3857)) RETURNING id::text`, fmt.Sprintf("api-fixture-%d-%d", time.Now().UnixNano(), i), fmt.Sprintf("APITEST%d", i), 2.5+float64(i), float64(i) != 2, -79.9+float64(i)*.002, -79.899+float64(i)*.002).Scan(&ids[i])
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(context.Background(), "DELETE FROM parcels WHERE id=$1", ids[i])
	}
	h := New(pool)
	request := func(path string) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var body map[string]any
		if w.Code == 200 && json.Unmarshal(w.Body.Bytes(), &body) != nil {
			t.Fatalf("invalid JSON on %s", path)
		}
		return w.Code, body
	}
	bbox := "-79.901,33.099,-79.894,33.102"
	filter := "bbox=" + bbox + "&likely_vacant=true"
	code, list := request("/api/v1/parcels?" + filter + "&page_size=1")
	if code != 200 || list["count"] != float64(2) || len(list["items"].([]any)) != 1 {
		t.Fatalf("first page %d %v", code, list)
	}
	cursor := list["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing next cursor")
	}
	code, later := request("/api/v1/parcels?" + filter + "&page_size=1&cursor=" + url.QueryEscape(cursor))
	if code != 200 || later["count"] != float64(2) || later["items"].([]any)[0].(map[string]any)["id"] == list["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatalf("later page %d %v", code, later)
	}
	if _, err = pool.Exec(ctx, `UPDATE parcels SET updated_at=now()+interval '1 hour' WHERE id=$1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if code, _ := request("/api/v1/parcels?" + filter + "&page_size=1&cursor=" + url.QueryEscape(cursor)); code != 400 {
		t.Fatalf("stale cursor returned %d", code)
	}
	for _, path := range []string{"/api/v1/parcels?bbox=" + bbox + "&likely_vacant=false&cursor=" + url.QueryEscape(cursor), "/api/v1/parcels?" + filter + "&cursor=bad", "/api/v1/parcels?zoom=14", "/api/v1/parcels/map?bbox=" + bbox + "&zoom=14&page_size=1", "/api/v1/parcels/map?bbox=" + bbox + "&zoom=23", "/api/v1/parcels/map?bbox=0,86,1,87&zoom=8"} {
		if code, _ := request(path); code != 400 {
			t.Errorf("invalid %s returned %d", path, code)
		}
	}
	if code, body := request("/api/v1/parcels"); code != 200 || body["count"] != float64(0) {
		t.Fatalf("no filter %d %v", code, body)
	}
	if code, body := request("/api/v1/parcels/lookup?q=APITEST"); code != 200 || len(body["items"].([]any)) != 3 {
		t.Fatalf("lookup %d %v", code, body)
	}
	for _, zoom := range []int{8, 14} {
		code, body := request(fmt.Sprintf("/api/v1/parcels/map?bbox=%s&zoom=%d&likely_vacant=true", bbox, zoom))
		if code != 200 || body["context_count"] != float64(3) || body["match_count"] != float64(2) {
			t.Fatalf("map %d %d %v", zoom, code, body)
		}
		var contexts, matches float64
		if zoom == 8 {
			for _, v := range body["cells"].([]any) {
				c := v.(map[string]any)
				contexts += c["count"].(float64)
				matches += c["matches"].(float64)
			}
		} else {
			for _, v := range body["features"].([]any) {
				contexts++
				if v.(map[string]any)["match"].(bool) {
					matches++
				}
			}
		}
		if contexts != 3 || matches != 2 {
			t.Fatalf("map %d aggregate %v %v", zoom, contexts, matches)
		}
	}
	for _, path := range []string{"/api/v1/parcels/" + ids[0], "/api/v1/parcels/" + ids[0] + "/tax", "/api/v1/parcels/" + ids[0] + "/soil", "/api/v1/sources"} {
		if code, _ := request(path); code != 200 {
			t.Errorf("%s: %d", path, code)
		}
	}
	code, tax := request("/api/v1/parcels/" + ids[0] + "/tax")
	if code != 200 || tax["reported_delinquent_total"] != nil || tax["delinquent_year_count"] != float64(0) {
		t.Fatalf("missing tax %d %v", code, tax)
	}
	code, soil := request("/api/v1/parcels/" + ids[0] + "/soil")
	if code != 200 || soil["available"] != false {
		t.Fatalf("missing soil %d %v", code, soil)
	}
	canceled, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/parcels?likely_vacant=true", nil).WithContext(canceled))
	if w.Code == 200 {
		t.Fatal("canceled request returned data")
	}
}
