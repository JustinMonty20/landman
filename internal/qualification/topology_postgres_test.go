//go:build postgres

package qualification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
)

func TestStoredTopologyMeasuredEvidence(t *testing.T) {
	const x, y = -8950000., 4180000.
	rect := func(a, b, c, d float64) string {
		return fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g))", x+a, y+b, x+c, y+b, x+c, y+d, x+a, y+d, x+a, y+b)
	}
	outer := rect(0, 0, 1000, 100)
	withHole := outer[:len(outer)-1] + fmt.Sprintf(",(%g %g,%g %g,%g %g,%g %g,%g %g))", x+400, y+40, x+400, y+60, x+600, y+60, x+600, y+40, x+400, y+40)
	cases := []struct {
		name, parcel                string
		zones                       []string
		codes                       []string
		wantGap, wantAmb, wantSplit bool
		wantHits                    int
	}{
		{"midpoint-lower", rect(0, 0, 1000, 100), []string{rect(0, 0, 500, 100), fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g,%g %g))", x+500, y, x+1000, y, x+1000, y+100, x+500, y+100, x+500, y+50, x+500, y)}, []string{"A", "B"}, false, false, true, 2},
		{"midpoint-upper", rect(0, 0, 1000, 100), []string{fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g,%g %g))", x, y, x+500, y, x+500, y+50, x+500, y+100, x, y+100, x, y), rect(500, 0, 1000, 100)}, []string{"A", "B"}, false, false, true, 2},
		{"exact-cover-extra-bottom", rect(0, 0, 1000, 100), []string{fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g,%g %g))", x, y, x+500, y, x+1000, y, x+1000, y+100, x, y+100, x, y)}, []string{"A"}, false, false, false, 1},
		{"exact-cover-extra-top", rect(0, 0, 1000, 100), []string{fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g,%g %g))", x, y, x+1000, y, x+1000, y+100, x+500, y+100, x, y+100, x, y)}, []string{"A"}, false, false, false, 1},
		{"thin-valid-strip", rect(0, 0, 1000, 100), []string{rect(500, 0, 500.001, 100)}, []string{"A"}, true, false, false, 1},
		{"positive-tiny-conflict", rect(0, 0, 1000, 100), []string{rect(0, 0, 1000, 100), rect(500, 50, 500.01, 50.01)}, []string{"A", "B"}, false, true, false, 2},
		{"duplicate-district", rect(0, 0, 1000, 100), []string{rect(0, 0, 1000, 100), rect(0, 0, 1000, 100)}, []string{"A", "A"}, false, false, false, 2},
		{"exact-cover-hole", withHole, []string{withHole}, []string{"A"}, false, false, false, 1},
		{"near-full-notch", outer, []string{fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g,%g %g,%g %g,%g %g))", x, y, x+1000, y, x+1000, y+100, x+500.5, y+100, x+500, y+99, x+499.5, y+100, x, y+100, x, y)}, []string{"A"}, true, false, false, 1},
		{"filled-notch", outer, []string{fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g,%g %g,%g %g,%g %g))", x, y, x+1000, y, x+1000, y+100, x+500.5, y+100, x+500, y+99, x+499.5, y+100, x, y+100, x, y), fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g))", x+499.5, y+100, x+500, y+99, x+500.5, y+100, x+499.5, y+100)}, []string{"A", "B"}, false, false, true, 2},
		{"unnamed-jurisdiction", rect(0, 0, 1000, 100), []string{rect(0, 0, 1000, 100)}, []string{"A"}, false, false, false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := correctedPool(t)
			ctx := context.Background()
			store := Store{Pool: pool}
			var gen, parcel string
			if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('zoning',now()) RETURNING id::text`).Scan(&gen); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('zoning',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, gen); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_GeomFromText($2,3857)) RETURNING id::text`, t.Name(), tc.parcel).Scan(&parcel); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				pool.Exec(ctx, `DELETE FROM qualification_zone_hits WHERE parcel_id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM qualification_jurisdiction_hits WHERE parcel_id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_zones WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_jurisdictions WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
			})
			for i, wkt := range tc.zones {
				_, err := pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,jurisdiction,district_code) SELECT $1,190,$2,3857,g,g,'original',0,'Union',$3 FROM (SELECT ST_Multi(ST_GeomFromText($4,3857)) g) s`, gen, i+1, tc.codes[i], wkt)
				if err != nil {
					t.Fatal(err)
				}
			}
			jurisdictionName := "Town"
			if tc.name == "unnamed-jurisdiction" {
				jurisdictionName = ""
			}
			for i, wkt := range tc.zones {
				name := jurisdictionName
				if tc.name == "positive-tiny-conflict" && i == 1 {
					name = "Other town"
				}
				_, err := pool.Exec(ctx, `INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,name) SELECT $1,195,$2,3857,g,g,'original',0,$3 FROM (SELECT ST_Multi(ST_GeomFromText($4,3857)) g) s`, gen, i+1, name, wkt)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.reconcileOne(ctx, union.QualificationZoning, ZoningMethod); err != nil {
				t.Fatal(err)
			}
			z, err := CurrentZoning(ctx, pool, parcel)
			if err != nil {
				t.Fatal(err)
			}
			if len(z.Zones) != tc.wantHits || z.Split != tc.wantSplit || (z.UnmappedAcres > 0) != tc.wantGap || (z.AmbiguousAcres > 0) != tc.wantAmb {
				t.Fatalf("unexpected evidence %+v", z)
			}
			if tc.name == "unnamed-jurisdiction" {
				if len(z.Jurisdictions) != 1 || z.Jurisdictions[0].Name != nil || z.JurisdictionAmbiguousAcres <= 0 || z.JurisdictionUnmappedAcres != 0 {
					t.Fatalf("unnamed coverage %+v", z)
				}
			} else if (z.JurisdictionUnmappedAcres > 0) != tc.wantGap || (z.JurisdictionAmbiguousAcres > 0) != tc.wantAmb {
				t.Fatalf("jurisdiction evidence %+v", z)
			}
			if tc.name == "near-full-notch" || tc.name == "filled-notch" {
				var fine float64
				if err := pool.QueryRow(ctx, `SELECT ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(ST_Intersection(p.shape,h.shape),0),1),2264)) / ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(p.shape,0),1),2264)) FROM parcels p, qualification_zones h WHERE p.id=$1 AND h.generation_id=$2 AND h.source_id=1`, parcel, gen).Scan(&fine); err != nil {
					t.Fatal(err)
				}
				for _, hit := range []Area{z.Zones[0].Area, z.Jurisdictions[0].Area} {
					if math.Abs(hit.Share-0.999995) > 1e-7 || math.Abs(hit.Share-fine) > 1e-7 {
						t.Fatalf("notched district share %+v; 1m reference %.11f", z, fine)
					}
				}
				t.Logf("%s 10m share=%.11f 1m share=%.11f delta=%.11f", tc.name, z.Zones[0].Area.Share, fine, math.Abs(z.Zones[0].Area.Share-fine))
			}
			for _, h := range z.Zones {
				if h.Area.Share > 1 || h.Area.Share <= 0 || ((tc.name == "exact-cover-extra-bottom" || tc.name == "exact-cover-extra-top") && math.Abs(h.Area.Share-1) > 1e-12) {
					t.Fatalf("invalid share %+v", h)
				}
			}
		})
	}
}

func TestStoredDisjointRoadClearance(t *testing.T) {
	for _, tc := range []struct {
		name              string
		length, clearance float64
	}{{"midpoint-1mm", 100, 0.001}, {"long-edge-20cm", 5000, 0.2}} {
		t.Run(tc.name, func(t *testing.T) {
			pool := correctedPool(t)
			ctx := context.Background()
			store := Store{Pool: pool}
			var gen, parcel string
			if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('roads',now()) RETURNING id::text`).Scan(&gen); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('roads',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, gen); err != nil {
				t.Fatal(err)
			}
			x, y := -8950000., 4180000.
			wkt := fmt.Sprintf("POLYGON((%g %g,%g %g,%g %g,%g %g,%g %g))", x, y, x+tc.length, y, x+tc.length, y+100, x, y+100, x, y)
			if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_GeomFromText($2,3857)) RETURNING id::text`, t.Name(), wkt).Scan(&parcel); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				pool.Exec(ctx, `DELETE FROM qualification_road_ties WHERE parcel_id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
				pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_roads WHERE generation_id=$1`, gen)
				pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
			})
			road := fmt.Sprintf("LINESTRING(%g %g,%g %g,%g %g)", x, y+100+tc.clearance, x+tc.length/2, y+100+tc.clearance, x+tc.length, y+100+tc.clearance)
			if _, err := pool.Exec(ctx, `INSERT INTO qualification_roads(generation_id,source_id,source_srid,shape) VALUES($1,1,3857,ST_Multi(ST_GeomFromText($2,3857)))`, gen, road); err != nil {
				t.Fatal(err)
			}
			if _, err := store.reconcileOne(ctx, union.QualificationRoads, RoadsMethod); err != nil {
				t.Fatal(err)
			}
			r, err := CurrentRoads(ctx, pool, parcel)
			if err != nil {
				t.Fatal(err)
			}
			if r.DistanceFeet == nil || *r.DistanceFeet <= 0 || len(r.Ties) != 1 || math.Abs(*r.DistanceFeet-r.Ties[0].DistanceFeet) > 1e-12 {
				t.Fatalf("disjoint road %+v", r)
			}
		})
	}
}

func TestFailedHostWaitMarksOldEvidenceStale(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	var gen, parcel string
	if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('roads',now()) RETURNING id::text`).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('roads',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, gen); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES('union','union','union_county_gis',$1) RETURNING id::text`, t.Name()).Scan(&parcel); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
		pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='roads'`)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_assessments(parcel_id,source_group,generation_id,geometry_fingerprint,method_version,road_state,road_search_radius_feet) VALUES($1,'roads',$2,'missing',$3,'missing_parcel_geometry',5280)`, parcel, gen, RoadsMethod); err != nil {
		t.Fatal(err)
	}
	before, err := CurrentRoads(ctx, pool, parcel)
	if err != nil || before.Freshness.Stale && before.Freshness.RefreshFailedAt != nil {
		t.Fatalf("initial %+v %v", before, err)
	}
	for _, fail := range []string{"timeout", "cancel", "connection"} {
		t.Run(fail, func(t *testing.T) {
			waitCtx, cancel := context.WithCancel(ctx)
			if fail == "cancel" {
				cancel()
			} else {
				defer cancel()
			}
			_, err := store.RefreshWithGate(waitCtx, nil, union.QualificationRoads, func(ctx context.Context, work func(context.Context) error) error {
				if fail == "cancel" {
					return ctx.Err()
				}
				return fmt.Errorf("host lock %s", fail)
			})
			if err == nil {
				t.Fatal("lock failure missing")
			}
			after, e := CurrentRoads(ctx, pool, parcel)
			if e != nil || after.GenerationID != gen || after.Freshness.RefreshFailedAt == nil || !after.Freshness.Stale {
				t.Fatalf("old head should be stale %+v %v", after, e)
			}
		})
	}
}

func TestOlderSuccessfulPublicationKeepsNewerFailure(t *testing.T) {
	pool := correctedPool(t)
	ctx := context.Background()
	store := Store{Pool: pool}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := "metadata"
		if strings.HasSuffix(r.URL.Path, "/query") {
			kind = "sample"
		}
		b, err := os.ReadFile(filepath.Join("..", "connector", "union", "testdata", "qualification", "170-"+kind+".json"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if r.URL.Query().Get("returnCountOnly") == "true" {
			var rows struct {
				Features []json.RawMessage `json:"features"`
			}
			if err := json.Unmarshal(b, &rows); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			fmt.Fprintf(w, `{"count":%d}`, len(rows.Features))
			return
		}
		w.Write(b)
	}))
	defer server.Close()
	cfg := union.DefaultGISSourceConfig()
	cfg.BaseURL = server.URL + "/215/query"
	cfg.RateLimit = 1000
	source, err := union.NewGISSource(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var generations []string
	t.Cleanup(func() {
		for _, gen := range generations {
			pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
			pool.Exec(ctx, `DELETE FROM qualification_layer_receipts WHERE generation_id=$1`, gen)
			pool.Exec(ctx, `DELETE FROM qualification_roads WHERE generation_id=$1`, gen)
			pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
		}
		pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='roads'`)
	})
	for i := 0; i < 2; i++ {
		n, e := store.Refresh(ctx, source, union.QualificationRoads)
		if e != nil || n != 2 {
			t.Fatalf("source refresh %d count=%d err=%v", i, n, e)
		}
		var gen string
		if e = pool.QueryRow(ctx, `SELECT generation_id::text FROM qualification_heads WHERE source_group='roads'`).Scan(&gen); e != nil {
			t.Fatal(e)
		}
		generations = append(generations, gen)
		if i == 0 {
			store.recordRefreshFailure(union.QualificationRoads, time.Now().Add(time.Hour), errors.New("newer failure"))
		}
	}
	var failure string
	if err := pool.QueryRow(ctx, `SELECT error FROM qualification_refresh_failures WHERE source_group='roads'`).Scan(&failure); err != nil || failure != "newer failure" {
		t.Fatalf("older success erased newer failure: %q %v", failure, err)
	}
	store.recordRefreshFailure(union.QualificationRoads, time.Now().Add(-time.Hour), errors.New("older failure"))
	if err := pool.QueryRow(ctx, `SELECT error FROM qualification_refresh_failures WHERE source_group='roads'`).Scan(&failure); err != nil || failure != "newer failure" {
		t.Fatalf("older failure replaced newer status: %q %v", failure, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='roads'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE qualification_generations SET started_at=now()-interval '1 minute' WHERE id=$1`, generations[1]); err != nil {
		t.Fatal(err)
	}
	store.recordRefreshFailure(union.QualificationRoads, time.Now().Add(-30*time.Second), errors.New("failed waiter after older holder"))
	if err := pool.QueryRow(ctx, `SELECT error FROM qualification_refresh_failures WHERE source_group='roads'`).Scan(&failure); err != nil || failure != "failed waiter after older holder" {
		t.Fatalf("older holder's later generation suppressed failed waiter: %q %v", failure, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='roads'`); err != nil {
		t.Fatal(err)
	}
	barrier, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Release()
	const key = 753982511
	if _, err := barrier.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	defer barrier.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, key)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION qualification_failure_barrier() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(753982511); RETURN NEW; END $$; CREATE TRIGGER qualification_failure_barrier BEFORE INSERT ON qualification_refresh_failures FOR EACH ROW EXECUTE FUNCTION qualification_failure_barrier()`); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS qualification_failure_barrier ON qualification_refresh_failures; DROP FUNCTION IF EXISTS qualification_failure_barrier()`)
	oldDone := make(chan struct{})
	go func() {
		store.recordRefreshFailure(union.QualificationRoads, time.Now(), errors.New("older paused failure"))
		close(oldDone)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND objid=753982511)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("older failure never reached insert barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	published := make(chan error, 1)
	go func() {
		_, err := store.Refresh(ctx, source, union.QualificationRoads)
		published <- err
	}()
	for {
		var waiting int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= 2 {
			break
		}
		if time.Now().After(deadline.Add(3 * time.Second)) {
			t.Fatal("publication did not wait behind paused failure")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-published:
		t.Fatalf("publication overtook paused older failure: %v", err)
	default:
	}
	if _, err := barrier.Exec(ctx, `SELECT pg_advisory_unlock($1)`, key); err != nil {
		t.Fatal(err)
	}
	<-oldDone
	if err := <-published; err != nil {
		t.Fatal(err)
	}
	var failures int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM qualification_refresh_failures WHERE source_group='roads'`).Scan(&failures); err != nil || failures != 0 {
		t.Fatalf("older failure survived newer publication: %d %v", failures, err)
	}
}
