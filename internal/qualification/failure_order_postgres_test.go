//go:build postgres

package qualification

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFailedGateDoesNotWaitForBlockedOlderPublisher(t *testing.T) {
	ctx := context.Background()
	pool := correctedPool(t)
	store := Store{Pool: pool}
	var oldGen, parcel string
	if err := pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,completed_at) VALUES('roads',now()) RETURNING id::text`).Scan(&oldGen); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('roads',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, oldGen); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES('union','union','union_county_gis',$1) RETURNING id::text`, t.Name()).Scan(&parcel); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_assessments WHERE parcel_id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel)
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE source_group='roads'`)
		pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='roads'`)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE source_group='roads'`)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO qualification_assessments(parcel_id,source_group,generation_id,geometry_fingerprint,method_version,road_state,road_search_radius_feet) VALUES($1,'roads',$2,'missing',$3,'missing_parcel_geometry',5280)`, parcel, oldGen, RoadsMethod); err != nil {
		t.Fatal(err)
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	if _, err = blocker.Exec(ctx, `SELECT generation_id FROM qualification_heads WHERE source_group='roads' FOR SHARE`); err != nil {
		t.Fatal(err)
	}
	olderStarted := time.Now().UTC()
	var newGen string
	if err = pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,started_at,completed_at) VALUES('roads',$1,now()) RETURNING id::text`, olderStarted).Scan(&newGen); err != nil {
		t.Fatal(err)
	}
	published := make(chan error, 1)
	go func() {
		tx, e := pool.Begin(ctx)
		if e != nil {
			published <- e
			return
		}
		defer tx.Rollback(ctx)
		if e = lockQualificationPublication(ctx, tx, union.QualificationRoads); e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES('roads',$1) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, newGen)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		published <- e
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE 'INSERT INTO qualification_heads%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("publisher did not block on head")
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	_, err = store.RefreshWithGate(ctx, nil, union.QualificationRoads, func(context.Context, func(context.Context) error) error { return errors.New("newer gate failure") })
	if err == nil || !strings.Contains(err.Error(), "newer gate failure") || time.Since(start) > time.Second {
		t.Fatalf("failure recorder waited on publisher: %v, duration %v", err, time.Since(start))
	}
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-published; err != nil {
		t.Fatal(err)
	}
	result, err := CurrentRoads(ctx, pool, parcel)
	if err != nil || !result.Freshness.Stale || result.Freshness.RefreshFailedAt == nil || result.Freshness.RefreshError == nil || !strings.Contains(*result.Freshness.RefreshError, "newer gate failure") {
		t.Fatalf("newer failure lost: %+v %v", result.Freshness, err)
	}
}

func TestFailureRecorderErrorIsReturned(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires task-owned database URL")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	_, err = (Store{Pool: pool}).RefreshWithGate(context.Background(), nil, union.QualificationRoads, func(context.Context, func(context.Context) error) error { return errors.New("host gate failed") })
	if err == nil || !strings.Contains(err.Error(), "host gate failed") || !strings.Contains(err.Error(), "record qualification roads refresh failure") {
		t.Fatalf("missing persistence error: %v", err)
	}
}
