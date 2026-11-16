//go:build postgres

package taxstore_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
	"github.com/JustinMonty20/landman/internal/taxstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

const billingStoreURL = "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000"
const billingHeader = "property_key,source,year,TOTAL TAXES AND CHARGES,PROPERTY ADDRESS,Taxpayer Name 1,Taypayer Address 1,Taxpayer City,Taxpayer State,Taxpayer ZIP Code\n"

func billingDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name a disposable migrated database")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func billingRows(t *testing.T, contents string) ([]union.BillRow, taxsource.Meta, int) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("WKBILA-test.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "operator.zip")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := union.NewBillingFile(path, billingStoreURL, store)
	if err != nil {
		t.Fatal(err)
	}
	var rows []union.BillRow
	meta, err := list.Read(context.Background(), func(row union.BillRow) error { rows = append(rows, row); return nil })
	if err != nil {
		t.Fatal(err)
	}
	pp, err := list.Skipped(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}
	return rows, meta, pp
}

func TestSaveBill(t *testing.T) {
	pool := billingDatabase(t)
	ctx := context.Background()
	latest := taxstore.NewBills(pool)
	if _, ok, err := latest.Latest(ctx); err != nil || ok {
		t.Fatalf("empty billing latest: ok=%v err=%v", ok, err)
	}
	rows, meta, skipped := billingRows(t, billingHeader+
		"09336350,RE,2026,\"1,359.48\",100 SAMPLE ROAD,TEST OWNER,PO BOX 1,SAMPLE,NC,28000\n"+
		"07105038A,RE,2026,.65,200 SAMPLE ROAD,TEST HOUSE,PO BOX 2,SAMPLE,NC,28001\n"+
		"NOT-IN-PARCELS,RE,2026,1.00,OTHER,TEST PERSON,PO BOX 3,SAMPLE,NC,28002\n"+
		"407618,PP,2026,10.00,PERSONAL,TEST BUSINESS,PO BOX 4,SAMPLE,NC,28003\n")
	if skipped != 1 || meta.SHA256 != meta.Artifact.SHA256() || !meta.Artifact.Verified() {
		t.Fatalf("billing evidence=%+v skipped=%d", meta, skipped)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var vacantID, housedID string
	for i, item := range []struct {
		key    string
		vacant bool
		id     *string
	}{
		{rows[0].ParcelKey, true, &vacantID}, {rows[1].ParcelKey, false, &housedID},
	} {
		if err := tx.QueryRow(ctx, `INSERT INTO parcels (county_id, county_code, source_name, source_parcel_id, likely_vacant)
VALUES ('union','179','union_county_gis',$1,$2) RETURNING id::text`, item.key, item.vacant).Scan(item.id); err != nil {
			t.Fatalf("seed parcel %d: %v", i, err)
		}
	}
	var before int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	first, err := taxstore.SaveBill(ctx, tx, rows, meta, skipped)
	if err != nil || first.Saved != 2 || first.Deleted != 0 || first.SkippedPP != 1 || first.Unmatched != 1 {
		t.Fatalf("first bill save=%+v err=%v", first, err)
	}
	var id, parcelID, total, address, mailing, city, state, postal string
	var digest []byte
	if err := tx.QueryRow(ctx, `SELECT id::text,parcel_id::text,total_taxes::text,property_address,mailing_line,mailing_city,mailing_state,mailing_postal_code,file_sha256
FROM tax_bills WHERE parcel_id::text=$1`, vacantID).Scan(&id, &parcelID, &total, &address, &mailing, &city, &state, &postal, &digest); err != nil {
		t.Fatal(err)
	}
	if id == "" || parcelID != vacantID || total != "1359.48" || address != "100 SAMPLE ROAD" || mailing != "PO BOX 1" || city != "SAMPLE" || state != "NC" || postal != "28000" || len(digest) != sha256.Size || !bytes.Equal(digest, meta.SHA256[:]) {
		t.Fatalf("stored bill id=%s parcel=%s total=%s address=%q mailing=%q city=%q state=%q postal=%q hash=%d", id, parcelID, total, address, mailing, city, state, postal, len(digest))
	}
	if repeat, err := taxstore.SaveBill(ctx, tx, rows, meta, skipped); err != nil || repeat.Saved != 2 || repeat.Deleted != 0 {
		t.Fatalf("repeat bill save=%+v err=%v", repeat, err)
	}
	var after, housedRows, owedRows, evidence int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_bills WHERE parcel_id::text=$1`, housedID).Scan(&housedRows); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_owed WHERE parcel_id::text=$1`, housedID).Scan(&owedRows); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE county_id='union' AND source_name='union_billing' AND active`).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if after != before || housedRows != 1 || owedRows != 0 || evidence != 1 {
		t.Fatalf("parcels=%d/%d housed=%d owed=%d active=%d", after, before, housedRows, owedRows, evidence)
	}
	if _, ok, err := latest.Latest(ctx); err != nil || ok {
		t.Fatalf("uncommitted latest: ok=%v err=%v", ok, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := latest.Latest(ctx); err != nil || !ok || got.Before(meta.FetchedAt.Add(-time.Second)) {
		t.Fatalf("billing latest=%v ok=%v err=%v", got, ok, err)
	}
	newRows, newMeta, pp := billingRows(t, billingHeader+
		"09336350,RE,2026,2.00,100 SAMPLE ROAD,TEST OWNER,PO BOX 1,SAMPLE,NC,28000\n"+
		"NOT-IN-PARCELS,RE,2026,1.00,OTHER,TEST PERSON,PO BOX 3,SAMPLE,NC,28002\n")
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback(ctx)
	changed, err := taxstore.SaveBill(ctx, tx2, newRows, newMeta, pp)
	if err != nil || changed.Saved != 1 || changed.Deleted != 1 || changed.Unmatched != 1 {
		t.Fatalf("changed bill save=%+v err=%v", changed, err)
	}
	var retainedID, newTotal string
	if err := tx2.QueryRow(ctx, `SELECT id::text,total_taxes::text FROM tax_bills WHERE parcel_id::text=$1`, vacantID).Scan(&retainedID, &newTotal); err != nil || retainedID != id || newTotal != "2.00" {
		t.Fatalf("retained bill id=%s/%s total=%s err=%v", retainedID, id, newTotal, err)
	}
	var oldRows, imports, active int
	if err := tx2.QueryRow(ctx, `SELECT count(*) FROM tax_bills WHERE parcel_id::text=$1`, housedID).Scan(&oldRows); err != nil {
		t.Fatal(err)
	}
	if err := tx2.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE source_name='union_billing'`).Scan(&imports); err != nil {
		t.Fatal(err)
	}
	if err := tx2.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE source_name='union_billing' AND active`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if oldRows != 0 || imports != 2 || active != 1 {
		t.Fatalf("omitted bill rows=%d imports=%d active=%d", oldRows, imports, active)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM parcels WHERE id::text IN ($1,$2)`, vacantID, housedID)
		_, _ = pool.Exec(ctx, `DELETE FROM tax_file_imports WHERE source_name='union_billing' AND source_url=$1`, meta.URL)
	})
}

func TestSaveBillRelinksUnmatched(t *testing.T) {
	pool := billingDatabase(t)
	ctx := context.Background()
	rows, meta, pp := billingRows(t, billingHeader+"LATER-PARCEL,RE,2026,.65,ROAD,TEST PERSON,PO BOX 1,SAMPLE,NC,28000\n")
	save := func() taxstore.BillResult {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		result, err := taxstore.SaveBill(ctx, tx, rows, meta, pp)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if first := save(); first.Saved != 0 || first.Unmatched != 1 {
		t.Fatalf("initial unmatched bill=%+v", first)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE source_name='union_billing' AND active`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active import without bills=%d err=%v", active, err)
	}
	if _, ok, err := taxstore.NewBills(pool).Latest(ctx); err != nil || ok {
		t.Fatalf("unmatched billing latest: ok=%v err=%v", ok, err)
	}
	var parcelID string
	if err := pool.QueryRow(ctx, `INSERT INTO parcels (county_id, county_code, source_name, source_parcel_id, likely_vacant)
VALUES ('union','179','union_county_gis',$1,false) RETURNING id::text`, rows[0].ParcelKey).Scan(&parcelID); err != nil {
		t.Fatal(err)
	}
	if second := save(); second.Saved != 1 || second.Unmatched != 0 || second.Deleted != 0 {
		t.Fatalf("relinked bill=%+v", second)
	}
	var imported int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_bills WHERE parcel_id::text=$1`, parcelID).Scan(&imported); err != nil || imported != 1 {
		t.Fatalf("relinked row=%d err=%v", imported, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE source_name='union_billing' AND active`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("relinked active evidence=%d err=%v", active, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM parcels WHERE id::text=$1`, parcelID)
		_, _ = pool.Exec(ctx, `DELETE FROM tax_file_imports WHERE source_name='union_billing' AND source_url=$1`, meta.URL)
	})
}

func TestSaveBillRejectsInvalidInput(t *testing.T) {
	pool := billingDatabase(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Put(ctx, "union_billing", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	meta := taxsource.Meta{URL: billingStoreURL, Name: "union_billing", SHA256: artifact.SHA256(), FetchedAt: time.Now().UTC(), Artifact: artifact}
	row := union.BillRow{ParcelKey: "09336350", Year: 2026, TotalTaxes: 100}
	invalid := meta
	invalid.Name = "union_delinquent"
	if _, err := taxstore.SaveBill(ctx, tx, []union.BillRow{row}, invalid, 0); err == nil {
		t.Fatal("accepted wrong source")
	}
	if _, err := taxstore.SaveBill(ctx, tx, []union.BillRow{row}, meta, -1); err == nil {
		t.Fatal("accepted negative PP skip count")
	}
	for _, bad := range []union.BillRow{{Year: 2026, TotalTaxes: 100}, {ParcelKey: "09336350", Year: 0, TotalTaxes: 100}, {ParcelKey: "09336350", Year: 2026, TotalTaxes: -1}, {ParcelKey: "09336350", Year: 2026, TotalTaxes: 100000000000000}} {
		if _, err := taxstore.SaveBill(ctx, tx, []union.BillRow{bad}, meta, 0); err == nil {
			t.Fatalf("accepted invalid bill %+v", bad)
		}
	}
	badMeta := meta
	badMeta.FetchedAt = meta.FetchedAt.Add(time.Second)
	if _, err := taxstore.SaveImportEvidence(ctx, tx, taxstore.ImportEvidence{CountyID: county.Union, SourceName: badMeta.Name, SourceURL: badMeta.URL, FetchedAt: badMeta.FetchedAt, Artifact: artifact, Validation: taxstore.ImportInvalid}); err != nil {
		t.Fatal(err)
	}
	if _, err := taxstore.SaveBill(ctx, tx, nil, badMeta, 0); err == nil {
		t.Fatal("accepted invalid existing import")
	}
	if _, ok, err := taxstore.NewBills(nil).Latest(ctx); err == nil || ok {
		t.Fatalf("accepted missing database: ok=%v err=%v", ok, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, ok, err := taxstore.NewBills(pool).Latest(cancelled); err == nil || ok {
		t.Fatalf("accepted canceled latest: ok=%v err=%v", ok, err)
	}
	if _, err := taxstore.SaveBill(cancelled, tx, nil, meta, 0); err == nil {
		t.Fatal("accepted canceled evidence query")
	}
}
