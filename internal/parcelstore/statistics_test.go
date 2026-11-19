//go:build postgres

package parcelstore_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5"
)

func TestShapeWriterUpdatesStatisticsWithoutPublishingPartialShapes(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is empty")
	}
	conn := openConn(t, dsn)
	t.Cleanup(func() { _ = conn.Close(ctx) })
	schema := fmt.Sprintf("parcel_statistics_%d", time.Now().UnixNano())
	name := pgx.Identifier{schema}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(ctx, "DROP SCHEMA "+name+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	for _, query := range []string{
		"CREATE TABLE " + name + ".parcels (LIKE public.parcels INCLUDING ALL) WITH (autovacuum_enabled = false)",
		"CREATE TABLE " + name + ".parcel_sales (LIKE public.parcel_sales INCLUDING ALL) WITH (autovacuum_enabled = false)",
		"CREATE TABLE " + name + ".parcel_owner_labels (LIKE public.parcel_owner_labels INCLUDING ALL)",
		"ALTER TABLE " + name + ".parcel_sales ADD FOREIGN KEY (parcel_id) REFERENCES " + name + ".parcels(id)",
		"ALTER TABLE " + name + ".parcel_owner_labels ADD FOREIGN KEY (parcel_id) REFERENCES " + name + ".parcels(id)",
	} {
		if _, err := conn.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		values := u.Query()
		values.Set("search_path", schema+",public")
		u.RawQuery = values.Encode()
		dsn = u.String()
	} else {
		dsn += " search_path=" + schema + ",public"
	}
	writer, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(writer.Close)
	parcel := shapedParcel(t, "shared")
	parcel.Sales = []models.ParcelSale{{Sequence: 1}, {Sequence: 3}}
	if err := writer.SavePart(ctx, parcel, false); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 2000; i++ {
		parcel.Identity.SourceParcelID = fmt.Sprintf("part-%d", i)
		if err := writer.SavePart(ctx, parcel, false); err != nil {
			t.Fatal(err)
		}
	}
	var published int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+name+".parcels").Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != 0 {
		t.Fatalf("published %d parcels before the complete scan", published)
	}
	parcel.Identity.SourceParcelID = "shared"
	second := square(2)
	parcel.Geometry.Shape = &second
	parcel.Sales = []models.ParcelSale{{Sequence: 3}}
	if err := writer.SavePart(ctx, parcel, true); err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var parcelEstimate, saleEstimate float64
	if err := conn.QueryRow(ctx, `SELECT
		(SELECT reltuples FROM pg_class WHERE oid = $1::regclass),
		(SELECT reltuples FROM pg_class WHERE oid = $2::regclass)`, schema+".parcels", schema+".parcel_sales").Scan(&parcelEstimate, &saleEstimate); err != nil {
		t.Fatal(err)
	}
	if parcelEstimate < 2000 || saleEstimate < 4000 {
		t.Fatalf("table estimates did not include staged rows: parcels=%v sales=%v", parcelEstimate, saleEstimate)
	}
	var area float64
	var vacant bool
	var sales int
	if err := conn.QueryRow(ctx, "SELECT ST_Area(shape), likely_vacant, (SELECT count(*) FROM "+name+".parcel_sales s WHERE s.parcel_id = p.id) FROM "+name+".parcels p WHERE source_parcel_id = 'shared'").Scan(&area, &vacant, &sales); err != nil {
		t.Fatal(err)
	}
	if area != 2 || !vacant || sales != 1 {
		t.Fatalf("multipart parcel changed across statistics update: area=%v vacant=%v sales=%d", area, vacant, sales)
	}
}
