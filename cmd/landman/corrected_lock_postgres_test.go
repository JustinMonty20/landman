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

func TestCountyHostCooldownAfterFailureCancellationAndLostSession(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires task-owned PostGIS DATABASE_URL")
	}
	for _, mode := range []string{"failed", "cancelled", "lost"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			first := time.Time{}
			if mode == "failed" {
				err := withCountyHostLock(ctx, dsn, func(context.Context) error { first = time.Now(); return errors.New("request failed") })
				if err == nil || !strings.Contains(err.Error(), "request failed") {
					t.Fatal(err)
				}
			} else if mode == "cancelled" {
				work, cancel := context.WithCancel(ctx)
				err := withCountyHostLock(work, dsn, func(context.Context) error { first = time.Now(); cancel(); return context.Canceled })
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else {
				pool, err := pgxpool.New(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				entered := make(chan struct{})
				done := make(chan error, 1)
				go func() {
					done <- withCountyHostLock(ctx, dsn, func(work context.Context) error { first = time.Now(); close(entered); <-work.Done(); return work.Err() })
				}()
				<-entered
				var killed bool
				err = pool.QueryRow(ctx, `SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype='advisory' AND granted AND pid<>pg_backend_pid() AND database=(SELECT oid FROM pg_database WHERE datname=current_database()) LIMIT 1`).Scan(&killed)
				if err != nil || !killed {
					t.Fatalf("terminate: %v %v", killed, err)
				}
				if err = <-done; err == nil || !strings.Contains(err.Error(), "lock session lost") {
					t.Fatalf("lost lock: %v", err)
				}
			}
			next := time.Time{}
			if err := withCountyHostLock(ctx, dsn, func(context.Context) error { next = time.Now(); return nil }); err != nil {
				t.Fatal(err)
			}
			if next.Sub(first) < time.Second {
				t.Fatalf("callbacks %v apart", next.Sub(first))
			}
		})
	}
}
