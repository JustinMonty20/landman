-- +goose Up
CREATE FUNCTION parcel_boundary_usable(g geometry) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS '
 SELECT g IS NOT NULL AND ST_SRID(g)=3857 AND ST_NDims(g)=2
 AND ST_GeometryType(g) IN (''ST_Polygon'',''ST_MultiPolygon'')
 AND NOT ST_IsEmpty(g) AND ST_IsValid(g,0)';

CREATE TABLE parcel_geometry_decisions (
    artifact_id text NOT NULL REFERENCES parcel_evidence_observations(artifact_id),
    source_parcel_id text NOT NULL,
    policy_version text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('repaired','invalid_geometry','zero_movement_refused')),
    object_ids bigint[] NOT NULL CHECK (cardinality(object_ids)>0),
    geometry_hash text CHECK (geometry_hash ~ '^[0-9a-f]{64}$'),
    prior_valid_shape geometry(Geometry,3857),
    prior_shape_at timestamptz,
    CHECK ((outcome='repaired') = (geometry_hash IS NOT NULL)),
    CHECK ((prior_valid_shape IS NULL) = (prior_shape_at IS NULL)),
    CHECK (prior_valid_shape IS NULL OR parcel_boundary_usable(prior_valid_shape)),
    CHECK (outcome<>'repaired' OR prior_valid_shape IS NULL),
    PRIMARY KEY (artifact_id,source_parcel_id,policy_version)
);
CREATE INDEX ON parcel_geometry_decisions(source_parcel_id) WHERE outcome<>'repaired';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM parcel_geometry_decisions) THEN
  RAISE EXCEPTION 'cannot discard parcel geometry decisions';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE parcel_geometry_decisions;
DROP FUNCTION parcel_boundary_usable(geometry);
