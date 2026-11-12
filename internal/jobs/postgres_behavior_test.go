//go:build postgres

package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRunnerRecordsOverlap(t *testing.T) {
	pool, ctx := postgresTestPool(t)
	jobID := postgresTestJobID("overlap")
	cleanupPostgresJobHistory(t, pool, jobID)
	runner, err := NewRunner(pool)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	release := make(chan struct{})
	started := make(chan struct{})
	done := make(chan error, 1)
	defer func() {
		close(release)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("first run: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("first run did not finish after release")
		}
	}()

	go func() {
		_, _, runErr := runner.Run(ctx, Definition{
			ID: jobID,
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
		t.Fatal("first run did not start")
	}

	invoked := make(chan struct{}, 1)
	outcome, _, err := runner.Run(ctx, Definition{
		ID: jobID,
		Run: func(context.Context) (Result, error) {
			invoked <- struct{}{}
			return Result{}, nil
		},
	})
	if err != nil {
		t.Fatalf("overlapping run: %v", err)
	}
	if outcome != OutcomeSkippedOverlap {
		t.Fatalf("overlapping outcome = %q, want %q", outcome, OutcomeSkippedOverlap)
	}
	select {
	case <-invoked:
		t.Fatal("overlapping run invoked the job")
	default:
	}

	var running, skipped int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE outcome = 'running'),
		       count(*) FILTER (WHERE outcome = 'skipped_overlap')
		FROM job_attempts WHERE job_id = $1`, jobID).Scan(&running, &skipped); err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if running != 1 || skipped != 1 {
		t.Fatalf("running attempts = %d, skipped attempts = %d, want 1 each", running, skipped)
	}
}

func TestPostgresRunnerPersistsOutcomesAndLastCleanSuccess(t *testing.T) {
	pool, ctx := postgresTestPool(t)
	store := &postgresStore{pool: pool}
	runner, err := NewRunner(pool)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	cases := []struct {
		name    string
		result  Result
		runErr  error
		outcome Outcome
	}{
		{name: "success", result: Result{Processed: 2}, outcome: OutcomeSuccess},
		{name: "success with quarantine", result: Result{Processed: 3, Quarantined: 1}, outcome: OutcomeSuccessWithQuarantine},
		{name: "failed", runErr: errors.New("source failed"), outcome: OutcomeFailed},
		{name: "interrupted", runErr: context.Canceled, outcome: OutcomeInterrupted},
	}
	jobIDs := make([]string, 0, len(cases)+1)
	for _, tc := range cases {
		jobIDs = append(jobIDs, postgresTestJobID("outcome."+strings.ReplaceAll(tc.name, " ", ".")))
	}
	cleanID := postgresTestJobID("last-clean")
	jobIDs = append(jobIDs, cleanID)
	cleanupPostgresJobHistory(t, pool, jobIDs...)
	for i, tc := range cases {
		jobID := jobIDs[i]
		outcome, _, runErr := runner.Run(ctx, Definition{
			ID:  jobID,
			Run: func(context.Context) (Result, error) { return tc.result, tc.runErr },
		})
		if outcome != tc.outcome || !errors.Is(runErr, tc.runErr) {
			t.Errorf("Run(%s) = (%q, %v), want (%q, %v)", tc.name, outcome, runErr, tc.outcome, tc.runErr)
		}
		var persisted string
		var processed, quarantined int
		var errorText *string
		if err := pool.QueryRow(ctx, `SELECT outcome, processed_count, quarantined_count, error_text
			FROM job_attempts WHERE job_id = $1`, jobID).Scan(&persisted, &processed, &quarantined, &errorText); err != nil {
			t.Fatalf("read %s outcome: %v", tc.name, err)
		}
		if persisted != string(tc.outcome) || processed != tc.result.Processed || quarantined != tc.result.Quarantined {
			t.Errorf("persisted %s = (%q, %d, %d), want (%q, %d, %d)", tc.name,
				persisted, processed, quarantined, tc.outcome, tc.result.Processed, tc.result.Quarantined)
		}
		if tc.runErr == nil && errorText != nil || tc.runErr != nil && (errorText == nil || *errorText != tc.runErr.Error()) {
			t.Errorf("persisted %s error = %v, want %v", tc.name, errorText, tc.runErr)
		}
	}

	if _, _, err := runner.Run(ctx, Definition{
		ID:  cleanID,
		Run: func(context.Context) (Result, error) { return Result{Processed: 4}, nil },
	}); err != nil {
		t.Fatalf("clean run: %v", err)
	}
	clean, found, err := store.LastCleanSuccess(ctx, cleanID)
	if err != nil || !found {
		t.Fatalf("LastCleanSuccess after clean run = (%v, %v), want found", clean, err)
	}
	if _, _, err := runner.Run(ctx, Definition{
		ID:  cleanID,
		Run: func(context.Context) (Result, error) { return Result{Quarantined: 1}, nil },
	}); err != nil {
		t.Fatalf("quarantined run: %v", err)
	}
	latestClean, found, err := store.LastCleanSuccess(ctx, cleanID)
	if err != nil || !found || latestClean != clean {
		t.Fatalf("LastCleanSuccess after quarantined run = (%v, %v), want prior clean result %v", latestClean, err, clean)
	}
	missing, found, err := store.LastCleanSuccess(ctx, jobIDs[2])
	if err != nil || found {
		t.Fatalf("LastCleanSuccess for failed-only job = (%v, %v), want no result", missing, err)
	}
}

func TestPostgresQuarantineDeduplicatesAndTracksLatestAttempt(t *testing.T) {
	pool, ctx := postgresTestPool(t)
	store := &postgresStore{pool: pool}
	runner, err := NewRunner(pool)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	jobID := postgresTestJobID("quarantine-dedupe")
	cleanupPostgresJobHistory(t, pool, jobID)
	record := QuarantineRecord{
		JobID:        jobID,
		CountyID:     "union",
		SourceName:   "union_county_gis",
		RawRecordKey: "object-1",
		Payload:      []byte(`{"objectId":1}`),
		Reason:       "invalid geometry",
	}
	var priorFirstSeen time.Time
	var previousAttemptID string
	for attempt := 0; attempt < 2; attempt++ {
		var currentAttemptID string
		_, _, err := runner.Run(ctx, Definition{
			ID: jobID,
			Run: func(runCtx context.Context) (Result, error) {
				if err := pool.QueryRow(runCtx, "SELECT id::text FROM job_attempts WHERE job_id = $1 AND outcome = 'running'", jobID).Scan(&currentAttemptID); err != nil {
					return Result{}, err
				}
				if err := store.Quarantine(runCtx, record); err != nil {
					return Result{}, err
				}
				return Result{Quarantined: 1}, nil
			},
		})
		if err != nil {
			t.Fatalf("run %d: %v", attempt+1, err)
		}
		if currentAttemptID == previousAttemptID {
			t.Fatal("retry reused the same attempt ID")
		}
		previousAttemptID = currentAttemptID

		var attemptID string
		var occurrences int
		var firstSeen, lastSeen time.Time
		if err := pool.QueryRow(ctx, `
			SELECT attempt_id::text, occurrence_count, first_seen_at, last_seen_at
			FROM quarantined_records
			WHERE job_id = $1 AND raw_record_key = $2 AND reason = $3`,
			jobID, record.RawRecordKey, record.Reason).Scan(&attemptID, &occurrences, &firstSeen, &lastSeen); err != nil {
			t.Fatalf("read quarantine after attempt %d: %v", attempt+1, err)
		}
		if attemptID != currentAttemptID {
			t.Errorf("quarantine attempt ID = %q, want latest attempt %q", attemptID, currentAttemptID)
		}
		if occurrences != attempt+1 {
			t.Errorf("occurrence count = %d, want %d", occurrences, attempt+1)
		}
		if attempt == 0 {
			priorFirstSeen = firstSeen
		} else if !firstSeen.Equal(priorFirstSeen) || lastSeen.Before(firstSeen) {
			t.Errorf("seen times = (%v, %v), want preserved first time %v and last time no earlier", firstSeen, lastSeen, priorFirstSeen)
		}
	}
}

func TestPostgresQuarantineRequiresActiveAttempt(t *testing.T) {
	pool, ctx := postgresTestPool(t)
	store := &postgresStore{pool: pool}
	jobID := postgresTestJobID("quarantine-no-active-attempt")
	cleanupPostgresJobHistory(t, pool, jobID)
	if _, err := pool.Exec(ctx, `INSERT INTO job_attempts (job_id, outcome) VALUES ($1, 'running')`, jobID); err != nil {
		t.Fatalf("insert abandoned attempt: %v", err)
	}
	if err := store.Quarantine(ctx, QuarantineRecord{
		JobID: jobID, CountyID: "union", SourceName: "union_county_gis",
		RawRecordKey: "object-1", Payload: []byte(`{"objectId":1}`), Reason: "invalid record",
	}); err == nil {
		t.Fatal("Quarantine() accepted an abandoned attempt without an active runner")
	}
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM quarantined_records WHERE job_id = $1", jobID).Scan(&rows); err != nil {
		t.Fatalf("count quarantine rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("quarantine rows = %d, want 0", rows)
	}
}

func TestPostgresQuarantineRejectsOversizedPayloadInCodeAndDatabase(t *testing.T) {
	const maxPayloadBytes = 64 << 10
	pool, ctx := postgresTestPool(t)
	store := &postgresStore{pool: pool}
	runner, err := NewRunner(pool)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	jobID := postgresTestJobID("quarantine-payload")
	cleanupPostgresJobHistory(t, pool, jobID)
	oversized := []byte(`"` + strings.Repeat("x", maxPayloadBytes) + `"`)
	outcome, _, runErr := runner.Run(ctx, Definition{
		ID: jobID,
		Run: func(runCtx context.Context) (Result, error) {
			err := store.Quarantine(runCtx, QuarantineRecord{
				JobID: jobID, CountyID: "union", SourceName: "union_county_gis",
				RawRecordKey: "object-large", Payload: oversized, Reason: "invalid record",
			})
			return Result{}, err
		},
	})
	if runErr == nil || outcome != OutcomeFailed {
		t.Fatalf("oversized Quarantine() = (%q, %v), want failure", outcome, runErr)
	}
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM quarantined_records WHERE job_id = $1", jobID).Scan(&rows); err != nil {
		t.Fatalf("count quarantine rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("oversized Quarantine() wrote %d rows, want 0", rows)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO quarantined_records (job_id, county_id, source_name, raw_record_key, payload, reason)
		VALUES ($1, 'union', 'union_county_gis', 'object-db-large', to_jsonb($2::text), 'invalid record')`,
		jobID, strings.Repeat("x", maxPayloadBytes))
	if err == nil {
		t.Fatal("database accepted a payload larger than 64 KiB")
	}
}

func postgresTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cancel()
	})
	return pool, ctx
}

func postgresTestJobID(name string) string {
	return fmt.Sprintf("test.%s.%d", name, time.Now().UnixNano())
}

func cleanupPostgresJobHistory(t *testing.T, pool *pgxpool.Pool, jobIDs ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, jobID := range jobIDs {
			if _, err := pool.Exec(ctx, "DELETE FROM quarantined_records WHERE job_id = $1", jobID); err != nil {
				t.Errorf("delete quarantined records for %q: %v", jobID, err)
			}
			if _, err := pool.Exec(ctx, "DELETE FROM job_attempts WHERE job_id = $1", jobID); err != nil {
				t.Errorf("delete attempts for %q: %v", jobID, err)
			}
		}
	})
}
