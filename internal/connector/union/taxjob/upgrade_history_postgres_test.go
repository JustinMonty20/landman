//go:build postgres

package taxjob

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs the membership migration against disposable v34 tables before any post-upgrade import.
func TestForeclosureMembershipUpgradePreservesSourceHistory(t *testing.T) {
	dsn := os.Getenv("TAX_UPGRADE_TEST_DSN")
	if os.Getenv("TAX_UPGRADE_TEST_DISPOSABLE") != "1" || !strings.Contains(dsn, "/taxworker_upgrade_") {
		t.Skip("requires explicitly guarded disposable v34 upgrade database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var version int
	if err := tx.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 34 {
		t.Fatalf("expected v34 before membership upgrade: %d %v", version, err)
	}
	// Seed a pre-upgrade active import, an inactive case, and genuine prior trigger history.
	stamp := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	var activeID, retiredID string
	for _, entry := range []struct {
		number    string
		id        *string
		fetchedAt time.Time
	}{{"UPGRADE-ACTIVE", &activeID, stamp}, {"UPGRADE-RETIRED", &retiredID, stamp.Add(-time.Hour)}} {
		if err := tx.QueryRow(ctx, `INSERT INTO tax_foreclosure_cases(county_id,source_name,case_number,parcel_set_sha256,legal_sha256,source_url,file_sha256,fetched_at) VALUES ('union','upgrade_history_test',$1,decode(repeat('a',64),'hex'),decode(repeat('b',64),'hex'),'fixture',decode(repeat('c',64),'hex'),$2) RETURNING id::text`, entry.number, entry.fetchedAt).Scan(entry.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tax_file_imports(county_id,source_name,source_url,fetched_at,byte_size,file_sha256,artifact_key,storage_mode,validation_result,active) VALUES ('union','upgrade_history_test','fixture',$1,0,decode(repeat('c',64),'hex'),'tax/upgrade_history_test/sha256/'||repeat('c',64),'local','valid',true)`, stamp); err != nil {
		t.Fatal(err)
	}
	var before int
	var originalHistory string
	if err := tx.QueryRow(ctx, `SELECT count(*), md5(jsonb_agg(to_jsonb(h) ORDER BY h.id)::text) FROM tax_ingest_history h WHERE record_id IN ($1::uuid,$2::uuid)`, activeID, retiredID).Scan(&before, &originalHistory); err != nil || before != 2 {
		t.Fatalf("genuine history before upgrade: %d %v", before, err)
	}
	migration, err := os.ReadFile("../../../../db/migrations/000035_foreclosure_active_membership.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.TrimPrefix(strings.Split(string(migration), "-- +goose Down")[0], "-- +goose Up")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	// Assert immediately after the upgrade, before any import can create legitimate history.
	var active, retired bool
	if err := tx.QueryRow(ctx, `SELECT active FROM tax_foreclosure_cases WHERE id=$1`, activeID).Scan(&active); err != nil || !active {
		t.Fatalf("active membership: %t %v", active, err)
	}
	if err := tx.QueryRow(ctx, `SELECT active FROM tax_foreclosure_cases WHERE id=$1`, retiredID).Scan(&retired); err != nil || retired {
		t.Fatalf("retired membership: %t %v", retired, err)
	}
	var after int
	var retainedHistory string
	if err := tx.QueryRow(ctx, `SELECT count(*), md5(jsonb_agg(to_jsonb(h) ORDER BY h.id)::text) FROM tax_ingest_history h WHERE record_id IN ($1::uuid,$2::uuid)`, activeID, retiredID).Scan(&after, &retainedHistory); err != nil || after != before || retainedHistory != originalHistory {
		t.Fatalf("upgrade altered source history: %d -> %d, fingerprint %s -> %s (%v)", before, after, originalHistory, retainedHistory, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE tax_foreclosure_cases SET status='confirmed' WHERE id=$1`, activeID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_ingest_history WHERE record_id=$1`, activeID).Scan(&after); err != nil || after != 2 {
		t.Fatalf("history trigger not restored: %d %v", after, err)
	}
}
