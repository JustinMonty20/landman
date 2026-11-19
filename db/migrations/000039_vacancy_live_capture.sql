-- +goose Up
CREATE TABLE vacancy_live_generations (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 source_name TEXT NOT NULL CHECK (source_name IN ('union_structures_171', 'union_assessed_value_0')),
 capture_id UUID NOT NULL UNIQUE REFERENCES ingest_captures(id),
 retrieved_at TIMESTAMPTZ NOT NULL,
 observed_count INTEGER NOT NULL CHECK (observed_count >= 0),
 coverage_verified BOOLEAN NOT NULL DEFAULT false CHECK (coverage_verified = false),
 validation_note TEXT NOT NULL
);
CREATE TABLE vacancy_live_structures (
 generation_id UUID NOT NULL REFERENCES vacancy_live_generations(id),
 ordinal INTEGER NOT NULL,
 object_id TEXT NOT NULL,
 outline_id TEXT,
 pin TEXT,
 status TEXT,
 geometry JSONB NOT NULL,
 source_crs INTEGER NOT NULL,
 PRIMARY KEY (generation_id, ordinal)
);
CREATE TABLE vacancy_live_assessments (
 generation_id UUID NOT NULL REFERENCES vacancy_live_generations(id),
 ordinal INTEGER NOT NULL,
 object_id TEXT,
 parcel_number TEXT NOT NULL,
 parcel_year INTEGER,
 assess_category TEXT,
 market_building NUMERIC(16,2),
 market_land NUMERIC(16,2),
 market_total NUMERIC(16,2),
 PRIMARY KEY (generation_id, ordinal)
);
CREATE INDEX vacancy_live_assessments_parcel_idx ON vacancy_live_assessments(parcel_number);
-- +goose Down
DROP TABLE vacancy_live_assessments;
DROP TABLE vacancy_live_structures;
DROP TABLE vacancy_live_generations;
