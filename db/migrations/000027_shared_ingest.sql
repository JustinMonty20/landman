-- +goose Up
CREATE TABLE ingest_captures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_name TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    manifest BYTEA NOT NULL,
    manifest_sha256 BYTEA NOT NULL CHECK (octet_length(manifest_sha256) = 32),
    storage_mode TEXT NOT NULL CHECK (storage_mode IN ('local', 's3')),
    registered_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    published_at TIMESTAMPTZ
);
CREATE INDEX ingest_captures_source_idx ON ingest_captures(source_name, observed_at DESC) WHERE published_at IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM ingest_captures) THEN
        RAISE EXCEPTION 'cannot discard retained ingestion captures';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE ingest_captures;
