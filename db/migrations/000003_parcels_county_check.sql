-- +goose Up
ALTER TABLE parcels
    ADD CONSTRAINT parcels_county_id_check CHECK (county_id IN ('union'));

-- +goose Down
ALTER TABLE parcels DROP CONSTRAINT parcels_county_id_check;
