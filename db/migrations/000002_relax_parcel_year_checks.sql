-- +goose Up
ALTER TABLE parcels
    DROP CONSTRAINT parcels_parcel_year_check,
    ADD CONSTRAINT parcels_parcel_year_check CHECK (parcel_year IS NULL OR parcel_year BETWEEN 0 AND 2200),
    DROP CONSTRAINT parcels_year_built_check,
    ADD CONSTRAINT parcels_year_built_check CHECK (year_built IS NULL OR year_built BETWEEN 0 AND 2200);

-- +goose Down
ALTER TABLE parcels
    DROP CONSTRAINT parcels_parcel_year_check,
    ADD CONSTRAINT parcels_parcel_year_check CHECK (parcel_year IS NULL OR parcel_year BETWEEN 1900 AND 2200),
    DROP CONSTRAINT parcels_year_built_check,
    ADD CONSTRAINT parcels_year_built_check CHECK (year_built IS NULL OR year_built BETWEEN 1600 AND 2200);
