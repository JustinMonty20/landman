//go:build postgres

package qualification

import (
	"context"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
)

func TestSharedZoningCandidatesPreserveParcelCoverage(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	var gen string
	if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at)
VALUES('zoning',now()) RETURNING id::text`).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	parcels := []struct {
		name, id string
		right    float64
		wantHits int
		wantGap  bool
	}{
		{name: "full", right: -8949980, wantHits: 2},
		{name: "partial", right: -8949970, wantHits: 2, wantGap: true},
		{name: "fine", right: -8949980, wantHits: 2, wantGap: true},
		{name: "single-cover", right: -8949980, wantHits: 1},
	}
	t.Cleanup(func() {
		for _, p := range parcels {
			id := p.id
			if id == "" {
				continue
			}
			exec(`DELETE FROM qualification_zone_hits WHERE parcel_id=$1`, id)
			exec(`DELETE FROM qualification_jurisdiction_hits WHERE parcel_id=$1`, id)
			exec(`DELETE FROM qualification_assessments WHERE parcel_id=$1`, id)
			exec(`DELETE FROM parcels WHERE id=$1`, id)
		}
		exec(`DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
		exec(`DELETE FROM qualification_zones WHERE generation_id=$1`, gen)
		exec(`DELETE FROM qualification_jurisdictions WHERE generation_id=$1`, gen)
		exec(`DELETE FROM qualification_generations WHERE id=$1`, gen)
	})
	exec(`INSERT INTO qualification_heads(source_group,generation_id) VALUES('zoning',$1)`, gen)
	for i := range parcels {
		p := &parcels[i]
		if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape)
VALUES('union','union','union_county_gis',$1,ST_MakeEnvelope(-8950000,$2::float8,$3,$2::float8+10,3857)) RETURNING id::text`,
			p.name, float64(4180000+20*i), p.right).Scan(&p.id); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,jurisdiction,district_code)
SELECT $1,layer_id,1,3857,g,g,'original',0,'Union','A' FROM (VALUES
 (190,ST_Multi(ST_MakeEnvelope(-8950000,4180000,-8949990,4180050,3857))),
 (191,ST_Multi(ST_MakeEnvelope(-8949989.9999998,4180000,-8949980,4180050,3857))),
 (192,ST_Multi(ST_MakeEnvelope(-8950010,4180055,-8949970,4180080,3857)))
) AS zones(layer_id,g)`, gen)
	// The fine parcel sees the same zones as the first two but uses finer precision.
	exec(`INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,name)
SELECT $1,195,1,3857,g,g,'original',0,'Town' FROM (SELECT
ST_Multi(ST_MakeEnvelope(-8949995,4180041,-8949994.9999998,4180042,3857)) g) s`, gen)
	store := Store{Pool: pool}
	if n, err := store.reconcileOne(ctx, union.QualificationZoning, ZoningMethod); err != nil || n != len(parcels) {
		t.Fatalf("reconcile count=%d err=%v", n, err)
	}
	for _, p := range parcels {
		z, err := CurrentZoning(ctx, pool, p.id)
		if err != nil || len(z.Zones) != p.wantHits || (z.UnmappedAcres > 0) != p.wantGap {
			t.Fatalf("parcel %s: zoning=%+v err=%v", p.name, z, err)
		}
	}
}
