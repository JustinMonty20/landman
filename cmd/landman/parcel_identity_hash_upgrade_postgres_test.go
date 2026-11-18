//go:build postgres

package main

import (
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
)

func TestOversizedPIDStillParticipatesInObjectIdentityChecks(t *testing.T) {
	for _, oversizedFirst := range []bool{true, false} {
		name := "oversized-last"
		if oversizedFirst {
			name = "oversized-first"
		}
		t.Run(name, func(t *testing.T) {
			ctx, dsn, pool := parcelFixtureDB(t)
			long := feature(1, strings.Repeat("x", 9000), "oversized", true, 0)
			short := feature(1, "A", "conflicting", true, 0)
			parts := []map[string]interface{}{long, short, feature(2, "B", "neighbor", true, 0)}
			if !oversizedFirst {
				parts[0], parts[1] = parts[1], parts[0]
			}
			fixture := &parcelHTTPFixture{features: parts}
			outcome, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0)
			if outcome != jobs.OutcomeFailed || err == nil || !strings.Contains(err.Error(), "conflicting PID for OBJECTID 1") {
				t.Fatalf("expected conflicting identity to fail: outcome=%s error=%v", outcome, err)
			}
			for _, table := range []string{"parcels", "parcel_sales", "parcel_evidence_observations"} {
				if countRows(t, ctx, pool, table) != 0 {
					t.Fatalf("conflicting import published rows in %s", table)
				}
			}
		})
	}
}

func TestHashUpgradeRestoresMissingServingShape(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "same", true, 1)}}
	source := fixture.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var id string
	var ewkb []byte
	if err := pool.QueryRow(ctx, `SELECT id::text,ST_AsEWKB(shape) FROM parcels`).Scan(&id, &ewkb); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcels SET shape=NULL;
		UPDATE parcel_evidence_current SET normalized_hash_version='parcel-storage-v1',normalized_hash=repeat('0',64)`); err != nil {
		t.Fatal(err)
	}
	outcome, result, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
	if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != 1 {
		t.Fatalf("shape repair: outcome=%s result=%+v error=%v", outcome, result, err)
	}
	var restored bool
	var version string
	if err := pool.QueryRow(ctx, `SELECT (ST_AsEWKB(p.shape)=$2) IS TRUE,c.normalized_hash_version
		FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) WHERE p.id=$1::uuid`, id, ewkb).Scan(&restored, &version); err != nil {
		t.Fatal(err)
	}
	if !restored || version != parcelevidence.NormalizedHashVersion {
		t.Fatalf("shape restored=%t hash version=%s", restored, version)
	}
}
