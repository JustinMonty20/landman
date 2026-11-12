//go:build postgres

package jobs

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecoverInterruptedLeavesActiveAttemptsRunning(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	suffix := time.Now().UnixNano()
	activeID := fmt.Sprintf("test.recovery.active.%d", suffix)
	orphanID := fmt.Sprintf("test.recovery.orphan.%d", suffix)
	release := make(chan struct{})
	started := make(chan struct{})
	done := make(chan error, 1)
	runner, err := NewRunner(pool)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Errorf("active job: %v", err)
		}
		_, err := pool.Exec(ctx, "DELETE FROM job_attempts WHERE job_id IN ($1, $2)", activeID, orphanID)
		if err != nil {
			t.Errorf("clean up attempts: %v", err)
		}
	}()

	go func() {
		_, _, runErr := runner.Run(ctx, Definition{
			ID: activeID,
			Run: func(context.Context) (Result, error) {
				close(started)
				<-release
				return Result{}, nil
			},
		})
		done <- runErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("active job did not start")
	}

	_, err = pool.Exec(ctx, `INSERT INTO job_attempts (job_id, outcome) VALUES ($1, 'running')`, orphanID)
	if err != nil {
		t.Fatalf("insert orphan attempt: %v", err)
	}
	if err := runner.RecoverInterrupted(ctx); err != nil {
		t.Fatalf("RecoverInterrupted: %v", err)
	}

	var activeOutcome, orphanOutcome string
	for _, check := range []struct {
		jobID  string
		target *string
	}{{activeID, &activeOutcome}, {orphanID, &orphanOutcome}} {
		if err := pool.QueryRow(ctx, "SELECT outcome FROM job_attempts WHERE job_id = $1", check.jobID).Scan(check.target); err != nil {
			t.Fatalf("read attempt %q: %v", check.jobID, err)
		}
	}
	if activeOutcome != "running" {
		t.Errorf("active attempt outcome = %q, want running", activeOutcome)
	}
	if orphanOutcome != string(OutcomeInterrupted) {
		t.Errorf("orphan attempt outcome = %q, want %q", orphanOutcome, OutcomeInterrupted)
	}
}
