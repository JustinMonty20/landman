//go:build postgres

package taxjob

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test only runs against an explicitly named, freshly migrated disposable database.
func taxSharedDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAX_SHARED_TEST_DSN")
	if os.Getenv("TAX_SHARED_TEST_DISPOSABLE") != "1" || !strings.Contains(dsn, "/taxworker?") {
		t.Skip("requires a dedicated migrated taxworker database and explicit disposable guard")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func taxFixture(t *testing.T, target, source string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "tax", source))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(target, source)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func billingZIP(t *testing.T, target string) string {
	t.Helper()
	csv, err := os.ReadFile(filepath.Join("..", "testdata", "tax", "billing.csv"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(target, "billing.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(file)
	entry, err := zipWriter.Create("WKBILA-2026.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(csv); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
func taxCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSharedTaxRunRetainsAndReplaysFourFiles(t *testing.T) {
	pool := taxSharedDatabase(t)
	ctx := context.Background()
	root := t.TempDir()
	cfg := Config{
		ArtifactDir:      filepath.Join(root, "private"),
		DelinquentPath:   taxFixture(t, root, "delinquent.csv"),
		BillingPath:      billingZIP(t, root),
		ForeclosurePath:  taxFixture(t, root, "foreclosure.pdf"),
		PipelinePath:     taxFixture(t, root, "pipeline.pdf"),
		BillingSourceURL: union.BillingURL, PipelineSourceURL: union.PipelineURL,
	}
	for _, key := range []string{"09125043", "09336350", "07129160", "02239077"} {
		if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES ('union','179','union_county_gis',$1,true)`, key); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Run(ctx, pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed < 4 {
		t.Fatalf("four feeds not published: %+v", result)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_publications`); n != 4 {
		t.Fatalf("published feeds: %d", n)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM tax_file_imports WHERE active`); n != 4 {
		t.Fatalf("active imports: %d", n)
	}
	before := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history`)
	if before < 4 || taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history WHERE capture_id IS NULL`) != 0 {
		t.Fatal("typed changes are not attributed to their verified captures")
	}
	if _, err := Run(ctx, pool, cfg); err != nil {
		t.Fatal(err)
	}
	if got := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history`); got != before {
		t.Fatalf("identical files wrote business history: %d -> %d", before, got)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM ingest_captures WHERE published_at IS NOT NULL`); n != 4 {
		t.Fatalf("unchanged capture advanced freshness: %d", n)
	}
	var captureID string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM tax_ingest_publications WHERE source_name='union_delinquent'`).Scan(&captureID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES ('union','179','union_county_gis','07105038A',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Replay(ctx, pool, cfg.ArtifactDir, captureID); err != nil {
		t.Fatal(err)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM tax_delinquent_balances WHERE tax_year=2025`); n != 1 {
		t.Fatalf("replay did not link previously unmatched key: %d", n)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history WHERE cause='offline_retry' AND table_name='tax_delinquent_balances'`); n != 1 {
		t.Fatalf("replay history: %d", n)
	}
	if _, err := Replay(ctx, pool, cfg.ArtifactDir, captureID); err != nil {
		t.Fatal(err)
	}
	var foreclosureCapture string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM tax_ingest_publications WHERE source_name='union_foreclosure'`).Scan(&foreclosureCapture); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES ('union','179','union_county_gis','07129161',false)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Replay(ctx, pool, cfg.ArtifactDir, foreclosureCapture); err != nil {
		t.Fatal(err)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM tax_foreclosure_parcels j JOIN parcels p ON p.id=j.parcel_id WHERE p.source_parcel_id='07129161'`); n != 1 {
		t.Fatalf("foreclosure replay did not link previously unmatched PIN: %d", n)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history WHERE cause='offline_retry' AND table_name='tax_foreclosure_parcels'`); n != 1 {
		t.Fatalf("foreclosure replay provenance: %d", n)
	}
	data, err := os.ReadFile(cfg.DelinquentPath)
	if err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(cfg.ArtifactDir)
	if err != nil {
		t.Fatal(err)
	}
	formatted := filepath.Join(root, "formatted-delinquent.csv")
	if err := os.WriteFile(formatted, append(append([]byte(nil), data...), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ingestFile(ctx, pool, store, cfg.ArtifactDir, "union_delinquent", union.DelinquentURL, formatted); err != nil {
		t.Fatal(err)
	}
	if got := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history`); got != before+2 {
		t.Fatalf("format-only change wrote business values: %d", got)
	}
	broken := filepath.Join(root, "broken-delinquent.csv")
	if err := os.WriteFile(broken, []byte("invalid header\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ingestFile(ctx, pool, store, cfg.ArtifactDir, "union_delinquent", union.DelinquentURL, broken); err == nil || !strings.Contains(err.Error(), "retained capture ") {
		t.Fatalf("malformed capture lost its retained reference: %v", err)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM ingest_captures WHERE source_name='union_delinquent' AND published_at IS NULL`); n < 1 {
		t.Fatal("failed input left no retained registered capture")
	}
	changed := filepath.Join(root, "changed-delinquent.csv")
	data, err = os.ReadFile(cfg.DelinquentPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte(strings.Replace(string(data), "$14,355.02", "$14,355.03", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ingestFile(ctx, pool, store, cfg.ArtifactDir, "union_delinquent", union.DelinquentURL, changed); err != nil {
		t.Fatal(err)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history WHERE table_name='tax_delinquent_balances' AND before_value IS NOT NULL AND after_value IS NOT NULL`); n != 1 {
		t.Fatalf("changed file should update one balance, got %d", n)
	}
	if _, err := Replay(ctx, pool, cfg.ArtifactDir, captureID); err != nil {
		t.Fatal(err)
	}
	var amount string
	if err := pool.QueryRow(ctx, `SELECT balance_due::text FROM tax_delinquent_balances b JOIN parcels p ON p.id=b.parcel_id WHERE p.source_parcel_id='09125043'`).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != "14355.03" {
		t.Fatal(fmt.Sprintf("stale replay rolled back balance to %s", amount))
	}
}
