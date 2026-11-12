//go:build postgres

package parcelstore_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5"
	"github.com/twpayne/go-geom"
)

func TestShapeWriterLiveMultipleShells(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "connector", "union", "testdata", "parcel-multiple-shells.json"))
	if err != nil {
		t.Fatal(err)
	}
	var feature map[string]interface{}
	if err := json.Unmarshal(data, &feature); err != nil {
		t.Fatal(err)
	}
	parcel, err := union.NewGISTranslator().Translate(connector.RawRecord{SourceName: "union_county_gis", ParcelID: "03042001A", RawData: feature})
	if err != nil {
		t.Fatal(err)
	}
	parcel.Identity.SourceParcelID = "topology-test-" + t.Name()
	multi, ok := (*parcel.Geometry.Shape).(*geom.MultiPolygon)
	if !ok || multi.NumPolygons() != 2 {
		t.Fatalf("expected two shells, got %T", *parcel.Geometry.Shape)
	}
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is empty")
	}
	writer, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		cleanupConn, err := pgx.Connect(cleanupCtx, dsn)
		if err != nil {
			t.Errorf("connect to clean up parcel: %v", err)
			return
		}
		defer cleanupConn.Close(cleanupCtx)
		if _, err := cleanupConn.Exec(cleanupCtx, `DELETE FROM parcels WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1`, parcel.Identity.SourceParcelID); err != nil {
			t.Errorf("clean up parcel: %v", err)
		}
	})
	if err := writer.SavePart(ctx, *parcel, false); err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var valid bool
	var count int
	if err := conn.QueryRow(ctx, `SELECT ST_IsValid(shape), ST_NumGeometries(shape) FROM parcels WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1`, parcel.Identity.SourceParcelID).Scan(&valid, &count); err != nil {
		t.Fatal(err)
	}
	if !valid || count != 2 {
		t.Fatalf("valid=%v parts=%d", valid, count)
	}
}
