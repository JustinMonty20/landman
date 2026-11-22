//go:build postgres

package main

import (
	"slices"
	"testing"

	"github.com/JustinMonty20/landman/internal/jobs"
)

func TestParcelGeometryRefusalRecovery(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	good := feature(1, "A", "owner", true, 2)
	f := &parcelHTTPFixture{features: []map[string]any{good}}
	source := f.source(t, 2)
	run := func(want jobs.Outcome) {
		t.Helper()
		out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
		if err != nil || out != want {
			t.Fatal(out, err)
		}
	}
	run(jobs.OutcomeSuccess)
	before := parcelSnapshot(t, ctx, pool)[0]
	if _, err := pool.Exec(ctx, `CREATE TABLE tax_bills (LIKE public.tax_bills INCLUDING ALL); ALTER TABLE tax_bills ADD FOREIGN KEY(parcel_id) REFERENCES parcels(id) ON DELETE CASCADE; INSERT INTO tax_bills(county_id,source_name,parcel_id,tax_year,total_taxes,source_url,file_sha256,fetched_at) SELECT 'union','fixture',id,2026,123,'fixture',decode(repeat('00',32),'hex'),now() FROM parcels`); err != nil {
		t.Fatal(err)
	}
	var accepted string
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations`).Scan(&accepted); err != nil {
		t.Fatal(err)
	}
	spike := feature(2, "A", "must not publish", true, 0)
	spike["geometry"] = map[string]any{"rings": [][][]int{{{0, 0}, {0, 4}, {2, 4}, {2, 5}, {2, 4}, {4, 4}, {4, 0}, {0, 0}}}}
	f.features = append(f.features, spike)
	run(jobs.OutcomeSuccessWithQuarantine)
	assertBoundaryCleared(t, before, parcelSnapshot(t, ctx, pool)[0])
	var rejected string
	if err := pool.QueryRow(ctx, `SELECT d.artifact_id FROM parcel_geometry_decisions d JOIN parcel_evidence_current c USING(source_parcel_id) JOIN parcel_evidence_observations o ON o.id=c.observation_id WHERE d.outcome='zero_movement_refused' AND object_ids=ARRAY[1,2]::bigint[] AND prior_shape_at=o.observed_at AND o.artifact_id=$1 AND ST_IsValid(prior_valid_shape)`, accepted).Scan(&rejected); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	replay := func(id string) {
		t.Helper()
		if _, _, err := replayFollowup(t, ctx, dsn, runner, pool, id); err != nil {
			t.Fatal(err)
		}
	}
	replay(accepted)
	replay(rejected)
	assertBoundaryCleared(t, before, parcelSnapshot(t, ctx, pool)[0])
	if countRows(t, ctx, pool, "quarantined_records WHERE occurrence_count=1 AND reprocessed_at IS NULL") != 2 {
		t.Fatal("quarantine changed during replay")
	}
	f.features = []map[string]any{good}
	run(jobs.OutcomeSuccess)
	replay(rejected)
	if parcelSnapshot(t, ctx, pool)[0] != before {
		t.Fatal("recovery did not restore identical facts and shape")
	}
	if countRows(t, ctx, pool, "parcel_derived_handoff WHERE request_generation=1 AND screening_pending AND qualification_pending") != 3 {
		t.Fatal("expected handoff only on publish, clear and restore")
	}
	if countRows(t, ctx, pool, "tax_bills JOIN parcels p ON p.id=tax_bills.parcel_id WHERE p.source_parcel_id='A' AND total_taxes=123") != 1 {
		t.Fatal("lost tax link")
	}
	if countRows(t, ctx, pool, "parcel_geometry_decisions") != 1 {
		t.Fatal("duplicate geometry decision")
	}
}

func TestParcelRepairRecordsEvidenceWithoutChangingEqualBoundary(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	part := feature(17, "A", "owner", true, 1)
	part["geometry"] = map[string]any{"rings": [][][]int{{{0, 0}, {0, 2}, {2, 2}, {2, 0}, {0, 0}}, {{0, 0}, {0, -2}, {-2, -2}, {-2, 0}, {0, 0}}}}
	f := &parcelHTTPFixture{features: []map[string]any{part}}
	source := f.source(t, 1)
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || out != jobs.OutcomeSuccess {
		t.Fatal(out, err)
	}
	before := parcelSnapshot(t, ctx, pool)[0]
	part["geometry"] = map[string]any{"rings": [][][]int{{{0, 0}, {0, 2}, {2, 2}, {2, 0}, {0, 0}, {0, -2}, {-2, -2}, {-2, 0}, {0, 0}}}}
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || out != jobs.OutcomeSuccess {
		t.Fatal(out, err)
	}
	var capture string
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_geometry_decisions d JOIN parcel_evidence_current c USING(source_parcel_id) WHERE d.outcome='repaired' AND object_ids=ARRAY[17]::bigint[] AND d.geometry_hash=c.geometry_hash AND d.prior_valid_shape IS NULL`).Scan(&capture); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	if _, _, err := replayFollowup(t, ctx, dsn, runner, pool, capture); err != nil {
		t.Fatal(err)
	}
	if parcelSnapshot(t, ctx, pool)[0] != before || countRows(t, ctx, pool, "parcel_derived_handoff") != 1 || countRows(t, ctx, pool, "parcel_geometry_decisions") != 1 || countRows(t, ctx, pool, "parcel_evidence_history") != 1 {
		t.Fatal("equivalent repair changed serving facts or repeated work")
	}
}

func TestGeometryRefusalStorageErrorAbortsPublication(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]any{feature(1, "A", "first", true, 1), feature(2, "B", "second", true, 1)}}
	source := f.source(t, 2)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	before := parcelSnapshot(t, ctx, pool)
	if _, err := pool.Exec(ctx, `ALTER TABLE parcel_geometry_decisions ADD CONSTRAINT fixture_failure CHECK(outcome<>'zero_movement_refused')`); err != nil {
		t.Fatal(err)
	}
	f.features[0] = feature(1, "A", "must roll back", true, 1)
	f.features[1]["geometry"] = map[string]any{"rings": [][][]int{{{0, 0}, {0, 4}, {2, 4}, {2, 5}, {2, 4}, {4, 4}, {4, 0}, {0, 0}}}}
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err == nil || out != jobs.OutcomeFailed {
		t.Fatal(out, err)
	}
	after := parcelSnapshot(t, ctx, pool)
	if !slices.Equal(before, after) || countRows(t, ctx, pool, "parcel_evidence_observations") != 1 || countRows(t, ctx, pool, "parcel_geometry_decisions") != 0 || countRows(t, ctx, pool, "parcel_derived_handoff") != 1 || countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 1 {
		t.Fatal("partial publication or lost independent quarantine after decision failure")
	}
}
