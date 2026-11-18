-- +goose Up
CREATE INDEX parcels_source_update_stamp_idx ON parcels (county_id, source_name, updated_at DESC);

-- +goose Down
DROP INDEX parcels_source_update_stamp_idx;
