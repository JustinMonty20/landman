-- +goose Up
ALTER TABLE app_users ADD COLUMN parcel_detail_groups text[] NOT NULL DEFAULT ARRAY['acreage','soil']::text[]
  CHECK (parcel_detail_groups <@ ARRAY['acreage','ownership','tax','vacancy','soil','zoning','roads','flood']::text[]
    AND array_position(parcel_detail_groups, NULL) IS NULL);
-- +goose Down
ALTER TABLE app_users DROP COLUMN parcel_detail_groups;
