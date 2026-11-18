-- +goose Up
ALTER TABLE qualification_assessments ADD COLUMN has_zone_conflict BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE qualification_generations ADD COLUMN started_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE qualification_generations SET started_at=fetched_at;

-- +goose Down
ALTER TABLE qualification_generations DROP COLUMN started_at;
ALTER TABLE qualification_assessments DROP COLUMN has_zone_conflict;
