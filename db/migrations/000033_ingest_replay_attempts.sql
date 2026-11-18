-- +goose Up
CREATE TABLE ingest_replay_attempts (
    capture_id UUID NOT NULL REFERENCES ingest_captures(id),
    attempt_id UUID NOT NULL REFERENCES job_attempts(id),
    PRIMARY KEY (capture_id, attempt_id)
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM ingest_replay_attempts) THEN
  RAISE EXCEPTION 'cannot discard source replay attempt provenance';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE ingest_replay_attempts;
