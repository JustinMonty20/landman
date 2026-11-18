//go:build postgres

package qualification

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
)

func TestLongEdgeNearestInMeasurementCRS(t *testing.T) {
	for _, tc := range []struct {
		name   string
		road2  string
		radius bool
		want   int
	}{
		{"winner", "LINESTRING(999.8 0,999.8 1)", false, 1},
		{"tie", "LINESTRING(1000.001 4990,1000.001 5000)", false, 2},
		{"radius-and-county-edge", "", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			pool := correctedPool(t)
			store := Store{Pool: pool}
			var gen, parcel, boundaryGen string
			if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('roads',now()) RETURNING id::text`).Scan(&gen); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('roads',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, gen); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_MakeEnvelope(-8950100,4160000,-8950000,4165000,3857)) RETURNING id::text`, t.Name()).Scan(&parcel); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				pool.Exec(ctx, `DELETE FROM qualification_road_ties WHERE parcel_id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_roads WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
				if boundaryGen != "" {
					pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, boundaryGen)
					pool.Exec(ctx, `DELETE FROM qualification_boundaries WHERE generation_id=$1`, boundaryGen)
					pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, boundaryGen)
				}
			})
			addRoad := func(id int, wkt string) {
				t.Helper()
				if _, err := pool.Exec(ctx, `INSERT INTO qualification_roads(generation_id,source_id,source_srid,shape) VALUES($1,$2,3857,ST_Multi(ST_Translate(ST_GeomFromText($3,3857),-8950000,4160000)))`, gen, id, wkt); err != nil {
					t.Fatal(err)
				}
			}
			offset := 1000.0
			if tc.radius {
				offset = 1962.5
			}
			addRoad(1, fmt.Sprintf("LINESTRING(%g 0,%g 5000)", offset, offset+0.001))
			if tc.road2 != "" {
				addRoad(2, tc.road2)
			}
			if tc.radius {
				if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('zoning',now()) RETURNING id::text`).Scan(&boundaryGen); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('zoning',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, boundaryGen); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, `INSERT INTO qualification_boundaries(generation_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2) SELECT $1,1,3857,b,b,'original',0 FROM (SELECT ST_Multi(ST_Translate(ST_GeomFromText('POLYGON((-5000 -3000,1962.4994 -3000,1962.5016 8000,-5000 8000,-5000 -3000))',3857),-8950000,4160000)) b) s`, boundaryGen); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.reconcileOne(ctx, union.QualificationRoads, RoadsMethod); err != nil {
				t.Fatal(err)
			}
			r, err := CurrentRoads(ctx, pool, parcel)
			if err != nil {
				t.Fatal(err)
			}
			var coarse, fine float64
			if err := pool.QueryRow(ctx, `SELECT ST_Distance(ST_Transform(ST_Segmentize(p.shape,10),2264),ST_Transform(ST_Segmentize(r.shape,10),2264)),ST_Distance(ST_Transform(ST_Segmentize(p.shape,1),2264),ST_Transform(ST_Segmentize(r.shape,1),2264)) FROM parcels p, qualification_roads r WHERE p.id=$1 AND r.generation_id=$2 AND r.source_id=1`, parcel, gen).Scan(&coarse, &fine); err != nil {
				t.Fatal(err)
			}
			if math.Abs(coarse-fine) > 0.001 {
				t.Fatalf("10m vs 1m reference %.9f %.9f", coarse, fine)
			}
			if tc.radius {
				if math.Abs(fine-5278.8874572) > 0.002 || r.State != RoadNearest || !r.CountyEdgeWarning || len(r.Ties) != 1 {
					t.Fatalf("radius %.9f vs %.9f: %+v", coarse, fine, r)
				}
			} else if math.Abs(fine-2689.88028094) > 0.002 || r.State != RoadNearest || len(r.Ties) != tc.want || r.Ties[0].SourceID != 1 || r.DistanceFeet == nil || math.Abs(*r.DistanceFeet-fine) > 0.002 {
				t.Fatalf("nearest %.9f vs %.9f: %+v", coarse, fine, r)
			}
			t.Logf("%s 10m=%.9f ft 1m=%.9f ft difference=%.9f ft", tc.name, coarse, fine, math.Abs(coarse-fine))
		})
	}
}
