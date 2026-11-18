//go:build postgres

package main

import (
	"context"
	"encoding/json"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
)

func TestParcelReplayRejectsRewrittenReceiptAndManifest(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "trusted", true, 1)}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	var id string
	var saved []byte
	if err := pool.QueryRow(ctx, `SELECT c.id::text,c.manifest FROM ingest_captures c`).Scan(&id, &saved); err != nil {
		t.Fatal(err)
	}
	var manifest ingest.Manifest
	if err := json.Unmarshal(saved, &manifest); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(os.Getenv("PARCEL_EVIDENCE_DIR"), filepath.FromSlash(manifest.Receipts[1].ArtifactKey))
	original, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte(`{"features":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	replay := func() (jobs.Outcome, error) {
		outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
			return (parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}).Replay(ctx, id)
		}})
		return outcome, err
	}
	if outcome, err := replay(); err == nil || outcome != jobs.OutcomeFailed {
		t.Fatalf("altered response accepted: %s %v", outcome, err)
	}
	if err := os.WriteFile(page, original, 0600); err != nil {
		t.Fatal(err)
	}
	manifest.Receipts[1].Request = "forged"
	altered, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ingest_captures SET manifest=$2 WHERE id=$1::uuid`, id, altered); err != nil {
		t.Fatal(err)
	}
	if outcome, err := replay(); err == nil || outcome != jobs.OutcomeFailed {
		t.Fatalf("altered manifest accepted: %s %v", outcome, err)
	}
	var owner string
	if err := pool.QueryRow(ctx, `SELECT current_name_1 FROM parcels WHERE source_parcel_id='A'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "trusted" || countRows(t, ctx, pool, "parcel_evidence_history") != 1 {
		t.Fatal("tampered replay changed current state")
	}
}

func TestParcelOfflineRebuildAndHistoricalValidation(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "first", true, 1)}}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL, BatchSize: 1, RateLimit: 1000000})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	id, observedAt := recordOfflineParcelCapture(t, ctx, pool, source)
	server.Close()
	fixture.features[0] = feature(1, "A", "newer", true, 1)
	runner, _ := jobs.NewRunner(pool)
	job := parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}
	run := func(id string) (jobs.Outcome, error) {
		outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return job.Replay(ctx, id) }})
		return outcome, err
	}
	outcome, err := run(id)
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("offline rebuild: %s %v", outcome, err)
	}
	if countRows(t, ctx, pool, "parcel_evidence_observations") != 1 {
		t.Fatal("rebuild missing observation")
	}
	var kind string
	var observed bool
	if err := pool.QueryRow(ctx, `SELECT publication_kind,observed_at=($1::timestamptz) FROM parcel_evidence_observations`, observedAt).Scan(&kind, &observed); err != nil {
		t.Fatal(err)
	}
	if kind != "offline_rebuild" || !observed {
		t.Fatal("rebuild replaced original observation time")
	}
	if _, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	outcome, err = run(id)
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("historical validate: %s %v", outcome, err)
	}
	var owner string
	if err := pool.QueryRow(ctx, `SELECT current_name_1 FROM parcels WHERE source_parcel_id='A'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "newer" {
		t.Fatal("historical replay superseded newer evidence")
	}
}

func TestParcelCompletedCaptureRetriesOfflineAfterFailedFirstPublication(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "saved", true, 1)}}
	source := fixture.source(t, 1)
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT reject_first_owner CHECK(current_name_1 <> 'saved')`); err != nil {
		t.Fatal(err)
	}
	outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
	if outcome != jobs.OutcomeFailed || err == nil {
		t.Fatal(outcome, err)
	}
	root := os.Getenv("PARCEL_EVIDENCE_DIR")
	var id string
	if err := pool.QueryRow(ctx, `SELECT c.id::text FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE a.outcome='failed'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT reject_first_owner`); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	outcome, _, err = runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return (parceljob.Job{Root: root, DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}).Replay(ctx, id)
	}})
	if outcome != jobs.OutcomeSuccess || err != nil {
		t.Fatal(outcome, err)
	}
	var kind, owner string
	if err := pool.QueryRow(ctx, `SELECT o.publication_kind,p.current_name_1 FROM parcel_evidence_observations o,parcels p`).Scan(&kind, &owner); err != nil {
		t.Fatal(err)
	}
	if kind != "offline_retry" || owner != "saved" {
		t.Fatal(kind, owner)
	}
}

func TestParcelEvidenceReferencesRunnerAttempt(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "first", true, 1)}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	var missing int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM parcel_evidence_observations o LEFT JOIN job_attempts a ON a.id=o.attempt_id WHERE a.id IS NULL)+(SELECT count(*) FROM parcel_evidence_history h LEFT JOIN job_attempts a ON a.id=h.attempt_id WHERE a.id IS NULL)`).Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing != 0 {
		t.Fatal("source provenance lacks runner attempt")
	}
}
