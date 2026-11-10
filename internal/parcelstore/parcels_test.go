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
