//go:build postgres

package fema

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom"
)

func TestReleasePreservedOnFailureAndReplay(t *testing.T) {
	dsn := os.Getenv("FEMA_API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("FEMA_API_TEST_DATABASE_URL not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "landman_api_test_fema" || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port == 5432 || len(cfg.ConnConfig.Fallbacks) != 0 {
		t.Fatal("use the disposable FEMA API database")
	}
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	at := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	shape := geom.NewMultiPolygonFlat(geom.XY, []float64{-80.6, 35, -80.6, 35.01, -80.59, 35.01, -80.59, 35, -80.6, 35}, [][]int{{10}}).SetSRID(4269)
	makeRelease := func(i int, date time.Time) (*publication, ingest.Observation) {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO ingest_captures(source_name,observed_at,manifest,manifest_sha256,storage_mode) VALUES('fema_union',$1,'',decode(repeat('00',32),'hex'),'local') RETURNING id::text`, date).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return &publication{product: Product{ID: int64(i), IssueMillis: date.UnixMilli(), EffectiveMillis: at.UnixMilli()}, archiveHash: id, release: Release{Zones: []Zone{{ID: "zone", Code: "AE", Shape: shape}}, Panels: []Panel{{ID: "panel", Number: "test", Shape: shape}}}}, ingest.Observation{ID: id, Manifest: ingest.Manifest{ObservedAt: date}}
	}
	apply := func(p *publication, o ingest.Observation) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return p.Apply(ctx, tx, o) })
	}
	old, older := makeRelease(1, at)
	current, accepted := makeRelease(2, at.Add(time.Hour))
	if err := apply(old, older); err != nil {
		t.Fatal(err)
	}
	if err := apply(current, accepted); err != nil {
		t.Fatal(err)
	}
	newer, failed := makeRelease(3, at.Add(2*time.Hour))
	newer.release.Panels = nil
	if err := apply(newer, failed); err == nil {
		t.Fatal("published a release with no panels")
	}
	older.Replay, older.Published = true, true
	if err := apply(old, older); !errors.Is(err, ingest.ErrNoPublication) {
		t.Fatalf("published old replay: %v", err)
	}
	older.Published = false
	if err := apply(old, older); !errors.Is(err, ingest.ErrNoPublication) {
		t.Fatalf("published superseded capture: %v", err)
	}
	var head string
	var observed time.Time
	var partial bool
	err = pool.QueryRow(ctx, `SELECT capture_id::text,latest_observed_at,EXISTS(SELECT FROM fema_releases WHERE capture_id=$1) FROM fema_release_head WHERE study_id='37179C'`, failed.ID).Scan(&head, &observed, &partial)
	if err != nil || head != accepted.ID || !observed.Equal(accepted.Manifest.ObservedAt) || partial {
		t.Fatalf("current release changed or partial release remained: %v", err)
	}
}
