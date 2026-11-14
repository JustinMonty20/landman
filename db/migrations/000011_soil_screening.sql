-- +goose Up
CREATE TABLE soil_screening_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    source_url TEXT NOT NULL,
    method TEXT NOT NULL,
    UNIQUE (id, county_id, source_name)
);
CREATE TABLE soil_screening_heads (
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    run_id UUID NOT NULL REFERENCES soil_screening_runs(id),
    refresh_failed_at TIMESTAMPTZ,
    refresh_error TEXT,
    PRIMARY KEY (county_id, source_name)
);
CREATE TABLE soil_screening_members (
    run_id UUID NOT NULL REFERENCES soil_screening_runs(id),
    polygon_id UUID NOT NULL REFERENCES soil_polygons(id),
    PRIMARY KEY (run_id, polygon_id)
);
CREATE INDEX soil_screening_members_polygon_idx ON soil_screening_members(polygon_id);
CREATE TABLE soil_screening_ratings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES soil_screening_runs(id),
    map_unit_key TEXT NOT NULL REFERENCES soil_map_units(map_unit_key),
    rule_key TEXT NOT NULL,
    aggregation TEXT NOT NULL,
    rating TEXT,
    availability TEXT NOT NULL CHECK (availability IN ('rated','not-rated')),
    components JSONB NOT NULL,
    evidence JSONB NOT NULL,
    source_query TEXT NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    UNIQUE (run_id, map_unit_key, rule_key, aggregation),
    CHECK ((availability='rated' AND rating IS NOT NULL) OR (availability='not-rated' AND rating IS NULL))
);
CREATE INDEX soil_screening_ratings_key_idx ON soil_screening_ratings(map_unit_key,rule_key);
-- +goose Down
DROP TABLE soil_screening_ratings;
DROP TABLE soil_screening_members;
DROP TABLE soil_screening_heads;
DROP TABLE soil_screening_runs;
