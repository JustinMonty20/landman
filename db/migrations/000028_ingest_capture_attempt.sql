-- +goose Up
ALTER TABLE ingest_captures ADD COLUMN attempt_id UUID REFERENCES job_attempts(id);
ALTER TABLE ingest_captures ADD COLUMN attempt_kind TEXT;
ALTER TABLE ingest_captures ADD CONSTRAINT ingest_capture_attempt_pair CHECK ((attempt_id IS NULL) = (attempt_kind IS NULL));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM ingest_captures WHERE attempt_id IS NOT NULL) THEN
  RAISE EXCEPTION 'cannot discard source capture attempt binding';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE ingest_captures DROP CONSTRAINT ingest_capture_attempt_pair;
ALTER TABLE ingest_captures DROP COLUMN attempt_kind;
ALTER TABLE ingest_captures DROP COLUMN attempt_id;
