//go:build postgres

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
)

func TestParcelReplayRejectsRewrittenReceiptAndManifest(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "trusted", true, 1)}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("PARCEL_EVIDENCE_DIR")
	snapshot, err := parcelevidence.Open(root, id)
	if err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(root, id, "000001.json")
	body, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	attrs := doc["features"].([]any)[0].(map[string]any)["attributes"].(map[string]any)
	attrs["CURR_NAME1"] = "forged"
	body, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(page, body, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	snapshot.Manifest.Receipts[1].SHA256 = hex.EncodeToString(sum[:])
	snapshot.Manifest.Receipts[1].Bytes = len(body)
	h := sha256.New()
	for i := range snapshot.Manifest.Receipts {
		response, err := os.ReadFile(filepath.Join(root, id, snapshot.Manifest.Receipts[i].Path))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = h.Write([]byte(fmt.Sprintf("%d:", len(response))))
		_, _ = h.Write(response)
	}
	snapshot.Manifest.ArtifactHash = hex.EncodeToString(h.Sum(nil))
	manifest, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, id, "manifest.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = parcelevidence.Open(root, id); err != nil {
		t.Fatalf("fixture local integrity failed: %v", err)
	}
	runner, _ := jobs.NewRunner(pool)
	outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return (parceljob.Job{Root: root, DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}).Replay(ctx, id)
	}})
	if err == nil || outcome != jobs.OutcomeFailed {
		t.Fatalf("trusted manifest substitution accepted: %s %v", outcome, err)
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
	snapshot, err := parcelevidence.New(os.Getenv("PARCEL_EVIDENCE_DIR"), "full_scan")
	if err != nil {
		t.Fatal(err)
	}
	total, err := source.CaptureParcelPages(ctx, snapshot.Save, func([]connector.RawRecord) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Complete(total); err != nil {
		server.Close()
		t.Fatal(err)
	}
	server.Close()
	fixture.features[0] = feature(1, "A", "newer", true, 1)
	runner, _ := jobs.NewRunner(pool)
	job := parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}
	run := func(id string) (jobs.Outcome, error) {
		outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return job.Replay(ctx, id) }})
		return outcome, err
	}
	outcome, err := run(snapshot.Manifest.ID)
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("offline rebuild: %s %v", outcome, err)
	}
	if countRows(t, ctx, pool, "parcel_evidence_observations") != 1 {
		t.Fatal("rebuild missing observation")
	}
	var kind string
	var observed bool
	if err := pool.QueryRow(ctx, `SELECT publication_kind,observed_at=($1::timestamptz) FROM parcel_evidence_observations`, snapshot.Manifest.CreatedAt).Scan(&kind, &observed); err != nil {
		t.Fatal(err)
	}
	if kind != "offline_rebuild" || !observed {
		t.Fatal("rebuild replaced original observation time")
	}
	if _, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	outcome, err = run(snapshot.Manifest.ID)
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
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("capture missing %v %d", err, len(entries))
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT reject_first_owner`); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	outcome, _, err = runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return (parceljob.Job{Root: root, DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}).Replay(ctx, entries[0].Name())
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
