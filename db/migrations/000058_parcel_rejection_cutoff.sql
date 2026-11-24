-- +goose Up
-- A rejection is an observation even when no accepted facts replace the serving row.
CREATE TABLE parcel_evidence_rejection_cutoff (
 county_id text NOT NULL,
 source_name text NOT NULL,
 source_parcel_id text NOT NULL,
 observed_at timestamptz NOT NULL,
 artifact_id text NOT NULL,
 PRIMARY KEY(county_id,source_name,source_parcel_id)
);
-- Retain OBJECTID linkage so a PID transition touches only captured identities,
-- not every source_feature in the county on each import.
CREATE TABLE parcel_source_object_identity (
 county_id text NOT NULL,
 source_name text NOT NULL,
 raw_key text NOT NULL,
 source_parcel_id text NOT NULL,
 observed_at timestamptz NOT NULL,
 PRIMARY KEY(county_id,source_name,raw_key)
);
INSERT INTO parcel_source_object_identity(county_id,source_name,raw_key,source_parcel_id,observed_at)
SELECT DISTINCT ON (c.county_id,c.source_name,part->'attributes'->>'OBJECTID')
 c.county_id,c.source_name,part->'attributes'->>'OBJECTID',c.source_parcel_id,o.observed_at
FROM parcel_evidence_current c JOIN parcel_evidence_observations o ON o.id=c.observation_id
CROSS JOIN LATERAL jsonb_array_elements(c.source_features) part
WHERE c.county_id='union' AND c.source_name='union_county_gis'
AND part->'attributes'->>'OBJECTID' IS NOT NULL
ORDER BY c.county_id,c.source_name,part->'attributes'->>'OBJECTID',o.observed_at DESC;
-- +goose Down
DROP TABLE parcel_source_object_identity;
DROP TABLE parcel_evidence_rejection_cutoff;
