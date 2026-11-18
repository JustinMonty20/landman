-- +goose Up
ALTER TABLE qualification_assessments ADD COLUMN has_zone_conflict BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE qualification_assessments DROP COLUMN has_zone_conflict;
