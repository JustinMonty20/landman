-- +goose Up
ALTER TABLE qualification_layer_receipts ADD COLUMN missing_geometry_count INTEGER NOT NULL DEFAULT 0
    CHECK (missing_geometry_count BETWEEN 0 AND feature_count);

ALTER TABLE qualification_zones
    ALTER COLUMN shape DROP NOT NULL,
    ALTER COLUMN raw_shape DROP NOT NULL,
    DROP CONSTRAINT qualification_zones_repair_status_check,
    ADD CONSTRAINT qualification_zones_repair_status_check CHECK (
        (repair_status = 'missing_geometry' AND shape IS NULL AND raw_shape IS NULL AND repair_delta_m2 = 0)
        OR (repair_status IN ('original', 'repaired') AND shape IS NOT NULL AND raw_shape IS NOT NULL)
    );

ALTER TABLE qualification_jurisdictions
    ALTER COLUMN shape DROP NOT NULL,
    ALTER COLUMN raw_shape DROP NOT NULL,
    DROP CONSTRAINT qualification_jurisdictions_repair_status_check,
    ADD CONSTRAINT qualification_jurisdictions_repair_status_check CHECK (
        (repair_status = 'missing_geometry' AND shape IS NULL AND raw_shape IS NULL AND repair_delta_m2 = 0)
        OR (repair_status IN ('original', 'repaired') AND shape IS NOT NULL AND raw_shape IS NOT NULL)
    );

-- +goose Down
-- Do not delete retained records to restore the non-null constraints.
ALTER TABLE qualification_jurisdictions
    ALTER COLUMN shape SET NOT NULL,
    ALTER COLUMN raw_shape SET NOT NULL,
    DROP CONSTRAINT qualification_jurisdictions_repair_status_check,
    ADD CONSTRAINT qualification_jurisdictions_repair_status_check CHECK (repair_status IN ('original', 'repaired'));

ALTER TABLE qualification_zones
    ALTER COLUMN shape SET NOT NULL,
    ALTER COLUMN raw_shape SET NOT NULL,
    DROP CONSTRAINT qualification_zones_repair_status_check,
    ADD CONSTRAINT qualification_zones_repair_status_check CHECK (repair_status IN ('original', 'repaired'));

ALTER TABLE qualification_layer_receipts DROP COLUMN missing_geometry_count;
