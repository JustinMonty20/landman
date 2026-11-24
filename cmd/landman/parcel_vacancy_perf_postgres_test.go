//go:build postgres && perf

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
)

// A three-row local HTTP fixture is a correctness/timing diagnostic, not
// representative county throughput or a topology-trigger optimization proof.
func TestVacancySmallImportPipelineTimings(t *testing.T) {
	for _, failedFirst := range []bool{false, true} {
		name := "nonempty_first_empty_database"
		if failedFirst {
			name = "failed_first_retry_on_empty_database"
		}
		t.Run(name, func(t *testing.T) {
			ctx, dsn, pool := parcelFixtureDB(t)
			f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 2), feature(2, "B", "owner", true, 2), feature(3, "C", "owner", true, 2)}}
			source := f.source(t, 1)
			measure := func(stage string, want jobs.Outcome) {
				t.Helper()
				start := time.Now()
				out, result, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
				if out != want {
					t.Fatalf("%s outcome=%s result=%+v err=%v", stage, out, result, err)
				}
				t.Logf("stage=%s elapsed=%s status=%s processed=%d requests=%d", stage, time.Since(start), out, result.Processed, f.requests)
			}
			if failedFirst {
				f.failOffset = 2
				f.failStatus = 503
				measure("failed_first", jobs.OutcomeFailed)
				if n := countRows(t, ctx, pool, "parcels"); n != 0 {
					t.Fatal("failed import published", n)
				}
				f.failStatus = 0
			}
			measure(name, jobs.OutcomeSuccess)
			if n := countRows(t, ctx, pool, "parcel_sales"); n != 6 {
				t.Fatal("expected six initial sales", n)
			}
			measure("unchanged_rerun", jobs.OutcomeSuccess)
			f.features[0]["geometry"].(map[string]interface{})["rings"] = [][][]int{{{2, 0}, {2, 2}, {3, 2}, {3, 0}, {2, 0}}}
			delete(f.features[0]["attributes"].(map[string]interface{}), "s2_SALESAMT")
			delete(f.features[0]["attributes"].(map[string]interface{}), "s2_SALEDATE")
			measure("changed_geometry_and_omitted_sale", jobs.OutcomeSuccess)
			if n := countRows(t, ctx, pool, "parcel_sales"); n != 5 {
				t.Fatal("omitted sale not removed", n)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, `DELETE FROM parcel_sales WHERE parcel_id=(SELECT id FROM parcels WHERE source_parcel_id='B')`); err != nil {
				t.Fatal(err)
			}
			for _, step := range []struct{ name, sql string }{
				{"omitted_sale_delete", `EXPLAIN (ANALYZE,BUFFERS) DELETE FROM parcel_sales WHERE parcel_id=(SELECT id FROM parcels WHERE source_parcel_id='A') AND sequence=1`},
				{"fk_check", `EXPLAIN (ANALYZE,BUFFERS) DELETE FROM parcels WHERE source_parcel_id='B'`},
			} {
				rows, err := tx.Query(ctx, step.sql)
				if err != nil {
					t.Fatal(err)
				}
				var plans []string
				for rows.Next() {
					var line string
					if err = rows.Scan(&line); err != nil {
						t.Fatal(err)
					}
					plans = append(plans, line)
				}
				rows.Close()
				if err = rows.Err(); err != nil {
					t.Fatal(err)
				}
				t.Logf("stage=%s plan=\n%s", step.name, strings.Join(plans, "\n"))
			}
			t.Log("stages_not_individually_measured=network_capture,normalization,retention,shape_dissolve,derived_handoff; whole-stage timing includes them")
		})
	}
}
