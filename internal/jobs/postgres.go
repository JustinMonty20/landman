package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRunner stores execution history in pool. The pool must outlive active jobs.
func NewRunner(pool *pgxpool.Pool) (*Runner, error) {
	if pool == nil {
		return nil, fmt.Errorf("jobs: database pool is required")
	}
	return newRunner(&postgresStore{pool: pool}), nil
}

const quarantinePayloadLimit = 64 * 1024

type postgresStore struct {
	pool *pgxpool.Pool
}

func (s *postgresStore) claim(ctx context.Context, jobID string, kind AttemptKind) (attempt, bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}

	var claimed bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtext($1))", jobID).Scan(&claimed); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !claimed {
		conn.Release()
		return nil, false, nil
	}

	var id string
	err = conn.QueryRow(ctx, `
		INSERT INTO job_attempts (job_id, outcome, attempt_kind)
		VALUES ($1, 'running', $2)
		RETURNING id::text`, jobID, kind).Scan(&id)
	if err != nil {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock(hashtext($1))", jobID)
		conn.Release()
		return nil, false, err
	}
	return &postgresAttempt{conn: conn, attemptID: id, jobID: jobID}, true, nil
}

func (s *postgresStore) recordOverlap(ctx context.Context, jobID string, kind AttemptKind) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO job_attempts (job_id, started_at, finished_at, outcome, attempt_kind)
		VALUES ($1, now(), now(), 'skipped_overlap', $2)`, jobID, kind)
	return err
}

func (s *postgresStore) recoverInterrupted(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT job_id
		FROM job_attempts
		WHERE outcome = 'running'`)
	if err != nil {
		return err
	}
	var jobIDs []string
	for rows.Next() {
		var jobID string
		if err := rows.Scan(&jobID); err != nil {
			rows.Close()
			return err
		}
		jobIDs = append(jobIDs, jobID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, jobID := range jobIDs {
		if err := s.recoverJob(ctx, jobID); err != nil {
			return fmt.Errorf("recover job %q: %w", jobID, err)
		}
	}
	return nil
}

func (s *postgresStore) recoverJob(ctx context.Context, jobID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var claimed bool
	if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtext($1))", jobID).Scan(&claimed); err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	_, err = tx.Exec(ctx, `
		UPDATE job_attempts
		SET outcome = 'interrupted', finished_at = now(), error_text = 'process ended before attempt completed'
		WHERE job_id = $1 AND outcome = 'running'`, jobID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// LastCleanSuccess returns the latest full scan that completed without quarantine.
func (s *postgresStore) LastCleanSuccess(ctx context.Context, jobID string) (LastCleanSuccess, bool, error) {
	var success LastCleanSuccess
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, finished_at
		FROM job_attempts
		WHERE job_id = $1 AND outcome = 'success' AND attempt_kind = 'full_scan'
		ORDER BY finished_at DESC
		LIMIT 1`, jobID).Scan(&success.JobID, &success.FinishedAt)
	if err == pgx.ErrNoRows {
		return LastCleanSuccess{}, false, nil
	}
	if err != nil {
		return LastCleanSuccess{}, false, err
	}
	return success, true, nil
}

type postgresAttempt struct {
	conn      *pgxpool.Conn
	attemptID string
	jobID     string
}

func (a *postgresAttempt) id() string { return a.attemptID }

func (a *postgresAttempt) finish(ctx context.Context, outcome Outcome, result Result, runErr error) error {
	defer a.conn.Release()
	defer func() { _, _ = a.conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext($1))", a.jobID) }()

	var errorText *string
	if runErr != nil {
		text := runErr.Error()
		errorText = &text
	}
	_, err := a.conn.Exec(ctx, `
		UPDATE job_attempts
		SET finished_at = now(), outcome = $2, processed_count = $3, quarantined_count = $4, error_text = $5
		WHERE id = $1`, a.attemptID, outcome, result.Processed, result.Quarantined, errorText)
	return err
}
