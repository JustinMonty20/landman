-- +goose Up
CREATE TABLE tax_unmatched_records (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    import_id UUID NOT NULL REFERENCES tax_file_imports(id) ON DELETE CASCADE,
    capture_id UUID REFERENCES ingest_captures(id),
    parcel_key TEXT NOT NULL CHECK (parcel_key <> ''),
    tax_year SMALLINT NOT NULL CHECK (tax_year = 0 OR tax_year BETWEEN 1900 AND 2200),
    reason TEXT NOT NULL DEFAULT 'parcel_not_found' CHECK (reason = 'parcel_not_found'),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (import_id, parcel_key, tax_year)
);
CREATE INDEX tax_unmatched_records_page ON tax_unmatched_records(import_id, id);

-- +goose Down
DROP TABLE tax_unmatched_records;
