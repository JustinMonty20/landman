-- +goose Up
CREATE TABLE soil_map_units (
    map_unit_key TEXT PRIMARY KEY,
    map_unit_symbol TEXT,
    map_unit_name TEXT,
    area_symbol TEXT,
    area_name TEXT,
    fetched_at TIMESTAMPTZ,
    CONSTRAINT soil_map_units_key_nonempty CHECK (map_unit_key <> '')
);

CREATE TABLE soil_polygons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    object_id TEXT NOT NULL,
    map_unit_key TEXT NOT NULL REFERENCES soil_map_units (map_unit_key),
    spatial_version DOUBLE PRECISION,
    survey_saved_on DATE,
    shape GEOMETRY(Geometry, 3857) NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT soil_polygons_source_unique UNIQUE (county_id, source_name, object_id),
    CONSTRAINT soil_polygons_county_check CHECK (county_id IN ('union')),
    CONSTRAINT soil_polygons_srid_check CHECK (ST_SRID(shape) = 3857)
);

CREATE TABLE soil_polygon_parcels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    soil_polygon_id UUID NOT NULL REFERENCES soil_polygons (id) ON DELETE CASCADE,
    parcel_id UUID NOT NULL REFERENCES parcels (id) ON DELETE CASCADE,
    overlap_acres NUMERIC(14, 6) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT soil_polygon_parcels_pair_unique UNIQUE (soil_polygon_id, parcel_id),
    CONSTRAINT soil_polygon_parcels_acres_check CHECK (overlap_acres > 0)
);

CREATE INDEX soil_polygons_shape_gist_idx ON soil_polygons USING GIST (shape);
CREATE INDEX soil_polygons_map_unit_key_idx ON soil_polygons (map_unit_key);
CREATE INDEX soil_polygon_parcels_parcel_id_idx ON soil_polygon_parcels (parcel_id);

-- +goose Down
DROP TABLE soil_polygon_parcels;
DROP TABLE soil_polygons;
DROP TABLE soil_map_units;
