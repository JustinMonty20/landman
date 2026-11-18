//go:build postgres

package main

import (
	"github.com/JustinMonty20/landman/internal/jobs"
	"testing"
)

func TestParcelBaselineHasNoInventedHistoricalSource(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	var oldID string
	err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,current_name_1,shape) VALUES ('union','union','union_county_gis','A','baseline owner',ST_GeomFromText('POLYGON((0 0,0 1,1 1,1 0,0 0))',3857)) RETURNING id::text`).Scan(&oldID)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "observed owner", true, 0)}}
	outcome, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0)
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatal(outcome, err)
	}
	var newID, priorSource, oldOwner, newOwner string
	if err := pool.QueryRow(ctx, `SELECT p.id::text,coalesce(h.old_observation_id::text,''),h.old_fields->>'current_name_1',h.new_fields->>'current_name_1' FROM parcels p JOIN parcel_evidence_history h ON h.source_parcel_id=p.source_parcel_id`).Scan(&newID, &priorSource, &oldOwner, &newOwner); err != nil {
		t.Fatal(err)
	}
	if oldID != newID || priorSource != "" || oldOwner != "baseline owner" || newOwner != "observed owner" {
		t.Fatal(oldID, newID, priorSource, oldOwner, newOwner)
	}
}
