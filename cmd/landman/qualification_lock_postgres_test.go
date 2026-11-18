//go:build postgres

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCountyHostExclusionAndCancellation(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostGIS DATABASE_URL")
	}
	ctx := context.Background()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- withCountyHostLock(ctx, dsn, func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	waitingCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	err := withCountyHostLock(waitingCtx, dsn, func(context.Context) error { t.Error("overlapping host access"); return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting lock: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = withCountyHostLock(ctx, dsn, func(context.Context) error { return nil })
	if err != nil || time.Since(start) < time.Second {
		t.Fatalf("released host lock early: %v %v", time.Since(start), err)
	}
}

func TestCountyHostSessionLossCancelsWork(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostGIS DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- withCountyHostLock(ctx, dsn, func(workCtx context.Context) error {
			close(entered)
			<-workCtx.Done()
			return workCtx.Err()
		})
	}()
	<-entered
	var terminated bool
	err = pool.QueryRow(ctx, `SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype='advisory' AND granted AND pid<>pg_backend_pid() AND database=(SELECT oid FROM pg_database WHERE datname=current_database()) LIMIT 1`).Scan(&terminated)
	if err != nil || !terminated {
		t.Fatalf("terminate owned lock session: %v %v", terminated, err)
	}
	select {
	case err = <-done:
		if err == nil || !strings.Contains(err.Error(), "lock session lost") {
			t.Fatalf("lost lock did not cancel import: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("import continued after lock session loss")
	}
}
