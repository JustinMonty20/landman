//go:build postgres

package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/qualification"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWaitingCountyLockFailureStalesPublishedRoadEvidence(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostGIS DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := qualification.Store{Pool: pool}
	var gen, parcel string
	if err = pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('roads',now()) RETURNING id::text`).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('roads',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, gen); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES('union','union','union_county_gis',$1) RETURNING id::text`, t.Name()).Scan(&parcel); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE generation_id=$1`, gen)
		pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='roads'`)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE id=$1`, gen)
	})
	if _, err = pool.Exec(ctx, `INSERT INTO qualification_assessments(parcel_id,source_group,generation_id,geometry_fingerprint,method_version,road_state,road_search_radius_feet) VALUES($1,'roads',$2,'missing',$3,'missing_parcel_geometry',5280)`, parcel, gen, qualification.RoadsMethod); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- withCountyHostLock(ctx, dsn, func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	for _, mode := range []string{"deadline", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			wait, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
			if mode == "cancel" {
				cancel()
			}
			defer cancel()
			_, failure := store.RefreshWithGate(wait, nil, union.QualificationRoads, func(ctx context.Context, work func(context.Context) error) error {
				return withCountyHostLock(ctx, dsn, work)
			})
			if mode == "deadline" && !errors.Is(failure, context.DeadlineExceeded) || mode == "cancel" && !errors.Is(failure, context.Canceled) {
				t.Fatalf("wait failed unexpectedly: %v", failure)
			}
			evidence, e := qualification.CurrentRoads(ctx, pool, parcel)
			if e != nil || evidence.GenerationID != gen || evidence.Freshness.RefreshFailedAt == nil || !evidence.Freshness.Stale {
				t.Fatalf("old evidence not stale: %+v %v", evidence, e)
			}
		})
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
