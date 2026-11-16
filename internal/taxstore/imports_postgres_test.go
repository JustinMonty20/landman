//go:build postgres

package taxstore_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxstore"
	"github.com/jackc/pgx/v5"
)

func TestImportEvidenceAndAtomicActivation(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name an empty migrated disposable database")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Put(ctx, "union_delinquent", strings.NewReader("first source file"))
	if err != nil {
		t.Fatal(err)
	}
	if err := taxartifact.Verify(ctx, store, first); err != nil {
		t.Fatal(err)
	}
	base := taxstore.ImportEvidence{
		CountyID: county.Union, SourceName: "union_delinquent",
		SourceURL: "https://example.invalid/delinquent.csv",
		FetchedAt: time.Date(2026, 11, 16, 1, 2, 3, 0, time.UTC),
		Artifact:  first, Validation: taxstore.ImportValid,
	}
	forged := base
	forged.Artifact, err = taxartifact.Reference(first.Key(), first.Size(), first.SHA256())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := taxstore.SaveImportEvidence(ctx, tx, forged); err == nil {
		t.Fatal("accepted a file reference without store verification")
	}
	firstID, err := taxstore.SaveImportEvidence(ctx, tx, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := taxstore.ActivateImport(ctx, tx, firstID); err != nil {
		t.Fatal(err)
	}
	var storedMode string
	if err := tx.QueryRow(ctx, `SELECT storage_mode FROM tax_file_imports WHERE id = $1`, firstID).Scan(&storedMode); err != nil || storedMode != store.Mode() {
		t.Fatalf("stored artifact mode %q, want %q: %v", storedMode, store.Mode(), err)
	}
	var parcelID string
	err = tx.QueryRow(ctx, `
INSERT INTO parcels (county_id, county_code, source_name, source_parcel_id)
VALUES ('union', '179', 'union_county_gis', 'artifact-test') RETURNING id::text`).Scan(&parcelID)
	if err != nil {
		t.Fatal(err)
	}
	digest := first.SHA256()
	_, err = tx.Exec(ctx, `
INSERT INTO tax_delinquent_balances (
    county_id, source_name, parcel_id, tax_year, balance_due, primary_source,
    source_url, file_sha256, fetched_at
) VALUES ($1,$2,$3,2025,12.34,'RE',$4,$5,$6)`, base.CountyID, base.SourceName, parcelID,
		base.SourceURL, digest[:], base.FetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	var linked int
	err = tx.QueryRow(ctx, `
SELECT count(*) FROM tax_delinquent_balances b
JOIN tax_file_imports i ON i.county_id = b.county_id
AND i.source_name = b.source_name AND i.source_url = b.source_url
AND i.file_sha256 = b.file_sha256 AND i.fetched_at = b.fetched_at
WHERE i.id = $1 AND i.active`, firstID).Scan(&linked)
	if err != nil || linked != 1 {
		t.Fatalf("typed row link: count=%d error=%v", linked, err)
	}
	second, err := store.Put(ctx, "union_delinquent", strings.NewReader("invalid source file"))
	if err != nil {
		t.Fatal(err)
	}
	bad := base
	bad.Artifact = second
	bad.FetchedAt = bad.FetchedAt.Add(time.Minute)
	bad.Validation = taxstore.ImportInvalid
	badID, err := taxstore.SaveImportEvidence(ctx, tx, bad)
	if err != nil {
		t.Fatal(err)
	}
	if err := taxstore.ActivateImport(ctx, tx, badID); err == nil {
		t.Fatal("invalid import became active")
	}
	assertActive(t, ctx, tx, firstID)

	third, err := store.Put(ctx, "union_delinquent", strings.NewReader("valid replacement"))
	if err != nil {
		t.Fatal(err)
	}
	next := base
	next.Artifact = third
	next.FetchedAt = next.FetchedAt.Add(2 * time.Minute)
	attempt, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	attemptID, err := taxstore.SaveImportEvidence(ctx, attempt, next)
	if err != nil {
		t.Fatal(err)
	}
	if err := taxstore.ActivateImport(ctx, attempt, attemptID); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertActive(t, ctx, tx, firstID)

	nextID, err := taxstore.SaveImportEvidence(ctx, tx, next)
	if err != nil {
		t.Fatal(err)
	}
	if err := taxstore.ActivateImport(ctx, tx, nextID); err != nil {
		t.Fatal(err)
	}
	assertActive(t, ctx, tx, nextID)
	if _, err := taxstore.SaveImportEvidence(ctx, tx, taxstore.ImportEvidence{}); err == nil {
		t.Fatal("accepted missing import evidence")
	}
}

func assertActive(t *testing.T, ctx context.Context, tx pgx.Tx, want string) {
	t.Helper()
	var got string
	if err := tx.QueryRow(ctx, `
SELECT id::text FROM tax_file_imports
WHERE county_id = 'union' AND source_name = 'union_delinquent' AND active`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("active import %s, want %s", got, want)
	}
}
