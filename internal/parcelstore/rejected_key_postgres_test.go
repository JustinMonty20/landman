//go:build postgres

package parcelstore

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOversizedQuarantinedPIDNeverRequiresBtreeKey(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable API_TEST_DATABASE_URL required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.ConnConfig.Database, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || db != cfg.ConnConfig.Database {
		t.Fatal("database mismatch", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	writer, err := NewShapeWriterTx(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	// Unique digits prevent PostgreSQL's index compression from hiding the oversized key.
	var key strings.Builder
	for i := 0; i < 4096; i++ {
		key.WriteByte(byte('0' + (i*17+i*i)%10))
	}
	if err = writer.RejectPID(ctx, key.String(), false); err != nil {
		t.Fatal(err)
	}
	if err = writer.RejectPID(ctx, key.String(), true); err != nil {
		t.Fatal(err)
	}
	if err = writer.ObserveIdentity(ctx, "7", key.String()); err != nil {
		t.Fatal(err)
	}
	var refused, invalid bool
	if err = tx.QueryRow(ctx, `SELECT source_parcel_id=$1,invalid_geometry FROM parcel_import_rejected WHERE source_parcel_hash=digest($1,'sha256')`, key.String()).Scan(&refused, &invalid); err != nil || !refused || !invalid {
		t.Fatalf("lost rejected identity or geometry flag: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO parcel_source_object_identity(county_id,source_name,raw_key,source_parcel_id,observed_at)
 SELECT 'union','union_county_gis',raw_key,source_parcel_id,now() FROM parcel_import_identities
 WHERE octet_length(source_parcel_id)<=2048`); err != nil {
		t.Fatal(err)
	}
	if err = writer.ObserveIdentity(ctx, "8", "fallback-8"); err != nil {
		t.Fatal("valid identity after oversized rejection: ", err)
	}
}
