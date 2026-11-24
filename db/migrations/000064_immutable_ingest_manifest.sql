-- +goose Up
-- A published parser proof is valid only if the sealed capture bytes cannot be
-- rewritten after verification. Publication may still advance published_at.
-- +goose StatementBegin
CREATE FUNCTION refuse_ingest_manifest_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.manifest IS DISTINCT FROM NEW.manifest OR OLD.manifest_sha256 IS DISTINCT FROM NEW.manifest_sha256 THEN
  RAISE EXCEPTION 'sealed capture manifest and digest are immutable';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER ingest_manifest_immutable BEFORE UPDATE OF manifest,manifest_sha256 ON ingest_captures
FOR EACH ROW EXECUTE FUNCTION refuse_ingest_manifest_rewrite();
-- +goose Down
DROP TRIGGER ingest_manifest_immutable ON ingest_captures;
DROP FUNCTION refuse_ingest_manifest_rewrite();
