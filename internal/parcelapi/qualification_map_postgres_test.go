//go:build postgres

package parcelapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQualificationMapActiveGenerations(t *testing.T) {
	dsn := os.Getenv("FEMA_API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("FEMA_API_TEST_DATABASE_URL not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "landman_api_test_fema" || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port == 5432 || len(cfg.ConnConfig.Fallbacks) != 0 {
		t.Fatal("use the disposable FEMA API database")
	}
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const zoning = "b0000000-0000-4000-8000-000000000001"
	const old = "b0000000-0000-4000-8000-000000000002"
	const roads = "b0000000-0000-4000-8000-000000000003"
	const polygon = `ST_Multi(ST_Transform(ST_MakeEnvelope(-80.6,34.99,-80.59,35,4326),3857))`
	for _, q := range []string{
		`INSERT INTO qualification_generations(id,source_group,completed_at) VALUES ('` + zoning + `','zoning',now()),('` + old + `','zoning',now()),('` + roads + `','roads',now())`,
		`INSERT INTO qualification_heads VALUES ('zoning','` + zoning + `'),('roads','` + roads + `')`,
		`INSERT INTO qualification_zones(generation_id,layer_id,source_id,jurisdiction,district_code,source_srid,shape,raw_shape,repair_status,repair_delta_m2) SELECT id,1,1,'Union',code,3857,` + polygon + `,` + polygon + `,'original',0 FROM (VALUES ('` + zoning + `'::uuid,'current'), ('` + old + `'::uuid,'retired')) x(id,code)`,
		`INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,name,source_srid,shape,raw_shape,repair_status,repair_delta_m2) VALUES ('` + zoning + `',194,1,'Town',3857,` + polygon + `,` + polygon + `,'original',0)`,
		`INSERT INTO qualification_zones(generation_id,layer_id,source_id,jurisdiction,source_srid,repair_status,repair_delta_m2) VALUES ('` + zoning + `',190,231245,'Union',3857,'missing_geometry',0)`,
		`INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,name,source_srid,repair_status,repair_delta_m2) VALUES ('` + zoning + `',195,117793,'No boundary',3857,'missing_geometry',0)`,
		`INSERT INTO qualification_roads(generation_id,source_id,name,source_srid,shape) VALUES ('` + roads + `',1,'Main Road',3857,ST_Multi(ST_Transform(ST_GeomFromText('LINESTRING(-80.62 35,-80.59 35)',4326),3857)))`,
		`INSERT INTO qualification_refresh_failures VALUES ('roads',now(),'fixture failure'),('zoning',now()-interval '1 day','superseded failure')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	h := New(pool)
	request := func(layer, zoom string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/qualification/map?bbox=-80.61,34.98,-80.58,35.01&layer="+layer+"&zoom="+zoom, nil))
		return w
	}
	for _, c := range []struct {
		layer, zoom, id string
		count           int
		failed          bool
	}{{"zoning", "9", zoning, 2, false}, {"roads", "13", roads, 1, true}} {
		w := request(c.layer, c.zoom)
		var data struct {
			Features   []qualificationFeature
			Generation struct {
				ID              string
				RefreshFailedAt *string `json:"refresh_failed_at"`
			}
		}
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", c.layer, w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data.Generation.ID != c.id || len(data.Features) != c.count || (data.Generation.RefreshFailedAt != nil) != c.failed {
			t.Fatalf("unexpected %s response: %+v", c.layer, data)
		}
		for _, f := range data.Features {
			if f.Properties.Kind == "jurisdiction" && !strings.Contains(string(f.Geometry), `LineString`) {
				t.Fatalf("expected jurisdiction boundary line, got %s", f.Geometry)
			}
			if f.Properties.Name == "retired" || len(f.Geometry) == 0 {
				t.Fatalf("invalid feature: %+v", f)
			}
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/qualification/map?bbox=-80.598,34.992,-80.592,34.998&layer=zoning&zoom=14", nil))
	var inside struct{ Features []qualificationFeature }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &inside) != nil || len(inside.Features) != 1 || inside.Features[0].Properties.Kind != "zoning" {
		t.Fatalf("viewport inside jurisdiction: %d %s", w.Code, w.Body.String())
	}
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,jurisdiction,source_srid,shape,raw_shape,repair_status,repair_delta_m2)
 SELECT $1,1,seq,'Union',3857,g,g,'original',0 FROM generate_series(2,502) seq CROSS JOIN LATERAL
 (SELECT ST_Multi(ST_Transform(ST_MakeEnvelope(CASE WHEN seq=2 THEN -80.62 ELSE -80.6 END,34.99,CASE WHEN seq=2 THEN -80.61 ELSE -80.59 END,35,4326),3857)) AS g) s`, zoning); err != nil {
		t.Fatal(err)
	}
	if w := request("zoning", "14"); w.Code != 413 {
		t.Fatalf("over-limit zoning request: %d", w.Code)
	}
	if w := request("roads", "12"); w.Code != 400 {
		t.Fatalf("low zoom: %d", w.Code)
	}
	if w := request("other", "13"); w.Code != 400 {
		t.Fatalf("invalid layer: %d", w.Code)
	}
}
