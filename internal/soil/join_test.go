package soil

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelstore"
)

func TestSoilJoinStatement(t *testing.T) {
	for _, part := range []string{"ST_Transform", "2264", "soil_polygons", "soil_polygon_parcels", "ST_Intersection", "overlap_acres > 0", "p.shape IS NOT NULL", "s.source_name = p.source_name", "ON CONFLICT (soil_polygon_id, parcel_id)"} {
		if !strings.Contains(overlapInsert, part) {
			t.Errorf("missing %s", part)
		}
	}
	for _, part := range []string{"likely_vacant", "source_parcel_id", "INSERT INTO parcels", "INSERT INTO soil_polygon_parcels (id"} {
		if strings.Contains(overlapInsert, part) {
			t.Errorf("unexpected %s", part)
		}
	}
	if strings.Count(overlapInsert, "INSERT INTO") != 1 {
		t.Fatal("join must be one insert")
	}
}

func TestSoilJoinCancelledBeforeConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Store(ctx, "postgres://localhost:1/no-db", county.Union, "union_county_gis", nil, nil); err == nil {
		t.Fatal("canceled import succeeded")
	}
}

// TestSoilJoinPersistence exercises the public join boundary against a migrated PostGIS database.
func TestSoilJoinPersistence(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := fmt.Sprintf("soil-test-%d", time.Now().UnixNano())
	key := source
	defer func() {
		_, _ = db.Exec(ctx, `DELETE FROM soil_polygon_parcels WHERE soil_polygon_id IN (SELECT id FROM soil_polygons WHERE source_name=$1) OR parcel_id IN (SELECT id FROM parcels WHERE source_name=$1)`, source)
		_, _ = db.Exec(ctx, `DELETE FROM soil_polygons WHERE source_name=$1`, source)
		_, _ = db.Exec(ctx, `DELETE FROM parcels WHERE source_name=$1`, source)
		_, _ = db.Exec(ctx, `DELETE FROM soil_map_units WHERE map_unit_key=$1`, key)
	}()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 500) }))
	defer server.Close()
	shape := func(wkt string) *geom.T {
		var raw []byte
		if err := db.QueryRow(ctx, `SELECT ST_AsEWKB(ST_Transform(ST_GeomFromText($1,2264),3857))`, wkt).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		g, err := ewkb.Unmarshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		return &g
	}
	square := func(x1, y1, x2, y2 int) string {
		return fmt.Sprintf("POLYGON((%d %d,%d %d,%d %d,%d %d,%d %d))", x1, y1, x2, y1, x2, y2, x1, y2, x1, y1)
	}
	parcel := square(1533265, 462768, 1533465, 462968)
	for i, vacant := range []bool{true, false} {
		shapeValue := shape(parcel)
		record := models.GISParcel{Identity: models.ParcelIdentity{CountyID: "union", CountyCode: "179", SourceName: source, SourceParcelID: fmt.Sprint(i)}, Geometry: models.GISGeometry{Shape: shapeValue, SRID: 3857}}
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = parcelstore.SaveParcel(ctx, tx, record, vacant); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	polygons := []Polygon{
		{County: county.Union, SourceName: source, ObjectID: "half", MapUnitKey: key, Shape: shape(square(1533265, 462768, 1533365, 462968))},
		{County: county.Union, SourceName: source, ObjectID: "sliver", MapUnitKey: key, Shape: shape(square(1533464, 462768, 1533465, 462968))},
		{County: county.Union, SourceName: source, ObjectID: "edge", MapUnitKey: key, Shape: shape(square(1533465, 462768, 1533565, 462968))},
	}
	units := []MapUnit{{MapUnitKey: key, MapUnitSymbol: "Test", AreaSymbol: "NC179"}}
	run := func(items []Polygon) {
		t.Helper()
		if err := store(ctx, db, county.Union, source, items, units, server.Client(), server.URL); err != nil {
			t.Fatal(err)
		}
	}
	run(polygons)
	var halfID, joinID string
	var acres float64
	err = db.QueryRow(ctx, `SELECT s.id::text, j.id::text,j.overlap_acres FROM soil_polygons s JOIN soil_polygon_parcels j ON j.soil_polygon_id=s.id WHERE s.source_name=$1 AND s.object_id='half' LIMIT 1`, source).Scan(&halfID, &joinID, &acres)
	if err != nil || acres < 0.4545 || acres > 0.4637 {
		t.Fatalf("half: %s %s %.6f %v", halfID, joinID, acres, err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM soil_polygon_parcels j JOIN soil_polygons s ON j.soil_polygon_id=s.id WHERE s.source_name=$1`, source).Scan(&count); err != nil || count != 4 {
		t.Fatalf("overlaps: %d %v", count, err)
	}
	err = db.QueryRow(ctx, `SELECT j.overlap_acres FROM soil_polygon_parcels j JOIN soil_polygons s ON j.soil_polygon_id=s.id WHERE s.source_name=$1 AND s.object_id='sliver' LIMIT 1`, source).Scan(&acres)
	if err != nil || acres < 0.0045 || acres > 0.0047 {
		t.Fatalf("sliver: %.6f %v", acres, err)
	}
	polygons[0].Shape = shape(square(1533265, 462768, 1533315, 462968))
	run(polygons[:1])
	var newHalfID, newJoinID string
	err = db.QueryRow(ctx, `SELECT s.id::text,j.id::text,j.overlap_acres FROM soil_polygons s JOIN soil_polygon_parcels j ON j.soil_polygon_id=s.id WHERE s.source_name=$1 AND s.object_id='half' LIMIT 1`, source).Scan(&newHalfID, &newJoinID, &acres)
	if err != nil || halfID != newHalfID || joinID != newJoinID || acres < 0.22 || acres > 0.24 {
		t.Fatalf("repeat: %s %s %.6f %v", newHalfID, newJoinID, acres, err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM soil_polygon_parcels j JOIN soil_polygons s ON j.soil_polygon_id=s.id WHERE s.source_name=$1`, source).Scan(&count); err != nil || count != 4 {
		t.Fatalf("retired rows: %d %v", count, err)
	}
	// An empty source still persists geometry without inventing parcel links.
	emptySource := source + "-empty"
	emptyPolygon := Polygon{County: county.Union, SourceName: emptySource, ObjectID: "empty", MapUnitKey: key, Shape: polygons[0].Shape}
	if err := store(ctx, db, county.Union, emptySource, []Polygon{emptyPolygon}, units, server.Client(), server.URL); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(ctx, `DELETE FROM soil_polygon_parcels WHERE soil_polygon_id IN (SELECT id FROM soil_polygons WHERE source_name=$1)`, emptySource)
		_, _ = db.Exec(ctx, `DELETE FROM soil_polygons WHERE source_name=$1`, emptySource)
	}()
	if err := db.QueryRow(ctx, `SELECT count(*) FROM soil_polygon_parcels j JOIN soil_polygons s ON s.id=j.soil_polygon_id WHERE s.source_name=$1`, emptySource).Scan(&count); err != nil || count != 0 {
		t.Fatalf("empty join: %d %v", count, err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM soil_polygons WHERE source_name=$1`, emptySource).Scan(&count); err != nil || count != 1 {
		t.Fatalf("second-source polygon: %d %v", count, err)
	}
	// The second-source polygon overlaps both stored parcels, but importing the first
	// source must not attach either parcel to it.
	run(polygons[:1])
	if err := db.QueryRow(ctx, `SELECT count(*) FROM soil_polygon_parcels j JOIN soil_polygons s ON s.id=j.soil_polygon_id WHERE s.source_name=$1`, emptySource).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cross-source joins: %d %v", count, err)
	}
	// An unresponsive optional name service is bounded; the geometry and stub still commit.
	release := make(chan struct{})
	stallServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer stallServer.Close()
	defer close(release)
	stallSource := source + "-stall"
	stallKey := key + "-stall"
	stallPolygon := Polygon{County: county.Union, SourceName: stallSource, ObjectID: "stall", MapUnitKey: stallKey, Shape: polygons[0].Shape}
	defer func() {
		_, _ = db.Exec(ctx, `DELETE FROM soil_polygons WHERE source_name=$1`, stallSource)
		_, _ = db.Exec(ctx, `DELETE FROM soil_map_units WHERE map_unit_key=$1`, stallKey)
	}()
	start := time.Now()
	if err := store(ctx, db, county.Union, stallSource, []Polygon{stallPolygon}, []MapUnit{{MapUnitKey: stallKey}}, stallServer.Client(), stallServer.URL); err != nil {
		t.Fatalf("stalled names prevented geometry: %v", err)
	}
	if elapsed := time.Since(start); elapsed > nameLookupTimeout+3*time.Second {
		t.Fatalf("optional names blocked geometry for %s", elapsed)
	}
	var stubName *string
	if err := db.QueryRow(ctx, `SELECT m.map_unit_name FROM soil_polygons s JOIN soil_map_units m ON m.map_unit_key=s.map_unit_key WHERE s.source_name=$1`, stallSource).Scan(&stubName); err != nil || stubName != nil {
		t.Fatalf("stalled names stub: %v %v", stubName, err)
	}
	// A failure after the stub insert cannot publish a partial snapshot.
	badSource := source + "-bad"
	badKey := key + "-bad"
	bad := Polygon{County: county.Union, SourceName: badSource, ObjectID: "bad", MapUnitKey: badKey}
	if err := store(ctx, db, county.Union, badSource, []Polygon{bad}, []MapUnit{{MapUnitKey: badKey}}, server.Client(), server.URL); err == nil {
		t.Fatal("missing geometry accepted")
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM soil_map_units WHERE map_unit_key=$1`, badKey).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial stub: %d %v", count, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store(canceled, db, county.Union, source, polygons, units, server.Client(), server.URL); err == nil {
		t.Fatal("canceled transaction accepted")
	}
	cancelCtx, cancelRun := context.WithCancel(ctx)
	cancelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cancelRun(); w.WriteHeader(http.StatusOK) }))
	defer cancelServer.Close()
	canceledKey := key + "-canceled"
	canceledSource := source + "-canceled"
	canceledPolygon := Polygon{County: county.Union, SourceName: canceledSource, ObjectID: "cancel", MapUnitKey: canceledKey, Shape: polygons[0].Shape}
	if err := store(cancelCtx, db, county.Union, canceledSource, []Polygon{canceledPolygon}, []MapUnit{{MapUnitKey: canceledKey}}, cancelServer.Client(), cancelServer.URL); err == nil {
		t.Fatal("canceled USDA request committed")
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM soil_map_units WHERE map_unit_key=$1`, canceledKey).Scan(&count); err != nil || count != 0 {
		t.Fatalf("canceled stub: %d %v", count, err)
	}
	var name *string
	if err := db.QueryRow(ctx, `SELECT map_unit_name FROM soil_map_units WHERE map_unit_key=$1`, key).Scan(&name); err != nil || name != nil {
		t.Fatalf("stub: %v %v", name, err)
	}
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"Table":[[%q,"Test","Local name","NC179","Union County","9/3/2026 1:43:46 PM"]]}`, key)
	}))
	defer okServer.Close()
	if err := store(ctx, db, county.Union, source, polygons[:1], units, okServer.Client(), okServer.URL); err != nil {
		t.Fatal(err)
	}
	var survey time.Time
	if err := db.QueryRow(ctx, `SELECT m.map_unit_name,s.survey_saved_on FROM soil_map_units m JOIN soil_polygons s ON s.map_unit_key=m.map_unit_key WHERE s.source_name=$1 AND s.object_id='half'`, source).Scan(&name, &survey); err != nil || name == nil || *name != "Local name" || survey.Format("2006-01-02") != "2026-09-03" {
		t.Fatalf("enriched: %v %v %v", name, survey, err)
	}
	missingSource := source + "-missing-name"
	missingKey := key + "-missing-name"
	defer func() {
		_, _ = db.Exec(ctx, `DELETE FROM soil_polygons WHERE source_name=$1`, missingSource)
		_, _ = db.Exec(ctx, `DELETE FROM soil_map_units WHERE map_unit_key=$1`, missingKey)
	}()
	missingPolygon := Polygon{County: county.Union, SourceName: missingSource, ObjectID: "missing", MapUnitKey: missingKey, Shape: polygons[0].Shape}
	lookups := 0
	missingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups++
		if lookups == 1 {
			_, _ = fmt.Fprintf(w, `{"Table":[[%q,"Test",null,"NC179","Union County",null]]}`, missingKey)
			return
		}
		_, _ = fmt.Fprintf(w, `{"Table":[[%q,"Test","Available name","NC179","Union County",null]]}`, missingKey)
	}))
	defer missingServer.Close()
	missingUnits := []MapUnit{{MapUnitKey: missingKey, MapUnitSymbol: "Test", AreaSymbol: "NC179"}}
	if err := store(ctx, db, county.Union, missingSource, []Polygon{missingPolygon}, missingUnits, missingServer.Client(), missingServer.URL); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT map_unit_name FROM soil_map_units WHERE map_unit_key=$1`, missingKey).Scan(&name); err != nil || name != nil {
		t.Fatalf("missing name must remain null: %v %v", name, err)
	}
	if err := store(ctx, db, county.Union, missingSource, []Polygon{missingPolygon}, missingUnits, missingServer.Client(), missingServer.URL); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT map_unit_name FROM soil_map_units WHERE map_unit_key=$1`, missingKey).Scan(&name); err != nil || name == nil || *name != "Available name" || lookups != 2 {
		t.Fatalf("missing name retry: %v lookups=%d err=%v", name, lookups, err)
	}
}
