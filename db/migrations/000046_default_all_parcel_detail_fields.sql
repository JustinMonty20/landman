-- +goose Up
ALTER TABLE app_users ALTER COLUMN parcel_detail_fields SET DEFAULT ARRAY[
    'mapped_acres', 'owner_name', 'entity_indicator', 'sale_count',
    'delinquent_total', 'tax_bill_count', 'foreclosure_count', 'vacancy_tier', 'vacancy_reason',
    'soil_acres', 'soil_septic', 'soil_dwelling_without', 'soil_dwelling_with',
    'zoning_district', 'zoning_jurisdiction', 'zoning_unmapped',
    'road_distance', 'road_name', 'road_status', 'flood_unknown', 'floodway_acres', 'flood_zones'
]::text[];

-- +goose Down
ALTER TABLE app_users ALTER COLUMN parcel_detail_fields SET DEFAULT ARRAY['mapped_acres', 'soil_septic']::text[];
