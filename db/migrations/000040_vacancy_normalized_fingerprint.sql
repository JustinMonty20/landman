-- +goose Up
ALTER TABLE vacancy_live_generations ADD COLUMN normalized_sha256 TEXT NOT NULL DEFAULT '', ADD COLUMN normalizer_version TEXT NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE vacancy_live_generations DROP COLUMN normalized_sha256, DROP COLUMN normalizer_version;
