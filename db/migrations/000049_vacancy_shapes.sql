-- +goose Up
ALTER TABLE vacancy_live_validation ADD COLUMN current_statuses TEXT[] NOT NULL DEFAULT ARRAY[]::text[]
 CHECK (cardinality(current_statuses) > 0 OR source_name='union_assessed_value_0');
CREATE TABLE vacancy_live_shapes (
 generation_id UUID NOT NULL REFERENCES vacancy_live_generations(id),
 ordinal INTEGER NOT NULL,
 shape geometry(Polygon,3857),
 PRIMARY KEY(generation_id,ordinal)
);
CREATE INDEX vacancy_live_shapes_gist ON vacancy_live_shapes USING gist(shape);
-- +goose Down
DROP TABLE vacancy_live_shapes;
ALTER TABLE vacancy_live_validation DROP COLUMN current_statuses;
