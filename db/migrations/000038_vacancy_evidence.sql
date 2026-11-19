-- +goose Up
CREATE TABLE vacancy_evidence_generations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_name TEXT NOT NULL CHECK (source_name IN ('union_structures','union_assessed_value')),
    source_url TEXT NOT NULL,
    artifact_sha256 TEXT NOT NULL CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
    retrieved_at TIMESTAMPTZ NOT NULL,
    effective_at TIMESTAMPTZ,
    observed_count INTEGER NOT NULL CHECK (observed_count >= 0),
    coverage_verified BOOLEAN NOT NULL DEFAULT false CHECK (coverage_verified = false),
    validation_note TEXT NOT NULL,
    UNIQUE (source_name, artifact_sha256)
);
CREATE TABLE vacancy_structures (
    generation_id UUID NOT NULL REFERENCES vacancy_evidence_generations(id),
    feature_id TEXT NOT NULL,
    pin TEXT,
    status TEXT NOT NULL,
    shape geometry(Geometry,3857) NOT NULL,
    PRIMARY KEY (generation_id,feature_id),
    CHECK (ST_IsValid(shape) AND NOT ST_IsEmpty(shape) AND ST_Dimension(shape)=2)
);
CREATE INDEX vacancy_structures_shape_idx ON vacancy_structures USING GIST(shape);
CREATE TABLE vacancy_assessment_rows (
    generation_id UUID NOT NULL REFERENCES vacancy_evidence_generations(id),
    row_id TEXT NOT NULL,
    parcel_number TEXT NOT NULL,
    assessment_year INTEGER NOT NULL,
    category TEXT NOT NULL,
    building_value NUMERIC(16,2),
    PRIMARY KEY (generation_id,row_id)
);
CREATE INDEX vacancy_assessment_rows_key_idx ON vacancy_assessment_rows(generation_id,parcel_number);
CREATE TABLE vacancy_assessments (
    parcel_id UUID PRIMARY KEY REFERENCES parcels(id) ON DELETE CASCADE,
    tier TEXT NOT NULL CHECK (tier = 'unknown'),
    rule_version INTEGER NOT NULL CHECK (rule_version > 0),
    parcel_generation TEXT NOT NULL,
    structure_generation UUID NOT NULL REFERENCES vacancy_evidence_generations(id),
    assessment_generation UUID NOT NULL REFERENCES vacancy_evidence_generations(id),
    facts JSONB NOT NULL,
    reason TEXT NOT NULL,
    assessed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX vacancy_assessments_tier_idx ON vacancy_assessments(tier);
-- +goose Down
DROP TABLE vacancy_assessments;
DROP TABLE vacancy_assessment_rows;
DROP TABLE vacancy_structures;
DROP TABLE vacancy_evidence_generations;
