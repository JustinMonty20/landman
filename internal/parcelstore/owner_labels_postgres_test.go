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

func TestOwnerLabelsRefreshRollbackAndConstraints(t *testing.T) {
	ctx, tx := testTx(t)
	parcel := unionParcel(t)
	sc, llc := "SC", "ACME LLC"
	parcel.Mailing.State = &sc
	parcel.Owner.CurrentName1 = &llc
	id, err := parcelstore.SaveParcel(ctx, tx, parcel)
	if err != nil {
		t.Fatal(err)
	}
	check := func(mailing, entity, corp string) {
		t.Helper()
		var a, b, c string
		err := tx.QueryRow(ctx, `SELECT out_of_state_mailing,entity_name_indicator,corporate_name_indicator FROM parcel_owner_labels WHERE parcel_id=$1`, id).Scan(&a, &b, &c)
		if err != nil || a != mailing || b != entity || c != corp {
			t.Fatalf("labels %q %q %q: %v", a, b, c, err)
		}
	}
	check("yes", "matched", "not_detected")
	if _, err = parcelstore.SaveParcel(ctx, tx, parcel); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM parcel_owner_labels WHERE parcel_id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rerun rows %d: %v", count, err)
	}
	nc, corp := "NC", "OTHER INC."
	parcel.Mailing.State = &nc
	parcel.Owner.CurrentName1 = &corp
	if _, err = parcelstore.SaveParcel(ctx, tx, parcel); err != nil {
		t.Fatal(err)
	}
	check("no", "matched", "matched")
	parcel.Mailing.State = nil
	parcel.Owner.CurrentName1 = nil
	if _, err = parcelstore.SaveParcel(ctx, tx, parcel); err != nil {
		t.Fatal(err)
	}
	check("unknown", "unknown", "unknown")
	var suffix, state *string
	if err = tx.QueryRow(ctx, `SELECT matched_suffix,mailing_state FROM parcel_owner_labels WHERE parcel_id=$1`, id).Scan(&suffix, &state); err != nil || suffix != nil || state != nil {
		t.Fatalf("stale evidence %v %v %v", suffix, state, err)
	}
	_, err = tx.Exec(ctx, `SAVEPOINT invalid_owner_label`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE parcel_owner_labels SET corporate_name_indicator='matched' WHERE parcel_id=$1`, id); err == nil {
		t.Fatal("accepted inconsistent corporation")
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_owner_label`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE parcel_owner_labels SET out_of_state_mailing='resident' WHERE parcel_id=$1`, id); err == nil {
		t.Fatal("accepted invalid status")
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_owner_label`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE parcel_owner_labels SET out_of_state_mailing='yes',mailing_state='XX' WHERE parcel_id=$1`, id); err == nil {
		t.Fatal("accepted unsupported mailing state")
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_owner_label`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE parcel_owner_labels SET matched_suffix='LLC',matched_owner_line=NULL WHERE parcel_id=$1`, id); err == nil {
		t.Fatal("accepted partial name evidence")
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_owner_label`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM parcel_owner_labels WHERE parcel_id=$1`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade rows %d: %v", count, err)
	}
}

func TestOwnerLabelsBackfillStoredSnapshots(t *testing.T) {
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
	id := fmt.Sprintf("owner-backfill-%d", time.Now().UnixNano())
	var parcelID string
	err = pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,current_name_1,mailing_state) VALUES('union','union','union_county_gis',$1,'ACME LLC','SC') RETURNING id::text`, id).Scan(&parcelID)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, parcelID)
	n, total, err := parcelstore.BackfillOwnerLabels(ctx, pool)
	if err != nil || n < 1 || total < 1 {
		t.Fatalf("backfill %d %d %v", n, total, err)
	}
	n, _, err = parcelstore.BackfillOwnerLabels(ctx, pool)
	if err != nil || n != 0 {
		t.Fatalf("repeat backfill %d %v", n, err)
	}
	var suffix string
	err = pool.QueryRow(ctx, `SELECT matched_suffix FROM parcel_owner_labels l JOIN parcels p ON p.id=l.parcel_id WHERE p.id=$1 AND out_of_state_mailing='yes' AND entity_name_indicator='matched'`, parcelID).Scan(&suffix)
	if err != nil || suffix != "LLC" {
		t.Fatalf("joined label %q: %v", suffix, err)
	}
}
