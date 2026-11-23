-- +goose Up
-- Existing observations cannot be asserted to match a parcel row by a
-- migration. The next accepted parcel publication records its own row version.
ALTER TABLE parcel_evidence_current ADD COLUMN parcel_row_version TEXT;
-- +goose Down
ALTER TABLE parcel_evidence_current DROP COLUMN parcel_row_version;
