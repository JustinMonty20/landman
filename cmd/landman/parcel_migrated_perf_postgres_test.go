//go:build postgres && perf

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run only on a freshly migrated, disposable database. Unlike parcelFixtureDB,
// this uses the actual migrated tables and their BEFORE-statement triggers.
func TestMigratedParcel500WriteTiming(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.ConnConfig.Database, "landman_parcel_perf_") {
		t.Fatal("refusing non-disposable performance database")
	}
	ctx, done := context.WithTimeout(context.Background(), 90*time.Second)
	defer done()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || db != cfg.ConnConfig.Database {
		t.Fatal("database mismatch", err)
	}
	for _, table := range []string{"parcels", "parcel_evidence_observations"} {
		if countRows(t, ctx, pool, table) != 0 {
			t.Fatalf("%s not empty; use a new disposable DB", table)
		}
	}
	root := t.TempDir()
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARCEL_EVIDENCE_DIR", root)
	fixture := &parcelHTTPFixture{}
	for i := 1; i <= 500; i++ {
		fixture.features = append(fixture.features, feature(i, fmt.Sprintf("perf-%d", i), "owner", true, 2))
	}
	source := fixture.source(t, 100)
	measure := func(label string, want jobs.Outcome, wantParts int) {
		t.Helper()
		start := time.Now()
		outcome, result, e := runParcelFixture(ctx, pool, dsn, source, false, 0)
		if outcome != want || (want == jobs.OutcomeSuccess && (e != nil || result.Processed != wantParts)) || (want == jobs.OutcomeFailed && e == nil) {
			t.Fatalf("%s status=%s result=%+v error=%v", label, outcome, result, e)
		}
		t.Logf("stage=%s elapsed=%s status=%s processed=%d requests=%d", label, time.Since(start), outcome, result.Processed, fixture.requests)
	}
	fixture.failOffset = 400
	fixture.failStatus = 503
	measure("failed_first", jobs.OutcomeFailed, 0)
	if countRows(t, ctx, pool, "parcels") != 0 {
		t.Fatal("failed capture published")
	}
	fixture.failStatus = 0
	measure("fresh", jobs.OutcomeSuccess, 500)
	if n := countRows(t, ctx, pool, "parcel_sales"); n != 1000 {
		t.Fatalf("sales=%d", n)
	}
	measure("unchanged", jobs.OutcomeSuccess, 500)
	fixture.features[0]["geometry"].(map[string]interface{})["rings"] = [][][]int{{{2, 0}, {2, 2}, {3, 2}, {3, 0}, {2, 0}}}
	delete(fixture.features[0]["attributes"].(map[string]interface{}), "s2_SALESAMT")
	delete(fixture.features[0]["attributes"].(map[string]interface{}), "s2_SALEDATE")
	measure("changed_geometry_and_omitted_sale", jobs.OutcomeSuccess, 500)
	if n := countRows(t, ctx, pool, "parcel_sales"); n != 999 {
		t.Fatalf("omitted sale not removed: %d", n)
	}
	anchor, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Rollback(context.Background())
	if _, err = anchor.Exec(ctx, `SELECT 1 FROM vacancy_source_revision WHERE county_id='union' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE parcels SET current_name_1='independent' WHERE source_parcel_id='perf-2'`,
		`UPDATE parcel_evidence_current SET normalized_hash=repeat('d',64) WHERE source_parcel_id='perf-3'`,
	} {
		statement := statement
		writer, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer writer.Rollback(context.Background())
		if _, e = writer.Exec(ctx, `SET LOCAL lock_timeout='4s'`); e != nil {
			t.Fatal(e)
		}
		var pid int
		if e = writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
			t.Fatal(e)
		}
		result := make(chan error, 1)
		go func() { _, e := writer.Exec(ctx, statement); result <- e }()
		waiting := false
		for !waiting && ctx.Err() == nil {
			if e = pool.QueryRow(ctx, `SELECT coalesce(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); e != nil {
				t.Fatal(e)
			}
			if !waiting {
				time.Sleep(5 * time.Millisecond)
			}
		}
		if !waiting {
			t.Fatal("independent writer did not reach revision lock")
		}
		if e = anchor.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		select {
		case e = <-result:
			if e != nil {
				t.Fatal("independent write deadlocked: ", e)
			}
		case <-ctx.Done():
			t.Fatal("independent write timed out")
		}
		if e = writer.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		anchor, e = pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = anchor.Exec(ctx, `SELECT 1 FROM vacancy_source_revision WHERE county_id='union' FOR UPDATE`); e != nil {
			t.Fatal(e)
		}
	}
	if err = anchor.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("independent parcel and evidence writers blocked on revision and completed without deadlock within 4s")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	for _, step := range []struct{ name, query string }{
		{"omitted_sale", `EXPLAIN (ANALYZE,BUFFERS) DELETE FROM parcel_sales WHERE parcel_id=(SELECT id FROM parcels WHERE source_parcel_id='perf-2') AND sequence=2`},
		{"fk_cascade", `EXPLAIN (ANALYZE,BUFFERS) DELETE FROM parcels WHERE source_parcel_id='perf-2'`},
	} {
		rows, e := tx.Query(ctx, step.query)
		if e != nil {
			t.Fatal(e)
		}
		var plan []string
		for rows.Next() {
			var line string
			if e = rows.Scan(&line); e != nil {
				break
			}
			plan = append(plan, line)
		}
		rows.Close()
		if e != nil {
			t.Fatal(e)
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		t.Logf("stage=%s plan=\n%s", step.name, strings.Join(plan, "\n"))
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("unmeasured_stages=network_capture,normalization,retention,shape_dissolve,derived_handoff; whole-stage includes these; no county-throughput inference")
}
