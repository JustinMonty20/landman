//go:build postgres

package parcelstore_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOwnerLabelsBackfillAcrossBatches(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(context.Background(), `SELECT current_database()`).Scan(&db); err != nil || !(strings.HasPrefix(db, "landman_owner_test_") || db == "landman_parcelstore_test_ci") {
		t.Fatal("refusing non-task database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	prefix := fmt.Sprintf("owner-batch-%d-", time.Now().UnixNano())
	_, err = pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,current_name_1,mailing_state) SELECT 'union','union','union_county_gis',$1||n::text,'ACME INC','SC' FROM generate_series(1,501) n`, prefix)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM parcels WHERE source_parcel_id LIKE $1||'%'`, prefix)
	var before, after int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM parcel_owner_label_revision WHERE id`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	processed, total, err := parcelstore.BackfillOwnerLabels(ctx, pool)
	if err != nil || processed != 501 || total < 501 {
		t.Fatalf("batches: %d processed, %d total: %v", processed, total, err)
	}
	if err := pool.QueryRow(ctx, `SELECT revision FROM parcel_owner_label_revision WHERE id`).Scan(&after); err != nil || after != before+2 {
		t.Fatalf("expected one revision per 500-row transaction: %d -> %d: %v", before, after, err)
	}
	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM parcels p JOIN parcel_owner_labels l ON l.parcel_id=p.id WHERE p.source_parcel_id LIKE $1||'%' AND l.corporate_name_indicator='matched' AND l.out_of_state_mailing='yes'`, prefix).Scan(&count)
	if err != nil || count != 501 {
		t.Fatalf("classified %d: %v", count, err)
	}
}
