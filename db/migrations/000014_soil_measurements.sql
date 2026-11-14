-- +goose Up
CREATE TABLE soil_measurements (
 run_id UUID NOT NULL REFERENCES soil_screening_runs(id),
 interpretation_id UUID NOT NULL REFERENCES soil_interpretation_keys(id),
 component_key TEXT NOT NULL,
 kind TEXT NOT NULL CHECK (kind IN ('water_table_depth','flood_frequency','ponding_frequency','linear_extensibility','restriction_depth')),
 source_key TEXT NOT NULL,
 position INTEGER NOT NULL,
 payload JSONB NOT NULL,
 PRIMARY KEY (run_id,interpretation_id,component_key,kind,source_key)
);
-- +goose Down
DROP TABLE soil_measurements;
