-- +goose Up
COMMENT ON COLUMN parcels.shape IS 'Geometry column. 3857 is the storage SRID. Writers transform into it before insert. source_srid is the native code.';
COMMENT ON COLUMN parcels.srid IS 'Storage SRID of parcels.shape. 3857 is the storage SRID. Writers transform into it. source_srid is the native code.';

-- +goose Down
COMMENT ON COLUMN parcels.shape IS NULL;
COMMENT ON COLUMN parcels.srid IS NULL;
