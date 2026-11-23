//go:build postgres

package vacancy

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
)

func TestValidationRefusesFixtureAndUnpublishedCapture(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("requires disposable test database", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var fixture string
	if err := tx.QueryRow(ctx, `INSERT INTO vacancy_evidence_generations(source_name,source_url,artifact_sha256,retrieved_at,observed_count,validation_note) VALUES('vacancy_fixture_structures','https://example.test/fixture',repeat('a',64),now(),1,'fixture') RETURNING id::text`).Scan(&fixture); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO vacancy_live_validation(generation_id,capture_id,source_name,normalized_sha256,source_url,terms_evidence,coverage_evidence,count_evidence,snapshot_evidence,source_as_of,status_evidence) VALUES($1,$1,'union_structures_171',repeat('a',64),'https://example.test/171','terms','county','one','scan',current_date,'status')`, fixture)
	if err == nil {
		t.Fatal("fixture generation admitted as live")
	}
}
