//go:build postgres

package taxstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSaveForeclosure(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name a disposable migrated database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	latest := taxstore.NewForeclosures(pool)
	if _, ok, err := latest.Latest(ctx); err != nil || ok {
		t.Fatalf("empty latest: ok=%v err=%v", ok, err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := union.NewForeclosureList(filepath.Join("..", "connector", "union", "testdata", "tax", "foreclosure.pdf"), store)
	if err != nil {
		t.Fatal(err)
	}
	var sales []union.ForeclosureCase
	meta, err := list.Read(ctx, func(sale union.ForeclosureCase) error { sales = append(sales, sale); return nil })
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := list.Skipped(ctx, meta)
	if err != nil || skipped != 1 {
		t.Fatalf("skipped=%d err=%v", skipped, err)
	}
	var ids []string
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for i, pin := range sales[0].PINs {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO parcels (county_id,county_code,source_name,source_parcel_id,likely_vacant)
VALUES ('union','179','union_county_gis',$1,$2) RETURNING id::text`, pin.ParcelKey, i != 1).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	var secondID string
	if err := tx.QueryRow(ctx, `INSERT INTO parcels (county_id,county_code,source_name,source_parcel_id,likely_vacant)
VALUES ('union','179','union_county_gis',$1,false) RETURNING id::text`, sales[2].PINs[0].ParcelKey).Scan(&secondID); err != nil {
		t.Fatal(err)
	}
	ids = append(ids, secondID)
	var before int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	first, err := taxstore.SaveForeclosure(ctx, tx, sales, meta, skipped)
	if err != nil || first.SavedCases != 3 || first.SavedJoins != 4 || first.SkippedBlocks != 1 || first.Unmatched != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	var numberedID, nullID, firstNumber, bid string
	if err := tx.QueryRow(ctx, `SELECT id::text,opening_bid::text FROM tax_foreclosure_cases WHERE case_number=$1`, sales[0].CaseNumber).Scan(&numberedID, &bid); err != nil || bid != "3800.00" {
		t.Fatalf("case id=%s bid=%s err=%v", numberedID, bid, err)
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_cases WHERE case_number IS NULL AND owner_name=$1`, sales[2].OwnerName).Scan(&nullID); err != nil {
		t.Fatal(err)
	}
	if repeat, err := taxstore.SaveForeclosure(ctx, tx, sales, meta, skipped); err != nil || repeat.SavedCases != 3 || repeat.SavedJoins != 4 {
		t.Fatalf("repeat=%+v err=%v", repeat, err)
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_cases WHERE case_number=$1`, sales[0].CaseNumber).Scan(&firstNumber); err != nil || firstNumber != numberedID {
		t.Fatalf("repeat id=%s/%s err=%v", firstNumber, numberedID, err)
	}
	var nullCases, joins, after, owed int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_cases WHERE case_number IS NULL`).Scan(&nullCases); err != nil {
		t.Fatal(err)
	}
	if nullCases != 2 {
		t.Fatalf("TBD cases=%d", nullCases)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_parcels WHERE case_id=$1`, numberedID).Scan(&joins); err != nil || joins != 3 {
		t.Fatalf("joins=%d err=%v", joins, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_owed WHERE parcel_id=$1`, ids[0]).Scan(&owed); err != nil || owed != 0 {
		t.Fatalf("foreclosure changed tax_owed=%d err=%v", owed, err)
	}
	// A later numbered case does not absorb the earlier null-number identity.
	numbered := sales[2]
	numbered.CaseNumber = "26M000998-890"
	if result, err := taxstore.SaveForeclosure(ctx, tx, []union.ForeclosureCase{numbered}, meta, skipped); err != nil || result.SavedCases != 1 || result.SavedJoins != 1 || result.Unmatched != 0 {
		t.Fatalf("new number=%+v err=%v", result, err)
	}
	var sameNull int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_cases WHERE id=$1 AND case_number IS NULL`, nullID).Scan(&sameNull); err != nil || sameNull != 1 {
		t.Fatalf("old TBD id=%d err=%v", sameNull, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels`).Scan(&after); err != nil || before != after {
		t.Fatalf("parcels before=%d after=%d err=%v", before, after, err)
	}
	if _, ok, err := latest.Latest(ctx); err != nil || ok {
		t.Fatalf("uncommitted latest: ok=%v err=%v", ok, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := latest.Latest(ctx); err != nil || !ok {
		t.Fatalf("latest: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM parcels WHERE id::text = ANY($1)`, ids)
		_, _ = pool.Exec(ctx, `DELETE FROM tax_foreclosure_cases WHERE county_id='union' AND source_name='union_foreclosure_sales' AND file_sha256=$1`, meta.SHA256[:])
		_, _ = pool.Exec(ctx, `DELETE FROM tax_file_imports WHERE county_id='union' AND source_name='union_foreclosure_sales' AND source_url=$1`, meta.URL)
	})
}

func TestSaveForeclosureRejectsInvalidEvidence(t *testing.T) {
	if _, ok, err := taxstore.NewForeclosures(nil).Latest(context.Background()); err == nil || ok {
		t.Fatalf("nil database ok=%v err=%v", ok, err)
	}
}
