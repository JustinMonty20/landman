//go:build postgres

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/connector/union/taxjob"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxstore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func requireDisposableTaxDB(t *testing.T) {
	t.Helper()
	if os.Getenv("TAX_REAL_INPUTS") != "1" {
		t.Skip("requires operator files and a disposable migrated database")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "landman_tax_integration" || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port == 5432 {
		t.Fatal("real-file test requires an isolated local database on a non-default port")
	}
}

func TestRealTaxInputsDisposable(t *testing.T) {
	requireDisposableTaxDB(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	seed := func(key string) string {
		t.Helper()
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO parcels (county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES ('union','179','union_county_gis',$1,true) ON CONFLICT (county_id,source_name,source_parcel_id) DO UPDATE SET likely_vacant=true RETURNING id::text`, key).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	var count int
	// Delinquent: parse the actual full CSV, but persist only one known RE row.
	dl, err := union.NewDelinquentList("artifacts/tax-input/delinquent.csv", store)
	if err != nil {
		t.Fatal(err)
	}
	var delinquent []union.DelinquentRow
	dm, err := dl.Read(ctx, func(row union.DelinquentRow) error { delinquent = append(delinquent, row); return nil })
	if err != nil {
		t.Fatal(err)
	}
	pp, rr, err := dl.Skipped(ctx, dm)
	if err != nil {
		t.Fatal(err)
	}
	if len(delinquent) == 0 || !dm.Artifact.Verified() {
		t.Fatal("missing verified delinquent rows")
	}
	did := seed(delinquent[0].ParcelKey)
	dr, err := taxstore.SaveDelinquent(ctx, tx, delinquent[:1], dm, pp, rr)
	if err != nil || dr.Saved != 1 {
		t.Fatalf("delinquent save: %+v %v", dr, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_delinquent_balances WHERE parcel_id=$1`, did).Scan(&count); err != nil || count != 1 {
		t.Fatalf("delinquent link: %d %v", count, err)
	}
	t.Logf("delinquent parsed=%d PP=%d RR=%d saved=%d matched UUID=%s", len(delinquent), pp, rr, dr.Saved, did)
	// Sales: parse the actual full PDF, persist one case and one of its PINs.
	fl, err := union.NewForeclosureList("artifacts/tax-input/foreclosure-sales.pdf", store)
	if err != nil {
		t.Fatal(err)
	}
	var cases []union.ForeclosureCase
	fm, err := fl.Read(ctx, func(row union.ForeclosureCase) error { cases = append(cases, row); return nil })
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := fl.Skipped(ctx, fm)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 || len(cases[0].PINs) == 0 || !fm.Artifact.Verified() {
		t.Fatal("missing verified sales cases")
	}
	fid := seed(cases[0].PINs[0].ParcelKey)
	fr, err := taxstore.SaveForeclosure(ctx, tx, cases[:1], fm, skipped)
	if err != nil || fr.SavedCases != 1 || fr.SavedJoins < 1 {
		t.Fatalf("foreclosure save: %+v %v", fr, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_parcels WHERE parcel_id=$1`, fid).Scan(&count); err != nil || count != 1 {
		t.Fatalf("sales link: %d %v", count, err)
	}
	t.Logf("sales cases parsed=%d skipped blocks=%d saved cases=%d joins=%d matched UUID=%s", len(cases), skipped, fr.SavedCases, fr.SavedJoins, fid)
	// Pipeline: parse the actual full PDF, persist one matched row and reconcile.
	pl, err := union.NewPipelineList("artifacts/tax-input/foreclosure-pipeline.pdf", "https://www.unioncountync.gov/home/showdocument?id=4784", store)
	if err != nil {
		t.Fatal(err)
	}
	var properties []union.PipelineProperty
	pm, err := pl.Read(ctx, func(row union.PipelineProperty) error { properties = append(properties, row); return nil })
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pl.Skipped(ctx, pm)
	if err != nil {
		t.Fatal(err)
	}
	if len(properties) == 0 || !pm.Artifact.Verified() {
		t.Fatal("missing verified pipeline parcels")
	}
	pid := seed(properties[0].ParcelKey)
	pr, err := taxstore.SavePipeline(ctx, tx, properties[:1], pm, pages)
	if err != nil || pr.Saved != 1 {
		t.Fatalf("pipeline save: %+v %v", pr, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_pipeline WHERE parcel_id=$1`, pid).Scan(&count); err != nil || count != 1 {
		t.Fatalf("pipeline link: %d %v", count, err)
	}
	t.Logf("pipeline parsed=%d skipped pages=%d saved=%d matched UUID=%s", len(properties), pages, pr.Saved, pid)
	// Rollback removes all test writes. The whole container is also removed after the test.
}

func TestRealUnionTaxJobDisposable(t *testing.T) {
	requireDisposableTaxDB(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, key := range []string{"09336350", "09125043", "02239077"} {
		if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES('union','179','union_county_gis',$1,true) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("TAX_ALL_BILLS") == "1" {
		sourceStore, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "billing-artifact"))
		if err != nil {
			t.Fatal(err)
		}
		source, err := union.NewBillingFile("artifacts/tax-input/billing.zip", "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000", sourceStore)
		if err != nil {
			t.Fatal(err)
		}
		seen := make(map[string]bool)
		var parcels [][]any
		_, err = source.Read(ctx, func(row union.BillRow) error {
			if !seen[row.ParcelKey] {
				seen[row.ParcelKey] = true
				parcels = append(parcels, []any{"union", "179", "union_county_gis", row.ParcelKey, false})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// The three selected rows above are already present. Remove them from COPY input.
		filtered := parcels[:0]
		for _, parcel := range parcels {
			key := parcel[3].(string)
			if key != "09336350" && key != "09125043" && key != "02239077" {
				filtered = append(filtered, parcel)
			}
		}
		n, err := pool.CopyFrom(ctx, pgx.Identifier{"parcels"}, []string{"county_id", "county_code", "source_name", "source_parcel_id", "likely_vacant"}, pgx.CopyFromRows(filtered))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("seeded %d distinct real billing parcel keys in disposable database", n+3)
	}
	cfg := taxjob.Config{ArtifactDir: filepath.Join(t.TempDir(), "private"), DelinquentPath: "artifacts/tax-input/delinquent.csv", BillingPath: "artifacts/tax-input/billing.zip", ForeclosurePath: "artifacts/tax-input/foreclosure-sales.pdf", PipelinePath: "artifacts/tax-input/foreclosure-pipeline.pdf", BillingSourceURL: "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000", PipelineSourceURL: "https://www.unioncountync.gov/home/showdocument?id=4784"}
	previousConfig, previousArgs := taxConfig, os.Args
	taxConfig = taxRunConfig{Enabled: true, Config: cfg}
	os.Args = []string{"landman", "run", taxJobID}
	defer func() { taxConfig, os.Args = previousConfig, previousArgs }()
	if err := run(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_attempts WHERE job_id='union.tax' AND outcome='success'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("job history=%d err=%v", count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE active AND storage_mode='local' AND validation_result='valid'`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("active imports=%d err=%v", count, err)
	}
	var amount string
	if err := pool.QueryRow(ctx, `SELECT total_taxes::text FROM tax_bills b JOIN parcels p ON p.id=b.parcel_id WHERE p.source_parcel_id='09336350'`).Scan(&amount); err != nil || amount != "2737.12" {
		t.Fatalf("real billing amount=%s err=%v", amount, err)
	}
	for _, table := range []string{"tax_delinquent_balances", "tax_bills", "tax_foreclosure_cases", "tax_foreclosure_parcels", "tax_foreclosure_pipeline"} {
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count < 1 {
			t.Fatalf("%s rows=%d err=%v", table, count, err)
		}
		t.Logf("%s rows=%d", table, count)
		if table == "tax_bills" && os.Getenv("TAX_ALL_BILLS") == "1" && count < 100000 {
			t.Fatalf("expected full real-estate bill set, got %d", count)
		}
	}
	t.Logf("union.tax wrote four active imports and real 2026 bill 09336350 = %s", amount)
	if os.Getenv("TAX_ALL_BILLS") == "1" {
		var firstID, afterID string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM tax_file_imports WHERE source_name='union_billing' AND active`).Scan(&firstID); err != nil {
			t.Fatal(err)
		}
		if err := run(ctx); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT id::text FROM tax_file_imports WHERE source_name='union_billing' AND active`).Scan(&afterID); err != nil || firstID != afterID {
			t.Fatalf("unchanged billing import changed %s -> %s: %v", firstID, afterID, err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE source_name='union_billing'`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("unchanged billing created %d imports: %v", count, err)
		}
		t.Log("second union.tax run kept the fully reconciled billing import")
	}
}
