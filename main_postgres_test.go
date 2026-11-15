//go:build postgres

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresConcurrentSourceRefreshes(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	otherPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer otherPool.Close()
	first, _ := jobs.NewRunner(pool)
	second, _ := jobs.NewRunner(otherPool)
	prefix := fmt.Sprintf("test.union-refresh.%d", time.Now().UnixNano())
	screenID := prefix + ".screening"
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM job_attempts WHERE job_id LIKE $1`, prefix+"%")
	}()

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 2)
	firstRefresh := func(ctx context.Context) error {
		return withRefreshLock(ctx, dsn, func(ctx context.Context) error {
			return runRequired(ctx, first, jobs.Definition{ID: screenID, Run: func(ctx context.Context) (jobs.Result, error) {
				close(entered)
				select {
				case <-release:
					return jobs.Result{}, nil
				case <-ctx.Done():
					return jobs.Result{}, ctx.Err()
				}
			}})
		})
	}
	secondRefresh := func(ctx context.Context) error {
		return withRefreshLock(ctx, dsn, func(ctx context.Context) error {
			return runRequired(ctx, second, jobs.Definition{ID: screenID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }})
		})
	}
	go func() {
		_, _, err := (unionExecutor{runner: first, refresh: firstRefresh}).Run(ctx, jobs.Definition{ID: prefix + ".parcels", Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }})
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		_, _, err := (unionExecutor{runner: second, refresh: secondRefresh}).Run(ctx, jobs.Definition{ID: prefix + ".soil", Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }})
		done <- err
	}()
	var waiting bool
	for !waiting && ctx.Err() == nil {
		err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory' AND query LIKE '%pg_advisory_lock%')`).Scan(&waiting)
		if err != nil {
			break
		}
		if !waiting {
			time.Sleep(5 * time.Millisecond)
		}
	}
	close(release)
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err != nil || !waiting {
		t.Fatalf("second source did not wait for refresh exclusion: %v", err)
	}
	var sources, screens, skipped int
	err = pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE job_id<>$1 AND outcome='success'),count(*) FILTER(WHERE job_id=$1 AND outcome='success'),count(*) FILTER(WHERE outcome='skipped_overlap') FROM job_attempts WHERE job_id LIKE $2`, screenID, prefix+"%").Scan(&sources, &screens, &skipped)
	if err != nil || sources != 2 || screens != 2 || skipped != 0 {
		t.Fatalf("sources=%d screens=%d skipped=%d error=%v", sources, screens, skipped, err)
	}
}

func TestPostgresRefreshWaitCancellation(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- withRefreshLock(ctx, dsn, func(ctx context.Context) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer waitCancel()
	err := withRefreshLock(waitCtx, dsn, func(context.Context) error { t.Error("canceled waiter refreshed"); return nil })
	close(release)
	if firstErr := <-done; firstErr != nil {
		t.Fatal(firstErr)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait error=%v", err)
	}
	called := false
	if err := withRefreshLock(ctx, dsn, func(context.Context) error { called = true; return nil }); err != nil || !called {
		t.Fatalf("lock not released: %v", err)
	}
}
