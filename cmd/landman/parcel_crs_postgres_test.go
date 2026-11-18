//go:build postgres

package main

import (
	"github.com/JustinMonty20/landman/internal/jobs"
	"testing"
)

func TestParcelSourceCRSAliasAndOriginalEvidence(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{crs: 102100, features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	outcome, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0)
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatal(outcome, err)
	}
	var sourceSRID, working, declared int
	if err := pool.QueryRow(ctx, `SELECT p.source_srid,ST_SRID(p.shape),(c.manifest #>> '{receipts,1,declared_crs}')::int FROM parcels p JOIN parcel_evidence_current e ON e.source_parcel_id=p.source_parcel_id JOIN parcel_evidence_observations c ON c.id=e.observation_id`).Scan(&sourceSRID, &working, &declared); err != nil {
		t.Fatal(err)
	}
	if sourceSRID != 102100 || working != 3857 || declared != 102100 {
		t.Fatalf("source=%d working=%d receipt=%d", sourceSRID, working, declared)
	}
}
