-- +goose Up
ALTER TABLE app_users ADD COLUMN parcel_detail_fields text[] NOT NULL DEFAULT ARRAY['mapped_acres','soil_septic']::text[]
  CHECK (parcel_detail_fields <@ ARRAY['mapped_acres','owner_name','entity_indicator','sale_count','delinquent_total','tax_bill_count','foreclosure_count','vacancy_tier','vacancy_reason','soil_acres','soil_septic','soil_dwelling_without','soil_dwelling_with','zoning_district','zoning_jurisdiction','zoning_unmapped','road_distance','road_name','road_status','flood_unknown','floodway_acres','flood_zones']::text[]
    AND array_position(parcel_detail_fields, NULL) IS NULL);
-- +goose Down
ALTER TABLE app_users DROP COLUMN parcel_detail_fields;
