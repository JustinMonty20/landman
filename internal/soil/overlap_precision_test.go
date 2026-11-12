//go:build postgres

package soil_test

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSoilOverlapPrecision(t *testing.T) {
	ctx := context.Background()
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL is empty")
	}
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	var polygonID, parcelID string
	if _, err := tx.Exec(ctx, `INSERT INTO soil_map_units (map_unit_key) VALUES ('precision-test')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO soil_polygons (county_id, source_name, object_id, map_unit_key, shape, fetched_at)
VALUES ('union', 'precision-test', 'sliver', 'precision-test', ST_Transform(ST_MakeEnvelope(0, 0, 1, 1, 2264), 3857), now()) RETURNING id`).Scan(&polygonID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO parcels (county_id, county_code, source_name, source_parcel_id, shape)
VALUES ('union', '179', 'precision-test', 'sliver', ST_Transform(ST_MakeEnvelope(0, 0, 0.01, 0.01, 2264), 3857)) RETURNING id`).Scan(&parcelID); err != nil {
		t.Fatal(err)
	}

	// The same EPSG:2264 area calculation used by the soil join must survive storage.
	var measured, stored float64
	err = tx.QueryRow(ctx, `WITH overlap AS (
SELECT ST_Area(ST_Transform(ST_Intersection(p.shape, s.shape), 2264)) / 43560 AS acres
FROM parcels p CROSS JOIN soil_polygons s WHERE p.id = $2 AND s.id = $1
), saved AS (
INSERT INTO soil_polygon_parcels (soil_polygon_id, parcel_id, overlap_acres)
SELECT $1, $2, acres FROM overlap WHERE acres > 0
RETURNING overlap_acres
)
SELECT (SELECT acres FROM overlap), (SELECT overlap_acres FROM saved)`, polygonID, parcelID).Scan(&measured, &stored)
	if err != nil {
		t.Fatalf("store positive sliver: %v", err)
	}
	if measured <= 0 || measured >= 0.0000005 || stored <= 0 || math.Abs(stored-measured)/measured > 1e-12 {
		t.Fatalf("EPSG:2264 sliver measured %g acres, stored %g acres", measured, stored)
	}

	for _, acres := range []string{"0.458218", "0.000000000000000000000000000001", "100000000"} {
		var got string
		err := tx.QueryRow(ctx, `UPDATE soil_polygon_parcels SET overlap_acres = $1::numeric
WHERE soil_polygon_id = $2 AND parcel_id = $3 RETURNING overlap_acres::text`, acres, polygonID, parcelID).Scan(&got)
		if err != nil || got != acres {
			t.Fatalf("store %s acres: got %q, err %v", acres, got, err)
		}
	}

	for _, acres := range []string{"0", "-0.000000000001"} {
		if _, err := tx.Exec(ctx, `SAVEPOINT invalid_overlap`); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `UPDATE soil_polygon_parcels SET overlap_acres = $1::numeric WHERE soil_polygon_id = $2 AND parcel_id = $3`, acres, polygonID, parcelID)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s acres: expected check violation (23514), got %v", acres, err)
		}
		if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_overlap`); err != nil {
			t.Fatal(err)
		}
	}
}
