-- +goose Up
ALTER TABLE job_attempts DROP CONSTRAINT job_attempts_outcome_check;
ALTER TABLE job_attempts ADD CONSTRAINT job_attempts_outcome_check
CHECK (outcome IN ('running', 'success', 'success_with_quarantine', 'failed', 'interrupted', 'skipped_overlap', 'pending'));

-- +goose Down
-- Pending attempts must be resolved or archived explicitly before rollback.
ALTER TABLE job_attempts DROP CONSTRAINT job_attempts_outcome_check;
ALTER TABLE job_attempts ADD CONSTRAINT job_attempts_outcome_check
CHECK (outcome IN ('running', 'success', 'success_with_quarantine', 'failed', 'interrupted', 'skipped_overlap'));
