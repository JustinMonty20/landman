//go:build postgres

package main

import (
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"testing"
)

func TestParcelPersistedGeometryAndNormalizedFingerprintStable(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	shell := [][]int{{2, 0}, {2, 1}, {3, 1}, {3, 0}, {2, 0}}
	second := [][]int{{8, 0}, {8, 1}, {9, 1}, {9, 0}, {8, 0}}
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 1)}}
	fixture.features[0]["geometry"] = map[string]interface{}{"rings": [][][]int{shell, second}}
	source := fixture.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var oldNormalized, oldGeometry, oldSource, oldTimestamp string
	if err := pool.QueryRow(ctx, `SELECT c.normalized_hash,c.geometry_hash,c.source_hash,p.updated_at::text FROM parcel_evidence_current c JOIN parcels p USING(county_id,source_name,source_parcel_id)`).Scan(&oldNormalized, &oldGeometry, &oldSource, &oldTimestamp); err != nil {
		t.Fatal(err)
	}
	var reversed []byte
	if err := pool.QueryRow(ctx, `SELECT ST_AsEWKB(ST_Normalize(ST_Multi(ST_Reverse(shape)))) FROM parcels WHERE source_parcel_id='A'`).Scan(&reversed); err != nil {
		t.Fatal(err)
	}
	if h := parcelevidence.GeometryHash(reversed); h != oldGeometry {
		t.Fatalf("typed geometry ring direction changed fingerprint: %s != %s", h, oldGeometry)
	}
	fixture.features[0]["geometry"] = map[string]interface{}{"rings": [][][]int{{{9, 1}, {9, 0}, {8, 0}, {8, 1}, {9, 1}}, {{3, 1}, {3, 0}, {2, 0}, {2, 1}, {3, 1}}}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var normalized, geometry, sourceHash, timestamp string
	if err := pool.QueryRow(ctx, `SELECT c.normalized_hash,c.geometry_hash,c.source_hash,p.updated_at::text FROM parcel_evidence_current c JOIN parcels p USING(county_id,source_name,source_parcel_id)`).Scan(&normalized, &geometry, &sourceHash, &timestamp); err != nil {
		t.Fatal(err)
	}
	if oldNormalized != normalized || oldGeometry != geometry || oldSource != sourceHash || oldTimestamp != timestamp {
		t.Fatalf("equal component order/rotation rewrote serving row: normalized %v geometry %v source %v updated %v", oldNormalized == normalized, oldGeometry == geometry, oldSource == sourceHash, oldTimestamp == timestamp)
	}
	fixture.features[0]["attributes"].(map[string]interface{})["SOURCE_NOTE"] = "source-only change"
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var sourceOnlyNormalized, sourceOnlyHash, sourceOnlyTime string
	if err := pool.QueryRow(ctx, `SELECT c.normalized_hash,c.source_hash,p.updated_at::text FROM parcel_evidence_current c JOIN parcels p USING(county_id,source_name,source_parcel_id)`).Scan(&sourceOnlyNormalized, &sourceOnlyHash, &sourceOnlyTime); err != nil {
		t.Fatal(err)
	}
	if sourceOnlyHash == sourceHash || sourceOnlyNormalized != normalized || sourceOnlyTime != timestamp {
		t.Fatal("source-only change rewrote serving content or lost source provenance")
	}

}

func TestArcGISInteriorRingDirectionChangesTopologyAndSourceFingerprint(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	outer := [][]int{{0, 0}, {0, 4}, {4, 4}, {4, 0}, {0, 0}}
	inner := [][]int{{1, 1}, {2, 1}, {2, 2}, {1, 2}, {1, 1}}
	fixture.features[0]["geometry"] = map[string]interface{}{"rings": [][][]int{outer, inner}}
	source := fixture.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var firstArea float64
	var firstHash string
	if err := pool.QueryRow(ctx, `SELECT ST_Area(p.shape),c.source_hash FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id)`).Scan(&firstArea, &firstHash); err != nil {
		t.Fatal(err)
	}
	fixture.features[0]["geometry"] = map[string]interface{}{"rings": [][][]int{outer, {{1, 1}, {1, 2}, {2, 2}, {2, 1}, {1, 1}}}}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var secondArea float64
	var secondHash string
	if err := pool.QueryRow(ctx, `SELECT ST_Area(p.shape),c.source_hash FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id)`).Scan(&secondArea, &secondHash); err != nil {
		t.Fatal(err)
	}
	if firstArea == secondArea || firstHash == secondHash {
		t.Fatalf("direction lost topology or source meaning: areas %v,%v hashes equal %v", firstArea, secondArea, firstHash == secondHash)
	}
}
