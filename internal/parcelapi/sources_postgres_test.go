//go:build postgres

package parcelapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSourcesRetainsAssessmentGenerationAfterFailedRefresh(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var database string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil || !strings.HasPrefix(database, "landman_api_test_") {
		t.Fatal("refusing non-disposable API database")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var oldSet, newSet, run string
	for fingerprint, id := range map[string]*string{"api-source-old": &oldSet, "api-source-new": &newSet} {
		if err := tx.QueryRow(ctx, `INSERT INTO soil_polygon_sets(county_id,source_name,fingerprint) VALUES('union','union_county_gis',$1) RETURNING id::text`, fingerprint).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.QueryRow(ctx, `INSERT INTO soil_screening_runs(county_id,source_name,source_url,method,soil_set_id,fresh_until) VALUES('union','union_county_gis','https://example.test/soil','fixture',$1,now()+interval '1 day') RETURNING id::text`, oldSet).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO soil_polygon_set_heads(county_id,source_name,set_id) VALUES('union','union_county_gis',$1)`, oldSet); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO soil_screening_heads(county_id,source_name,run_id) VALUES('union','union_county_gis',$1)`, run); err != nil {
		t.Fatal(err)
	}
	jobIDs := []string{}
	for _, kind := range []string{"full_scan", "reprocess"} {
		started := "2026-01-01T00:00:00Z"
		if kind == "reprocess" {
			started = "2026-01-02T00:00:00Z"
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO job_attempts(job_id,started_at,finished_at,outcome,attempt_kind) VALUES('union.gis.parcels',$1::timestamptz,$1::timestamptz+interval '1 hour','success',$2) RETURNING id::text`, started, kind).Scan(&id); err != nil {
			t.Fatal(err)
		}
		jobIDs = append(jobIDs, id)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup := context.Background()
		pool.Exec(cleanup, `DELETE FROM soil_screening_refresh_status WHERE county_id='union' AND source_name='union_county_gis'`)
		pool.Exec(cleanup, `DELETE FROM soil_screening_heads WHERE run_id=$1`, run)
		pool.Exec(cleanup, `DELETE FROM soil_polygon_set_heads WHERE set_id=$1 OR set_id=$2`, oldSet, newSet)
		pool.Exec(cleanup, `DELETE FROM soil_screening_runs WHERE id=$1`, run)
		pool.Exec(cleanup, `DELETE FROM soil_polygon_sets WHERE id=$1 OR id=$2`, oldSet, newSet)
		for _, id := range jobIDs {
			pool.Exec(cleanup, `DELETE FROM job_attempts WHERE id=$1`, id)
		}
	}()
	type generation struct {
		SetID     string     `json:"soil_set_id"`
		CurrentID string     `json:"current_soil_set_id"`
		RunID     string     `json:"run_id"`
		Changed   bool       `json:"geometry_set_changed"`
		FailedAt  *time.Time `json:"refresh_failed_at"`
	}
	type response struct {
		Soil  generation `json:"published_soil"`
		Stamp string     `json:"source_stamp"`
		Clean []struct {
			JobID    string    `json:"job_id"`
			Finished time.Time `json:"finished_at"`
		} `json:"last_clean_success"`
	}
	request := func() response {
		t.Helper()
		w := httptest.NewRecorder()
		New(pool).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/sources", nil))
		if w.Code != 200 || strings.Contains(w.Body.String(), "private refresh payload") {
			t.Fatalf("source response status=%d", w.Code)
		}
		var result response
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := request()
	if before.Soil.SetID != oldSet || before.Soil.CurrentID != oldSet || before.Soil.RunID != run || before.Soil.Changed {
		t.Fatalf("wrong initial generation: %+v", before.Soil)
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_polygon_set_heads SET set_id=$1 WHERE county_id='union' AND source_name='union_county_gis'`, newSet); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO soil_screening_refresh_status(county_id,source_name,failed_at,error) VALUES('union','union_county_gis',now(),'private refresh payload')`); err != nil {
		t.Fatal(err)
	}
	after := request()
	if after.Soil.SetID != oldSet || after.Soil.CurrentID != newSet || after.Soil.RunID != run || !after.Soil.Changed || after.Soil.FailedAt == nil {
		t.Fatalf("mixed published generations: %+v", after.Soil)
	}
	if before.Stamp == after.Stamp {
		t.Fatal("source stamp did not change after source refresh")
	}
	found := false
	for _, job := range after.Clean {
		if job.JobID == "union.gis.parcels" {
			found = true
			if !job.Finished.Equal(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)) {
				t.Fatal("parcel repair replaced full-import freshness")
			}
		}
	}
	if !found {
		t.Fatal("missing clean parcel import")
	}
}
