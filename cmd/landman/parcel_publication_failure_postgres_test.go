//go:build postgres

package main

import (
	"github.com/JustinMonty20/landman/internal/jobs"
	"testing"
)

func TestParcelFailedPublicationPreservesCurrentAndLinks(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "old", true, 1)}}
	source := fixture.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	before := parcelSnapshot(t, ctx, pool)
	var original string
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT reject_next_owner CHECK(current_name_1 <> 'new')`); err != nil {
		t.Fatal(err)
	}
	fixture.features[0] = feature(1, "A", "new", true, 0)
	outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
	if err == nil || outcome != jobs.OutcomeFailed {
		t.Fatal(outcome, err)
	}
	after := parcelSnapshot(t, ctx, pool)
	if len(after) != len(before) || after[0] != before[0] {
		t.Fatal("publication failure replaced current content")
	}
	if countRows(t, ctx, pool, "parcel_evidence_observations") != 1 || countRows(t, ctx, pool, "parcel_evidence_history") != 1 {
		t.Fatal("publication failure advanced evidence")
	}
	var current string
	if err := pool.QueryRow(ctx, `SELECT o.artifact_id FROM parcel_evidence_current c JOIN parcel_evidence_observations o ON o.id=c.observation_id`).Scan(&current); err != nil || current != original {
		t.Fatal(current, original, err)
	}
}
