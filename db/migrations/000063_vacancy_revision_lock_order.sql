-- +goose Up
-- Take the shared revision lock before either table can lock an individual
-- serving or evidence row. A row-level geometry trigger is too late: a direct
-- shape edit would already hold the parcel tuple while awaiting source revision.
-- +goose StatementBegin
CREATE FUNCTION lock_vacancy_source_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM 1 FROM vacancy_source_revision WHERE county_id='union' FOR UPDATE;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_parcel_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON parcels
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
CREATE TRIGGER vacancy_evidence_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON parcel_evidence_current
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
-- +goose Down
DROP TRIGGER vacancy_evidence_revision_lock ON parcel_evidence_current;
DROP TRIGGER vacancy_parcel_revision_lock ON parcels;
DROP FUNCTION lock_vacancy_source_revision();
