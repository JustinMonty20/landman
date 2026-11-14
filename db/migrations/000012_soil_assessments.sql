-- +goose Up
CREATE TABLE soil_polygon_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    fingerprint TEXT NOT NULL,
    UNIQUE (id, county_id, source_name)
);
CREATE TABLE soil_polygon_set_members (
    set_id UUID NOT NULL REFERENCES soil_polygon_sets(id),
    polygon_id UUID NOT NULL REFERENCES soil_polygons(id),
    map_unit_key TEXT NOT NULL REFERENCES soil_map_units(map_unit_key),
    shape GEOMETRY(Geometry,3857) NOT NULL,
    spatial_version DOUBLE PRECISION,
    survey_saved_on DATE,
    PRIMARY KEY (set_id, polygon_id)
);
CREATE INDEX soil_polygon_set_members_shape_idx ON soil_polygon_set_members USING GIST(shape);
CREATE TABLE soil_polygon_set_heads (
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    set_id UUID NOT NULL,
    PRIMARY KEY (county_id, source_name),
    FOREIGN KEY (set_id, county_id, source_name) REFERENCES soil_polygon_sets(id, county_id, source_name)
);
ALTER TABLE soil_screening_runs ADD COLUMN soil_set_id UUID REFERENCES soil_polygon_sets(id);
ALTER TABLE soil_screening_runs ADD COLUMN assessment_method TEXT;
ALTER TABLE soil_screening_runs ADD COLUMN fresh_until TIMESTAMPTZ;
ALTER TABLE soil_screening_ratings DROP CONSTRAINT soil_screening_ratings_availability_check;
ALTER TABLE soil_screening_ratings DROP CONSTRAINT soil_screening_ratings_check;
ALTER TABLE soil_screening_ratings ADD CHECK (availability IN ('rated','not-rated','unavailable'));
ALTER TABLE soil_screening_ratings ADD CHECK ((availability='rated' AND rating IS NOT NULL) OR (availability<>'rated' AND rating IS NULL));
ALTER TABLE soil_screening_ratings ADD COLUMN provenance JSONB NOT NULL DEFAULT '{}';
CREATE TABLE soil_interpretation_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    map_unit_key TEXT NOT NULL REFERENCES soil_map_units(map_unit_key),
    rule_key TEXT NOT NULL,
    aggregation TEXT NOT NULL,
    UNIQUE (map_unit_key, rule_key, aggregation)
);
CREATE TABLE soil_screening_refresh_status (
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL,
    error TEXT NOT NULL,
    PRIMARY KEY (county_id,source_name)
);
CREATE TABLE soil_assessment_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parcel_id UUID NOT NULL UNIQUE REFERENCES parcels(id)
);
CREATE TABLE soil_parcel_assessments (
    run_id UUID NOT NULL REFERENCES soil_screening_runs(id),
    parcel_id UUID NOT NULL REFERENCES parcels(id),
    geometry_fingerprint TEXT NOT NULL,
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    total_acres DOUBLE PRECISION,
    uncovered_acres DOUBLE PRECISION NOT NULL CHECK (uncovered_acres >= 0),
    ambiguous_acres DOUBLE PRECISION NOT NULL CHECK (ambiguous_acres >= 0),
    PRIMARY KEY (run_id, parcel_id),
    CHECK (total_acres IS NULL OR total_acres > 0)
);
CREATE INDEX soil_parcel_assessments_parcel_idx ON soil_parcel_assessments(parcel_id);
CREATE TABLE soil_assessment_map_units (
    run_id UUID NOT NULL,
    parcel_id UUID NOT NULL,
    map_unit_key TEXT NOT NULL REFERENCES soil_map_units(map_unit_key),
    exclusive_acres DOUBLE PRECISION NOT NULL CHECK (exclusive_acres >= 0),
    mapped_acres DOUBLE PRECISION NOT NULL CHECK (mapped_acres > 0),
    polygon_ids UUID[] NOT NULL,
    PRIMARY KEY (run_id, parcel_id, map_unit_key),
    FOREIGN KEY (run_id, parcel_id) REFERENCES soil_parcel_assessments(run_id, parcel_id)
);

-- +goose Down
DROP TABLE soil_assessment_map_units;
DROP TABLE soil_parcel_assessments;
DROP TABLE soil_assessment_keys;
DROP TABLE soil_screening_refresh_status;
DROP TABLE soil_interpretation_keys;
ALTER TABLE soil_screening_ratings DROP COLUMN provenance;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM soil_screening_ratings WHERE availability='unavailable') THEN
        RAISE EXCEPTION 'cannot remove unavailable evidence status';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE soil_screening_ratings DROP CONSTRAINT soil_screening_ratings_availability_check;
ALTER TABLE soil_screening_ratings DROP CONSTRAINT soil_screening_ratings_check;
ALTER TABLE soil_screening_ratings ADD CHECK (availability IN ('rated','not-rated'));
ALTER TABLE soil_screening_ratings ADD CHECK ((availability='rated' AND rating IS NOT NULL) OR (availability='not-rated' AND rating IS NULL));
ALTER TABLE soil_screening_runs DROP COLUMN fresh_until;
ALTER TABLE soil_screening_runs DROP COLUMN assessment_method;
ALTER TABLE soil_screening_runs DROP COLUMN soil_set_id;
DROP TABLE soil_polygon_set_heads;
DROP TABLE soil_polygon_set_members;
DROP TABLE soil_polygon_sets;
