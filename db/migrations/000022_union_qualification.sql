-- +goose Up
CREATE TABLE qualification_generations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_group TEXT NOT NULL CHECK (source_group IN ('zoning', 'roads')),
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    CHECK (completed_at IS NULL OR completed_at >= fetched_at)
);
CREATE TABLE qualification_heads (
    source_group TEXT PRIMARY KEY CHECK (source_group IN ('zoning', 'roads')),
    generation_id UUID NOT NULL REFERENCES qualification_generations(id)
);
CREATE TABLE qualification_refresh_failures (
    source_group TEXT PRIMARY KEY CHECK (source_group IN ('zoning', 'roads')),
    failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    error TEXT NOT NULL
);
CREATE TABLE qualification_zones (
    generation_id UUID NOT NULL REFERENCES qualification_generations(id),
    layer_id INTEGER NOT NULL,
    source_id BIGINT NOT NULL,
    jurisdiction TEXT NOT NULL,
    district_code TEXT,
    district_description TEXT,
    case_reference TEXT,
    official_url TEXT,
    source_date TEXT,
    source_srid INTEGER NOT NULL,
    shape GEOMETRY(MultiPolygon,3857) NOT NULL,
raw_shape GEOMETRY(MultiPolygon,3857) NOT NULL,
repair_status TEXT NOT NULL CHECK (repair_status IN ('original','repaired')),
repair_delta_m2 DOUBLE PRECISION NOT NULL CHECK (repair_delta_m2 >= 0 AND repair_delta_m2 <= 0.01),
    PRIMARY KEY (generation_id,layer_id,source_id),
    CHECK (ST_IsValid(shape) AND NOT ST_IsEmpty(shape))
);
CREATE INDEX qualification_zones_shape_idx ON qualification_zones USING GIST(shape);
CREATE TABLE qualification_jurisdictions (
    generation_id UUID NOT NULL REFERENCES qualification_generations(id),
    layer_id INTEGER NOT NULL CHECK (layer_id IN (194,195)),
    source_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    source_date TEXT,
    source_srid INTEGER NOT NULL,
    shape GEOMETRY(MultiPolygon,3857) NOT NULL,
raw_shape GEOMETRY(MultiPolygon,3857) NOT NULL,
repair_status TEXT NOT NULL CHECK (repair_status IN ('original','repaired')),
repair_delta_m2 DOUBLE PRECISION NOT NULL CHECK (repair_delta_m2 >= 0 AND repair_delta_m2 <= 0.01),
    PRIMARY KEY (generation_id,layer_id,source_id),
    CHECK (ST_IsValid(shape) AND NOT ST_IsEmpty(shape))
);
CREATE INDEX qualification_jurisdictions_shape_idx ON qualification_jurisdictions USING GIST(shape);
CREATE TABLE qualification_boundaries (
    generation_id UUID NOT NULL REFERENCES qualification_generations(id),
    layer_id INTEGER NOT NULL DEFAULT 221 CHECK (layer_id=221),
    source_id BIGINT NOT NULL,
    source_srid INTEGER NOT NULL,
    shape GEOMETRY(MultiPolygon,3857) NOT NULL,
raw_shape GEOMETRY(MultiPolygon,3857) NOT NULL,
repair_status TEXT NOT NULL CHECK (repair_status IN ('original','repaired')),
repair_delta_m2 DOUBLE PRECISION NOT NULL CHECK (repair_delta_m2 >= 0 AND repair_delta_m2 <= 0.01),
    PRIMARY KEY (generation_id,layer_id,source_id),
    CHECK (ST_IsValid(shape) AND NOT ST_IsEmpty(shape))
);
CREATE INDEX qualification_boundaries_shape_idx ON qualification_boundaries USING GIST(shape);
CREATE TABLE qualification_roads (
    generation_id UUID NOT NULL REFERENCES qualification_generations(id),
    layer_id INTEGER NOT NULL DEFAULT 170 CHECK (layer_id=170),
    source_id BIGINT NOT NULL,
    name TEXT,
    road_class TEXT,
    status TEXT,
    source_date TIMESTAMPTZ,
    source_srid INTEGER NOT NULL,
    shape GEOMETRY(MultiLineString,3857) NOT NULL,
    PRIMARY KEY (generation_id,layer_id,source_id),
    CHECK (ST_IsValid(shape) AND NOT ST_IsEmpty(shape))
);
CREATE INDEX qualification_roads_shape_idx ON qualification_roads USING GIST(shape);
CREATE TABLE qualification_layer_receipts (
    generation_id UUID NOT NULL REFERENCES qualification_generations(id),
    layer_id INTEGER NOT NULL,
    feature_count INTEGER NOT NULL CHECK (feature_count>=0),
    source_srid INTEGER NOT NULL,
    service_url TEXT NOT NULL,
    source_metadata JSONB NOT NULL,
    PRIMARY KEY (generation_id,layer_id)
);
CREATE TABLE qualification_assessments (
    parcel_id UUID NOT NULL REFERENCES parcels(id),
    source_group TEXT NOT NULL CHECK (source_group IN ('zoning','roads')),
    generation_id UUID NOT NULL REFERENCES qualification_generations(id),
    boundary_generation_id UUID REFERENCES qualification_generations(id),
    geometry_fingerprint TEXT NOT NULL,
    method_version TEXT NOT NULL,
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    total_acres DOUBLE PRECISION,
    unmapped_acres DOUBLE PRECISION NOT NULL DEFAULT 0,
    ambiguous_acres DOUBLE PRECISION NOT NULL DEFAULT 0,
    jurisdiction_unmapped_acres DOUBLE PRECISION NOT NULL DEFAULT 0,
    jurisdiction_ambiguous_acres DOUBLE PRECISION NOT NULL DEFAULT 0,
    road_state TEXT CHECK (road_state IN ('nearest','none_within_radius','missing_parcel_geometry','source_unavailable')),
    road_distance_feet DOUBLE PRECISION,
    road_search_radius_feet INTEGER,
    county_edge_warning BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (parcel_id,source_group),
    CHECK (unmapped_acres>=0 AND ambiguous_acres>=0 AND jurisdiction_unmapped_acres>=0 AND jurisdiction_ambiguous_acres>=0),
    CHECK ((source_group='zoning' AND road_state IS NULL) OR (source_group='roads' AND road_state IS NOT NULL))
);
CREATE INDEX qualification_assessments_generation_idx ON qualification_assessments(generation_id);
CREATE TABLE qualification_zone_hits (
    parcel_id UUID NOT NULL,
    source_group TEXT NOT NULL DEFAULT 'zoning' CHECK (source_group='zoning'),
    layer_id INTEGER NOT NULL,
    source_id BIGINT NOT NULL,
    jurisdiction TEXT NOT NULL,
    district_code TEXT,
    acres DOUBLE PRECISION NOT NULL CHECK (acres>0),
    share DOUBLE PRECISION NOT NULL CHECK (share>0),
    PRIMARY KEY(parcel_id,layer_id,source_id),
    FOREIGN KEY(parcel_id,source_group) REFERENCES qualification_assessments(parcel_id,source_group) ON DELETE CASCADE
);
CREATE TABLE qualification_jurisdiction_hits (
    parcel_id UUID NOT NULL,
    source_group TEXT NOT NULL DEFAULT 'zoning' CHECK (source_group='zoning'),
    layer_id INTEGER NOT NULL,
    source_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    acres DOUBLE PRECISION NOT NULL CHECK (acres>0),
    share DOUBLE PRECISION NOT NULL CHECK (share>0),
    PRIMARY KEY(parcel_id,layer_id,source_id),
    FOREIGN KEY(parcel_id,source_group) REFERENCES qualification_assessments(parcel_id,source_group) ON DELETE CASCADE
);
CREATE TABLE qualification_road_ties (
    parcel_id UUID NOT NULL,
    source_group TEXT NOT NULL DEFAULT 'roads' CHECK (source_group='roads'),
    source_id BIGINT NOT NULL,
    name TEXT,
    road_class TEXT,
    status TEXT,
    distance_feet DOUBLE PRECISION NOT NULL CHECK(distance_feet>=0),
    PRIMARY KEY(parcel_id,source_id),
    FOREIGN KEY(parcel_id,source_group) REFERENCES qualification_assessments(parcel_id,source_group) ON DELETE CASCADE
);
-- +goose Down
DROP TABLE qualification_road_ties;
DROP TABLE qualification_jurisdiction_hits;
DROP TABLE qualification_zone_hits;
DROP TABLE qualification_assessments;
DROP TABLE qualification_layer_receipts;
DROP TABLE qualification_roads;
DROP TABLE qualification_boundaries;
DROP TABLE qualification_jurisdictions;
DROP TABLE qualification_zones;
DROP TABLE qualification_refresh_failures;
DROP TABLE qualification_heads;
DROP TABLE qualification_generations;
