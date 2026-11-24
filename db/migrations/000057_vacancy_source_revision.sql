-- +goose Up
-- A fixed-size transactional cursor token for serving facts and evidence membership.
CREATE TABLE vacancy_source_revision (county_id text PRIMARY KEY, revision bigint NOT NULL);
INSERT INTO vacancy_source_revision VALUES ('union', 0);
-- +goose StatementBegin
CREATE FUNCTION bump_vacancy_source_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE changed boolean;
BEGIN
 IF TG_OP='INSERT' THEN
  SELECT EXISTS(SELECT 1 FROM inserted WHERE county_id='union' AND source_name='union_county_gis') INTO changed;
 ELSIF TG_OP='DELETE' THEN
  SELECT EXISTS(SELECT 1 FROM deleted WHERE county_id='union' AND source_name='union_county_gis') INTO changed;
 ELSE
  SELECT EXISTS(SELECT 1 FROM inserted n FULL JOIN deleted o USING (id)
   WHERE ((n.county_id='union' AND n.source_name='union_county_gis') OR (o.county_id='union' AND o.source_name='union_county_gis'))
   AND to_jsonb(n) IS DISTINCT FROM to_jsonb(o)) INTO changed;
 END IF;
 IF changed AND current_setting('landman.vacancy_revision_bumped',true) IS DISTINCT FROM '1' THEN
  UPDATE vacancy_source_revision SET revision=revision+1 WHERE county_id='union';
  PERFORM set_config('landman.vacancy_revision_bumped','1',true);
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_source_insert AFTER INSERT ON parcels REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_source_revision();
CREATE TRIGGER vacancy_source_delete AFTER DELETE ON parcels REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_source_revision();
CREATE TRIGGER vacancy_source_update AFTER UPDATE ON parcels REFERENCING OLD TABLE AS deleted NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_source_revision();
-- +goose StatementBegin
CREATE FUNCTION bump_vacancy_evidence_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE changed boolean;
BEGIN
 IF TG_OP='INSERT' THEN
  SELECT EXISTS(SELECT 1 FROM inserted WHERE county_id='union' AND source_name='union_county_gis') INTO changed;
 ELSIF TG_OP='DELETE' THEN
  SELECT EXISTS(SELECT 1 FROM deleted WHERE county_id='union' AND source_name='union_county_gis') INTO changed;
 ELSE
  SELECT EXISTS(SELECT 1 FROM inserted n FULL JOIN deleted o USING (county_id,source_name,source_parcel_id)
    WHERE (n.county_id='union' AND n.source_name='union_county_gis' OR o.county_id='union' AND o.source_name='union_county_gis')
    AND to_jsonb(n) IS DISTINCT FROM to_jsonb(o)) INTO changed;
 END IF;
 IF changed AND current_setting('landman.vacancy_revision_bumped',true) IS DISTINCT FROM '1' THEN
  UPDATE vacancy_source_revision SET revision=revision+1 WHERE county_id='union';
  PERFORM set_config('landman.vacancy_revision_bumped','1',true);
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_evidence_insert AFTER INSERT ON parcel_evidence_current REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_evidence_revision();
CREATE TRIGGER vacancy_evidence_delete AFTER DELETE ON parcel_evidence_current REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_evidence_revision();
CREATE TRIGGER vacancy_evidence_update AFTER UPDATE ON parcel_evidence_current REFERENCING OLD TABLE AS deleted NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_evidence_revision();
-- +goose Down
DROP TRIGGER vacancy_evidence_update ON parcel_evidence_current;
DROP TRIGGER vacancy_evidence_delete ON parcel_evidence_current;
DROP TRIGGER vacancy_evidence_insert ON parcel_evidence_current;
DROP FUNCTION bump_vacancy_evidence_revision();
DROP TRIGGER vacancy_source_update ON parcels;
DROP TRIGGER vacancy_source_delete ON parcels;
DROP TRIGGER vacancy_source_insert ON parcels;
DROP FUNCTION bump_vacancy_source_revision();
DROP TABLE vacancy_source_revision;
