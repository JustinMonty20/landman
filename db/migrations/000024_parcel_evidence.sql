-- +goose Up
ALTER TABLE job_attempts DROP CONSTRAINT job_attempts_attempt_kind_check;
ALTER TABLE job_attempts ADD CONSTRAINT job_attempts_attempt_kind_check CHECK (attempt_kind IN ('full_scan','reprocess','replay'));

CREATE TABLE parcel_evidence_captures (
 artifact_id TEXT PRIMARY KEY,
 artifact_hash TEXT NOT NULL CHECK (artifact_hash ~ '^[0-9a-f]{64}$'),
 manifest JSONB NOT NULL,
 attempt_id UUID NOT NULL REFERENCES job_attempts(id),
 attempt_kind TEXT NOT NULL CHECK (attempt_kind IN ('full_scan','reprocess')),
 registered_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE parcel_evidence_observations (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 county_id TEXT NOT NULL DEFAULT 'union',
 source_name TEXT NOT NULL DEFAULT 'union_county_gis',
 artifact_id TEXT NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL,
 published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 attempt_kind TEXT NOT NULL CHECK (attempt_kind IN ('full_scan','reprocess')),
 manifest JSONB NOT NULL,
 publication_kind TEXT NOT NULL DEFAULT 'observed' CHECK (publication_kind IN ('observed','offline_rebuild','offline_retry')),
 attempt_id UUID REFERENCES job_attempts(id),
 UNIQUE(artifact_id)
);
CREATE TABLE parcel_evidence_current (
 county_id TEXT NOT NULL,
 source_name TEXT NOT NULL,
 source_parcel_id TEXT NOT NULL,
 observation_id UUID NOT NULL REFERENCES parcel_evidence_observations(id),
 source_hash TEXT NOT NULL CHECK (source_hash ~ '^[0-9a-f]{64}$'),
 source_hash_version TEXT NOT NULL,
 normalized_hash TEXT NOT NULL CHECK (normalized_hash ~ '^[0-9a-f]{64}$'),
 normalizer_version TEXT NOT NULL,
 normalized_hash_version TEXT NOT NULL,
 geometry_hash TEXT NOT NULL CHECK (geometry_hash ~ '^[0-9a-f]{64}$'),
 geometry_hash_version TEXT NOT NULL,
 source_features JSONB NOT NULL,
 PRIMARY KEY(county_id,source_name,source_parcel_id)
);
CREATE TABLE parcel_evidence_history (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 county_id TEXT NOT NULL,
 source_name TEXT NOT NULL,
 source_parcel_id TEXT NOT NULL,
 old_observation_id UUID REFERENCES parcel_evidence_observations(id),
 new_observation_id UUID NOT NULL REFERENCES parcel_evidence_observations(id),
 cause TEXT NOT NULL CHECK (cause IN ('source_observation','normalization_correction','offline_rebuild','offline_retry')),
 attempt_id UUID REFERENCES job_attempts(id),
 normalized_hash_version TEXT NOT NULL,
 old_normalized_hash_version TEXT,
 geometry_hash_version TEXT NOT NULL,
 old_geometry_hash_version TEXT,
 source_hash_version TEXT NOT NULL,
 old_source_hash_version TEXT,
 normalizer_version TEXT NOT NULL,
 old_normalizer_version TEXT,
 old_source_hash TEXT,
 new_source_hash TEXT NOT NULL,
 old_normalized_hash TEXT,
 new_normalized_hash TEXT NOT NULL,
 old_geometry_hash TEXT,
 new_geometry_hash TEXT NOT NULL,
 old_fields JSONB,
 new_fields JSONB NOT NULL,
 changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX parcel_evidence_history_pid_idx ON parcel_evidence_history(county_id,source_name,source_parcel_id,changed_at);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM parcel_evidence_observations) OR EXISTS (SELECT 1 FROM parcel_evidence_captures) THEN
  RAISE EXCEPTION 'cannot discard registered parcel capture or published source evidence';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE parcel_evidence_history;
DROP TABLE parcel_evidence_current;
DROP TABLE parcel_evidence_observations;
DROP TABLE parcel_evidence_captures;
ALTER TABLE job_attempts DROP CONSTRAINT job_attempts_attempt_kind_check;
ALTER TABLE job_attempts ADD CONSTRAINT job_attempts_attempt_kind_check CHECK (attempt_kind IN ('full_scan','reprocess'));
