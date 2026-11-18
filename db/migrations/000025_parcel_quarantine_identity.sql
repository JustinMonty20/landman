-- +goose Up
ALTER TABLE quarantined_records ADD COLUMN source_parcel_id TEXT;
-- +goose Down
ALTER TABLE quarantined_records DROP COLUMN source_parcel_id;
