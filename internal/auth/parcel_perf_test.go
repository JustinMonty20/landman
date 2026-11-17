//go:build perf

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

// TestParcelRealData measures authenticated HTTP on an explicitly disposable copy.
// It never truncates county data or fetches a remote source.
func TestParcelRealData(t *testing.T) {
	dsn := os.Getenv("API_PERF_DATABASE_URL")
	if dsn == "" {
		t.Skip("set API_PERF_DATABASE_URL to a disposable copied database")
	}
	pool, e := pgxpool.New(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	var db string
	if e = pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&db); e != nil || !strings.HasPrefix(db, "landman_api_perf_") {
		t.Fatal("refusing database without landman_api_perf_ prefix")
	}
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
	}))
	defer issuer.Close()
	s, e := newServer(context.Background(), pool, Config{ClientID: "perf", ClientSecret: "perf", RedirectURI: "http://127.0.0.1:5173/auth/google/callback", AllowedEmails: []string{"perf@example.test"}}, issuer.URL)
	if e != nil {
		t.Fatal(e)
	}
	s.mountData(parcelapi.New(pool))
	token := randomToken()
	sum := sha256.Sum256([]byte(token))
	var userID string
	// Session setup is outside timed requests. Cleanup only this session and user.
	e = pool.QueryRow(context.Background(), `INSERT INTO app_users(google_subject,email,display_name) VALUES($1,'perf@example.test','Performance') ON CONFLICT(google_subject) DO UPDATE SET email=excluded.email RETURNING id::text`, `parcel-perf-`+token).Scan(&userID)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Exec(context.Background(), "DELETE FROM app_users WHERE id=$1", userID)
	if _, e = pool.Exec(context.Background(), "INSERT INTO auth_sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", sum[:], userID); e != nil {
		t.Fatal(e)
	}
	var postgresVersion, postgisVersion string
	if e = pool.QueryRow(context.Background(), "SELECT version(),PostGIS_Version()").Scan(&postgresVersion, &postgisVersion); e != nil {
		t.Fatal(e)
	}
	var parcels, assessments, imports int64
	if e = pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM parcels WHERE county_id='union'),(SELECT count(*) FROM soil_parcel_assessments),(SELECT count(*) FROM tax_file_imports WHERE active AND county_id='union')`).Scan(&parcels, &assessments, &imports); e != nil {
		t.Fatal(e)
	}
	t.Logf("database=%s postgres=%s postgis=%s parcels=%d assessments=%d active_files=%d cpus=%d", db, postgresVersion, postgisVersion, parcels, assessments, imports, runtime.NumCPU())
	plan, e := pool.Query(context.Background(), `EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM parcels WHERE county_id='union' AND source_name='union_county_gis' AND likely_vacant=true ORDER BY id LIMIT 20`)
	if e != nil {
		t.Fatal(e)
	}
	for plan.Next() {
		var line string
		if e = plan.Scan(&line); e != nil {
			t.Fatal(e)
		}
		t.Log("plan " + line)
	}
	e = plan.Err()
	plan.Close()
	if e != nil {
		t.Fatal(e)
	}
	var parcelID string
	_ = pool.QueryRow(context.Background(), `SELECT id::text FROM parcels WHERE county_id='union' AND source_name='union_county_gis' ORDER BY id LIMIT 1`).Scan(&parcelID)
	paths := []string{"/api/v1/sources", "/api/v1/parcels/lookup?q=01", "/api/v1/parcels?likely_vacant=true&page_size=20", "/api/v1/parcels?likely_vacant=true&min_acres=1&delinquent=false&page_size=20", "/api/v1/parcels/map?bbox=-81,34,-80,35&zoom=8&likely_vacant=true", "/api/v1/parcels/map?bbox=-80.61,34.89,-80.60,34.90&zoom=16&likely_vacant=true"}
	if parcelID != "" {
		for _, suffix := range []string{"", "/tax", "/soil"} {
			paths = append(paths, "/api/v1/parcels/"+parcelID+suffix)
		}
	}
	req := func(path string) (time.Duration, int, int, string) {
		r := httptest.NewRequest("GET", "http://127.0.0.1:5173"+path, nil)
		r.AddCookie(&http.Cookie{Name: "landman_session", Value: token})
		w := httptest.NewRecorder()
		start := time.Now()
		s.ServeHTTP(w, r)
		return time.Since(start), w.Code, w.Body.Len(), w.Body.String()
	}
	// Build a later-page case from an actual first response, not an invented cursor.
	_, code, _, body := req("/api/v1/parcels?likely_vacant=true&page_size=20")
	if code == 200 {
		var page struct {
			NextCursor string `json:"next_cursor"`
		}
		if json.Unmarshal([]byte(body), &page) == nil && page.NextCursor != "" {
			paths = append(paths, "/api/v1/parcels?likely_vacant=true&page_size=20&cursor="+page.NextCursor)
		}
	}
	for _, path := range paths {
		firstLatency, firstStatus, firstBytes, firstBody := req(path)
		t.Logf("first_use path=%q duration=%s status=%d bytes=%d", path, firstLatency, firstStatus, firstBytes)
		if firstStatus != 200 || !json.Valid([]byte(firstBody)) {
			t.Errorf("first use failed path=%q status=%d body=%q", path, firstStatus, firstBody)
		}
		for _, concurrency := range []int{1, 4} {
			var mu sync.Mutex
			lat := []time.Duration{}
			errs := 0
			bytes := 0
			firstResult := ""
			statuses := map[int]int{}
			var wg sync.WaitGroup
			for round := 0; round < 5; round++ {
				for worker := 0; worker < concurrency; worker++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						d, status, size, body := req(path)
						mu.Lock()
						defer mu.Unlock()
						lat = append(lat, d)
						bytes += size
						statuses[status]++
						if status == 200 && !json.Valid([]byte(body)) {
							status = 500
						}
						if status == 200 && firstResult == "" {
							if len(body) > 180 {
								firstResult = body[:180]
							} else {
								firstResult = body
							}
						}
						if status != 200 {
							errs++
							if len(body) > 100 {
								body = body[:100]
							}
							t.Logf("request error path=%s status=%d body=%q", path, status, body)
						}
					}()
				}
				wg.Wait()
			}
			sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
			p := func(q float64) time.Duration { return lat[int(float64(len(lat)-1)*q)] }
			t.Logf("path=%q concurrency=%d rounds=5 n=%d p50=%s p95=%s errors=%d bytes=%d statuses=%v first_result=%q", path, concurrency, len(lat), p(.5), p(.95), errs, bytes, statuses, firstResult)
			if errs > 0 {
				t.Errorf("%s: %d failed requests", path, errs)
			}
		}
	}
	fmt.Println("PostgreSQL and PostGIS versions and EXPLAIN plans should be captured alongside this HTTP run")
}
