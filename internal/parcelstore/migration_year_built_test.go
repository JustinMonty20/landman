//go:build postgres

package parcelstore_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestYearBuiltZeroMigrationRollback(t *testing.T) {
	ctx := context.Background()
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

	_, err = tx.Exec(ctx, `CREATE TEMP TABLE parcels (
		id integer PRIMARY KEY,
		year_built smallint,
		label text NOT NULL,
		CONSTRAINT parcels_year_built_check CHECK (year_built IS NULL OR year_built BETWEEN 1600 AND 2200)
	) ON COMMIT DROP`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/000008_parcels_year_built_zero.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down, ok := strings.Cut(string(migration), "-- +goose Down")
	if !ok {
		t.Fatal("migration has no Down section")
	}
	up = strings.TrimPrefix(up, "-- +goose Up")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("apply Up: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO parcels (id, year_built, label) VALUES
		(1, 0, 'imported zero'), (2, 1600, 'lower bound'),
		(3, 2200, 'upper bound'), (4, NULL, 'unknown')`); err != nil {
		t.Fatalf("insert rows permitted by Up: %v", err)
	}
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("apply Down with imported zero: %v", err)
	}
	rows, err := tx.Query(ctx, `SELECT id, year_built, label FROM parcels ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	var years []*int16
	var labels []string
	for rows.Next() {
		var id int
		var year *int16
		var label string
		if err := rows.Scan(&id, &year, &label); err != nil {
			t.Fatal(err)
		}
		ids, years, labels = append(ids, id), append(years, year), append(labels, label)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(ids) != 4 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 || ids[3] != 4 ||
		years[0] != nil || years[1] == nil || *years[1] != 1600 ||
		years[2] == nil || *years[2] != 2200 || years[3] != nil ||
		labels[0] != "imported zero" || labels[1] != "lower bound" ||
		labels[2] != "upper bound" || labels[3] != "unknown" {
		t.Fatalf("rolled-back rows: ids=%v years=%v labels=%v", ids, years, labels)
	}
	if _, err := tx.Exec(ctx, `SAVEPOINT before_invalid_year`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO parcels (id, year_built, label) VALUES (5, 0, 'invalid')`); err == nil {
		t.Fatal("rolled-back constraint accepted zero")
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_invalid_year`); err != nil {
		t.Fatal(err)
	}
}
