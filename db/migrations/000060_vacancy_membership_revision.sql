-- +goose Up
-- Publication and validation can change vacancy membership without writing parcels.
-- Each statement bumps the same fixed-size cursor revision at most once.
-- +goose StatementBegin
CREATE FUNCTION bump_vacancy_assessment_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE changed boolean;
BEGIN
 IF TG_OP='INSERT' THEN
  SELECT EXISTS(SELECT 1 FROM inserted) INTO changed;
 ELSIF TG_OP='DELETE' THEN
  SELECT EXISTS(SELECT 1 FROM deleted) INTO changed;
 ELSE
  SELECT EXISTS(SELECT 1 FROM inserted n FULL JOIN deleted o USING (parcel_id)
    WHERE to_jsonb(n) IS DISTINCT FROM to_jsonb(o)) INTO changed;
 END IF;
 IF changed AND current_setting('landman.vacancy_revision_bumped',true) IS DISTINCT FROM '1' THEN
  UPDATE vacancy_source_revision SET revision=revision+1 WHERE county_id='union';
  PERFORM set_config('landman.vacancy_revision_bumped','1',true);
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_assessment_insert_revision AFTER INSERT ON vacancy_assessments REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_assessment_revision();
CREATE TRIGGER vacancy_assessment_delete_revision AFTER DELETE ON vacancy_assessments REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_assessment_revision();
CREATE TRIGGER vacancy_assessment_update_revision AFTER UPDATE ON vacancy_assessments REFERENCING OLD TABLE AS deleted NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_assessment_revision();
-- +goose StatementBegin
CREATE FUNCTION bump_vacancy_validation_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE changed boolean := false;
BEGIN
 IF TG_OP='INSERT' THEN
  SELECT EXISTS(SELECT 1 FROM inserted) INTO changed;
 ELSE
  SELECT EXISTS(SELECT 1 FROM deleted) INTO changed;
 END IF;
 IF changed AND current_setting('landman.vacancy_revision_bumped',true) IS DISTINCT FROM '1' THEN
  UPDATE vacancy_source_revision SET revision=revision+1 WHERE county_id='union';
  PERFORM set_config('landman.vacancy_revision_bumped','1',true);
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_validation_insert_revision AFTER INSERT ON vacancy_live_validation REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_validation_revision();
CREATE TRIGGER vacancy_validation_delete_revision AFTER DELETE ON vacancy_live_validation REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION bump_vacancy_validation_revision();
-- +goose Down
DROP TRIGGER vacancy_validation_delete_revision ON vacancy_live_validation;
DROP TRIGGER vacancy_validation_insert_revision ON vacancy_live_validation;
DROP FUNCTION bump_vacancy_validation_revision();
DROP TRIGGER vacancy_assessment_update_revision ON vacancy_assessments;
DROP TRIGGER vacancy_assessment_delete_revision ON vacancy_assessments;
DROP TRIGGER vacancy_assessment_insert_revision ON vacancy_assessments;
DROP FUNCTION bump_vacancy_assessment_revision();
