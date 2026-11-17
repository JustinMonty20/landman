//go:build postgres

package qualification

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQualificationSpatialAndFreshness(t *testing.T) {
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
	store := Store{Pool: pool}
	suffix := "qual_postgres_" + strings.ReplaceAll(t.Name(), "/", "_")
	var parcel, zone, roads string
	e = pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_Transform(ST_GeomFromText('POLYGON((0 0,435.6 0,435.6 100,0 100,0 0))',2264),3857)) RETURNING id::text`, suffix).Scan(&parcel)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_zone_hits WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_jurisdiction_hits WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_road_ties WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE source_group IN ('zoning','roads')`)
		pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group IN ('zoning','roads')`)
		pool.Exec(ctx, `DELETE FROM qualification_layer_receipts`)
		pool.Exec(ctx, `DELETE FROM qualification_zones`)
		pool.Exec(ctx, `DELETE FROM qualification_jurisdictions`)
		pool.Exec(ctx, `DELETE FROM qualification_boundaries`)
		pool.Exec(ctx, `DELETE FROM qualification_roads`)
		pool.Exec(ctx, `DELETE FROM qualification_generations`)
	})
	for _, g := range []struct {
		group string
		id    *string
	}{{"zoning", &zone}, {"roads", &roads}} {
		e = pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES($1,now()) RETURNING id::text`, g.group).Scan(g.id)
		if e != nil {
			t.Fatal(e)
		}
		_, e = pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES($1,$2) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, g.group, *g.id)
		if e != nil {
			t.Fatal(e)
		}
	}
	polygon := func(id int, code, wkt string) {
		t.Helper()
		_, e := pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,jurisdiction,district_code,source_srid,shape) VALUES($1,190,$2,'Union County',$3,2264,ST_Multi(ST_Transform(ST_GeomFromText($4,2264),3857)))`, zone, id, code, wkt)
		if e != nil {
			t.Fatal(e)
		}
	}
	polygon(1, "A", "POLYGON((0 0,220 0,220 100,0 100,0 0))")
	polygon(2, "B", "POLYGON((200 0,400 0,400 100,200 100,200 0))")
	polygon(3, "B", "POLYGON((250 0,300 0,300 100,250 100,250 0))")
	polygon(4, "C", "POLYGON((435.6 0,500 0,500 100,435.6 100,435.6 0))")
	_, e = pool.Exec(ctx, `INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,name,source_srid,shape) VALUES($1,194,7,'ETJ',2264,ST_Multi(ST_Transform(ST_GeomFromText('POLYGON((0 0,400 0,400 100,0 100,0 0))',2264),3857)))`, zone)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,name,source_srid,shape) VALUES($1,195,8,'Municipality',2264,ST_Multi(ST_Transform(ST_GeomFromText('POLYGON((100 0,200 0,200 100,100 100,100 0))',2264),3857)))`, zone)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `INSERT INTO qualification_boundaries(generation_id,source_id,source_srid,shape) VALUES($1,1,2264,ST_Multi(ST_Transform(ST_GeomFromText('POLYGON((-1000 -1000,450 -1000,450 1000,-1000 1000,-1000 -1000))',2264),3857)))`, zone)
	if e != nil {
		t.Fatal(e)
	}
	for _, road := range []struct {
		id  int
		wkt string
	}{{1, "MULTILINESTRING((0 200,435.6 200))"}, {2, "MULTILINESTRING((0 -100,435.6 -100))"}, {3, "MULTILINESTRING((100 0,100 100))"}} {
		_, e = pool.Exec(ctx, `INSERT INTO qualification_roads(generation_id,source_id,source_srid,name,shape) VALUES($1,$2,2264,'Mapped line',ST_Transform(ST_GeomFromText($3,2264),3857))`, roads, road.id, road.wkt)
		if e != nil {
			t.Fatal(e)
		}
	}
	count, e := store.Reconcile(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if count != 2 {
		t.Fatalf("reconciled groups=%d", count)
	}
	zoning, e := CurrentZoning(ctx, pool, parcel)
	if e != nil {
		t.Fatal(e)
	}
	if !zoning.Available || zoning.Freshness.Stale || len(zoning.Zones) != 3 || len(zoning.Jurisdictions) != 2 || math.Abs(zoning.JurisdictionAmbiguousAcres-10000.0/43560.0) > 0.0001 || math.Abs(zoning.JurisdictionUnmappedAcres-35.6/435.6) > 0.0001 || math.Abs(zoning.TotalAcresValue()-1) > 0.0001 || math.Abs(zoning.UnmappedAcres-35.6/435.6) > 0.0001 || math.Abs(zoning.AmbiguousAcres-20/435.6) > 0.0001 {
		t.Fatalf("overlay: %+v", zoning)
	}
	roadsResult, e := CurrentRoads(ctx, pool, parcel)
	if e != nil {
		t.Fatal(e)
	}
	if roadsResult.State != "nearest" || roadsResult.DistanceFeet == nil || *roadsResult.DistanceFeet > 0.001 || len(roadsResult.Ties) != 1 || !roadsResult.CountyEdgeWarning {
		t.Fatalf("road contact and edge: %+v", roadsResult)
	}
	if n, e := store.Reconcile(ctx); e != nil || n != 0 {
		t.Fatalf("replay: %d %v", n, e)
	}
	var replacement string
	e = pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('roads',now()) RETURNING id::text`).Scan(&replacement)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `INSERT INTO qualification_roads(generation_id,source_id,source_srid,name,shape) SELECT $1,source_id,source_srid,name,shape FROM qualification_roads WHERE generation_id=$2 AND source_id IN (1,2)`, replacement, roads)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `UPDATE qualification_heads SET generation_id=$1 WHERE source_group='roads'`, replacement)
	if e != nil {
		t.Fatal(e)
	}
	roadsResult, e = CurrentRoads(ctx, pool, parcel)
	if e != nil || !roadsResult.Freshness.Stale {
		t.Fatalf("old roads marked current: %+v %v", roadsResult, e)
	}
	if n, e := store.Reconcile(ctx); e != nil || n != 1 {
		t.Fatalf("roads retirement recompute: %d %v", n, e)
	}
	roadsResult, e = CurrentRoads(ctx, pool, parcel)
	if e != nil || roadsResult.DistanceFeet == nil || math.Abs(*roadsResult.DistanceFeet-100) > 0.01 || len(roadsResult.Ties) != 2 {
		t.Fatalf("road ties: %+v %v", roadsResult, e)
	}
	var zoneReplacement string
	e = pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('zoning',now()) RETURNING id::text`).Scan(&zoneReplacement)
	if e != nil {
		t.Fatal(e)
	}
	for _, district := range []struct {
		id        int
		code, wkt string
	}{{1, "A", "POLYGON((0 0,200 0,200 100,0 100,0 0))"}, {2, "B", "POLYGON((200 0,435.6 0,435.6 100,200 100,200 0))"}} {
		_, e = pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,jurisdiction,district_code,shape) VALUES($1,190,$2,2264,'Union County',$3,ST_Multi(ST_Transform(ST_GeomFromText($4,2264),3857)))`, zoneReplacement, district.id, district.code, district.wkt)
		if e != nil {
			t.Fatal(e)
		}
	}
	_, e = pool.Exec(ctx, `INSERT INTO qualification_jurisdictions SELECT $1,layer_id,source_id,name,source_date,source_srid,shape FROM qualification_jurisdictions WHERE generation_id=$2`, zoneReplacement, zone)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `INSERT INTO qualification_boundaries SELECT $1,layer_id,source_id,source_srid,shape FROM qualification_boundaries WHERE generation_id=$2`, zoneReplacement, zone)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `UPDATE qualification_heads SET generation_id=$1 WHERE source_group='zoning'`, zoneReplacement)
	if e != nil {
		t.Fatal(e)
	}
	roadsResult, e = CurrentRoads(ctx, pool, parcel)
	if e != nil || !roadsResult.Freshness.Stale {
		t.Fatalf("boundary source changed: %+v %v", roadsResult, e)
	}
	if n, e := store.Reconcile(ctx); e != nil || n != 2 {
		t.Fatalf("zoning source recompute: %d %v", n, e)
	}
	zoning, e = CurrentZoning(ctx, pool, parcel)
	if e != nil || !zoning.Split || zoning.AmbiguousAcres > 1e-7 || len(zoning.Zones) != 2 {
		t.Fatalf("split without conflict: %+v %v", zoning, e)
	}
	_, e = pool.Exec(ctx, `UPDATE parcels SET shape=ST_Transform(ST_GeomFromText('POLYGON((0 0,100 0,100 100,0 100,0 0))',2264),3857) WHERE id=$1`, parcel)
	if e != nil {
		t.Fatal(e)
	}
	zoning, e = CurrentZoning(ctx, pool, parcel)
	if e != nil || !zoning.Freshness.Stale {
		t.Fatalf("stale parcel: %+v %v", zoning, e)
	}
	if n, e := store.Reconcile(ctx); e != nil || n != 2 {
		t.Fatalf("recompute: %d %v", n, e)
	}
	zoning, e = CurrentZoning(ctx, pool, parcel)
	if e != nil || zoning.Freshness.Stale || len(zoning.Zones) != 1 {
		t.Fatalf("recomputed: %+v %v", zoning, e)
	}
	_, e = pool.Exec(ctx, `INSERT INTO qualification_refresh_failures(source_group,error) VALUES('roads','fixture failure') ON CONFLICT(source_group) DO UPDATE SET failed_at=now(),error=excluded.error`)
	if e != nil {
		t.Fatal(e)
	}
	roadsResult, e = CurrentRoads(ctx, pool, parcel)
	if e != nil || !roadsResult.Freshness.Stale || roadsResult.Freshness.RefreshError == nil {
		t.Fatalf("independent source failure: %+v %v", roadsResult, e)
	}
	zoning, e = CurrentZoning(ctx, pool, parcel)
	if e != nil || zoning.Freshness.Stale {
		t.Fatalf("zoning affected by roads: %+v %v", zoning, e)
	}
	_, e = pool.Exec(ctx, `INSERT INTO qualification_refresh_failures(source_group,error) VALUES('zoning','boundary refresh failed')`)
	if e != nil {
		t.Fatal(e)
	}
	roadsResult, e = CurrentRoads(ctx, pool, parcel)
	if e != nil || !roadsResult.CountyEdgeWarning || !hasReason(roadsResult.Freshness.Reasons, "boundary_source_refresh_failed") {
		t.Fatalf("failed boundary source hid incomplete road coverage: %+v %v", roadsResult, e)
	}
	_, e = pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='zoning'`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `UPDATE qualification_generations SET fetched_at=now()-interval '91 days' WHERE id=$1`, zoneReplacement)
	if e != nil {
		t.Fatal(e)
	}
	roadsResult, e = CurrentRoads(ctx, pool, parcel)
	if e != nil || !roadsResult.CountyEdgeWarning || !hasReason(roadsResult.Freshness.Reasons, "boundary_source_age_or_unavailable") {
		t.Fatalf("aged boundary source hid incomplete road coverage: %+v %v", roadsResult, e)
	}
	_, e = pool.Exec(ctx, `CREATE FUNCTION qualification_fixture_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture zone write failed'; END $$`)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DROP FUNCTION IF EXISTS qualification_fixture_fail() CASCADE`) })
	_, e = pool.Exec(ctx, `CREATE TRIGGER qualification_fixture_fail BEFORE INSERT ON qualification_zone_hits FOR EACH ROW EXECUTE FUNCTION qualification_fixture_fail()`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `UPDATE parcels SET shape=ST_Transform(ST_GeomFromText('POLYGON((0 0,120 0,120 100,0 100,0 0))',2264),3857) WHERE id=$1`, parcel)
	if e != nil {
		t.Fatal(e)
	}
	if n, err := store.Reconcile(ctx); n != 1 || err == nil || !strings.Contains(err.Error(), "fixture zone write failed") {
		t.Fatalf("independent road refresh after zoning failure: %d %v", n, err)
	}
	roadsResult, e = CurrentRoads(ctx, pool, parcel)
	if e != nil || hasReason(roadsResult.Freshness.Reasons, "parcel_geometry_changed") {
		t.Fatalf("road evidence stayed stale after zoning failure: %+v %v", roadsResult, e)
	}
	zoning, e = CurrentZoning(ctx, pool, parcel)
	if e != nil || !hasReason(zoning.Freshness.Reasons, "parcel_geometry_changed") {
		t.Fatalf("failed zoning evidence did not remain stale: %+v %v", zoning, e)
	}
}

func hasReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
func (a ZoningAssessment) TotalAcresValue() float64 {
	if a.TotalAcres == nil {
		return 0
	}
	return *a.TotalAcres
}
