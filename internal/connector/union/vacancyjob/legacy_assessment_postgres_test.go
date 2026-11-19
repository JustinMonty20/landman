//go:build postgres

package vacancyjob

import (
	"context"
	"errors"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLegacyAssessmentOfflinePublicationProvenance(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	for _, sql := range []string{
		`TRUNCATE vacancy_live_assessments,vacancy_live_structures,vacancy_live_generations CASCADE`,
		`DELETE FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`,
		`DELETE FROM ingest_replay_attempts WHERE capture_id IN (SELECT id FROM ingest_captures WHERE source_name='union_assessed_value_0')`,
		`DELETE FROM ingest_captures WHERE source_name='union_assessed_value_0'`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := ingest.NewRecorder(store, "union_assessed_value_0", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	base := "https://example.test/server/rest/services/Property_Tax_Live/Assessed_Value/MapServer/0/query?"
	for _, receipt := range []struct{ url, body string }{
		{base + "f=json&returnCountOnly=true&where=1%3D1", `{"count":2}`},
		{base + "f=json&outFields=%2A&resultRecordCount=2000&returnGeometry=false&where=1%3D1", `{"features":[{"attributes":{"OBJECTID":7,"parcel_number":"001"}},{"attributes":{"OBJECTID":7,"parcel_number":"002"}}]}`},
	} {
		if err := recorder.RecordHTTP(ctx, receipt.url, "", 0, 200, true, strings.NewReader(receipt.body)); err != nil {
			t.Fatal(err)
		}
	}
	engine := ingest.Engine{Pool: pool, Store: store}
	var id string
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Run: func(ctx context.Context) (jobs.Result, error) {
		attempt, ok := jobs.AttemptID(ctx)
		if !ok {
			return jobs.Result{}, errors.New("missing attempt")
		}
		if err := recorder.BindAttempt(AssessedValueID, attempt, "full_scan", 0); err != nil {
			return jobs.Result{}, err
		}
		var err error
		id, err = engine.Seal(ctx, recorder)
		return jobs.Result{}, err
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Replay(ctx, id, AssessedValueID, &publisher{layer: union.AssessedValueLayer}); err != nil {
		t.Fatal(err)
	}
	var note string
	var count int
	if err := pool.QueryRow(ctx, `SELECT validation_note,observed_count FROM vacancy_live_generations WHERE capture_id=$1::uuid`, id).Scan(&note, &count); err != nil {
		t.Fatal(err)
	}
	if note != "bounded single-page assessment observation; county snapshot, partition membership, coverage and effective year unverified" || count != 2 {
		t.Fatalf("legacy publication provenance: %q count=%d", note, count)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_live_assessments WHERE object_id='7'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy duplicate feature rows lost: %d, %v", count, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vacancy_live_generations SET validation_note='bounded numeric-range assessment observation; county snapshot, query support, coverage and effective year unverified' WHERE capture_id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	var freshnessBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`).Scan(&freshnessBefore); err != nil {
		t.Fatal(err)
	}
	if err := engine.Replay(ctx, id, AssessedValueID, &publisher{layer: union.AssessedValueLayer}); err != nil {
		t.Fatal(err)
	}
	var freshnessAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`).Scan(&freshnessAfter); err != nil || freshnessAfter != freshnessBefore {
		t.Fatalf("replay advanced freshness: %d %d %v", freshnessBefore, freshnessAfter, err)
	}
	if err := pool.QueryRow(ctx, `SELECT validation_note,observed_count FROM vacancy_live_generations WHERE capture_id=$1::uuid`, id).Scan(&note, &count); err != nil || note != "bounded single-page assessment observation; county snapshot, partition membership, coverage and effective year unverified" || count != 2 {
		t.Fatalf("unchanged legacy replay provenance: %q count=%d err=%v", note, count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_live_assessments WHERE object_id='7'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("unchanged replay changed rows: %d %v", count, err)
	}
}
