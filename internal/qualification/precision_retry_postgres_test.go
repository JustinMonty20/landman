//go:build postgres

package qualification

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
)

func TestPrecisionRetryPartition(t *testing.T) {
	const x, y = -8950000., 4180000.
	rect := func(left, bottom, right, top float64) string {
		return fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g))", x+left, y+bottom, x+right, y+bottom, x+right, y+top, x+left, y+top, x+left, y+bottom)
	}
	for _, group := range []string{"zoning", "jurisdiction", "both"} {
		t.Run(group, func(t *testing.T) {
			pool := correctedPool(t)
			ctx := context.Background()
			store := Store{Pool: pool}
			var gen string
			if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('zoning',now()) RETURNING id::text`).Scan(&gen); err != nil {
				t.Fatal(err)
			}
			parcels := make([]string, 2)
			t.Cleanup(func() {
				for _, parcel := range parcels {
					pool.Exec(ctx, `DELETE FROM qualification_zone_hits WHERE parcel_id=$1`, parcel)
					pool.Exec(ctx, `DELETE FROM qualification_jurisdiction_hits WHERE parcel_id=$1`, parcel)
					pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
					pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
				}
				pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_zones WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_jurisdictions WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
			})
			if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('zoning',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, gen); err != nil {
				t.Fatal(err)
			}
			for i, wkt := range []string{rect(0, 0, 20, 20), rect(100, 0, 120, 20)} {
				if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_GeomFromText($2,3857)) RETURNING id::text`, fmt.Sprintf("%s-%d", t.Name(), i), wkt).Scan(&parcels[i]); err != nil {
					t.Fatal(err)
				}
			}
			insertZone := func(id int, wkt string) {
				t.Helper()
				if _, err := pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,jurisdiction,district_code) SELECT $1,190,$2,3857,g,g,'original',0,'Union','A' FROM (SELECT ST_Multi(ST_GeomFromText($3,3857)) g) s`, gen, id, wkt); err != nil {
					t.Fatal(err)
				}
			}
			insertJurisdiction := func(id int, wkt string) {
				t.Helper()
				if _, err := pool.Exec(ctx, `INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,name) SELECT $1,195,$2,3857,g,g,'original',0,'Town' FROM (SELECT ST_Multi(ST_GeomFromText($3,3857)) g) s`, gen, id, wkt); err != nil {
					t.Fatal(err)
				}
			}
			insertZone(1, rect(100, 0, 120, 20))
			insertJurisdiction(1, rect(100, 0, 120, 20))
			if group == "zoning" || group == "both" {
				insertZone(2, rect(10, 0, 10.0000002, 0.00003))
			}
			if group == "jurisdiction" || group == "both" {
				insertJurisdiction(2, rect(10, 0, 10.0000002, 0.00003))
			}
			insertZone(3, rect(20, 0, 30, 20))
			sourceTable := "qualification_zones"
			if group == "jurisdiction" {
				sourceTable = "qualification_jurisdictions"
			}
			var rawArea, coarseArea, fineArea float64
			query := fmt.Sprintf(`SELECT ST_Area(ST_Intersection(p.shape,z.shape)),ST_Area(ST_ReducePrecision(z.shape,0.000001)),ST_Area(ST_ReducePrecision(z.shape,0.0000001)) FROM parcels p JOIN %s z ON z.generation_id=$2 AND z.source_id=2 WHERE p.id=$1`, sourceTable)
			if err := pool.QueryRow(ctx, query, parcels[0], gen).Scan(&rawArea, &coarseArea, &fineArea); err != nil {
				t.Fatal(err)
			}
			if !(rawArea > 0 && coarseArea == 0 && fineArea > 0) {
				t.Fatalf("fixture must collapse at coarse grid only: raw=%g coarse=%g fine=%g", rawArea, coarseArea, fineArea)
			}
			if n, err := store.reconcileOne(ctx, union.QualificationZoning, ZoningMethod); err != nil || n != 2 {
				t.Fatalf("reconcile count=%d err=%v", n, err)
			}
			ordinary, err := CurrentZoning(ctx, pool, parcels[1])
			if err != nil || len(ordinary.Zones) != 1 || len(ordinary.Jurisdictions) != 1 || ordinary.Zones[0].Area.Share != 1 || ordinary.Jurisdictions[0].Area.Share != 1 {
				t.Fatalf("ordinary parcel changed: %+v err=%v", ordinary, err)
			}
			retry, err := CurrentZoning(ctx, pool, parcels[0])
			if err != nil || !retry.Available || retry.Freshness.Stale || retry.AmbiguousAcres != 0 || retry.JurisdictionAmbiguousAcres != 0 || retry.Split {
				t.Fatalf("retry assessment: %+v err=%v", retry, err)
			}
			zoneCount, jurisdictionCount := 0, 0
			if group != "jurisdiction" {
				zoneCount = 1
			}
			if group != "zoning" {
				jurisdictionCount = 1
			}
			if len(retry.Zones) != zoneCount || len(retry.Jurisdictions) != jurisdictionCount {
				t.Fatalf("positive hit or boundary contact misclassified: %+v", retry)
			}
			for _, hit := range retry.Zones {
				if hit.SourceID != 2 || hit.Area.Acres <= 0 || hit.Area.Share <= 0 || hit.Area.Share >= 1 {
					t.Fatalf("zoning positive area lost: %+v", hit)
				}
			}
			for _, hit := range retry.Jurisdictions {
				if hit.SourceID != 2 || hit.Area.Acres <= 0 || hit.Area.Share <= 0 || hit.Area.Share >= 1 {
					t.Fatalf("jurisdiction positive area lost: %+v", hit)
				}
			}
			if n, err := store.reconcileOne(ctx, union.QualificationZoning, ZoningMethod); err != nil || n != 0 {
				t.Fatalf("clean retry replay count=%d err=%v", n, err)
			}
		})
	}
}

func TestNoGridRetryPreservesSliverAndRollsBackOnFailure(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	const x, y = -8950000., 4180000.
	rect := func(left, right float64) string {
		return fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g))", x+left, y, x+right, y, x+right, y+20, x+left, y+20, x+left, y)
	}
	var gen string
	if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('zoning',now()) RETURNING id::text`).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	parcels := make([]string, 2)
	t.Cleanup(func() {
		for _, parcel := range parcels {
			pool.Exec(ctx, `DELETE FROM qualification_zone_hits WHERE parcel_id=$1`, parcel)
			pool.Exec(ctx, `DELETE FROM qualification_jurisdiction_hits WHERE parcel_id=$1`, parcel)
			pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
			pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
		}
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
		pool.Exec(ctx, `DELETE FROM qualification_zones WHERE generation_id=$1`, gen)
		pool.Exec(ctx, `DELETE FROM qualification_jurisdictions WHERE generation_id=$1`, gen)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('zoning',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, gen); err != nil {
		t.Fatal(err)
	}
	for i, wkt := range []string{rect(0, 20), rect(100, 120)} {
		if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_GeomFromText($2,3857)) RETURNING id::text`, fmt.Sprintf("%s-%d", t.Name(), i), wkt).Scan(&parcels[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.reconcileOne(ctx, union.QualificationZoning, ZoningMethod); err != nil {
		t.Fatal(err)
	}
	const stale = "stored-overlay-measured/old"
	if _, err := pool.Exec(ctx, `UPDATE qualification_assessments SET method_version=$1 WHERE parcel_id=ANY($2::uuid[]) AND source_group='zoning'`, stale, parcels); err != nil {
		t.Fatal(err)
	}
	// The full source polygon survives fine precision, but its clip to the
	// first parcel collapses to a shared edge. The second parcel is unaffected.
	thin := rect(20-0.00000002, 30)
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,jurisdiction,district_code) SELECT $1,190,1,3857,g,g,'original',0,'Union','A' FROM (SELECT ST_Multi(ST_GeomFromText($2,3857)) g) s`, gen, thin); err != nil {
		t.Fatal(err)
	}
	var rawArea, fineArea float64
	if err := pool.QueryRow(ctx, `SELECT ST_Area(ST_Intersection(p.shape,z.shape)),ST_Area(ST_Intersection(ST_ReducePrecision(p.shape,0.0000001),ST_ReducePrecision(z.shape,0.0000001),0.0000001)) FROM parcels p JOIN qualification_zones z ON z.generation_id=$2 WHERE p.id=$1`, parcels[0], gen).Scan(&rawArea, &fineArea); err != nil || !(rawArea > 0 && fineArea == 0) {
		t.Fatalf("exhaustion fixture raw=%g fine=%g err=%v", rawArea, fineArea, err)
	}
	// Simulate the loss of a hit at the last write. The positive-overlap
	// validation must fail closed and roll back both parcels' updates.
	if _, err := pool.Exec(ctx, `CREATE FUNCTION test_drop_precision_hit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$;
CREATE TRIGGER test_drop_precision_hit BEFORE INSERT ON qualification_zone_hits FOR EACH ROW EXECUTE FUNCTION test_drop_precision_hit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_drop_precision_hit ON qualification_zone_hits; DROP FUNCTION IF EXISTS test_drop_precision_hit()`)
	})
	if n, err := store.reconcileOne(ctx, union.QualificationZoning, ZoningMethod); n != 0 || err == nil ||
		!strings.Contains(err.Error(), "assessment pending precision repair: projected zone hit") ||
		!strings.Contains(err.Error(), parcels[0]+" layer_id=190 source_id=1 original_overlap_m2=") {
		t.Fatalf("retry must roll back with hit evidence: count=%d err=%v", n, err)
	}
	for _, parcel := range parcels {
		var method string
		var zoneHits, jurisdictionHits int
		if err := pool.QueryRow(ctx, `SELECT method_version,(SELECT count(*) FROM qualification_zone_hits WHERE parcel_id=$1),(SELECT count(*) FROM qualification_jurisdiction_hits WHERE parcel_id=$1) FROM qualification_assessments WHERE parcel_id=$1 AND source_group='zoning'`, parcel).Scan(&method, &zoneHits, &jurisdictionHits); err != nil || method != stale || zoneHits != 0 || jurisdictionHits != 0 {
			t.Fatalf("group was partially written: parcel=%s method=%s hits=%d/%d err=%v", parcel, method, zoneHits, jurisdictionHits, err)
		}
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER test_drop_precision_hit ON qualification_zone_hits`); err != nil {
		t.Fatal(err)
	}
	if n, err := store.reconcileOne(ctx, union.QualificationZoning, ZoningMethod); n != 2 || err != nil {
		t.Fatalf("recovery after rollback count=%d err=%v", n, err)
	}
	var share float64
	if err := pool.QueryRow(ctx, `SELECT share FROM qualification_zone_hits WHERE parcel_id=$1`, parcels[0]).Scan(&share); err != nil || math.IsNaN(share) || share <= 0 {
		t.Fatalf("recovered positive hit share=%g err=%v", share, err)
	}
}
