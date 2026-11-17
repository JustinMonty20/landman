//go:build perf

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/parcelapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestParcelRealData exercises authenticated TCP HTTP against an explicitly disposable copy.
func TestParcelRealData(t *testing.T) {
	dsn := os.Getenv("API_PERF_DATABASE_URL")
	if dsn == "" {
		t.Skip("set API_PERF_DATABASE_URL to a disposable copied database")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_perf_") {
		t.Fatal("refusing non-perf database")
	}
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
	}))
	defer issuer.Close()
	s, err := newServer(context.Background(), pool, Config{ClientID: "perf", ClientSecret: "perf", RedirectURI: "http://127.0.0.1:5173/auth/google/callback", AllowedEmails: []string{"perf@example.test"}}, issuer.URL)
	if err != nil {
		t.Fatal(err)
	}
	s.mountData(parcelapi.New(pool))
	token := randomToken()
	sum := sha256.Sum256([]byte(token))
	var userID string
	err = pool.QueryRow(context.Background(), `INSERT INTO app_users(google_subject,email,display_name) VALUES($1,'perf@example.test','Performance') ON CONFLICT(google_subject) DO UPDATE SET email=excluded.email RETURNING id::text`, `parcel-perf-`+token).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DELETE FROM app_users WHERE id=$1", userID)
	if _, err = pool.Exec(context.Background(), "INSERT INTO auth_sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", sum[:], userID); err != nil {
		t.Fatal(err)
	}
	var postgresVersion, postgisVersion string
	if err = pool.QueryRow(context.Background(), "SELECT version(),PostGIS_Version()").Scan(&postgresVersion, &postgisVersion); err != nil {
		t.Fatal(err)
	}
	var parcels, assessments, imports int64
	if err = pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM parcels WHERE county_id='union' AND source_name='union_county_gis'),(SELECT count(*) FROM soil_parcel_assessments),(SELECT count(*) FROM tax_file_imports WHERE active AND county_id='union')`).Scan(&parcels, &assessments, &imports); err != nil {
		t.Fatal(err)
	}
	if parcels < 100 || assessments < 1 || imports < 4 {
		t.Fatalf("insufficient copied fixture parcels=%d soil=%d feeds=%d", parcels, assessments, imports)
	}
	t.Logf("database=%s postgres=%s postgis=%s parcels=%d assessments=%d active_files=%d cpus=%d", db, postgresVersion, postgisVersion, parcels, assessments, imports, runtime.NumCPU())
	var parcelID, taxID, soilID, lookup string
	if err = pool.QueryRow(context.Background(), `SELECT id::text,coalesce(parcel_number,'') FROM parcels WHERE county_id='union' AND source_name='union_county_gis' AND length(parcel_number)>=2 AND mapped_acres BETWEEN 1 AND 3 ORDER BY id LIMIT 1`).Scan(&parcelID, &lookup); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT p.id::text FROM parcels p JOIN tax_delinquent_balances d ON d.parcel_id=p.id JOIN tax_file_imports i ON i.active AND i.county_id=d.county_id AND i.source_name=d.source_name AND i.source_url=d.source_url AND i.file_sha256=d.file_sha256 AND i.fetched_at=d.fetched_at WHERE p.county_id='union' AND p.source_name='union_county_gis' AND d.balance_due>0 LIMIT 1`).Scan(&taxID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT p.id::text FROM parcels p JOIN soil_parcel_assessments a ON a.parcel_id=p.id JOIN soil_screening_heads h ON h.run_id=a.run_id AND h.county_id=p.county_id AND h.source_name=p.source_name WHERE p.county_id='union' AND p.source_name='union_county_gis' LIMIT 1`).Scan(&soilID); err != nil {
		t.Fatal(err)
	}
	var west, south, east, north float64
	if err = pool.QueryRow(context.Background(), `SELECT ST_XMin(ST_Transform(ST_Envelope(shape),4326)),ST_YMin(ST_Transform(ST_Envelope(shape),4326)),ST_XMax(ST_Transform(ST_Envelope(shape),4326)),ST_YMax(ST_Transform(ST_Envelope(shape),4326)) FROM parcels WHERE id=$1`, parcelID).Scan(&west, &south, &east, &north); err != nil {
		t.Fatal(err)
	}
	closeBox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", west-.002, south-.002, east+.002, north+.002)
	broadBox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", west-.04, south-.04, east+.04, north+.04)
	var countyBox string
	if err = pool.QueryRow(context.Background(), `SELECT concat(ST_XMin(b)-0.001,',',ST_YMin(b)-0.001,',',ST_XMax(b)+0.001,',',ST_YMax(b)+0.001) FROM (SELECT ST_Transform(ST_SetSRID(ST_Extent(shape)::geometry,3857),4326) AS b FROM parcels WHERE county_id='union' AND source_name='union_county_gis') extent`).Scan(&countyBox); err != nil {
		t.Fatal(err)
	}
	origin := httptest.NewServer(s)
	defer origin.Close()
	client := origin.Client()
	request := func(path string) (time.Duration, int, int, []byte) {
		r, e := http.NewRequest("GET", origin.URL+path, nil)
		if e != nil {
			t.Fatal(e)
		}
		r.Host = "127.0.0.1:5173"
		r.AddCookie(&http.Cookie{Name: "landman_session", Value: token})
		start := time.Now()
		res, e := client.Do(r)
		if e != nil {
			return time.Since(start), 0, 0, nil
		}
		defer res.Body.Close()
		data, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		elapsed := time.Since(start)
		if e != nil {
			return elapsed, 0, len(data), nil
		}
		return elapsed, res.StatusCode, len(data), data
	}
	type route struct {
		label, path string
		want        int64
	}
	routes := []route{{"sources", "/api/v1/sources", 4}, {"lookup", "/api/v1/parcels/lookup?q=" + lookup[:2], 1}, {"no-filter", "/api/v1/parcels", 0}, {"list-first", "/api/v1/parcels?likely_vacant=true&page_size=20", -1}, {"list-combined-false", "/api/v1/parcels?likely_vacant=true&min_acres=1&delinquent=false&page_size=20", -1}, {"list-combined-true", "/api/v1/parcels?likely_vacant=true&min_acres=1&delinquent=true&page_size=20", -1}, {"grid", "/api/v1/parcels/map?bbox=" + broadBox + "&zoom=8&likely_vacant=true", -1}, {"shape", "/api/v1/parcels/map?bbox=" + closeBox + "&zoom=16&likely_vacant=true", -1}, {"facts", "/api/v1/parcels/" + parcelID, 1}, {"tax", "/api/v1/parcels/" + taxID + "/tax", 1}, {"soil", "/api/v1/parcels/" + soilID + "/soil", 1}}
	_, status, _, first := request(routes[3].path)
	if status != 200 {
		t.Fatal("list first-page failed")
	}
	var page struct {
		NextCursor string `json:"next_cursor"`
	}
	if err = json.Unmarshal(first, &page); err != nil || page.NextCursor == "" {
		t.Fatal("no later page")
	}
	routes = append(routes, route{"list-later", routes[3].path + "&cursor=" + page.NextCursor, -1}, route{"grid-county", "/api/v1/parcels/map?bbox=" + countyBox + "&zoom=8&likely_vacant=true", -1})
	for _, r := range routes {
		firstLatency, code, size, body := request(r.path)
		if code != 200 || !json.Valid(body) {
			t.Fatalf("route=%s first status=%d bytes=%d", r.label, code, size)
		}
		var result map[string]json.RawMessage
		if err = json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		switch r.label {
		case "sources":
			var files []any
			if err = json.Unmarshal(result["active_tax_files"], &files); err != nil || int64(len(files)) < r.want {
				t.Fatalf("missing feeds %s", r.label)
			}
		case "lookup":
			var items []any
			if err = json.Unmarshal(result["items"], &items); err != nil || len(items) == 0 {
				t.Fatal("lookup empty")
			}
		case "facts":
			var p struct {
				ID string `json:"id"`
			}
			if err = json.Unmarshal(result["parcel"], &p); err != nil || p.ID != parcelID {
				t.Fatal("wrong facts")
			}
		case "tax":
			var rows []any
			if err = json.Unmarshal(result["delinquent_balances"], &rows); err != nil || len(rows) == 0 {
				t.Fatal("missing active tax balance")
			}
		case "soil":
			var a struct {
				ParcelID  string `json:"parcel_id"`
				Available bool   `json:"available"`
			}
			if err = json.Unmarshal(body, &a); err != nil || a.ParcelID != soilID || !a.Available {
				t.Fatal("missing published soil")
			}
		case "grid", "shape", "grid-county":
			var count int64
			if err = json.Unmarshal(result["context_count"], &count); err != nil || count == 0 {
				t.Fatal("empty map viewport")
			}
		default:
			var count int64
			if err = json.Unmarshal(result["count"], &count); err != nil || count <= 0 && r.want != 0 || r.want == 0 && count != 0 {
				t.Fatalf("bad count route=%s count=%d", r.label, count)
			}
			if r.label == "list-first" || r.label == "list-later" {
				var expected int64
				if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM parcels WHERE county_id='union' AND source_name='union_county_gis' AND likely_vacant=true`).Scan(&expected); err != nil {
					t.Fatal(err)
				}
				if count != expected {
					t.Fatalf("route=%s count=%d expected=%d", r.label, count, expected)
				}
				t.Logf("route=%s expected_rows=%d", r.label, expected)
			}
		}
		t.Logf("first_observed route=%s duration=%s bytes=%d errors=0", r.label, firstLatency, size)
		expectedBody := sha256.Sum256(body)
		for _, parallel := range []int{1, 4} {
			lat := make([]time.Duration, 0, 5*parallel)
			totalBytes := 0
			errors := 0
			var mu sync.Mutex
			for round := 0; round < 5; round++ {
				var wg sync.WaitGroup
				for worker := 0; worker < parallel; worker++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						d, status, n, data := request(r.path)
						mu.Lock()
						defer mu.Unlock()
						lat = append(lat, d)
						totalBytes += n
						if status != 200 || !json.Valid(data) || sha256.Sum256(data) != expectedBody {
							errors++
						}
					}()
				}
				wg.Wait()
			}
			sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
			t.Logf("route=%s concurrency=%d rounds=5 n=%d min=%s p50=%s p95=%s max=%s errors=%d bytes=%d", r.label, parallel, len(lat), lat[0], lat[(len(lat)-1)/2], lat[int(float64(len(lat)-1)*.95)], lat[len(lat)-1], errors, totalBytes)
			if errors != 0 {
				t.Errorf("route=%s errors=%d", r.label, errors)
			}
		}
	}
}
