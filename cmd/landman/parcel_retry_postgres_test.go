//go:build postgres

package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/jobs"
)

func TestParcelEvidenceMigrationRejectsLossyRollback(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, f.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("db/migrations/000024_parcel_evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(migration)
	marker := "-- +goose Down"
	if offset := strings.Index(down, marker); offset >= 0 {
		down = down[offset+len(marker):]
	} else {
		t.Fatal("missing rollback")
	}
	if _, err = pool.Exec(ctx, down); err == nil {
		t.Fatal("migration discarded registered capture")
	}
	if countRows(t, ctx, pool, "ingest_captures") != 1 || countRows(t, ctx, pool, "parcel_evidence_observations") != 1 {
		t.Fatal("rejected rollback changed published provenance")
	}
}

func TestRegisteredFailedCaptureRetryPublishesSourceOnlyWithoutBusinessWrites(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{
		feature(1, "A", "initial", true, 0), feature(2, "B", "unchanged", true, 1),
	}}
	source := f.source(t, 2)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var previousObservation, previousHash, previousTimestamp, previousSaleTimestamp string
	if err := pool.QueryRow(ctx, `SELECT c.observation_id::text,c.source_hash,p.updated_at::text,s.updated_at::text
		FROM parcel_evidence_current c JOIN parcels p USING(county_id,source_name,source_parcel_id)
		JOIN parcel_sales s ON s.parcel_id=p.id WHERE c.source_parcel_id='B'`).Scan(&previousObservation, &previousHash, &previousTimestamp, &previousSaleTimestamp); err != nil {
		t.Fatal(err)
	}
	f.features[0] = feature(1, "A", "retry owner", true, 0)
	f.features[1]["attributes"].(map[string]interface{})["SOURCE_NOTE"] = "new assertion"
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT fail_owner_for_retry CHECK(current_name_1 <> 'retry owner')`); err != nil {
		t.Fatal(err)
	}
	outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
	if err == nil || outcome != jobs.OutcomeFailed {
		t.Fatalf("first registered capture unexpectedly published: %s %v", outcome, err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT c.id::text FROM ingest_captures c
		JOIN job_attempts a ON a.id=c.attempt_id WHERE a.outcome='failed' ORDER BY a.started_at DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT fail_owner_for_retry`); err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	outcome, result, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(runCtx context.Context) (jobs.Result, error) {
		return (parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}).Replay(runCtx, id)
	}})
	if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != 2 {
		t.Fatalf("authorized retry: %s %+v %v", outcome, result, err)
	}
	var owner, note, kind, currentObservation, currentHash, timestamp, saleTimestamp string
	if err := pool.QueryRow(ctx, `SELECT (SELECT current_name_1 FROM parcels WHERE source_parcel_id='A'),
		c.source_features#>>'{0,attributes,SOURCE_NOTE}',o.publication_kind,
		c.observation_id::text,c.source_hash,p.updated_at::text,s.updated_at::text
		FROM parcel_evidence_current c JOIN parcel_evidence_observations o ON o.id=c.observation_id
		JOIN parcels p ON p.county_id=c.county_id AND p.source_name=c.source_name AND p.source_parcel_id=c.source_parcel_id
		JOIN parcel_sales s ON s.parcel_id=p.id WHERE c.source_parcel_id='B'`).Scan(&owner, &note, &kind, &currentObservation, &currentHash, &timestamp, &saleTimestamp); err != nil {
		t.Fatal(err)
	}
	if owner != "retry owner" || note != "new assertion" || kind != "offline_retry" || currentObservation == previousObservation || currentHash == previousHash || timestamp != previousTimestamp || saleTimestamp != previousSaleTimestamp {
		t.Fatalf("retry provenance or unchanged serving mismatch: owner=%s note=%s kind=%s observation advanced=%v hash advanced=%v business unchanged=%v sale unchanged=%v",
			owner, note, kind, currentObservation != previousObservation, currentHash != previousHash, timestamp == previousTimestamp, saleTimestamp == previousSaleTimestamp)
	}
}
