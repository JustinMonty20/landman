-- +goose Up
ALTER TABLE parcels DROP CONSTRAINT parcels_year_built_check;
ALTER TABLE parcels ADD CONSTRAINT parcels_year_built_check
    CHECK (year_built IS NULL OR year_built = 0 OR year_built BETWEEN 1600 AND 2200);

-- +goose Down
UPDATE parcels SET year_built = NULL WHERE year_built = 0;
ALTER TABLE parcels DROP CONSTRAINT parcels_year_built_check;
ALTER TABLE parcels ADD CONSTRAINT parcels_year_built_check
    CHECK (year_built IS NULL OR year_built BETWEEN 1600 AND 2200);
