-- +goose Up
ALTER TABLE soil_polygon_parcels ALTER COLUMN overlap_acres TYPE NUMERIC;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM soil_polygon_parcels WHERE overlap_acres <> round(overlap_acres, 6) OR overlap_acres > 99999999.999999) THEN
        RAISE EXCEPTION 'cannot restore six-decimal overlap acres without losing stored values';
    END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE soil_polygon_parcels ALTER COLUMN overlap_acres TYPE NUMERIC(14, 6);
