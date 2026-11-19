//go:build postgres

package vacancyjob

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAssessmentRangesAtomicAndOffline(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL missing")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(ctx, "SELECT current_database()").Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	for _, sql := range []string{
		`TRUNCATE vacancy_live_assessments,vacancy_live_structures,vacancy_live_generations CASCADE`,
		`DELETE FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`,
		`DELETE FROM ingest_replay_attempts WHERE capture_id IN (SELECT id FROM ingest_captures WHERE source_name='union_assessed_value_0')`,
		`DELETE FROM ingest_captures WHERE source_name='union_assessed_value_0'`,
	} {
		if _, err = pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	failLater := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		where := q.Get("where")
		left := where == "1=1" || where == union.AssessmentWhere(0, math.MaxInt64) || where == union.AssessmentWhere(0, math.MaxInt64/2)
		right := where == "1=1" || where == union.AssessmentWhere(0, math.MaxInt64) || where == union.AssessmentWhere(math.MaxInt64/2+1, math.MaxInt64)
		if !left && !right {
			http.Error(w, "unsupported", 400)
			return
		}
		n := 0
		if left {
			n += 1001
		}
		if right {
			n += 1001
		}
		if q.Get("returnCountOnly") == "true" {
			fmt.Fprintf(w, `{"count":%d}`, n)
			return
		}
		if failLater && right && !left {
			fmt.Fprint(w, `{"features":[]}`)
			return
		}
		fmt.Fprint(w, `{"features":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			oid := int64(1)
			if right && !left {
				oid = math.MaxInt64
			}
			fmt.Fprintf(w, `{"attributes":{"OBJECTID":%d,"parcel_number":"001","assess_category":"AV","market_building":0}}`, oid)
		}
		fmt.Fprint(w, `]}`)
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Source: source, Pool: pool, Artifacts: store, Layer: union.AssessedValueLayer}
	run := func() error {
		_, _, e := runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Run: job.Import})
		return e
	}
	var count int
	check := func(want int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_live_assessments`).Scan(&count); err != nil || count != want {
			t.Fatalf("rows=%d want=%d err=%v", count, want, err)
		}
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	check(2002)
	var original string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	failLater = true
	if err := run(); err == nil {
		t.Fatal("later partition failure accepted")
	}
	check(2002)
	var accepted string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`).Scan(&accepted); err != nil || accepted != original {
		t.Fatalf("failed capture changed cursor: %v", err)
	}
	before := requests
	if err := job.Replay(ctx, original); err != nil || requests != before {
		t.Fatalf("offline replay: %v requests=%d", err, requests-before)
	}
	failLater = false
	if err := run(); err != nil {
		t.Fatal(err)
	}
	before = requests
	if err := job.Replay(ctx, original); err != nil || requests != before {
		t.Fatalf("stale replay: %v requests=%d", err, requests-before)
	}
	check(2002)
}
