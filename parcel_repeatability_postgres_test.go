//go:build postgres && ingestionfixture

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
)

func TestCountySizedParcelRepeatability(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	const features = 118000
	var invalid atomic.Bool
	invalid.Store(true)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		q := r.URL.Query()
		if q.Get("where") != "1=1" {
			http.Error(w, "fixture permits full scans only", 400)
			return
		}
		if q.Get("returnCountOnly") == "true" {
			fmt.Fprintf(w, `{"count":%d}`, features)
			return
		}
		offset, _ := strconv.Atoi(q.Get("resultOffset"))
		limit, _ := strconv.Atoi(q.Get("resultRecordCount"))
		if limit != 2000 {
			http.Error(w, "expected 2000-feature pages", 400)
			return
		}
		page := make([]map[string]interface{}, 0, limit)
		for oid := offset + 1; oid <= min(offset+limit, features); oid++ {
			pid := fmt.Sprintf("P%06d", oid)
			sales := 3
			if oid == features {
				pid = "P000001"
				sales = 1
			}
			if oid == 7 {
				pid = ""
			}
			page = append(page, feature(oid, pid, fmt.Sprintf("owner %d", oid), !(invalid.Load() && oid == 2), sales))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"features": page})
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL, BatchSize: 2000, RateLimit: 1000000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT fixture_failure CHECK(source_parcel_id <> 'P105000')`); err != nil {
		t.Fatal(err)
	}
	run := func() (jobs.Outcome, jobs.Result, error) {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		started := time.Now()
		before := requests.Load()
		outcome, result, err := runParcelFixture(attemptCtx, pool, dsn, source, false, 0)
		t.Logf("synthetic features=%d requests=%d outcome=%s processed=%d quarantined=%d elapsed=%s error=%v", features, requests.Load()-before, outcome, result.Processed, result.Quarantined, time.Since(started), err)
		if requests.Load()-before != 60 {
			t.Errorf("full scan did not fetch all 59 pages plus count")
		}
		return outcome, result, err
	}
	outcome, _, err := run()
	if err == nil || outcome != jobs.OutcomeFailed {
		t.Fatalf("first scan=%s %v", outcome, err)
	}
	if countRows(t, ctx, pool, "parcels") != 0 || countRows(t, ctx, pool, "parcel_sales") != 0 {
		t.Fatal("failed first import left partial rows")
	}
	if countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 1 {
		t.Fatal("rollback erased quarantine evidence")
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT fixture_failure`); err != nil {
		t.Fatal(err)
	}
	invalid.Store(false)
	outcome, result, err := run()
	if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != features || result.Quarantined != 0 {
		t.Fatalf("retry=%s %+v %v", outcome, result, err)
	}
	if countRows(t, ctx, pool, "parcels") != features-1 || countRows(t, ctx, pool, "parcel_sales") != (features-1)*3-2 {
		t.Fatal("wrong natural-key or sales cardinality")
	}
	if countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 0 {
		t.Fatal("clean retry did not resolve observed recovery")
	}
	var parcelsEstimate, salesEstimate float64
	if err := pool.QueryRow(ctx, `SELECT (SELECT reltuples FROM pg_class WHERE oid='parcels'::regclass),(SELECT reltuples FROM pg_class WHERE oid='parcel_sales'::regclass)`).Scan(&parcelsEstimate, &salesEstimate); err != nil {
		t.Fatal(err)
	}
	if parcelsEstimate < 116000 || salesEstimate < 348000 {
		t.Fatalf("statistics regressed: parcels=%v sales=%v", parcelsEstimate, salesEstimate)
	}
	var area float64
	var owner string
	if err := pool.QueryRow(ctx, `SELECT ST_Area(shape),current_name_1 FROM parcels WHERE source_parcel_id='P000001'`).Scan(&area, &owner); err != nil {
		t.Fatal(err)
	}
	if area != 2 || owner != "owner 118000" {
		t.Fatalf("cross-page multipart area=%v owner=%s", area, owner)
	}
	if countRows(t, ctx, pool, "parcels WHERE source_parcel_id='7' AND NOT likely_vacant") != 1 {
		t.Fatal("fallback vacancy override lost")
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE fixture_parcels_before AS SELECT id,to_jsonb(p)-'updated_at' AS value,ST_AsEWKB(shape) AS shape FROM parcels p;
 CREATE TABLE fixture_sales_before AS SELECT id,to_jsonb(s)-'updated_at' AS value FROM parcel_sales s`); err != nil {
		t.Fatal(err)
	}
	outcome, result, err = run()
	if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != features || result.Quarantined != 0 {
		t.Fatalf("identical rerun=%s %+v %v", outcome, result, err)
	}
	var differentParcels, differentSales int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fixture_parcels_before b FULL JOIN parcels p USING(id)
 WHERE b.id IS NULL OR p.id IS NULL OR b.value IS DISTINCT FROM to_jsonb(p)-'updated_at' OR b.shape IS DISTINCT FROM ST_AsEWKB(p.shape) OR NOT ST_Equals(ST_GeomFromEWKB(b.shape),p.shape)`).Scan(&differentParcels); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fixture_sales_before b FULL JOIN parcel_sales s USING(id)
 WHERE b.id IS NULL OR s.id IS NULL OR b.value IS DISTINCT FROM to_jsonb(s)-'updated_at'`).Scan(&differentSales); err != nil {
		t.Fatal(err)
	}
	if differentParcels != 0 || differentSales != 0 {
		t.Fatalf("unchanged rerun differences: parcels=%d sales=%d", differentParcels, differentSales)
	}
	if countRows(t, ctx, pool, "job_attempts WHERE outcome='success' AND attempt_kind='full_scan'") != 2 {
		t.Fatal("missing clean full-scan history")
	}
	t.Logf("persisted equality verified for %d parcel IDs and %d sale IDs, all mapped values, EWKB, topology, and sale-slot deletion", features-1, (features-1)*3-2)
}
