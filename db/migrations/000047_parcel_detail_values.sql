-- +goose Up
ALTER TABLE app_users DROP CONSTRAINT app_users_parcel_detail_fields_check;
ALTER TABLE app_users ADD CONSTRAINT app_users_parcel_detail_fields_check CHECK (parcel_detail_fields <@ ARRAY['mapped_acres','valuation_land','valuation_improvement','valuation_total','valuation_assessed','owner_mailing','owner_name','entity_indicator','sale_count','delinquent_total','tax_bill_count','foreclosure_count','vacancy_tier','vacancy_reason','soil_acres','soil_septic','soil_dwelling_without','soil_dwelling_with','zoning_district','zoning_jurisdiction','zoning_unmapped','road_distance','road_name','road_status','flood_unknown','floodway_acres','flood_zones']::text[] AND array_position(parcel_detail_fields, NULL) IS NULL);
ALTER TABLE app_users ALTER COLUMN parcel_detail_fields SET DEFAULT ARRAY[
    'mapped_acres', 'valuation_land','valuation_improvement','valuation_total','valuation_assessed', 'owner_name', 'owner_mailing', 'entity_indicator', 'sale_count',
    'delinquent_total', 'tax_bill_count', 'foreclosure_count', 'vacancy_tier', 'vacancy_reason',
    'soil_acres', 'soil_septic', 'soil_dwelling_without', 'soil_dwelling_with',
    'zoning_district', 'zoning_jurisdiction', 'zoning_unmapped',
    'road_distance', 'road_name', 'road_status', 'flood_unknown', 'floodway_acres', 'flood_zones'
]::text[];
-- Only the exact former default is upgraded; customized and empty selections stay untouched.
UPDATE app_users SET parcel_detail_fields = ARRAY[
    'mapped_acres','valuation_land','valuation_improvement','valuation_total','valuation_assessed','owner_name','owner_mailing','entity_indicator','sale_count',
    'delinquent_total','tax_bill_count','foreclosure_count','vacancy_tier','vacancy_reason',
    'soil_acres','soil_septic','soil_dwelling_without','soil_dwelling_with',
    'zoning_district','zoning_jurisdiction','zoning_unmapped','road_distance','road_name','road_status','flood_unknown','floodway_acres','flood_zones'
]::text[] WHERE parcel_detail_fields = ARRAY[
    'mapped_acres', 'owner_name', 'entity_indicator', 'sale_count',
    'delinquent_total', 'tax_bill_count', 'foreclosure_count', 'vacancy_tier', 'vacancy_reason',
    'soil_acres', 'soil_septic', 'soil_dwelling_without', 'soil_dwelling_with',
    'zoning_district', 'zoning_jurisdiction', 'zoning_unmapped',
    'road_distance', 'road_name', 'road_status', 'flood_unknown', 'floodway_acres', 'flood_zones'
]::text[];
-- +goose Down
ALTER TABLE app_users ALTER COLUMN parcel_detail_fields SET DEFAULT ARRAY[
    'mapped_acres', 'owner_name', 'entity_indicator', 'sale_count',
    'delinquent_total', 'tax_bill_count', 'foreclosure_count', 'vacancy_tier', 'vacancy_reason',
    'soil_acres', 'soil_septic', 'soil_dwelling_without', 'soil_dwelling_with',
    'zoning_district', 'zoning_jurisdiction', 'zoning_unmapped',
    'road_distance', 'road_name', 'road_status', 'flood_unknown', 'floodway_acres', 'flood_zones'
]::text[];
UPDATE app_users SET parcel_detail_fields = array_remove(array_remove(array_remove(array_remove(array_remove(parcel_detail_fields,'valuation_land'),'valuation_improvement'),'valuation_total'),'valuation_assessed'),'owner_mailing');
ALTER TABLE app_users DROP CONSTRAINT app_users_parcel_detail_fields_check;
ALTER TABLE app_users ADD CONSTRAINT app_users_parcel_detail_fields_check CHECK (parcel_detail_fields <@ ARRAY['mapped_acres','owner_name','entity_indicator','sale_count','delinquent_total','tax_bill_count','foreclosure_count','vacancy_tier','vacancy_reason','soil_acres','soil_septic','soil_dwelling_without','soil_dwelling_with','zoning_district','zoning_jurisdiction','zoning_unmapped','road_distance','road_name','road_status','flood_unknown','floodway_acres','flood_zones']::text[] AND array_position(parcel_detail_fields, NULL) IS NULL);
