//go:build postgres

package soil_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSoilPolygonSRIDConstraint(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is empty")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	key := fmt.Sprintf("test-srid-%d", time.Now().UnixNano())
	if _, err := tx.Exec(ctx, `INSERT INTO soil_map_units (map_unit_key) VALUES ($1)`, key); err != nil {
		t.Fatalf("insert map unit: %v", err)
	}

	for _, srid := range []int{0, 4326} {
		t.Run(fmt.Sprint(srid), func(t *testing.T) {
			if _, err := tx.Exec(ctx, `SAVEPOINT invalid_srid`); err != nil {
				t.Fatal(err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO soil_polygons
				(county_id, source_name, object_id, map_unit_key, shape, fetched_at)
				VALUES ('union', 'srid-test', $1, $2, ST_SetSRID(ST_GeomFromText('POLYGON((0 0,1 0,1 1,0 0))'), $3), now())`,
				fmt.Sprintf("%s-%d", key, srid), key, srid)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "soil_polygons_srid_check" {
				t.Errorf("SRID %d: expected soil_polygons_srid_check (23514), got %v", srid, err)
			}
			if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_srid`); err != nil {
				t.Fatal(err)
			}
		})
	}

	var storedSRID int
	err = tx.QueryRow(ctx, `INSERT INTO soil_polygons
		(county_id, source_name, object_id, map_unit_key, shape, fetched_at)
		VALUES ('union', 'srid-test', $1, $2, ST_SetSRID(ST_GeomFromText('POLYGON((0 0,1 0,1 1,0 0))'), 3857), now())
		RETURNING ST_SRID(shape)`, key+"-3857", key).Scan(&storedSRID)
	if err != nil {
		t.Fatalf("insert SRID 3857 polygon: %v", err)
	}
	if storedSRID != 3857 {
		t.Fatalf("stored SRID = %d, want 3857", storedSRID)
	}
}
