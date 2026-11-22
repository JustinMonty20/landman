//go:build postgres

package main

import (
	"context"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/jobs"
	"os"
	"testing"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/soil"
)

func TestParcelDerivedWorkChecksPersistedDependencies(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"soil_polygon_sets", "soil_polygon_set_heads", "soil_screening_runs", "soil_screening_heads", "soil_screening_refresh_status", "soil_parcel_assessments"} {
		if _, err := pool.Exec(ctx, "CREATE TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	due := func() bool {
		t.Helper()
		v, err := soil.NeedsScreenStored(ctx, pool, county.Union, "union_county_gis")
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if !due() {
		t.Fatal("missing assessment was skipped")
	}
	var setID, runID string
	if err := pool.QueryRow(ctx, `INSERT INTO soil_polygon_sets(county_id,source_name,fingerprint) VALUES ('union','union_county_gis','set-a') RETURNING id::text`).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO soil_polygon_set_heads(county_id,source_name,set_id) VALUES ('union','union_county_gis',$1)`, setID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO soil_screening_runs(county_id,source_name,source_url,method,soil_set_id,assessment_method,fresh_until) VALUES ('union','union_county_gis','fixture','USDA Dominant Condition; tie break 1; no component cutoff',$1,$2,now()+interval '1 day') RETURNING id::text`, setID, soil.AssessmentMethod).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`INSERT INTO soil_screening_heads(county_id,source_name,run_id) VALUES ('union','union_county_gis',$1)`, `INSERT INTO soil_parcel_assessments(run_id,parcel_id,geometry_fingerprint,uncovered_acres,ambiguous_acres) SELECT $1,id,md5(ST_AsEWKB(shape)),0,0 FROM parcels`} {
		if _, err := pool.Exec(ctx, query, runID); err != nil {
			t.Fatal(err)
		}
	}
	if due() {
		t.Fatal("unchanged assessed parcel scheduled spatial work")
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_screening_runs SET assessment_method='exclusive-map-unit-union/1' WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	if !due() {
		t.Fatal("previous soil assessment method was not due for refresh")
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_screening_runs SET assessment_method=$2 WHERE id=$1`, runID, soil.AssessmentMethod); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcels SET current_name_1='owner-only'`); err != nil {
		t.Fatal(err)
	}
	if due() {
		t.Fatal("owner-only update scheduled soil intersections")
	}
	var snapshotID string
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations`).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcels SET shape=ST_Translate(shape,100,0); UPDATE parcel_evidence_current SET normalized_hash=repeat('0',64),normalizer_version='old-version',geometry_hash=repeat('0',64) WHERE source_parcel_id='A'`); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return (parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}).Replay(ctx, snapshotID)
	}})
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("geometry correction %s %v", outcome, err)
	}
	var corrections int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcel_evidence_history WHERE cause='normalization_correction' AND old_geometry_hash IS DISTINCT FROM new_geometry_hash`).Scan(&corrections); err != nil || corrections != 1 {
		t.Fatalf("geometry correction history=%d error=%v", corrections, err)
	}
	if due() {
		t.Fatal("restored original geometry should match assessed fingerprint")
	}
	fixture.features[0]["geometry"] = map[string]interface{}{"rings": [][][]int{{{2, 0}, {2, 2}, {3, 2}, {3, 0}, {2, 0}}}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	if !due() {
		t.Fatal("real source geometry update did not stale stored assessment")
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_parcel_assessments SET geometry_fingerprint=(SELECT md5(ST_AsEWKB(shape)) FROM parcels)`); err != nil {
		t.Fatal(err)
	}
	if due() {
		t.Fatal("fresh source geometry assessment still due")
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_parcel_assessments SET geometry_fingerprint='stale'`); err != nil {
		t.Fatal(err)
	}
	if !due() {
		t.Fatal("stale dependency was skipped")
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_parcel_assessments SET geometry_fingerprint=(SELECT md5(ST_AsEWKB(shape)) FROM parcels)`); err != nil {
		t.Fatal(err)
	}
	if due() {
		t.Fatal("updated geometry assessment still due")
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_screening_runs SET fresh_until=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if !due() {
		t.Fatal("expired evidence was skipped")
	}
	if _, err := pool.Exec(ctx, `UPDATE soil_screening_runs SET fresh_until=now()+interval '1 day'; UPDATE soil_screening_heads SET refresh_failed_at=now()`); err != nil {
		t.Fatal(err)
	}
	if !due() {
		t.Fatal("failed refresh was skipped")
	}
}
