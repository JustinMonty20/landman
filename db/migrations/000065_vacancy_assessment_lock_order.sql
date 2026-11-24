-- +goose Up
-- All writers that can lock an assessment tuple or its FK parent first take
-- the revision lock, including direct SQL writes and parcel FK cascades.
CREATE TRIGGER vacancy_assessments_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_assessments
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
CREATE TRIGGER vacancy_validation_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_live_validation
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
CREATE TRIGGER vacancy_generations_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_evidence_generations
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
CREATE TRIGGER vacancy_structures_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_structures
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
CREATE TRIGGER vacancy_rows_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_assessment_rows
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
-- Updating a live generation can invalidate assessments from its row trigger.
CREATE TRIGGER vacancy_live_generations_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_live_generations
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
CREATE TRIGGER vacancy_live_structures_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_live_structures
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
CREATE TRIGGER vacancy_live_assessments_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_live_assessments
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
CREATE TRIGGER vacancy_latest_revision_lock BEFORE INSERT OR UPDATE OR DELETE ON vacancy_latest_observations
FOR EACH STATEMENT EXECUTE FUNCTION lock_vacancy_source_revision();
-- +goose Down
DROP TRIGGER vacancy_latest_revision_lock ON vacancy_latest_observations;
DROP TRIGGER vacancy_live_assessments_revision_lock ON vacancy_live_assessments;
DROP TRIGGER vacancy_live_structures_revision_lock ON vacancy_live_structures;
DROP TRIGGER vacancy_live_generations_revision_lock ON vacancy_live_generations;
DROP TRIGGER vacancy_rows_revision_lock ON vacancy_assessment_rows;
DROP TRIGGER vacancy_structures_revision_lock ON vacancy_structures;
DROP TRIGGER vacancy_generations_revision_lock ON vacancy_evidence_generations;
DROP TRIGGER vacancy_validation_revision_lock ON vacancy_live_validation;
DROP TRIGGER vacancy_assessments_revision_lock ON vacancy_assessments;
