-- +goose Up
ALTER TABLE parcels ADD COLUMN likely_vacant BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX parcels_likely_vacant_idx ON parcels (county_id, likely_vacant);

-- +goose Down
DROP INDEX IF EXISTS parcels_likely_vacant_idx;
ALTER TABLE parcels DROP COLUMN likely_vacant;
