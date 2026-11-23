-- +goose Up
DROP VIEW tax_owed;
CREATE VIEW tax_owed AS
SELECT d.parcel_id,d.tax_year,SUM(d.balance_due) AS balance_due
FROM tax_delinquent_balances d
WHERE d.balance_due > 0
GROUP BY d.parcel_id,d.tax_year;
DROP INDEX IF EXISTS parcels_likely_vacant_idx;
DROP INDEX IF EXISTS parcels_discovery_id_idx;
CREATE INDEX parcels_discovery_id_idx ON parcels(county_id,source_name,id) INCLUDE(mapped_acres);
ALTER TABLE parcels DROP COLUMN likely_vacant;
-- +goose Down
ALTER TABLE parcels ADD COLUMN likely_vacant BOOLEAN NOT NULL DEFAULT false;
DROP INDEX parcels_discovery_id_idx;
CREATE INDEX parcels_discovery_id_idx ON parcels(county_id,source_name,id) INCLUDE(mapped_acres,likely_vacant);
CREATE INDEX parcels_likely_vacant_idx ON parcels(county_id,likely_vacant);
DROP VIEW tax_owed;
CREATE VIEW tax_owed AS
SELECT d.parcel_id,d.tax_year,SUM(d.balance_due) AS balance_due
FROM tax_delinquent_balances d JOIN parcels p ON p.id=d.parcel_id
WHERE p.likely_vacant AND d.balance_due>0
GROUP BY d.parcel_id,d.tax_year;
