package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// QuarantineStore owns durable invalid-record evidence, pending selection, and resolution.
type QuarantineStore struct{ pool *pgxpool.Pool }

func NewQuarantineStore(pool *pgxpool.Pool) *QuarantineStore { return &QuarantineStore{pool: pool} }

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
func (s *QuarantineStore) Quarantine(ctx context.Context, record QuarantineRecord) error {
	if record.JobID == "" || record.CountyID == "" || record.SourceName == "" || record.RawRecordKey == "" || record.Reason == "" {
		return fmt.Errorf("jobs: quarantine identity is required")
	}
	if len(record.Payload) == 0 && record.PayloadReference == "" {
		return fmt.Errorf("jobs: quarantine payload or reference is required")
	}
	if len(record.Payload) > quarantinePayloadLimit {
		return fmt.Errorf("jobs: quarantine payload exceeds %d bytes", quarantinePayloadLimit)
	}
	attemptID, ok := ctx.Value(attemptIDKey{}).(string)
	if !ok || attemptID == "" {
		return fmt.Errorf("jobs: quarantine requires an active attempt")
	}
	var payload *string
	if len(record.Payload) > 0 {
		text := string(record.Payload)
		payload = &text
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO quarantined_records (job_id, county_id, source_name, raw_record_key, payload, payload_reference, reason, attempt_id)
		SELECT $1, $2, $3, $4, $5::jsonb, NULLIF($6, ''), $7, id
		FROM job_attempts
		WHERE id = $8::uuid AND job_id = $1 AND outcome = 'running'
		ON CONFLICT (job_id, raw_record_key, reason) DO UPDATE
		SET attempt_id = EXCLUDED.attempt_id,
		    payload = EXCLUDED.payload, payload_reference = EXCLUDED.payload_reference,
		    reprocessed_at = NULL, last_seen_at = clock_timestamp(), occurrence_count = quarantined_records.occurrence_count + 1`,
		record.JobID, record.CountyID, record.SourceName, record.RawRecordKey, payload, record.PayloadReference, record.Reason, attemptID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("jobs: no active attempt found for quarantine record")
	}
	return nil
}

// PendingRecord carries identity, not saved attributes for replay.
type PendingRecord struct {
	ID           string
	RawRecordKey string
	PID          string
	LastSeenAt   time.Time
}

// Pending selects a bounded deterministic set while the caller holds the source job claim.
func (s *QuarantineStore) Pending(ctx context.Context, jobID string, limit int) ([]PendingRecord, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("jobs: reprocess limit must be between 1 and 100")
	}
	if err := s.active(ctx, jobID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, raw_record_key,
 COALESCE(NULLIF(payload #>> '{attributes,PID}', ''), raw_record_key), last_seen_at
 FROM quarantined_records WHERE job_id=$1 AND reprocessed_at IS NULL
 ORDER BY last_seen_at, id LIMIT $2`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []PendingRecord
	for rows.Next() {
		var record PendingRecord
		if err := rows.Scan(&record.ID, &record.RawRecordKey, &record.PID, &record.LastSeenAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *QuarantineStore) active(ctx context.Context, jobID string) error {
	id, _ := ctx.Value(attemptIDKey{}).(string)
	var active bool
	if id == "" {
		return fmt.Errorf("jobs: quarantine operation requires an active attempt")
	}
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM job_attempts WHERE id=$1 AND job_id=$2 AND outcome='running')`, id, jobID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("jobs: no active attempt found for quarantine operation")
	}
	return nil
}

// ResolveObserved runs on the parcel publication transaction. A rollback leaves evidence pending.
// A nil selection resolves all older observed evidence. Repairs match selected UUIDs and last-seen tokens.
func (s *QuarantineStore) ResolveObserved(ctx context.Context, tx pgx.Tx, jobID string, keys []string, selected []PendingRecord) error {
	id, _ := ctx.Value(attemptIDKey{}).(string)
	if id == "" {
		return fmt.Errorf("jobs: quarantine resolution requires an active attempt")
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM job_attempts WHERE id=$1 AND job_id=$2 AND outcome='running')`, id, jobID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("jobs: no active attempt found for quarantine resolution")
	}
	var ids []string
	var lastSeen []time.Time
	if selected != nil {
		ids = make([]string, 0, len(selected))
		lastSeen = make([]time.Time, 0, len(selected))
		for _, record := range selected {
			ids = append(ids, record.ID)
			lastSeen = append(lastSeen, record.LastSeenAt)
		}
	}
	_, err := tx.Exec(ctx, `UPDATE quarantined_records SET reprocessed_at=clock_timestamp()
WHERE job_id=$1 AND raw_record_key=ANY($2::text[]) AND reprocessed_at IS NULL
AND last_seen_at <= (SELECT started_at FROM job_attempts WHERE id=$3 AND job_id=$1 AND outcome='running')
AND ($4::uuid[] IS NULL OR (id,last_seen_at) IN (SELECT * FROM unnest($4::uuid[],$5::timestamptz[])))`, jobID, keys, id, ids, lastSeen)
	return err
}
