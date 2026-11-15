-- +goose Up
ALTER TABLE job_attempts ADD COLUMN attempt_kind TEXT NOT NULL DEFAULT 'full_scan'
CHECK (attempt_kind IN ('full_scan', 'reprocess'));
CREATE INDEX job_attempts_clean_full_idx ON job_attempts (job_id, finished_at DESC)
WHERE outcome = 'success' AND attempt_kind = 'full_scan';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM job_attempts WHERE attempt_kind = 'reprocess') THEN
        RAISE EXCEPTION 'cannot remove attempt kind while repair history exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP INDEX job_attempts_clean_full_idx;
ALTER TABLE job_attempts DROP COLUMN attempt_kind;
