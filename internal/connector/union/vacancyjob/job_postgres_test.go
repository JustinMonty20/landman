//go:build postgres

package vacancyjob

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAssessmentCaptureReplayAndFailures(t *testing.T) {
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
	var name string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	if _, err = pool.Exec(ctx, `TRUNCATE vacancy_live_assessments,vacancy_live_structures,vacancy_live_generations CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM ingest_replay_attempts WHERE capture_id IN (SELECT id FROM ingest_captures WHERE source_name='union_assessed_value_0')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM ingest_captures WHERE source_name='union_assessed_value_0'`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	version := 0
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("returnCountOnly") == "true" {
			fmt.Fprint(w, `{"count":2}`)
			return
		}
		switch version {
		case 0:
			fmt.Fprint(w, `{"features":[{"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":0}},{"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":0}}]}`)
		case 1:
			fmt.Fprint(w, `{"features":[{"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":1}},{"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"PY","market_building":0}}]}`)
		default:
			fmt.Fprint(w, `{"features":[]}`)
		}
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Source: source, Pool: pool, Artifacts: artifacts, Layer: union.AssessedValueLayer}
	run := func() error {
		_, _, err := runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Run: job.Import})
		return err
	}
	query := func(sql string) int {
		var n int
		if err := pool.QueryRow(ctx, sql).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_assessments`) != 2 {
		t.Fatal("lost duplicate OIDs")
	}
	var older string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_live_generations ORDER BY retrieved_at LIMIT 1`).Scan(&older); err != nil {
		t.Fatal(err)
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 1 || query(`SELECT count(*) FROM ingest_captures WHERE source_name='union_assessed_value_0'`) != 2 {
		t.Fatal("repeat rewrote evidence or lost observation")
	}
	version = 2
	if err := run(); err == nil {
		t.Fatal("partial capture accepted")
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 1 {
		t.Fatal("failed capture published")
	}
	version = 1
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 2 {
		t.Fatal("changed capture not published")
	}
	requestsBefore := requests
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, older) }})
	if err != nil || requests != requestsBefore || query(`SELECT count(*) FROM vacancy_live_generations`) != 2 {
		t.Fatalf("older replay changed evidence or contacted HTTP: %v", err)
	}
	var manifest []byte
	if err := pool.QueryRow(ctx, `SELECT manifest FROM ingest_captures WHERE id=$1::uuid`, older).Scan(&manifest); err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Receipts []struct {
			ArtifactKey string `json:"artifact_key"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(manifest, &receipt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(receipt.Receipts[1].ArtifactKey)), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, older) }})
	if err == nil || query(`SELECT count(*) FROM vacancy_live_generations`) != 2 || requests != requestsBefore {
		t.Fatalf("corrupt replay accepted or contacted HTTP: %v", err)
	}
}
