//go:build postgres

package qualification

import (
	"context"
	"math"
	"testing"
)

func TestReconcileMixedDimensionSourceContacts(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	var gen string
	if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('zoning',now()) RETURNING id::text`).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
		pool.Exec(ctx, `DELETE FROM qualification_zones WHERE generation_id=$1`, gen)
		pool.Exec(ctx, `DELETE FROM qualification_jurisdictions WHERE generation_id=$1`, gen)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('zoning',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, gen); err != nil {
		t.Fatal(err)
	}
	const x, y = -8950000, 4180000
	parcels := make([]string, 2)
	for i, wkt := range []string{
		"POLYGON((0 0,30 0,30 15,0 15,0 0))",
		"POLYGON((40 0,60 0,60 15,40 15,40 0))",
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_Translate(ST_GeomFromText($2,3857),$3,$4)) RETURNING id::text`, t.Name()+[]string{"-conflict", "-independent"}[i], wkt, x, y).Scan(&parcels[i]); err != nil {
			t.Fatal(err)
		}
		parcel := parcels[i]
		t.Cleanup(func() {
			pool.Exec(ctx, `DELETE FROM qualification_zone_hits WHERE parcel_id=$1`, parcel)
			pool.Exec(ctx, `DELETE FROM qualification_jurisdiction_hits WHERE parcel_id=$1`, parcel)
			pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
			pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
		})
	}
	for i, wkt := range []string{
		"POLYGON((0 0,30 0,30 10,0 10,0 0))",
		"POLYGON((0 5,10 5,10 12,20 12,20 10,30 10,30 15,0 15,0 5))",
		"POLYGON((40 0,60 0,60 15,40 15,40 0))",
		"POLYGON((60 0,70 0,70 15,60 15,60 0))",
	} {
		id := i + 1
		if _, err := pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,jurisdiction,district_code)
SELECT $1,190,$2,3857,g,g,'original',0,'Union',$3 FROM (SELECT ST_Multi(ST_Translate(ST_GeomFromText($4,3857),$5,$6)) g) s`, gen, id, string(rune('A'+i)), wkt, x, y); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,name)
SELECT $1,195,$2,3857,g,g,'original',0,$3 FROM (SELECT ST_Multi(ST_Translate(ST_GeomFromText($4,3857),$5,$6)) g) s`, gen, id, string(rune('A'+i)), wkt, x, y); err != nil {
			t.Fatal(err)
		}
	}
	var kind string
	var polygonArea, lineLength, expectedAcres float64
	if err := pool.QueryRow(ctx, `SELECT ST_GeometryType(pair), ST_Area(ST_CollectionExtract(pair,3)), ST_Length(ST_CollectionExtract(pair,2))
FROM (SELECT ST_Intersection(a.shape,b.shape) pair FROM qualification_zones a JOIN qualification_zones b ON a.generation_id=b.generation_id
WHERE a.generation_id=$1 AND a.source_id=1 AND b.source_id=2) s`, gen).Scan(&kind, &polygonArea, &lineLength); err != nil {
		t.Fatal(err)
	}
	if kind != "ST_GeometryCollection" || polygonArea != 50 || lineLength != 10 {
		t.Fatalf("fixture must have 50m² polygon and separate 10m line: %s, %g, %g", kind, polygonArea, lineLength)
	}
	if err := pool.QueryRow(ctx, `SELECT ST_Area(ST_Transform(ST_Segmentize(g,10),2264))/43560
FROM (SELECT ST_Translate(ST_GeomFromText('POLYGON((0 5,10 5,10 10,0 10,0 5))',3857),$1,$2) g) s`, x, y).Scan(&expectedAcres); err != nil {
		t.Fatal(err)
	}
	if n, err := store.Reconcile(ctx); err != nil || n != 2 {
		t.Fatalf("reconcile both parcels: count=%d err=%v", n, err)
	}
	conflict, err := CurrentZoning(ctx, pool, parcels[0])
	if err != nil || !conflict.Available || conflict.Freshness.Stale || conflict.Split || len(conflict.Zones) != 2 || len(conflict.Jurisdictions) != 2 ||
		math.Abs(conflict.AmbiguousAcres-expectedAcres) > 1e-8 || math.Abs(conflict.JurisdictionAmbiguousAcres-expectedAcres) > 1e-8 {
		t.Fatalf("50m² mixed-contact conflict: %+v err=%v expected acres=%g", conflict, err, expectedAcres)
	}
	independent, err := CurrentZoning(ctx, pool, parcels[1])
	if err != nil || !independent.Available || independent.Freshness.Stale || independent.Split || len(independent.Zones) != 1 || len(independent.Jurisdictions) != 1 ||
		independent.Zones[0].SourceID != 3 || independent.Jurisdictions[0].SourceID != 3 || independent.AmbiguousAcres != 0 || independent.JurisdictionAmbiguousAcres != 0 || independent.UnmappedAcres != 0 || independent.JurisdictionUnmappedAcres != 0 {
		t.Fatalf("unaffected parcel with edge-only neighbor: %+v err=%v", independent, err)
	}
	if n, err := store.Reconcile(ctx); err != nil || n != 0 {
		t.Fatalf("clean replay: count=%d err=%v", n, err)
	}
}
