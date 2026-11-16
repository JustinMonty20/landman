-- +goose Up
CREATE TABLE tax_file_imports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL CHECK (source_name ~ '^[a-z][a-z0-9_-]*$'),
    publication_version TEXT,
    source_url TEXT NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    byte_size BIGINT NOT NULL CHECK (byte_size >= 0),
    file_sha256 BYTEA NOT NULL CHECK (octet_length(file_sha256) = 32),
    artifact_key TEXT NOT NULL,
    storage_mode TEXT NOT NULL CHECK (storage_mode IN ('s3', 'local')),
    validation_result TEXT NOT NULL CHECK (validation_result IN ('valid', 'invalid')),
    active BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at TIMESTAMPTZ,
    CONSTRAINT tax_file_import_active_valid CHECK (NOT active OR validation_result = 'valid'),
    CONSTRAINT tax_file_import_portable_key CHECK (
        artifact_key = 'tax/' || source_name || '/sha256/' || encode(file_sha256, 'hex')
    ),
    UNIQUE (county_id, source_name, source_url, file_sha256, fetched_at)
);

CREATE UNIQUE INDEX tax_file_imports_one_active
    ON tax_file_imports (county_id, source_name) WHERE active;

-- +goose Down
DROP TABLE tax_file_imports;
