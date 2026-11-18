-- +goose Up
CREATE TABLE parcel_derived_handoff (
  artifact_id TEXT PRIMARY KEY REFERENCES parcel_evidence_observations(artifact_id),
  requested_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
request_generation BIGINT NOT NULL DEFAULT 1,
  screening_pending BOOLEAN NOT NULL DEFAULT true,
  qualification_pending BOOLEAN NOT NULL DEFAULT true
);
-- +goose Down
DROP TABLE parcel_derived_handoff;
