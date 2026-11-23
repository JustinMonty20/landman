-- +goose Up
-- All Union parcel identities and shapes share one transactional revision. Existing
-- assessments lack a revision and remain unknown until the next publication.
CREATE TABLE vacancy_parcel_topology (
    county_id TEXT PRIMARY KEY,
    revision BIGINT NOT NULL
);
INSERT INTO vacancy_parcel_topology(county_id,revision) VALUES ('union',0);
ALTER TABLE vacancy_assessments ADD COLUMN topology_revision BIGINT;

-- +goose StatementBegin
CREATE FUNCTION bump_vacancy_parcel_topology() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (TG_OP = 'INSERT' AND NEW.county_id = 'union' AND NEW.source_name = 'union_county_gis')
       OR (TG_OP = 'DELETE' AND OLD.county_id = 'union' AND OLD.source_name = 'union_county_gis')
       OR (TG_OP = 'UPDATE' AND (OLD.county_id,OLD.source_name,OLD.source_parcel_id,OLD.shape)
           IS DISTINCT FROM (NEW.county_id,NEW.source_name,NEW.source_parcel_id,NEW.shape)
           AND ((OLD.county_id = 'union' AND OLD.source_name = 'union_county_gis')
             OR (NEW.county_id = 'union' AND NEW.source_name = 'union_county_gis'))) THEN
        UPDATE vacancy_parcel_topology SET revision=revision+1 WHERE county_id='union';
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_parcel_topology_change AFTER INSERT OR DELETE OR UPDATE OF county_id,source_name,source_parcel_id,shape
ON parcels FOR EACH ROW EXECUTE FUNCTION bump_vacancy_parcel_topology();

-- +goose Down
DROP TRIGGER vacancy_parcel_topology_change ON parcels;
DROP FUNCTION bump_vacancy_parcel_topology();
ALTER TABLE vacancy_assessments DROP COLUMN topology_revision;
DROP TABLE vacancy_parcel_topology;
