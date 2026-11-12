//go:build postgres

package parcelstore_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5"
	"github.com/twpayne/go-geom"
)

func TestSaveParcelKeepsID(t *testing.T) {
	ctx, tx := testTx(t)
	parcel := unionParcel(t)
	callerID := models.ParcelID("caller-parcel-id")
	parcel.ID = callerID
	acres := 1.0
	parcel.Land.MappedAcres = &acres

	id1, err := parcelstore.SaveParcel(ctx, tx, parcel)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	acres = 2.5
	parcel.Land.MappedAcres = &acres
	city := "Monroe"
	description := "LOT 4 BLOCK B"
	total := 250000.0
	parcel.Mailing.City = &city
	parcel.Legal.Description = &description
	parcel.Valuation.Total = &total

	id2, err := parcelstore.SaveParcel(ctx, tx, parcel)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	fmt.Printf("%s %s\n", id1, id2)

	if id1 == "" || id2 == "" {
		t.Fatalf("empty parcel id %q %q", id1, id2)
	}
	if id1 != id2 {
		t.Fatalf("parcel id changed from %s to %s", id1, id2)
	}
	if id1 == callerID || id2 == callerID {
		t.Fatalf("stored id %s is the caller id", id1)
	}

	var count int
	err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM parcels
		WHERE county_id = $1
		  AND source_name = $2
		  AND source_parcel_id = $3
	`, parcel.Identity.CountyID, parcel.Identity.SourceName, parcel.Identity.SourceParcelID).Scan(&count)
	if err != nil {
		t.Fatalf("count parcels: %v", err)
	}
	if count != 1 {
		t.Fatalf("parcel rows %d", count)
	}

	var (
		storedID models.ParcelID
		mapped   float64
		gotCity  string
		gotLegal string
		gotTotal float64
	)
	err = tx.QueryRow(ctx, `
		SELECT id, mapped_acres, mailing_city, legal_description, valuation_total
		FROM parcels
		WHERE county_id = $1
		  AND source_name = $2
		  AND source_parcel_id = $3
	`, parcel.Identity.CountyID, parcel.Identity.SourceName, parcel.Identity.SourceParcelID).Scan(
		&storedID, &mapped, &gotCity, &gotLegal, &gotTotal,
	)
	if err != nil {
		t.Fatalf("read parcel: %v", err)
	}
	if storedID != id1 {
		t.Fatalf("stored id %s, returned %s", storedID, id1)
	}
	if mapped != 2.5 {
		t.Fatalf("mapped_acres %v", mapped)
	}
	if gotCity != city {
		t.Fatalf("mailing_city %q", gotCity)
	}
	if gotLegal != description {
		t.Fatalf("legal_description %q", gotLegal)
	}
	if gotTotal != total {
		t.Fatalf("valuation_total %v", gotTotal)
	}
}

func TestShapeWriterUnionAndRetry(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is empty")
	}
	parcel := unionParcel(t)
	parcel.Identity.SourceParcelID = "shape-" + t.Name()
	first := square(0)
	parcel.Geometry.Shape = &first
	parcel.Geometry.SRID = 3857
	w, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.SavePart(ctx, parcel, false); err != nil {
		t.Fatal(err)
	}
	second := square(2)
	parcel.Geometry.Shape = &second
	if err := w.SavePart(ctx, parcel, true); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var id string
	var area float64
	var vacant bool
	query := `SELECT id, ST_Area(shape), likely_vacant FROM parcels WHERE source_parcel_id=$1`
	if err := conn.QueryRow(ctx, query, parcel.Identity.SourceParcelID).Scan(&id, &area, &vacant); err != nil {
		t.Fatal(err)
	}
	if id == "" || area != 2 || !vacant {
		t.Fatalf("id=%q area=%v vacant=%v", id, area, vacant)
	}
	again, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	parcel.Geometry.Shape = &first
	if err := again.SavePart(ctx, parcel, false); err != nil {
		t.Fatal(err)
	}
	if err := again.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var newID string
	if err := conn.QueryRow(ctx, query, parcel.Identity.SourceParcelID).Scan(&newID, &area, &vacant); err != nil {
		t.Fatal(err)
	}
	if newID != id || area != 1 || vacant {
		t.Fatalf("retry id=%q area=%v vacant=%v", newID, area, vacant)
	}
	if _, err := conn.Exec(ctx, `DELETE FROM parcels WHERE source_parcel_id=$1`, parcel.Identity.SourceParcelID); err != nil {
		t.Fatal(err)
	}
}

func TestShapeWriterRejectsMissingShapeAndRollsBack(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is empty")
	}
	parcel := unionParcel(t)
	parcel.Identity.SourceParcelID = "rollback-" + t.Name()
	shape := square(0)
	parcel.Geometry.Shape = &shape
	parcel.Geometry.SRID = 3857
	writer, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.SavePart(ctx, parcel, true); err != nil {
		t.Fatal(err)
	}
	parcel.Geometry.Shape = nil
	if err := writer.SavePart(ctx, parcel, false); err == nil {
		t.Fatal("missing rings saved")
	}
	writer.Close()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM parcels WHERE source_parcel_id=$1`, parcel.Identity.SourceParcelID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial import committed %d rows", count)
	}
}

func square(x float64) geom.T {
	var g geom.T = geom.NewPolygonFlat(geom.XY, []float64{x, 0, x + 1, 0, x + 1, 1, x, 1, x, 0}, []int{10}).SetSRID(3857)
	return g
}

func testTx(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is empty")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		_ = conn.Close(ctx)
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() {
		err := tx.Rollback(ctx)
		if err != nil {
			t.Errorf("rollback: %v", err)
		}
		err = conn.Close(ctx)
		if err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return ctx, tx
}

func unionParcel(t *testing.T) models.GISParcel {
	t.Helper()
	return models.GISParcel{
		Identity: models.ParcelIdentity{
			CountyID:       "union",
			CountyCode:     "union",
			SourceName:     "union_county_gis",
			SourceParcelID: t.Name(),
		},
	}
}
