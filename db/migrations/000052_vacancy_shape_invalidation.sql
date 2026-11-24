-- +goose Up
-- A corrected replay may reuse a generation ID. Never retain shapes from its
-- prior normalized rows after invalidating the review and parcel assessments.
-- +goose StatementBegin
CREATE FUNCTION vacancy_invalidate_changed_shapes() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.normalized_sha256 IS DISTINCT FROM NEW.normalized_sha256
  OR OLD.normalizer_version IS DISTINCT FROM NEW.normalizer_version
  OR OLD.observed_count IS DISTINCT FROM NEW.observed_count THEN
  DELETE FROM vacancy_live_shapes WHERE generation_id=OLD.id;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_invalidate_changed_shapes_trigger BEFORE UPDATE ON vacancy_live_generations
 FOR EACH ROW EXECUTE FUNCTION vacancy_invalidate_changed_shapes();
-- +goose Down
DROP TRIGGER vacancy_invalidate_changed_shapes_trigger ON vacancy_live_generations;
DROP FUNCTION vacancy_invalidate_changed_shapes;
