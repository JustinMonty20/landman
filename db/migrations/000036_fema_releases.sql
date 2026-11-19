-- +goose Up
CREATE TABLE fema_releases (
    capture_id uuid PRIMARY KEY REFERENCES ingest_captures(id),
    study_id text NOT NULL CHECK (study_id = '37179C'),
    product_id bigint NOT NULL,
    issued_at timestamptz NOT NULL,
    catalog_effective_at timestamptz NOT NULL,
    index_effective_date date,
    database_revision_date date,
    archive_sha256 text NOT NULL,
    source_srid integer NOT NULL CHECK (source_srid = 4269),
    observed_at timestamptz NOT NULL
);
CREATE TABLE fema_release_head (
    study_id text PRIMARY KEY CHECK (study_id = '37179C'),
    capture_id uuid NOT NULL REFERENCES fema_releases(capture_id),
    latest_observation_id uuid NOT NULL REFERENCES ingest_captures(id),
    latest_observed_at timestamptz NOT NULL
);
CREATE TABLE fema_zones (
    capture_id uuid NOT NULL REFERENCES fema_releases(capture_id),
    source_id text NOT NULL,
    zone_code text NOT NULL,
    zone_subtype text NOT NULL,
    sfha_flag text NOT NULL,
    shape geometry(MultiPolygon,4269) NOT NULL,
    repaired boolean NOT NULL,
    repair_delta_m2 double precision NOT NULL,
    PRIMARY KEY (capture_id, source_id)
);
CREATE INDEX fema_zones_shape_idx ON fema_zones USING gist (shape);
CREATE TABLE fema_panels (
    capture_id uuid NOT NULL REFERENCES fema_releases(capture_id),
    source_id text NOT NULL,
    panel_number text NOT NULL,
    effective_date date,
    shape geometry(MultiPolygon,4269) NOT NULL,
    repaired boolean NOT NULL,
    repair_delta_m2 double precision NOT NULL,
    PRIMARY KEY (capture_id, source_id)
);
CREATE INDEX fema_panels_shape_idx ON fema_panels USING gist (shape);
CREATE TABLE fema_coverage (
    capture_id uuid PRIMARY KEY REFERENCES fema_releases(capture_id),
    panel_extent geometry(MultiPolygon,4269) NOT NULL,
    mapped_extent geometry(MultiPolygon,4269) NOT NULL,
    panel_gap geometry(MultiPolygon,4269) NOT NULL
);

-- +goose Down
DROP TABLE fema_coverage, fema_panels, fema_zones, fema_release_head, fema_releases;
