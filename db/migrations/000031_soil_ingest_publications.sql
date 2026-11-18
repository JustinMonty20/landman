-- +goose Up
CREATE TABLE soil_ingest_publications (
    capture_id UUID PRIMARY KEY REFERENCES ingest_captures(id),
    source_name TEXT NOT NULL,
    canonical_version TEXT NOT NULL,
    canonical_digest TEXT NOT NULL,
    normalized_version TEXT NOT NULL,
    normalized_digest TEXT NOT NULL,
    geometry_version TEXT NOT NULL,
    geometry_digest TEXT NOT NULL,
    change_cause TEXT NOT NULL CHECK (change_cause IN ('source_observation','normalization_correction','offline_retry')),
    published_set_id UUID REFERENCES soil_polygon_sets(id),
    published_run_id UUID REFERENCES soil_screening_runs(id),
    observed_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX soil_ingest_publications_source_idx ON soil_ingest_publications(source_name, observed_at DESC);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM soil_ingest_publications) THEN
        RAISE EXCEPTION 'cannot discard retained soil ingest publications';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE soil_ingest_publications;
