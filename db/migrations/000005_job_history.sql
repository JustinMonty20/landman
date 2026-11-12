-- +goose Up
CREATE TABLE job_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    outcome TEXT NOT NULL,
    processed_count INTEGER NOT NULL DEFAULT 0 CHECK (processed_count >= 0),
    quarantined_count INTEGER NOT NULL DEFAULT 0 CHECK (quarantined_count >= 0),
    error_text TEXT,
    CONSTRAINT job_attempts_outcome_check CHECK (outcome IN ('running', 'success', 'success_with_quarantine', 'failed', 'interrupted', 'skipped_overlap'))
);

CREATE INDEX job_attempts_job_started_idx ON job_attempts (job_id, started_at DESC);

CREATE TABLE quarantined_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id TEXT NOT NULL,
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    raw_record_key TEXT NOT NULL,
    payload JSONB,
    payload_reference TEXT,
    reason TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    occurrence_count INTEGER NOT NULL DEFAULT 1 CHECK (occurrence_count > 0),
    reprocessed_at TIMESTAMPTZ,
    CONSTRAINT quarantined_records_identity_unique UNIQUE (job_id, raw_record_key, reason),
    CONSTRAINT quarantined_records_payload_check CHECK (payload IS NOT NULL OR payload_reference IS NOT NULL)
);

CREATE INDEX quarantined_records_pending_idx ON quarantined_records (job_id, last_seen_at DESC) WHERE reprocessed_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS quarantined_records;
DROP TABLE IF EXISTS job_attempts;
