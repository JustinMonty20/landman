//go:build postgres

package parcelstore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestZeroMovementParcelGeometry(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostGIS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var name string
	if err := conn.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "landman_parcelstore_test_ci" && name != "landman_parcel_repair_test" {
		t.Fatal("requires a disposable parcel test database")
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE parcel_import_parts(ordinal bigint NOT NULL, source_parcel_id text NOT NULL, shape bytea NOT NULL) ON COMMIT DROP`); err != nil {
		t.Fatal(err)
	}
	const valid = `POLYGON((10 10,10 14,14 14,14 10,10 10),(11 11,13 11,13 13,11 13,11 11))`
	const shells = `POLYGON((0 0,0 2,2 2,2 0,0 0,0 -2,-2 -2,-2 0,0 0))`
	const hole = `POLYGON((0 0,0 6,6 6,6 0,0 0,2 1,1 2,0 0))`
	const crossing = `POLYGON((0 0,3 3,0 3,3 0,0 0))`
	const spike = `POLYGON((0 0,0 4,2 4,2 5,2 4,4 4,4 0,0 0))`
	for _, tc := range []struct {
		name                      string
		parts                     []string
		polygons, holes, repaired int
	}{
		{"valid", []string{valid}, 1, 1, 0},
		{"touching shells", []string{shells}, 2, 0, 1},
		{"point touch hole", []string{hole}, 1, 1, 1},
		{"complete multipart parcel", []string{valid, shells}, 3, 1, 1},
		{"crossing", []string{crossing}, 0, 0, 0},
		{"spike with valid part", []string{valid, spike}, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, shape := range tc.parts {
				if _, err := tx.Exec(ctx, `INSERT INTO parcel_import_parts VALUES ($1,$2,ST_AsEWKB(ST_GeomFromText($3,3857)))`, int64(i)+1, tc.name, shape); err != nil {
					t.Fatal(err)
				}
			}
			var shape []byte
			var repaired []int64
			if err := tx.QueryRow(ctx, repairParcelGeometrySQL, tc.name).Scan(&shape, &repaired); err != nil {
				t.Fatal(err)
			}
			if len(repaired) != tc.repaired || (len(shape) != 0) != (tc.polygons != 0) {
				t.Fatalf("repaired %d parts; result present: %t", len(repaired), len(shape) != 0)
			}
			if len(shape) == 0 {
				return
			}
			var polygons, holes int
			if err := tx.QueryRow(ctx, `SELECT ST_NumGeometries(g),ST_NRings(g)-ST_NumGeometries(g) FROM (SELECT ST_GeomFromEWKB($1) g) x`, shape).Scan(&polygons, &holes); err != nil || polygons != tc.polygons || holes != tc.holes {
				t.Fatalf("polygons %d, holes %d: %v", polygons, holes, err)
			}
		})
	}
}
