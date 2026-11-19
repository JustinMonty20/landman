//go:build postgres

package vacancy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFixtureImportAtomicAndRepeatable(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var database string
	if err = pool.QueryRow(context.Background(), `SELECT current_database()`).Scan(&database); err != nil || !strings.HasPrefix(database, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	if _, err = pool.Exec(context.Background(), `TRUNCATE vacancy_assessments,vacancy_structures,vacancy_assessment_rows,vacancy_evidence_generations CASCADE`); err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	artifacts := filepath.Join(dir, "private")
	path := filepath.Join(dir, "snapshot.json")
	fixture := `{"manifest":{"structure_url":"https://example.test/171","assessment_url":"https://example.test/0","retrieved_at":"2026-11-15T00:00:00Z","geometry_srid":4326,"expected_count":2,"validation_note":"offline test; coverage unverified","provenance":"unverified_fixture"},"structures":[{"id":"outline-1","pin":"001","status":"existing","geometry":{"type":"Polygon","coordinates":[[[-80,35],[-79.999,35],[-79.999,35.001],[-80,35]]]}}],"assessments":[{"id":"row-1","parcel_number":"001","year":2027,"category":"AV","building_value":100}]}`
	run := func() (jobs.Outcome, error) {
		outcome, _, err := runner.Run(context.Background(), jobs.Definition{ID: JobID, Run: func(ctx context.Context) (jobs.Result, error) { return Import(ctx, pool, path, artifacts) }})
		return outcome, err
	}
	if err = os.WriteFile(path, []byte(strings.Replace(fixture, `"expected_count":2`, `"expected_count":3`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = run(); err == nil {
		t.Fatal("accepted mismatched count")
	}
	var count int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM vacancy_evidence_generations`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial generation: %d %v", count, err)
	}
	var capturesBefore int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM ingest_captures WHERE source_name=$1 AND published_at IS NOT NULL`, captureSource).Scan(&capturesBefore); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if outcome, err := run(); err != nil || outcome != jobs.OutcomeSuccess {
			t.Fatalf("run %d: %s %v", i, outcome, err)
		}
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM vacancy_evidence_generations WHERE coverage_verified=false`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("generation count: %d %v", count, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM vacancy_structures`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("structure count: %d %v", count, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM vacancy_assessment_rows`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("assessment count: %d %v", count, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM vacancy_evidence_generations WHERE (source_name='vacancy_fixture_structures' AND source_url='https://example.test/171') OR (source_name='vacancy_fixture_assessed_value' AND source_url='https://example.test/0')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("fixture provenance: %d %v", count, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM ingest_captures WHERE source_name=$1 AND published_at IS NOT NULL`, captureSource).Scan(&count); err != nil || count != capturesBefore+2 {
		t.Fatalf("retained published captures after repeat: %d before %d: %v", count, capturesBefore, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM vacancy_assessments`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unverified evidence classified parcels: %d %v", count, err)
	}
	broken := strings.Replace(fixture, `[-80,35],[-79.999,35],[-79.999,35.001],[-80,35]`, `[-80,35],[-79.999,35.001],[-79.999,35],[-80,35.001],[-80,35]`, 1)
	if err = os.WriteFile(path, []byte(broken), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = run(); err == nil {
		t.Fatal("accepted invalid stored geometry")
	}
	for table, want := range map[string]int{"vacancy_evidence_generations": 2, "vacancy_structures": 1, "vacancy_assessment_rows": 1} {
		if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != want {
			t.Fatalf("after atomic failure %s: %d %v", table, count, err)
		}
	}
}
