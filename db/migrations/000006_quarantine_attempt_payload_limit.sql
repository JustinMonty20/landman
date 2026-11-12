-- +goose Up
ALTER TABLE quarantined_records
    ADD COLUMN attempt_id UUID REFERENCES job_attempts(id);

ALTER TABLE quarantined_records
    ADD CONSTRAINT quarantined_records_payload_size_check
    CHECK (payload IS NULL OR octet_length(payload::text) <= 65536) NOT VALID;

ALTER TABLE quarantined_records
    VALIDATE CONSTRAINT quarantined_records_payload_size_check;

-- +goose Down
ALTER TABLE quarantined_records
    DROP CONSTRAINT IF EXISTS quarantined_records_payload_size_check;
ALTER TABLE quarantined_records
    DROP COLUMN IF EXISTS attempt_id;
