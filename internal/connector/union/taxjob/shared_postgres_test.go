//go:build postgres

package taxjob

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelapi"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test only runs against an explicitly named, freshly migrated disposable database.
func taxSharedDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAX_SHARED_TEST_DSN")
	if os.Getenv("TAX_SHARED_TEST_DISPOSABLE") != "1" || !strings.Contains(dsn, "/taxworker_") {
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
		if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES ('union','179','union_county_gis',$1)`, key); err != nil {
			t.Fatal(err)
		}
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	run := func() (jobs.Result, error) {
		_, result, err := runner.Run(ctx, jobs.Definition{ID: "union.tax", Run: func(ctx context.Context) (jobs.Result, error) {
			return Run(ctx, pool, cfg)
		}})
		return result, err
	}
	result, err := run()
	if err != nil {
		t.Fatal(err)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE a.job_id='union.tax' AND a.outcome='success' AND c.attempt_kind='full_scan'`); n != 4 {
		t.Fatalf("tax captures missing originating attempt: %d", n)
	}
	var originalBoundCapture string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM tax_ingest_publications WHERE source_name='union_delinquent'`).Scan(&originalBoundCapture); err != nil {
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
	if _, err := run(); err != nil {
		t.Fatal(err)
	}
	if got := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history`); got != before {
		t.Fatalf("identical files wrote business history: %d -> %d", before, got)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM ingest_captures WHERE published_at IS NOT NULL`); n != 4 {
		t.Fatalf("unchanged capture advanced freshness: %d", n)
	}
	// A prior normalizer's digest is not comparable to the new digest.
	if _, err := pool.Exec(ctx, `UPDATE tax_ingest_publications SET normalization_version='tax-normalized-legacy', normalized_sha256=decode(repeat('00',32),'hex') WHERE source_name='union_delinquent'`); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(cfg.ArtifactDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ingestFile(ctx, pool, store, cfg.ArtifactDir, "union_delinquent", union.DelinquentURL, cfg.DelinquentPath); err != nil {
		t.Fatal(err)
	}
	if got := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history`); got != before {
		t.Fatalf("normalizer version-only transition rewrote typed data: %d -> %d", before, got)
	}
	var captureID string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM tax_ingest_publications WHERE source_name='union_delinquent'`).Scan(&captureID); err != nil {
		t.Fatal(err)
	}
	// B is retained without publication, as after a failed publication. A
	// later identical observation of A must prevent B from replacing it.
	pending, err := ingest.NewRecorder(store, "union_delinquent", time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	pendingBytes, err := os.ReadFile(cfg.DelinquentPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := pending.Record(ctx, union.DelinquentURL, "", 0, strings.NewReader(strings.Replace(string(pendingBytes), "$14,355.02", "$14,355.03", 1))); err != nil {
		t.Fatal(err)
	}
	pendingID, err := (ingest.Engine{Pool: pool, Store: store}).Seal(ctx, pending)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := ingestFile(ctx, pool, store, cfg.ArtifactDir, "union_delinquent", union.DelinquentURL, cfg.DelinquentPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Replay(ctx, pool, cfg.ArtifactDir, pendingID); err != nil {
		t.Fatal(err)
	}
	if taxCount(t, pool, `SELECT count(*) FROM ingest_captures WHERE id=$1 AND published_at IS NOT NULL`, pendingID) != 0 {
		t.Fatal("stale retained B superseded later accepted A")
	}
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM tax_latest_observations WHERE source_name='union_delinquent'`).Scan(&captureID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES ('union','179','union_county_gis','07105038A')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.Run(ctx, jobs.Definition{ID: "union.gis.soil", Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return Replay(ctx, pool, cfg.ArtifactDir, captureID)
	}}); err == nil {
		t.Fatal("unrelated runner attempt claimed tax replay")
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM ingest_replay_attempts WHERE capture_id=$1::uuid`, captureID); n != 0 {
		t.Fatalf("wrong job recorded replay attempt: %d", n)
	}
	if _, _, err := runner.Run(ctx, jobs.Definition{ID: "union.tax", Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return Replay(ctx, pool, cfg.ArtifactDir, captureID)
	}}); err != nil {
		t.Fatal(err)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM ingest_replay_attempts r JOIN job_attempts a ON a.id=r.attempt_id WHERE r.capture_id=$1::uuid AND a.job_id='union.tax' AND a.attempt_kind='replay' AND a.outcome='success'`, captureID); n != 1 {
		t.Fatalf("replay missing separate attempt provenance: %d", n)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE c.id=$1::uuid AND a.attempt_kind='full_scan'`, originalBoundCapture); n != 1 {
		t.Fatalf("replay replaced original capture attempt: %d", n)
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
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM tax_latest_observations WHERE source_name='union_foreclosure'`).Scan(&foreclosureCapture); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES ('union','179','union_county_gis','07129161')`); err != nil {
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
	var priorImport string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM tax_file_imports WHERE source_name='union_delinquent' AND active`).Scan(&priorImport); err != nil {
		t.Fatal(err)
	}
	priorFindings := taxCount(t, pool, `SELECT count(*) FROM tax_unmatched_records WHERE import_id=$1`, priorImport)
	if priorFindings == 0 {
		t.Fatal("fixture must retain unmatched PINs")
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
	for _, suffix := range []string{"", "&import_id=" + priorImport} {
		w := httptest.NewRecorder()
		parcelapi.New(pool).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/tax/unmatched?source=union_delinquent"+suffix, nil))
		var body struct {
			Records []json.RawMessage `json:"records"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Records) != priorFindings {
			t.Fatalf("equivalent file lost review findings: %d %s", w.Code, w.Body)
		}
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
	invalid := cfg
	invalid.DelinquentPath = broken
	if _, _, err := runner.Run(ctx, jobs.Definition{ID: "union.tax", Run: func(ctx context.Context) (jobs.Result, error) {
		return Run(ctx, pool, invalid)
	}}); err == nil {
		t.Fatal("malformed operator file accepted through runner")
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE c.source_name='union_delinquent' AND c.published_at IS NULL AND a.job_id='union.tax' AND a.outcome='failed'`); n != 1 {
		t.Fatalf("failed tax capture missing originating attempt: %d", n)
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
	// A corrected typed projection and an absent link can coincide on replay.
	if _, err := pool.Exec(ctx, `UPDATE tax_ingest_publications SET canonical_sha256=decode(repeat('00',32),'hex'),canonical_version='legacy',normalization_version='legacy' WHERE source_name='union_delinquent'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tax_delinquent_balances WHERE parcel_id=(SELECT id FROM parcels WHERE source_parcel_id='09125043')`); err != nil {
		t.Fatal(err)
	}
	var correctedCapture string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM tax_latest_observations WHERE source_name='union_delinquent'`).Scan(&correctedCapture); err != nil {
		t.Fatal(err)
	}
	if _, err := Replay(ctx, pool, cfg.ArtifactDir, correctedCapture); err != nil {
		t.Fatal(err)
	}
	if n := taxCount(t, pool, `SELECT count(*) FROM tax_ingest_history WHERE cause='normalization_correction' AND table_name='tax_delinquent_balances' AND after_value IS NOT NULL`); n == 0 {
		t.Fatal("corrected projection relink attributed to offline retry")
	}
}
