//go:build postgres

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if err := disposableTaxDSN(os.Getenv("DATABASE_URL")); err != nil {
		t.Fatal(err)
	}
}

func disposableTaxDSN(dsn string) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	if cfg.ConnConfig.Database != "landman_tax_integration" || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port == 5432 || len(cfg.ConnConfig.Fallbacks) != 0 {
		return errors.New("real-file test requires an isolated local database on a non-default port with no fallback")
	}
	return nil
}

func TestDisposableTaxDSN(t *testing.T) {
	for _, dsn := range []string{"postgres://review:unused@127.0.0.1:1,example.invalid:5432/landman_tax_integration?sslmode=disable", "postgres://postgres@127.0.0.1:5432/landman_tax_integration?sslmode=disable", "postgres://postgres@example.invalid:5433/landman_tax_integration?sslmode=disable"} {
		if err := disposableTaxDSN(dsn); err == nil {
			t.Fatalf("accepted unsafe database URL %q", dsn)
		}
	}
	if err := disposableTaxDSN(fmt.Sprintf("postgres://postgres@127.0.0.1:%d/landman_tax_integration?sslmode=disable", 55433)); err != nil {
		t.Fatal(err)
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

func oneCaseForeclosurePDF() []byte {
	var content strings.Builder
	content.WriteString("BT /F1 9 Tf 40 760 Td 12 TL\n")
	for _, line := range []string{"Sale Date: November 4, 2026 Case # 26M000471-890", "Current Property Owner: TEST OWNER", "Parcel Identification Number (PIN): 09-125-043", "Lot 1", "Address: 1 SAMPLE ROAD", "Tax Value: $10,000.00"} {
		line = strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(line)
		fmt.Fprintf(&content, "(%s) Tj T*\n", line)
	}
	content.WriteString("ET\n")
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String())}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n%\xff\x00\xff\n")
	var offsets []int
	for i, obj := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	start := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start)
	return pdf.Bytes()
}

func TestGoldenUnionTaxJobDisposable(t *testing.T) {
	requireDisposableTaxDB(t)
	testGoldenUnionTaxJob(t)
}

func testGoldenUnionTaxJob(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM tax_foreclosure_cases; DELETE FROM tax_file_imports; DELETE FROM job_attempts WHERE job_id='union.tax'; DELETE FROM parcels`); err != nil {
			t.Errorf("clean golden test rows: %v", err)
		}
		pool.Close()
	}()
	for _, key := range []string{"09125043", "09336350", "02239077"} {
		if _, err = pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES('union','179','union_county_gis',$1,true)`, key); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile("internal/connector/union/testdata/tax/billing.csv")
	if err != nil {
		t.Fatal(err)
	}
	foreclosurePath := filepath.Join(t.TempDir(), "foreclosure.pdf")
	if err = os.WriteFile(foreclosurePath, oneCaseForeclosurePDF(), 0600); err != nil {
		t.Fatal(err)
	}
	billingPath := filepath.Join(t.TempDir(), "billing.zip")
	file, err := os.Create(billingPath)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("WKBILA-golden.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	previousConfig, previousArgs := taxConfig, os.Args
	taxConfig = taxRunConfig{Enabled: true, Config: taxjob.Config{ArtifactDir: filepath.Join(t.TempDir(), "private"), DelinquentPath: "internal/connector/union/testdata/tax/delinquent.csv", BillingPath: billingPath, ForeclosurePath: foreclosurePath, PipelinePath: "internal/connector/union/testdata/tax/pipeline.pdf", BillingSourceURL: "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000", PipelineSourceURL: "https://www.unioncountync.gov/home/showdocument?id=4784"}}
	os.Args = []string{"landman", "run", taxJobID}
	defer func() { taxConfig, os.Args = previousConfig, previousArgs }()
	start := time.Now()
	if err = run(ctx); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	for table, want := range map[string]int{"tax_delinquent_balances": 1, "tax_bills": 1, "tax_foreclosure_cases": 1, "tax_foreclosure_parcels": 1, "tax_foreclosure_pipeline": 1} {
		var got int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&got); err != nil || got != want {
			t.Fatalf("%s rows=%d want=%d err=%v", table, got, want, err)
		}
	}
	t.Logf("golden union.tax duration=%v", elapsed)
	if elapsed >= 5*time.Second {
		t.Fatalf("golden tax job exceeded 5s: %v", elapsed)
	}
	var caseID, currentLink string
	if err := pool.QueryRow(ctx, `SELECT c.id::text,j.id::text FROM tax_foreclosure_cases c
JOIN tax_foreclosure_parcels j ON j.case_id=c.id JOIN parcels p ON p.id=j.parcel_id
WHERE c.case_number='26M000471-890' AND p.source_parcel_id='09125043'`).Scan(&caseID, &currentLink); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant)
VALUES('union','179','union_county_gis','07129163',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tax_foreclosure_parcels(case_id,parcel_id)
SELECT $1::uuid,p.id FROM parcels p WHERE p.county_id='union' AND p.source_name='union_county_gis' AND p.source_parcel_id='07129163'`, caseID); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx); err != nil {
		t.Fatal(err)
	}
	var links int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_parcels WHERE case_id=$1`, caseID).Scan(&links); err != nil || links != 1 {
		t.Fatalf("unchanged foreclosure file retained obsolete link: links=%d err=%v", links, err)
	}
	var retained string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_parcels WHERE case_id=$1`, caseID).Scan(&retained); err != nil || retained != currentLink {
		t.Fatalf("current foreclosure PIN changed link ID: before=%s after=%s err=%v", currentLink, retained, err)
	}
}

func TestDuplicateBillingRetry(t *testing.T) {
	requireDisposableTaxDB(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM tax_foreclosure_cases; DELETE FROM tax_file_imports; DELETE FROM parcels`); err != nil {
			t.Errorf("clean duplicate test rows: %v", err)
		}
		pool.Close()
	}()
	if _, err = pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES ('union','179','union_county_gis','09336350',true)`); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("internal/connector/union/testdata/tax/billing.csv")
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) < 3 {
		t.Fatal("billing fixture lacks a RE row")
	}
	path := filepath.Join(t.TempDir(), "billing.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("WKBILA-duplicate.csv")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range [][]byte{lines[0], lines[1], lines[1]} {
		if _, err = entry.Write(append(append([]byte{}, line...), '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := taxjob.Config{ArtifactDir: filepath.Join(t.TempDir(), "private"), DelinquentPath: "artifacts/tax-input/delinquent.csv", BillingPath: path, ForeclosurePath: "artifacts/tax-input/foreclosure-sales.pdf", PipelinePath: "artifacts/tax-input/foreclosure-pipeline.pdf", BillingSourceURL: "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000", PipelineSourceURL: "https://www.unioncountync.gov/home/showdocument?id=4784"}
	for i := 0; i < 2; i++ {
		if _, err := taxjob.Run(ctx, pool, cfg); err != nil {
			t.Fatal(err)
		}
	}
	var bills, imports int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tax_bills`).Scan(&bills); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE source_name='union_billing'`).Scan(&imports); err != nil {
		t.Fatal(err)
	}
	if bills != 1 || imports != 1 {
		t.Fatalf("duplicate billing retry: bills=%d imports=%d", bills, imports)
	}
	var billID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM tax_bills`).Scan(&billID); err != nil {
		t.Fatal(err)
	}
	changedPath := filepath.Join(t.TempDir(), "changed.zip")
	changed, err := os.Create(changedPath)
	if err != nil {
		t.Fatal(err)
	}
	changedZIP := zip.NewWriter(changed)
	row, err := changedZIP.Create("WKBILA-changed.csv")
	if err != nil {
		t.Fatal(err)
	}
	changedLine := strings.Replace(string(lines[1]), "1,359.48", "1,359.49", 1)
	if changedLine == string(lines[1]) {
		t.Fatal("fixture tax did not change")
	}
	if _, err = row.Write([]byte(string(lines[0]) + "\n" + changedLine + "\n")); err != nil {
		t.Fatal(err)
	}
	if err = changedZIP.Close(); err != nil {
		t.Fatal(err)
	}
	if err = changed.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.BillingPath = changedPath
	if _, err = taxjob.Run(ctx, pool, cfg); err != nil {
		t.Fatal(err)
	}
	var retainedID, amount string
	if err = pool.QueryRow(ctx, `SELECT id::text,total_taxes::text FROM tax_bills`).Scan(&retainedID, &amount); err != nil || retainedID != billID || amount != "1359.49" {
		t.Fatalf("changed version lost bill id or amount: %s %s -> %s: %v", billID, retainedID, amount, err)
	}
}

func TestParcelCommitAfterTaxActivation(t *testing.T) {
	requireDisposableTaxDB(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM tax_foreclosure_cases; DELETE FROM tax_file_imports; DELETE FROM parcels`); err != nil {
			t.Errorf("clean concurrent test rows: %v", err)
		}
		pool.Close()
	}()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES('union','179','union_county_gis','09336350',true)`); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("internal/connector/union/testdata/tax/billing.csv")
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(data, []byte{'\n'})
	path := filepath.Join(t.TempDir(), "billing.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("WKBILA-concurrent.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte(string(lines[0]) + "\n" + string(lines[1]) + "\n")); err != nil {
		t.Fatal(err)
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := taxjob.Config{ArtifactDir: filepath.Join(t.TempDir(), "private"), DelinquentPath: "artifacts/tax-input/delinquent.csv", BillingPath: path, ForeclosurePath: "artifacts/tax-input/foreclosure-sales.pdf", PipelinePath: "artifacts/tax-input/foreclosure-pipeline.pdf", BillingSourceURL: "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000", PipelineSourceURL: "https://www.unioncountync.gov/home/showdocument?id=4784"}
	if _, err = taxjob.Run(ctx, pool, cfg); err != nil {
		t.Fatal(err)
	}
	var bills int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tax_bills`).Scan(&bills); err != nil || bills != 0 {
		t.Fatalf("uncommitted parcel matched: bills=%d err=%v", bills, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = taxjob.Run(ctx, pool, cfg); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tax_bills`).Scan(&bills); err != nil || bills != 1 {
		t.Fatalf("parcel committed after activation not relinked: bills=%d err=%v", bills, err)
	}
}

func TestForeclosureRetryChecksEachCasePIN(t *testing.T) {
	requireDisposableTaxDB(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM tax_foreclosure_cases; DELETE FROM tax_file_imports; DELETE FROM parcels`); err != nil {
			t.Errorf("clean foreclosure test rows: %v", err)
		}
		pool.Close()
	}()
	for _, key := range []string{"07129160", "07129161", "07129163"} {
		if _, err = pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES('union','179','union_county_gis',$1,true)`, key); err != nil {
			t.Fatal(err)
		}
	}
	cfg := taxjob.Config{ArtifactDir: filepath.Join(t.TempDir(), "private"), DelinquentPath: "artifacts/tax-input/delinquent.csv", BillingPath: "artifacts/tax-input/billing.zip", ForeclosurePath: "artifacts/tax-input/foreclosure-sales.pdf", PipelinePath: "artifacts/tax-input/foreclosure-pipeline.pdf", BillingSourceURL: "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000", PipelineSourceURL: "https://www.unioncountync.gov/home/showdocument?id=4784"}
	if _, err = taxjob.Run(ctx, pool, cfg); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO tax_foreclosure_parcels(case_id,parcel_id) SELECT c.id,p.id FROM tax_foreclosure_cases c, parcels p WHERE c.case_number='26M000471-890' AND p.source_parcel_id='07129163'`)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES('union','179','union_county_gis','07129162',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err = taxjob.Run(ctx, pool, cfg); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = taxjob.Run(ctx, pool, cfg); err != nil {
		t.Fatal(err)
	}
	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_parcels j JOIN tax_foreclosure_cases c ON c.id=j.case_id JOIN parcels p ON p.id=j.parcel_id WHERE c.case_number='26M000471-890' AND p.source_parcel_id='07129162'`).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("new numbered-case PIN not linked: count=%d err=%v", count, err)
	}
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
