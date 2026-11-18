//go:build postgres

package qualification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

func correctedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires task-owned migrated PostGIS DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestCapturedQualificationFullStoredReplay(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		id := parts[len(parts)-1]
		kind := "metadata"
		if id == "query" {
			id = parts[len(parts)-2]
			kind = "sample"
		}
		b, err := os.ReadFile(filepath.Join("..", "connector", "union", "testdata", "qualification", id+"-"+kind+".json"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if r.URL.Query().Get("returnCountOnly") == "true" {
			var sample struct {
				Features []json.RawMessage `json:"features"`
			}
			if err := json.Unmarshal(b, &sample); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			fmt.Fprintf(w, `{"count":%d}`, len(sample.Features))
			return
		}
		_, _ = w.Write(b)
	}))
	defer server.Close()
	cfg := union.DefaultGISSourceConfig()
	cfg.BaseURL = server.URL + "/215/query"
	cfg.RateLimit = 1000
	source, err := union.NewGISSource(cfg)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Pool: pool}
	var generations []string
	t.Cleanup(func() {
		for _, group := range []string{union.QualificationZoning, union.QualificationRoads} {
			pool.Exec(ctx, `DELETE FROM qualification_heads WHERE source_group=$1 AND generation_id=ANY($2::uuid[])`, group, generations)
		}
		for _, table := range []string{"qualification_layer_receipts", "qualification_zones", "qualification_jurisdictions", "qualification_boundaries", "qualification_roads", "qualification_generations"} {
			pool.Exec(ctx, `DELETE FROM `+table+` WHERE `+map[bool]string{true: "id", false: "generation_id"}[table == "qualification_generations"]+`=ANY($1::uuid[])`, generations)
		}
	})
	for _, group := range []string{union.QualificationZoning, union.QualificationRoads, union.QualificationZoning, union.QualificationRoads} {
		expected := 0
		for _, layer := range union.QualificationLayers(group) {
			var sample struct {
				Features []json.RawMessage `json:"features"`
			}
			b, e := os.ReadFile(filepath.Join("..", "connector", "union", "testdata", "qualification", fmt.Sprintf("%d-sample.json", layer)))
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(b, &sample); e != nil {
				t.Fatal(e)
			}
			expected += len(sample.Features)
		}
		count, err := store.Refresh(ctx, source, group)
		if err != nil || count != expected {
			t.Fatalf("%s count %d: %v", group, count, err)
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT generation_id::text FROM qualification_heads WHERE source_group=$1`, group).Scan(&id); err != nil {
			t.Fatal(err)
		}
		generations = append(generations, id)
	}
	for _, tc := range []struct {
		table     string
		layer, id int
	}{{"qualification_zones", 190, 229169}, {"qualification_jurisdictions", 195, 116697}} {
		var status string
		var delta float64
		var rawValid, valid bool
		query := fmt.Sprintf(`SELECT repair_status,repair_delta_m2,ST_IsValid(raw_shape),ST_IsValid(shape) FROM %s WHERE layer_id=$1 AND source_id=$2 ORDER BY generation_id DESC LIMIT 1`, tc.table)
		if err := pool.QueryRow(ctx, query, tc.layer, tc.id).Scan(&status, &delta, &rawValid, &valid); err != nil {
			t.Fatal(err)
		}
		if status != "repaired" || rawValid || !valid || delta > 0.01 {
			t.Fatalf("%d:%d status %s delta %g raw %v fixed %v", tc.layer, tc.id, status, delta, rawValid, valid)
		}
	}
}

func TestRepairOutsideToleranceFailsWithoutPublishing(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	var gen string
	if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group) VALUES('zoning') RETURNING id::text`).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen) })
	poly := geom.NewPolygon(geom.XY).SetSRID(3857)
	// The first ring is a bow-tie whose interpreted area is zero; repairing it creates positive area.
	if _, err := poly.SetCoords([][]geom.Coord{{{0, 0}, {10, 10}, {0, 10}, {10, 0}, {0, 0}}}); err != nil {
		t.Fatal(err)
	}
	shape, err := ewkb.Marshal(poly, ewkb.NDR)
	if err != nil {
		t.Fatal(err)
	}
	err = store.insertFeature(ctx, gen, union.ZoneFeature{SourceIdentity: union.SourceIdentity{LayerID: 190, SourceID: 999999, SourceSRID: 3857, Shape: shape}})
	if err == nil || !strings.Contains(err.Error(), "layer 190 OID 999999") {
		t.Fatalf("repair beyond tolerance must fail with source identity: %v", err)
	}
	var count int
	if e := pool.QueryRow(ctx, `SELECT count(*) FROM qualification_zones WHERE generation_id=$1`, gen).Scan(&count); e != nil || count != 0 {
		t.Fatalf("invalid row %d %v", count, e)
	}
}

func TestFailedFirstManifestInsertAndOldEvidenceAge(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	var parcel, old, newGen string
	if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES('union','union','union_county_gis',$1) RETURNING id::text`, t.Name()).Scan(&parcel); err != nil {
		t.Fatal(err)
	}

	if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,fetched_at,completed_at) VALUES('roads',now()-interval '99 days',now()-interval '99 days') RETURNING id::text`).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_assessments(parcel_id,source_group,generation_id,geometry_fingerprint,method_version,road_state,road_search_radius_feet) VALUES($1,'roads',$2,'missing',$3,'none_within_radius',5280)`, parcel, old, RoadsMethod); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('roads',now()) RETURNING id::text`).Scan(&newGen); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('roads',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, newGen); err != nil {
		t.Fatal(err)
	}
	result, err := CurrentRoads(ctx, pool, parcel)
	if err != nil {
		t.Fatal(err)
	}
	if result.Freshness.SourceFetchedAt == nil || time.Since(*result.Freshness.SourceFetchedAt) < 98*24*time.Hour || !containsReason(result.Freshness.Reasons, "source_age") {
		t.Fatalf("old evidence age %+v", result.Freshness)
	}
	// A writable database trigger rejects the first manifest operation, not the failure recorder.
	if _, err := pool.Exec(ctx, `CREATE OR REPLACE FUNCTION qualification_reject_test_manifest() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'rejected test manifest'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER qualification_reject_test_manifest BEFORE INSERT ON qualification_generations FOR EACH ROW EXECUTE FUNCTION qualification_reject_test_manifest()`); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DROP TRIGGER IF EXISTS qualification_reject_test_manifest ON qualification_generations; DROP FUNCTION IF EXISTS qualification_reject_test_manifest()`)
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='roads'`)
		pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, newGen)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id IN ($1::uuid,$2::uuid)`, old, newGen)
	})
	if _, err := store.Refresh(ctx, nil, union.QualificationRoads); err == nil {
		t.Fatal("manifest insert succeeded")
	}
	result, err = CurrentRoads(ctx, pool, parcel)
	if err != nil {
		t.Fatal(err)
	}
	if !containsReason(result.Freshness.Reasons, "source_refresh_failed") {
		t.Fatalf("first operation failure not visible: %+v", result.Freshness)
	}
}

func containsReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}
