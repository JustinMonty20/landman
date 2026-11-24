//go:build postgres

package main

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

// recordOfflineParcelCapture seals local fixture responses without publishing business rows.
func recordOfflineParcelCapture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, source *union.GISSource) (string, time.Time) {
	t.Helper()
	store, err := taxartifact.NewLocal(os.Getenv("PARCEL_EVIDENCE_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := ingest.NewRecorder(store, "union_county_gis", time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	count, err := source.CaptureParcelPages(ctx, func(request string, body []byte, crs int) error {
		return recorder.Record(ctx, request, "", crs, bytes.NewReader(body))
	}, func([]connector.RawRecord) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Run: func(ctx context.Context) (jobs.Result, error) {
		attempt, ok := jobs.AttemptID(ctx)
		if !ok {
			t.Fatal("missing attempt")
		}
		if err := recorder.BindAttempt(parcelJobID, attempt, "full_scan", count); err != nil {
			return jobs.Result{}, err
		}
		id, err = (ingest.Engine{Pool: pool, Store: store}).Seal(ctx, recorder)
		return jobs.Result{}, err
	}})
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("seal fixture: %s %v", outcome, err)
	}
	return id, recorder.Manifest().ObservedAt
}

func TestParcelStaleFailedCaptureDoesNotClaimPublishedFreshness(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "initial", true, 0)}}
	source := fixture.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	fixture.features[0] = feature(1, "A", "failed", true, 0)
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT reject_stale CHECK(current_name_1 <> 'failed')`); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err == nil || out != jobs.OutcomeFailed {
		t.Fatalf("failed capture: %s %v", out, err)
	}
	var staleID string
	if err := pool.QueryRow(ctx, `SELECT c.id::text FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE a.outcome='failed'`).Scan(&staleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT reject_stale`); err != nil {
		t.Fatal(err)
	}
	fixture.features[0] = feature(1, "A", "newest", true, 0)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	out, result, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return (parceljob.Job{DSN: dsn, Root: os.Getenv("PARCEL_EVIDENCE_DIR"), Quarantine: jobs.NewQuarantineStore(pool)}).Replay(ctx, staleID)
	}})
	if err != nil || out != jobs.OutcomeSuccess || result.Processed != 0 {
		t.Fatalf("stale replay: %s %+v %v", out, result, err)
	}
	var owner string
	var marked bool
	if err := pool.QueryRow(ctx, `SELECT p.current_name_1,c.published_at IS NOT NULL FROM parcels p,ingest_captures c WHERE p.source_parcel_id='A' AND c.id=$1::uuid`, staleID).Scan(&owner, &marked); err != nil {
		t.Fatal(err)
	}
	if owner != "newest" || marked || countRows(t, ctx, pool, "parcel_evidence_history") != 2 {
		t.Fatalf("stale capture claimed freshness or changed parcel: owner=%s marked=%t", owner, marked)
	}
}

func TestOlderFailedCaptureCannotCertifyAfterNewerRejection(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "initial", true, 0)}}
	source := fixture.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	fixture.features[0] = feature(1, "A", "failed", true, 0)
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT reject_stale CHECK(current_name_1 <> 'failed')`); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err == nil || out != jobs.OutcomeFailed {
		t.Fatalf("failed capture %s %v", out, err)
	}
	var staleID string
	if err := pool.QueryRow(ctx, `SELECT c.id::text FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE a.outcome='failed'`).Scan(&staleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT reject_stale`); err != nil {
		t.Fatal(err)
	}
	rejected := feature(1, "A", "rejected", true, 0)
	rejected["attributes"].(map[string]interface{})["mapped_acres"] = -1
	fixture.features[0] = rejected
	if out, result, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || out != jobs.OutcomeSuccessWithQuarantine || result.Quarantined != 1 {
		t.Fatalf("new rejection %s %+v %v", out, result, err)
	}
	runner, _ := jobs.NewRunner(pool)
	out, result, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return (parceljob.Job{DSN: dsn, Root: os.Getenv("PARCEL_EVIDENCE_DIR"), Quarantine: jobs.NewQuarantineStore(pool)}).Replay(ctx, staleID)
	}})
	if err != nil || out != jobs.OutcomeSuccess || result.Processed != 0 {
		t.Fatalf("stale replay %s %+v %v", out, result, err)
	}
	var owner string
	var certified *string
	if err := pool.QueryRow(ctx, `SELECT p.current_name_1,c.parcel_row_version FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) WHERE p.source_parcel_id='A'`).Scan(&owner, &certified); err != nil {
		t.Fatal(err)
	}
	if owner != "initial" || certified != nil {
		t.Fatalf("older replay recertified %q %v", owner, certified)
	}
}
