-- +goose Up
-- Older attestations remain as evidence but cannot publish a current tier.
ALTER TABLE vacancy_live_validation ADD COLUMN capture_contract_version INTEGER NOT NULL DEFAULT 0
 CHECK (capture_contract_version IN (0,1));
-- +goose StatementBegin
CREATE FUNCTION vacancy_admit_capture_receipts() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  RAISE EXCEPTION 'vacancy validation is immutable; withdraw and re-admit after invalidating assessments';
 END IF;
 IF NOT EXISTS (SELECT 1 FROM ingest_captures c
  CROSS JOIN LATERAL (SELECT convert_from(c.manifest,'UTF8')::jsonb AS body) m
  WHERE c.id=NEW.capture_id AND c.source_name=NEW.source_name AND c.published_at IS NOT NULL
  AND m.body->>'source'=NEW.source_name AND m.body->>'kind'='full_scan'
  AND (m.body->>'count')::integer=(SELECT observed_count FROM vacancy_live_generations WHERE id=NEW.generation_id)
  AND jsonb_array_length(m.body->'receipts')>0
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(m.body->'receipts') r
   WHERE coalesce((r->>'http_status')::integer,0)<>200
   OR split_part(coalesce(r->>'request',''), '?', 1)<>NEW.source_url
   OR coalesce((r->>'response_complete')::boolean,false)=false)) THEN
  RAISE EXCEPTION 'vacancy admission requires exact-source complete live HTTP receipts';
 END IF;
 NEW.capture_contract_version := 1;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_admit_capture_receipts_trigger BEFORE INSERT OR UPDATE ON vacancy_live_validation
 FOR EACH ROW EXECUTE FUNCTION vacancy_admit_capture_receipts();
-- +goose Down
DROP TRIGGER vacancy_admit_capture_receipts_trigger ON vacancy_live_validation;
DROP FUNCTION vacancy_admit_capture_receipts;
ALTER TABLE vacancy_live_validation DROP COLUMN capture_contract_version;
