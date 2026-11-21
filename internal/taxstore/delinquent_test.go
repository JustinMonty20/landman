//go:build postgres

package taxstore_test

import (
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

func TestSaveDelinquent(t *testing.T) {
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
	latest := taxstore.NewDelinquentBalances(pool)
	if _, ok, err := latest.Latest(ctx); err != nil || ok {
		t.Fatalf("empty latest: ok=%v err=%v", ok, err)
	}
	data, err := os.ReadFile(filepath.Join("..", "connector", "union", "testdata", "tax", "delinquent.csv"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "operator.csv")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(directory, "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := union.NewDelinquentList(path, store)
	if err != nil {
		t.Fatal(err)
	}
	var rows []union.DelinquentRow
	meta, err := list.Read(ctx, func(row union.DelinquentRow) error { rows = append(rows, row); return nil })
	if err != nil {
		t.Fatal(err)
	}
	pp, rr, err := list.Skipped(ctx, meta)
	if err != nil || pp != 1 || rr != 1 {
		t.Fatalf("skipped pp=%d rr=%d err=%v", pp, rr, err)
	}
	if meta.SHA256 != sha256.Sum256(data) || !meta.Artifact.Verified() {
		t.Fatal("source did not retain verified bytes")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var vacantID, housedID string
	for i, entry := range []struct {
		key    string
		vacant bool
		id     *string
	}{
		{rows[0].ParcelKey, true, &vacantID}, {rows[1].ParcelKey, false, &housedID},
	} {
		if err := tx.QueryRow(ctx, `INSERT INTO parcels (county_id, county_code, source_name, source_parcel_id, likely_vacant)
VALUES ('union','179','union_county_gis',$1,$2) RETURNING id::text`, entry.key, entry.vacant).Scan(entry.id); err != nil {
			t.Fatalf("seed parcel %d: %v", i, err)
		}
	}
	var before int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	rows = append(rows, union.DelinquentRow{ParcelKey: "NO-SUCH-PARCEL", Year: 2025, Balance: 100, PrimarySource: "RE"})
	first, err := taxstore.SaveDelinquent(ctx, tx, rows, meta, pp, rr)
	if err != nil || first.Saved != 2 || first.Unmatched != 3 || first.SkippedPP != 1 || first.SkippedRR != 1 {
		t.Fatalf("first save=%+v err=%v", first, err)
	}
	var id, parcelID, source string
	var balance string
	var digest []byte
	if err := tx.QueryRow(ctx, `SELECT id::text,parcel_id::text,balance_due::text,source_name,file_sha256
FROM tax_delinquent_balances WHERE parcel_id=$1`, vacantID).Scan(&id, &parcelID, &balance, &source, &digest); err != nil {
		t.Fatal(err)
	}
	if id == "" || parcelID != vacantID || balance != "14355.02" || source != "union_delinquent" || len(digest) != 32 {
		t.Fatalf("stored row id=%s parcel=%s balance=%s source=%s digest=%d", id, parcelID, balance, source, len(digest))
	}
	if result, err := taxstore.SaveDelinquent(ctx, tx, rows, meta, pp, rr); err != nil || result.Saved != 2 {
		t.Fatalf("identical save=%+v err=%v", result, err)
	}
	var findings int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_unmatched_records u JOIN tax_file_imports i ON i.id=u.import_id WHERE i.source_name='union_delinquent' AND i.fetched_at=$1`, meta.FetchedAt).Scan(&findings); err != nil || findings != 3 {
		t.Fatalf("retained unmatched findings=%d error=%v", findings, err)
	}
	rows[0].Balance = 111
	if result, err := taxstore.SaveDelinquent(ctx, tx, rows, meta, pp, rr); err != nil || result.Saved != 2 {
		t.Fatalf("replace save=%+v err=%v", result, err)
	}
	var afterID, newBalance string
	if err := tx.QueryRow(ctx, `SELECT id::text,balance_due::text FROM tax_delinquent_balances WHERE parcel_id=$1`, vacantID).Scan(&afterID, &newBalance); err != nil {
		t.Fatal(err)
	}
	if afterID != id || newBalance != "1.11" {
		t.Fatalf("changed identity=%s balance=%s", afterID, newBalance)
	}
	var countAfter, housedRows, owedVacant, owedHoused, evidence int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels`).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_delinquent_balances WHERE parcel_id=$1`, housedID).Scan(&housedRows); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_owed WHERE parcel_id=$1`, vacantID).Scan(&owedVacant); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_owed WHERE parcel_id=$1`, housedID).Scan(&owedHoused); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE source_name='union_delinquent' AND active`).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if countAfter != before || housedRows != 1 || owedVacant != 1 || owedHoused != 0 || evidence != 1 {
		t.Fatalf("parcels=%d/%d housed=%d owed=%d/%d evidence=%d", countAfter, before, housedRows, owedVacant, owedHoused, evidence)
	}
	if _, ok, err := latest.Latest(ctx); err != nil || ok {
		t.Fatalf("uncommitted latest: ok=%v err=%v", ok, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := latest.Latest(ctx); err != nil || !ok || got.Before(meta.FetchedAt.Add(-time.Second)) {
		t.Fatalf("latest=%v ok=%v err=%v", got, ok, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM parcels WHERE id::text IN ($1,$2)`, vacantID, housedID)
		_, _ = pool.Exec(ctx, `DELETE FROM tax_file_imports WHERE source_name='union_delinquent' AND source_url=$1`, meta.URL)
	})
}

func TestSaveDelinquentReplacesVersionAndRelinks(t *testing.T) {
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
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	readFile := func(body string) ([]union.DelinquentRow, taxsource.Meta) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "operator.csv")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		list, err := union.NewDelinquentList(path, store)
		if err != nil {
			t.Fatal(err)
		}
		var rows []union.DelinquentRow
		meta, err := list.Read(ctx, func(row union.DelinquentRow) error { rows = append(rows, row); return nil })
		if err != nil {
			t.Fatal(err)
		}
		return rows, meta
	}
	firstRows, firstMeta := readFile("Property Key,Year,Primary Source,Balance Due\n09125043,2022,RE,$1.00\n07105038A,2025,RE,$2.00\n07027002J80,2025,RE,$4.00\n")
	secondRows, secondMeta := readFile("Property Key,Year,Primary Source,Balance Due\n09125043,2022,RE,$3.00\n07027002J80,2025,RE,$4.00\n")
	if firstMeta.SHA256 == secondMeta.SHA256 {
		t.Fatal("versions must have different file hashes")
	}
	save := func(rows []union.DelinquentRow, meta taxsource.Meta) taxstore.DelinquentResult {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		result, err := taxstore.SaveDelinquent(ctx, tx, rows, meta, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return result
	}
	seedParcel := func(key string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO parcels (county_id, county_code, source_name, source_parcel_id, likely_vacant)
VALUES ('union','179','union_county_gis',$1,true) RETURNING id::text`, key).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM parcels WHERE id::text=$1`, id) })
		return id
	}
	if got := save(firstRows, firstMeta); got.Saved != 0 || got.Unmatched != 3 || got.Deleted != 0 {
		t.Fatalf("initial unmatched file: %+v", got)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE county_id='union' AND source_name='union_delinquent' AND active`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active evidence without typed rows=%d err=%v", active, err)
	}
	if _, ok, err := taxstore.NewDelinquentBalances(pool).Latest(ctx); err != nil || ok {
		t.Fatalf("typed latest must not imply a matched row: ok=%v err=%v", ok, err)
	}
	firstID := seedParcel(firstRows[0].ParcelKey)
	removedID := seedParcel(firstRows[1].ParcelKey)
	if got := save(firstRows, firstMeta); got.Saved != 2 || got.Unmatched != 1 {
		t.Fatalf("partial relink: %+v", got)
	}
	retainedID := seedParcel(firstRows[2].ParcelKey)
	if got := save(firstRows, firstMeta); got.Saved != 3 || got.Unmatched != 0 {
		t.Fatalf("complete relink: %+v", got)
	}
	var firstRowID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM tax_delinquent_balances WHERE parcel_id::text=$1`, firstID).Scan(&firstRowID); err != nil {
		t.Fatal(err)
	}
	if got := save(secondRows, secondMeta); got.Saved != 2 || got.Unmatched != 0 || got.Deleted != 1 {
		t.Fatalf("changed complete file: %+v", got)
	}
	var rowID, balance string
	if err := pool.QueryRow(ctx, `SELECT id::text,balance_due::text FROM tax_delinquent_balances WHERE parcel_id::text=$1`, firstID).Scan(&rowID, &balance); err != nil || rowID != firstRowID || balance != "3.00" {
		t.Fatalf("retained identity=%s/%s balance=%s err=%v", rowID, firstRowID, balance, err)
	}
	var oldRows, oldOwed, retainedRows, imports int
	for _, check := range []struct {
		query, id string
		value     *int
	}{
		{`SELECT count(*) FROM tax_delinquent_balances WHERE parcel_id::text=$1`, removedID, &oldRows},
		{`SELECT count(*) FROM tax_owed WHERE parcel_id::text=$1`, removedID, &oldOwed},
		{`SELECT count(*) FROM tax_delinquent_balances WHERE parcel_id::text=$1`, retainedID, &retainedRows},
	} {
		if err := pool.QueryRow(ctx, check.query, check.id).Scan(check.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE county_id='union' AND source_name='union_delinquent'`).Scan(&imports); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE county_id='union' AND source_name='union_delinquent' AND active`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if oldRows != 0 || oldOwed != 0 || retainedRows != 1 || imports != 2 || active != 1 {
		t.Fatalf("old rows=%d owed=%d retained=%d imports=%d active=%d", oldRows, oldOwed, retainedRows, imports, active)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM tax_file_imports WHERE county_id='union' AND source_name='union_delinquent' AND source_url=$1`, firstMeta.URL)
	})
}

func TestSaveDelinquentRejectsInvalidInput(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name a disposable migrated database")
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
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Put(ctx, "union_delinquent", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	meta := taxsource.Meta{URL: "https://example.invalid/tax.csv", Name: "union_delinquent", SHA256: artifact.SHA256(), FetchedAt: time.Now().UTC(), Artifact: artifact}
	row := union.DelinquentRow{ParcelKey: "09125043", Year: 2025, Balance: 123, PrimarySource: "RE"}
	invalid := meta
	invalid.Name = "union_billing"
	if _, err := taxstore.SaveDelinquent(ctx, tx, []union.DelinquentRow{row}, invalid, 0, 0); err == nil {
		t.Fatal("accepted wrong source")
	}
	if _, err := taxstore.SaveDelinquent(ctx, tx, []union.DelinquentRow{row}, meta, -1, 0); err == nil {
		t.Fatal("accepted negative skip count")
	}
	row.PrimarySource = "PP"
	if _, err := taxstore.SaveDelinquent(ctx, tx, []union.DelinquentRow{row}, meta, 0, 0); err == nil {
		t.Fatal("accepted personal-property row")
	}
	invalidMeta := meta
	invalidMeta.FetchedAt = meta.FetchedAt.Add(time.Second)
	evidence := taxstore.ImportEvidence{CountyID: county.Union, SourceName: invalidMeta.Name, SourceURL: invalidMeta.URL, FetchedAt: invalidMeta.FetchedAt, Artifact: artifact, Validation: taxstore.ImportInvalid}
	if _, err := taxstore.SaveImportEvidence(ctx, tx, evidence); err != nil {
		t.Fatal(err)
	}
	row.PrimarySource = "RE"
	if _, err := taxstore.SaveDelinquent(ctx, tx, []union.DelinquentRow{row}, invalidMeta, 0, 0); err == nil {
		t.Fatal("accepted invalid existing import")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := taxstore.SaveDelinquent(cancelled, tx, []union.DelinquentRow{row}, meta, 0, 0); err == nil {
		t.Fatal("accepted canceled lookup")
	}
	badArtifact, err := store.Put(ctx, "union_billing", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	evidence.Artifact = badArtifact
	if _, err := taxstore.SaveImportEvidence(ctx, tx, evidence); err == nil {
		t.Fatal("accepted wrong artifact source")
	}
	evidence.Artifact = artifact
	evidence.Validation = "unknown"
	if _, err := taxstore.SaveImportEvidence(ctx, tx, evidence); err == nil {
		t.Fatal("accepted unknown validation")
	}
	if _, ok, err := taxstore.NewDelinquentBalances(nil).Latest(ctx); err == nil || ok {
		t.Fatalf("accepted missing database: ok=%v err=%v", ok, err)
	}
	if _, ok, err := taxstore.NewDelinquentBalances(pool).Latest(cancelled); err == nil || ok {
		t.Fatalf("accepted canceled latest: ok=%v err=%v", ok, err)
	}
}

func TestSaveDelinquentDatabaseFailures(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name a disposable migrated database")
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
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Put(ctx, "union_delinquent", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	meta := taxsource.Meta{URL: "https://example.invalid/failures.csv", Name: "union_delinquent", SHA256: artifact.SHA256(), FetchedAt: time.Now().UTC(), Artifact: artifact}
	row := union.DelinquentRow{ParcelKey: "TAX3-SQL-FAILURE", Year: 2025, Balance: 100, PrimarySource: "RE"}
	if _, err := tx.Exec(ctx, `INSERT INTO parcels (county_id,county_code,source_name,source_parcel_id)
VALUES ('union','179','union_county_gis',$1)`, row.ParcelKey); err != nil {
		t.Fatal(err)
	}
	missing, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := missing.Exec(ctx, `ALTER TABLE parcels RENAME TO parcels_unavailable`); err != nil {
		t.Fatal(err)
	}
	if _, err := taxstore.SaveDelinquent(ctx, missing, []union.DelinquentRow{row}, meta, 0, 0); err == nil {
		t.Fatal("lookup succeeded without parcels")
	}
	if err := missing.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	overflow, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row.Balance = 1<<63 - 1
	if _, err := taxstore.SaveDelinquent(ctx, overflow, []union.DelinquentRow{row}, meta, 0, 0); err == nil {
		t.Fatal("accepted amount above database numeric range")
	}
	if err := overflow.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	evidence := taxstore.ImportEvidence{CountyID: county.Union, SourceName: meta.Name, SourceURL: meta.URL, FetchedAt: meta.FetchedAt, Artifact: artifact, Validation: taxstore.ImportValid}
	if _, err := taxstore.SaveImportEvidence(ctx, tx, evidence); err != nil {
		t.Fatal(err)
	}
	duplicate, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := taxstore.SaveImportEvidence(ctx, duplicate, evidence); err == nil {
		t.Fatal("duplicate file evidence inserted")
	}
	if err := duplicate.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_delinquent_balances WHERE source_name='union_delinquent'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed saves published %d rows: %v", count, err)
	}
}
