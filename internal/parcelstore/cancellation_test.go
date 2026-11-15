//go:build postgres

package parcelstore_test

import (
	"context"
	"os"
	"testing"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelstore"
)

func TestShapeWriterCancellationKeepsPublishedSnapshot(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is empty")
	}
	parcel := shapedParcel(t, "cancel-"+t.Name())
	originalAmount := 100.0
	parcel.Sales = []models.ParcelSale{{Sequence: 1, Amount: &originalAmount}}
	commitPart(t, dsn, parcel)
	conn := openConn(t, dsn)
	t.Cleanup(func() {
		defer conn.Close(context.Background())
		if _, err := conn.Exec(context.Background(), `DELETE FROM parcels WHERE source_parcel_id = $1`, parcel.Identity.SourceParcelID); err != nil {
			t.Error(err)
		}
	})
	original := saleRows(t, context.Background(), conn, parcel.Identity.SourceParcelID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	changedShape := square(10)
	parcel.Geometry.Shape = &changedShape
	changedAmount := 200.0
	parcel.Sales = []models.ParcelSale{{Sequence: 3, Amount: &changedAmount}}
	for i := 0; i < 2000; i++ {
		if err := writer.SavePart(ctx, parcel, false); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	if err := writer.Commit(ctx); err == nil {
		t.Fatal("canceled import committed")
	}
	writer.Close()
	var area, minX float64
	if err := conn.QueryRow(context.Background(), `SELECT ST_Area(shape), ST_XMin(shape)
		FROM parcels WHERE source_parcel_id = $1`, parcel.Identity.SourceParcelID).Scan(&area, &minX); err != nil {
		t.Fatal(err)
	}
	if area != 1 || minX != 0 {
		t.Fatalf("published shape changed: area=%v minX=%v", area, minX)
	}
	got := saleRows(t, context.Background(), conn, parcel.Identity.SourceParcelID)
	if len(got) != 1 || got[0] != original[0] {
		t.Fatalf("published sales changed: got=%+v original=%+v", got, original)
	}
}
