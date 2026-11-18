//go:build postgres

package main

import (
	"context"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"os"
	"testing"

	"github.com/JustinMonty20/landman/internal/jobs"
)

func TestParcelCurrentReplayCorrectsProjectionOnce(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "correct owner", true, 1)}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	var snapshotID string
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations`).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcels SET current_name_1='old projection' WHERE source_parcel_id='A'; UPDATE parcel_evidence_current SET normalized_hash=repeat('0',64),normalizer_version='previous-compiled-version' WHERE source_parcel_id='A'`); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	for range 2 {
		outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
			return parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}.Replay(ctx, snapshotID)
		}})
		if err != nil || outcome != jobs.OutcomeSuccess {
			t.Fatal(outcome, err)
		}
	}
	if countRows(t, ctx, pool, "parcel_evidence_observations") != 1 || countRows(t, ctx, pool, "parcel_evidence_history") != 2 {
		t.Fatal("replay invented source observation or repeated correction")
	}
	var oldOwner, newOwner, cause, oldRef, newRef string
	if err := pool.QueryRow(ctx, `SELECT old_fields->>'current_name_1',new_fields->>'current_name_1',cause,old_observation_id::text,new_observation_id::text FROM parcel_evidence_history ORDER BY id DESC LIMIT 1`).Scan(&oldOwner, &newOwner, &cause, &oldRef, &newRef); err != nil {
		t.Fatal(err)
	}
	if oldOwner != "old projection" || newOwner != "correct owner" || cause != "normalization_correction" || oldRef != newRef {
		t.Fatal(oldOwner, newOwner, cause, oldRef, newRef)
	}
}
