-- +goose Up
ALTER TABLE soil_interpretation_keys ADD COLUMN survey_version TEXT;
ALTER TABLE soil_interpretation_keys ADD COLUMN rule_version TEXT;
ALTER TABLE soil_interpretation_keys DROP CONSTRAINT soil_interpretation_keys_map_unit_key_rule_key_aggregation_key;
CREATE UNIQUE INDEX soil_interpretation_keys_revision_idx ON soil_interpretation_keys
(map_unit_key, rule_key, aggregation, survey_version, rule_version) NULLS NOT DISTINCT;
ALTER TABLE soil_screening_ratings ADD COLUMN interpretation_id UUID REFERENCES soil_interpretation_keys(id);
CREATE TABLE soil_component_facts (
    run_id UUID NOT NULL REFERENCES soil_screening_runs(id),
    interpretation_id UUID NOT NULL REFERENCES soil_interpretation_keys(id),
    component_key TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence >= 0),
    name TEXT NOT NULL,
    percent DOUBLE PRECISION CHECK (percent BETWEEN 0 AND 100),
    availability TEXT CHECK (availability IN ('rated','not-rated','unavailable')),
    original_rating TEXT,
    rating TEXT,
    reason_key TEXT,
    reason_name TEXT,
    reason TEXT,
    severity DOUBLE PRECISION CHECK (severity BETWEEN 0 AND 1),
    facts JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (run_id, interpretation_id, component_key, sequence),
    CHECK ((sequence=0 AND availability IS NOT NULL AND reason_key IS NULL)
        OR (sequence>0 AND availability IS NULL AND reason_key IS NOT NULL))
);
-- +goose Down
DROP TABLE soil_component_facts;
ALTER TABLE soil_screening_ratings DROP COLUMN interpretation_id;
DROP INDEX soil_interpretation_keys_revision_idx;
ALTER TABLE soil_interpretation_keys DROP COLUMN rule_version;
ALTER TABLE soil_interpretation_keys DROP COLUMN survey_version;
ALTER TABLE soil_interpretation_keys ADD UNIQUE (map_unit_key, rule_key, aggregation);
