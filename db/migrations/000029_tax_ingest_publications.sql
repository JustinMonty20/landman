-- +goose Up
CREATE TABLE tax_ingest_publications (
    source_name TEXT PRIMARY KEY,
    source_url TEXT NOT NULL,
    capture_id UUID NOT NULL REFERENCES ingest_captures(id),
    byte_sha256 BYTEA NOT NULL CHECK (octet_length(byte_sha256) = 32),
    canonical_sha256 BYTEA NOT NULL CHECK (octet_length(canonical_sha256) = 32),
    canonical_version TEXT NOT NULL,
    normalized_sha256 BYTEA NOT NULL CHECK (octet_length(normalized_sha256) = 32),
    normalization_version TEXT NOT NULL
);
-- +goose Down
DROP TABLE tax_ingest_publications;
