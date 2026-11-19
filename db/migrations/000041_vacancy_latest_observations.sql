-- +goose Up
CREATE TABLE vacancy_latest_observations (
 source_name TEXT PRIMARY KEY CHECK (source_name IN ('union_structures_171', 'union_assessed_value_0')),
 capture_id UUID NOT NULL REFERENCES ingest_captures(id),
 observed_at TIMESTAMPTZ NOT NULL
);
INSERT INTO vacancy_latest_observations(source_name,capture_id,observed_at)
SELECT DISTINCT ON (c.source_name) c.source_name,c.id,c.observed_at
FROM ingest_captures c
JOIN job_attempts a ON a.id=c.attempt_id
WHERE c.source_name IN ('union_structures_171', 'union_assessed_value_0')
  AND a.outcome='success' AND a.attempt_kind='full_scan'
ORDER BY c.source_name,c.observed_at DESC,c.id DESC;
-- +goose Down
DROP TABLE vacancy_latest_observations;
