-- +goose Up
-- This cursor advances for verified observations even when typed values and the
-- active import do not change. A retained failed capture cannot outrank a later
-- accepted observation merely because it follows the last business publication.
CREATE TABLE tax_latest_observations (
    source_name TEXT PRIMARY KEY,
    capture_id UUID NOT NULL REFERENCES ingest_captures(id),
    observed_at TIMESTAMPTZ NOT NULL
);
-- +goose Down
DROP TABLE tax_latest_observations;
