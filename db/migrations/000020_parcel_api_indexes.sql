-- +goose Up
CREATE INDEX parcels_discovery_id_idx ON parcels (county_id, source_name, id) INCLUDE (mapped_acres, likely_vacant);
CREATE INDEX parcels_address_prefix_idx ON parcels (county_id, source_name, lower(left(situs_line_1, 80)) text_pattern_ops);
CREATE INDEX parcels_number_prefix_idx ON parcels (county_id, source_name, lower(left(parcel_number, 80)) text_pattern_ops);
CREATE INDEX tax_foreclosure_parcels_parcel_idx ON tax_foreclosure_parcels (parcel_id, case_id);
-- +goose Down
DROP INDEX tax_foreclosure_parcels_parcel_idx;
DROP INDEX parcels_number_prefix_idx;
DROP INDEX parcels_address_prefix_idx;
DROP INDEX parcels_discovery_id_idx;
