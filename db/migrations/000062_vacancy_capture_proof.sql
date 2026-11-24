-- +goose Up
-- A parser result is a proof of one exact capture manifest, not a transferable flag.
ALTER TABLE vacancy_live_generations ADD COLUMN parser_manifest_sha256 bytea;
-- +goose StatementBegin
CREATE FUNCTION vacancy_check_parser_manifest() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
IF NEW.source_name='union_assessed_value_0' AND
 NOT EXISTS (SELECT 1 FROM vacancy_live_generations g JOIN ingest_captures c ON c.id=g.capture_id
 WHERE g.id=NEW.generation_id AND g.parser_admission_verified
 AND g.parser_manifest_sha256 IS NOT NULL AND c.manifest_sha256=g.parser_manifest_sha256
 AND digest(c.manifest,'sha256')=c.manifest_sha256) THEN
 RAISE EXCEPTION 'assessment admission requires parser proof bound to the exact retained capture';
END IF;
RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_check_parser_manifest_trigger BEFORE INSERT ON vacancy_live_validation
FOR EACH ROW EXECUTE FUNCTION vacancy_check_parser_manifest();
-- +goose Down
DROP TRIGGER vacancy_check_parser_manifest_trigger ON vacancy_live_validation;
DROP FUNCTION vacancy_check_parser_manifest();
ALTER TABLE vacancy_live_generations DROP COLUMN parser_manifest_sha256;
