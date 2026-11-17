//go:build postgres

package taxstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
	"github.com/JustinMonty20/landman/internal/taxstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSavePipeline(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must name a disposable migrated database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, _, err := taxstore.NewPipeline(nil).Latest(ctx); err == nil {
		t.Fatal("missing database accepted")
	}
	if _, ok, err := taxstore.NewPipeline(pool).Latest(ctx); err != nil || ok {
		t.Fatalf("initial latest ok=%v err=%v", ok, err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := union.NewPipelineList(filepath.Join("..", "connector", "union", "testdata", "tax", "pipeline.pdf"), "https://www.unioncountync.gov/home/showdocument?id=4784", store)
	if err != nil {
		t.Fatal(err)
	}
	var rows []union.PipelineProperty
	meta, err := list.Read(ctx, func(p union.PipelineProperty) error { rows = append(rows, p); return nil })
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var parcelID string
	if err := tx.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,likely_vacant) VALUES ('union','179','union_county_gis',$1,false) RETURNING id::text`, rows[0].ParcelKey).Scan(&parcelID); err != nil {
		t.Fatal(err)
	}
	rows = append(rows, union.PipelineProperty{ParcelKey: "NOT-IN-PARCELS"})
	if _, err := taxstore.SavePipeline(ctx, tx, rows, taxsource.Meta{}, 0); err == nil {
		t.Fatal("missing evidence accepted")
	}
	negative := int64(-1)
	if _, err := taxstore.SavePipeline(ctx, tx, []union.PipelineProperty{{ParcelKey: rows[0].ParcelKey, TaxValue: &negative}}, meta, 0); err == nil {
		t.Fatal("negative value accepted")
	}
	first, err := taxstore.SavePipeline(ctx, tx, rows, meta, 1)
	if err != nil || first.Saved != 1 || first.Deleted != 0 || first.Unmatched != 1 || first.SkippedPages != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	var id string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_pipeline WHERE parcel_id=$1`, parcelID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	second, err := taxstore.SavePipeline(ctx, tx, rows, meta, 1)
	if err != nil || second.Saved != 1 || second.Deleted != 0 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	var after string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tax_foreclosure_pipeline WHERE parcel_id=$1`, parcelID).Scan(&after); err != nil || after != id {
		t.Fatalf("id before=%s after=%s err=%v", id, after, err)
	}
	changed, err := taxstore.SavePipeline(ctx, tx, nil, meta, 0)
	if err != nil || changed.Deleted != 1 {
		t.Fatalf("delete=%+v err=%v", changed, err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_pipeline WHERE parcel_id=$1`, parcelID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("remaining=%d err=%v", count, err)
	}
	if _, err := taxstore.SavePipeline(ctx, tx, rows, meta, 0); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	latest, ok, err := taxstore.NewPipeline(pool).Latest(ctx)
	if err != nil || !ok || !latest.Equal(meta.FetchedAt.Truncate(time.Microsecond)) {
		t.Fatalf("latest=%v ok=%v err=%v", latest, ok, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcelID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tax_file_imports WHERE source_name='union_foreclosure_pipeline'`); err != nil {
		t.Fatal(err)
	}
}
