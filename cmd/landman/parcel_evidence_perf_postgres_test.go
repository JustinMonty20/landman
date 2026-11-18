//go:build postgres && perf

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
)

// This synthetic fixture measures the actual local HTTP capture, disk writes,
// grouped publication, omitted-sale delete and FK checks on a migrated schema.
func TestParcelEvidenceLocalFixtureBudget(t *testing.T) {
	for _, failedFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed_first=%v", failedFirst), func(t *testing.T) {
			ctx, dsn, pool := parcelFixtureDB(t)
			fixture := &parcelHTTPFixture{}
			for i := 1; i <= 500; i++ {
				fixture.features = append(fixture.features, feature(i, fmt.Sprintf("P-%d", i), "owner", true, 2))
			}
			source := fixture.source(t, 100)
			if failedFirst {
				fixture.failOffset = 400
				fixture.failStatus = 503
				if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err == nil || outcome != jobs.OutcomeFailed {
					t.Fatal(outcome, err)
				}
				if n := countRows(t, ctx, pool, "parcels"); n != 0 {
					t.Fatalf("failed first import published %d rows", n)
				}
				fixture.failStatus = 0
			}
			start := time.Now()
			outcome, result, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
			elapsed := time.Since(start)
			if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != 500 {
				t.Fatal(outcome, result, err)
			}
			if got := countRows(t, ctx, pool, "parcel_sales"); got != 1000 {
				t.Fatalf("omitted sales: got %d", got)
			}
			if got := countRows(t, ctx, pool, "parcel_evidence_current"); got != 500 {
				t.Fatalf("missing published fingerprints: %d", got)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			for _, entry := range []struct{ label, query string }{
				{"omitted_sales", `EXPLAIN (ANALYZE,BUFFERS) DELETE FROM parcel_sales WHERE parcel_id=(SELECT id FROM parcels LIMIT 1) AND NOT (sequence=ANY(ARRAY[1]::smallint[]))`},
				{"remaining_sales", `EXPLAIN (ANALYZE,BUFFERS) DELETE FROM parcel_sales WHERE parcel_id=(SELECT id FROM parcels LIMIT 1)`},
				{"fk_checks", `EXPLAIN (ANALYZE,BUFFERS) DELETE FROM parcels WHERE id=(SELECT id FROM parcels LIMIT 1)`},
			} {
				label, query := entry.label, entry.query
				rows, err := tx.Query(ctx, query)
				if err != nil {
					t.Fatal(err)
				}
				var plan []string
				for rows.Next() {
					var line string
					if err = rows.Scan(&line); err != nil {
						t.Fatal(err)
					}
					plan = append(plan, line)
				}
				rows.Close()
				if err = rows.Err(); err != nil {
					t.Fatal(err)
				}
				t.Logf("%s plan:\n%s", label, strings.Join(plan, "\n"))
			}
			t.Logf("synthetic 500-part local import took %s; status=%s parts=%d requests=%d", elapsed, outcome, result.Processed, fixture.requests)
		})
	}
}
