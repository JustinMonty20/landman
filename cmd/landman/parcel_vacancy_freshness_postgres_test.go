//go:build postgres

package main

import (
	"github.com/JustinMonty20/landman/internal/jobs"
	"testing"
)

func TestIdenticalImportRepairsUnsourcedServingEdit(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "source owner", true, 1)}}
	f.features[0]["attributes"].(map[string]interface{})["YEARBLT"] = 2000
	f.features[0]["attributes"].(map[string]interface{})["SQFT"] = 800
	source := f.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var id, before string
	if err := pool.QueryRow(ctx, `SELECT id::text,parcel_row_version FROM parcel_evidence_current c JOIN parcels p USING(county_id,source_name,source_parcel_id) WHERE p.source_parcel_id='A'`).Scan(&id, &before); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcels SET year_built=0,sqft=0,current_name_1='unsourced edit' WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var owner, current, certified string
	var year, sqft int
	if err := pool.QueryRow(ctx, `SELECT p.current_name_1,p.xmin::text,c.parcel_row_version,p.year_built,p.sqft FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) WHERE p.id=$1::uuid`, id).Scan(&owner, &current, &certified, &year, &sqft); err != nil {
		t.Fatal(err)
	}
	if owner != "source owner" || year != 2000 || sqft != 800 || current != certified || current == before {
		t.Fatalf("unsourced edit certified: owner=%q row=%q evidence=%q previous=%q", owner, current, certified, before)
	}
}

func TestRejectedNonGeometryFactsWithdrawPriorParcelFreshness(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "source owner", true, 0)}}
	source := f.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	f.features[0]["attributes"].(map[string]interface{})["YEARBLT"] = 2001
	f.features[0]["attributes"].(map[string]interface{})["mapped_acres"] = -1
	out, result, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
	if err != nil || out != jobs.OutcomeSuccessWithQuarantine || result.Quarantined != 1 {
		t.Fatalf("reject outcome %s %+v %v", out, result, err)
	}
	var id, owner string
	var certified *string
	if err := pool.QueryRow(ctx, `SELECT p.id::text,p.current_name_1,c.parcel_row_version FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) WHERE p.source_parcel_id='A'`).Scan(&id, &owner, &certified); err != nil {
		t.Fatal(err)
	}
	if id == "" || owner != "source owner" || certified != nil {
		t.Fatalf("rejection kept positive certification: id=%q owner=%q row=%v", id, owner, certified)
	}
}

func TestObjectIDFallbackWithdrawsOldPIDCertification(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(7, "A", "source owner", true, 0)}}
	source := f.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT p.id::text FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) WHERE p.source_parcel_id='A' AND c.parcel_row_version=p.xmin::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	f.features[0] = feature(7, "", "fallback owner", true, 0)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var retained string
	var certification *string
	if err := pool.QueryRow(ctx, `SELECT p.id::text,c.parcel_row_version FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) WHERE p.source_parcel_id='A'`).Scan(&retained, &certification); err != nil {
		t.Fatal(err)
	}
	if retained != id || certification != nil {
		t.Fatalf("fallback retained old eligibility: id=%s certification=%v", retained, certification)
	}
	var fallback int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcels WHERE source_parcel_id='7'`).Scan(&fallback); err != nil || fallback != 1 {
		t.Fatalf("fallback identity count=%d %v", fallback, err)
	}
}
