//go:build postgres

package qualification

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQualificationHoleAndUnavailableStates(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires migrated disposable PostGIS DATABASE_URL")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	var parcel, missing, zone, roads string
	for _, item := range []struct {
		key string
		id  *string
		wkt *string
	}{{"hole", &parcel, strptr("POLYGON((0 0,400 0,400 400,0 400,0 0),(100 100,100 300,300 300,300 100,100 100))")}, {"missing", &missing, nil}} {
		e = pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,CASE WHEN $2::text IS NULL THEN NULL ELSE ST_Transform(ST_GeomFromText($2,2264),3857) END) RETURNING id::text`, `qualification_hole_`+item.key, item.wkt).Scan(item.id)
		if e != nil {
			t.Fatal(e)
		}
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_zone_hits WHERE parcel_id IN ($1,$2)`, parcel, missing)
		pool.Exec(ctx, `DELETE FROM qualification_road_ties WHERE parcel_id IN ($1,$2)`, parcel, missing)
		pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id IN ($1,$2)`, parcel, missing)
		pool.Exec(ctx, `DELETE FROM parcels WHERE id IN ($1,$2)`, parcel, missing)
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE source_group IN ('zoning','roads')`)
		pool.Exec(ctx, `DELETE FROM qualification_zones WHERE generation_id=$1`, zone)
		pool.Exec(ctx, `DELETE FROM qualification_roads WHERE generation_id=$1`, roads)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id IN ($1,$2)`, zone, roads)
	})
	before, e := CurrentRoads(ctx, pool, parcel)
	if e != nil || before.Available || before.State != "source_unavailable" || len(before.Freshness.Reasons) != 1 || before.Freshness.Reasons[0] != "source_unavailable" {
		t.Fatalf("absent road source: %+v %v", before, e)
	}
	for _, g := range []struct {
		group string
		id    *string
	}{{"zoning", &zone}, {"roads", &roads}} {
		e = pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES($1,now()) RETURNING id::text`, g.group).Scan(g.id)
		if e != nil {
			t.Fatal(e)
		}
		_, e = pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES($1,$2)`, g.group, *g.id)
		if e != nil {
			t.Fatal(e)
		}
	}
	for _, wkt := range []string{"POLYGON((150 150,250 150,250 250,150 250,150 150))", "POLYGON((400 0,450 0,450 400,400 400,400 0))"} {
		_, e = pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,jurisdiction,district_code,source_srid,shape) VALUES($1,190,(SELECT count(*)+1 FROM qualification_zones WHERE generation_id=$1),'Union County','A',2264,ST_Multi(ST_Transform(ST_GeomFromText($2,2264),3857)))`, zone, wkt)
		if e != nil {
			t.Fatal(e)
		}
	}
	_, e = pool.Exec(ctx, `INSERT INTO qualification_roads(generation_id,source_id,source_srid,shape) VALUES($1,1,2264,ST_Transform(ST_GeomFromText('MULTILINESTRING((150 200,250 200))',2264),3857))`, roads)
	if e != nil {
		t.Fatal(e)
	}
	store := Store{Pool: pool}
	n, e := store.Reconcile(ctx)
	if e != nil || n != 4 {
		t.Fatalf("reconcile=%d %v", n, e)
	}
	z, e := CurrentZoning(ctx, pool, parcel)
	if e != nil || len(z.Zones) != 0 || z.TotalAcres == nil || math.Abs(z.UnmappedAcres-*z.TotalAcres) > 0.001 {
		t.Fatalf("hole and contact: %+v %v", z, e)
	}
	r, e := CurrentRoads(ctx, pool, parcel)
	if e != nil || r.State != "nearest" || r.DistanceFeet == nil || math.Abs(*r.DistanceFeet-50) > 0.01 {
		t.Fatalf("hole road: %+v %v", r, e)
	}
	r, e = CurrentRoads(ctx, pool, missing)
	if e != nil || r.State != "missing_parcel_geometry" || !r.Freshness.Stale {
		t.Fatalf("missing shape: %+v %v", r, e)
	}
	_, e = pool.Exec(ctx, `UPDATE qualification_heads SET generation_id=$1 WHERE source_group='roads'`, roads)
	if e != nil {
		t.Fatal(e)
	}
}
func strptr(s string) *string { return &s }
