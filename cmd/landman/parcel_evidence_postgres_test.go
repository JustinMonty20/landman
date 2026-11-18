//go:build postgres

package main

import (
	"context"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"os"
	"testing"

	"github.com/JustinMonty20/landman/internal/jobs"
)

func TestParcelEvidenceObservationAndOfflineReplay(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "first", true, 1), feature(2, "A", "last", true, 1)}}
	source := fixture.source(t, 1)
	if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatal(outcome, err)
	}
	var snapshotID, parcelID, saleID, contentTime string
	if err := pool.QueryRow(ctx, `SELECT o.artifact_id,p.id::text,s.id::text,p.updated_at::text FROM parcel_evidence_observations o,parcels p JOIN parcel_sales s ON s.parcel_id=p.id ORDER BY o.published_at DESC LIMIT 1`).Scan(&snapshotID, &parcelID, &saleID, &contentTime); err != nil {
		t.Fatal(err)
	}
	if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatal(outcome, err)
	}
	var currentParcel, currentSale, currentTime string
	if err := pool.QueryRow(ctx, `SELECT p.id::text,s.id::text,p.updated_at::text FROM parcels p JOIN parcel_sales s ON s.parcel_id=p.id`).Scan(&currentParcel, &currentSale, &currentTime); err != nil {
		t.Fatal(err)
	}
	if currentParcel != parcelID || currentSale != saleID || currentTime != contentTime {
		t.Fatal("identical observation changed parcel or sale content")
	}
	if countRows(t, ctx, pool, "parcel_evidence_observations") != 2 || countRows(t, ctx, pool, "parcel_evidence_history") != 1 {
		t.Fatal("unchanged observation lost or created false field history")
	}
	fixture.features[1] = feature(2, "A", "new owner", true, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var oldOwner, newOwner, cause string
	if err := pool.QueryRow(ctx, `SELECT old_fields->>'current_name_1',new_fields->>'current_name_1',cause FROM parcel_evidence_history ORDER BY id DESC LIMIT 1`).Scan(&oldOwner, &newOwner, &cause); err != nil {
		t.Fatal(err)
	}
	if oldOwner != "last" || newOwner != "new owner" || cause != "source_observation" {
		t.Fatalf("wrong field provenance %s %s %s", oldOwner, newOwner, cause)
	}
	requests := fixture.requests
	runner, _ := jobs.NewRunner(pool)
	outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}.Replay(ctx, snapshotID)
	}})
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatal(outcome, err)
	}
	if fixture.requests != requests {
		t.Fatal("replay contacted source")
	}
	if countRows(t, ctx, pool, "parcel_evidence_observations") != 3 || countRows(t, ctx, pool, "parcel_evidence_history") != 2 {
		t.Fatal("historical replay changed provenance")
	}
	var owner string
	if err := pool.QueryRow(ctx, `SELECT current_name_1 FROM parcels WHERE source_parcel_id='A'`).Scan(&owner); err != nil || owner != "new owner" {
		t.Fatal("historical replay replaced current owner", owner, err)
	}
}
