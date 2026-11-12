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

type postgresStore struct {
	pool *pgxpool.Pool
}

func (s *postgresStore) claim(ctx context.Context, jobID string) (attempt, bool, error) {
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
		INSERT INTO job_attempts (job_id, outcome)
		VALUES ($1, 'running')
		RETURNING id::text`, jobID).Scan(&id)
	if err != nil {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock(hashtext($1))", jobID)
		conn.Release()
		return nil, false, err
	}
	return &postgresAttempt{conn: conn, id: id, jobID: jobID}, true, nil
}

func (s *postgresStore) recordOverlap(ctx context.Context, jobID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO job_attempts (job_id, started_at, finished_at, outcome)
		VALUES ($1, now(), now(), 'skipped_overlap')`, jobID)
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

// LastCleanSuccess returns the latest attempt that completed without quarantine.
func (s *postgresStore) LastCleanSuccess(ctx context.Context, jobID string) (LastCleanSuccess, bool, error) {
	var success LastCleanSuccess
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, finished_at
		FROM job_attempts
		WHERE job_id = $1 AND outcome = 'success'
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
	conn  *pgxpool.Conn
	id    string
	jobID string
}

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
		WHERE id = $1`, a.id, outcome, result.Processed, result.Quarantined, errorText)
	return err
}

// QuarantineRecord is the durable evidence for one recoverable source record.
type QuarantineRecord struct {
	JobID            string
	CountyID         string
	SourceName       string
	RawRecordKey     string
	Payload          []byte
	PayloadReference string
	Reason           string
}

// Quarantine persists one invalid record independently of a job's transaction.
func (s *postgresStore) Quarantine(ctx context.Context, record QuarantineRecord) error {
	if record.JobID == "" || record.CountyID == "" || record.SourceName == "" || record.RawRecordKey == "" || record.Reason == "" {
		return fmt.Errorf("jobs: quarantine identity is required")
	}
	if len(record.Payload) == 0 && record.PayloadReference == "" {
		return fmt.Errorf("jobs: quarantine payload or reference is required")
	}
	var payload *string
	if len(record.Payload) > 0 {
		text := string(record.Payload)
		payload = &text
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO quarantined_records (job_id, county_id, source_name, raw_record_key, payload, payload_reference, reason)
		VALUES ($1, $2, $3, $4, $5::jsonb, NULLIF($6, ''), $7)
		ON CONFLICT (job_id, raw_record_key, reason) DO UPDATE
		SET last_seen_at = now(), occurrence_count = quarantined_records.occurrence_count + 1`,
		record.JobID, record.CountyID, record.SourceName, record.RawRecordKey, payload, record.PayloadReference, record.Reason)
	return err
}
