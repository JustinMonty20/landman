//go:build postgres

package qualification

import (
	"context"
	"math"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
)

func TestAsymmetricTouchAndSameNameConflict(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	var zoningGen, roadsGen, parcel string
	for _, g := range []struct {
		group string
		dest  *string
	}{{"zoning", &zoningGen}, {"roads", &roadsGen}} {
		if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES($1,now()) RETURNING id::text`, g.group).Scan(g.dest); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES($1,$2) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, g.group, *g.dest); err != nil {
			t.Fatal(err)
		}
	}
	const x = -8950000
	const y = 4180000
	parcelWKT := `POLYGON((-8950000 4180000,-8949900 4180000,-8949900 4180100,-8950000 4180100,-8950000 4180000))`
	touchWKT := `POLYGON((-8949900 4180000,-8949800 4180000,-8949800 4180100,-8949900 4180100,-8949900 4180050,-8949900 4180000))`
	overlappingWKT := `POLYGON((-8949950 4180020,-8949850 4180020,-8949850 4180080,-8949950 4180080,-8949950 4180020))`
	if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_GeomFromText($2,3857)) RETURNING id::text`, t.Name(), parcelWKT).Scan(&parcel); err != nil {
		t.Fatal(err)
	}
	_ = x
	_ = y
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_zone_hits WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_jurisdiction_hits WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_road_ties WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id IN ($1::uuid,$2::uuid)`, zoningGen, roadsGen)
		pool.Exec(ctx, `DELETE FROM qualification_zones WHERE generation_id=$1`, zoningGen)
		pool.Exec(ctx, `DELETE FROM qualification_jurisdictions WHERE generation_id=$1`, zoningGen)
		pool.Exec(ctx, `DELETE FROM qualification_boundaries WHERE generation_id=$1`, zoningGen)
		pool.Exec(ctx, `DELETE FROM qualification_roads WHERE generation_id=$1`, roadsGen)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id IN ($1::uuid,$2::uuid)`, zoningGen, roadsGen)
	})
	for _, item := range []struct {
		id  int
		wkt string
	}{{1, touchWKT}, {2, `POLYGON((-8949900.001 4180020,-8949899.999 4180020,-8949899.999 4180080,-8949900.001 4180080,-8949900.001 4180020))`}} {
		if _, err := pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,jurisdiction,district_code) SELECT $1,190,$2,3857,g,g,'original',0,'Union','A' FROM (SELECT ST_Multi(ST_GeomFromText($3,3857)) g) s`, zoningGen, item.id, item.wkt); err != nil {
			t.Fatal(err)
		}
	}
	for _, layer := range []int{194, 195} {
		if _, err := pool.Exec(ctx, `INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,name) SELECT $1,$2::integer,$2::bigint,3857,g,g,'original',0,'Monroe' FROM (SELECT ST_Multi(ST_GeomFromText($3,3857)) g) s`, zoningGen, layer, overlappingWKT); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_boundaries(generation_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2) SELECT $1,1,3857,g,g,'original',0 FROM (SELECT ST_Multi(ST_GeomFromText('POLYGON((-8950100 4179900,-8949700 4179900,-8949700 4180200,-8950100 4180200,-8950100 4179900))',3857)) g) s`, zoningGen); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_roads(generation_id,source_id,source_srid,shape) VALUES($1,1,3857,ST_Multi(ST_GeomFromText('LINESTRING(-8949900 4180000,-8949900 4180050,-8949900 4180100)',3857)))`, roadsGen); err != nil {
		t.Fatal(err)
	}
	pending, err := CurrentRoads(ctx, pool, parcel)
	if err != nil || pending.Available || pending.State != "pending_assessment" {
		t.Fatalf("pending state %+v %v", pending, err)
	}
	if _, err = store.reconcileOne(ctx, union.QualificationZoning, ZoningMethod); err != nil {
		t.Fatal(err)
	}
	if _, err = store.reconcileOne(ctx, union.QualificationRoads, RoadsMethod); err != nil {
		t.Fatal(err)
	}
	z, err := CurrentZoning(ctx, pool, parcel)
	if err != nil {
		t.Fatal(err)
	}
	if len(z.Zones) != 1 || z.Zones[0].SourceID != 2 || z.Zones[0].Area.Acres <= 0 || z.JurisdictionAmbiguousAcres <= 0 || len(z.Jurisdictions) != 2 {
		t.Fatalf("touch/positive area/ETJ conflict %+v", z)
	}
	r, err := CurrentRoads(ctx, pool, parcel)
	if err != nil || r.State != "nearest" || r.DistanceFeet == nil || math.Abs(*r.DistanceFeet) > 1e-12 || len(r.Ties) != 1 {
		t.Fatalf("exact stored touch %+v %v", r, err)
	}
	// Far outside the known county footprint must be flagged even beyond one mile.
	if _, err = pool.Exec(ctx, `UPDATE qualification_boundaries SET shape=ST_Multi(ST_GeomFromText('POLYGON((-8980000 4200000,-8979900 4200000,-8979900 4200100,-8980000 4200100,-8980000 4200000))',3857)) WHERE generation_id=$1`, zoningGen); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1 AND source_group='roads'`, parcel); err != nil {
		t.Fatal(err)
	}
	if _, err = store.reconcileOne(ctx, union.QualificationRoads, RoadsMethod); err != nil {
		t.Fatal(err)
	}
	r, err = CurrentRoads(ctx, pool, parcel)
	if err != nil || !r.CountyEdgeWarning {
		t.Fatalf("outside county %+v %v", r, err)
	}
}
