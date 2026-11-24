-- +goose Up
-- Only a successful count-first parser run can issue an admission proof.
-- Old weak approvals are retained for audit, but no longer qualify for publication.
ALTER TABLE vacancy_live_generations ADD COLUMN parser_admission_verified boolean NOT NULL DEFAULT false;
ALTER TABLE vacancy_live_validation DISABLE TRIGGER vacancy_admit_capture_receipts_trigger;
ALTER TABLE vacancy_live_validation DISABLE TRIGGER vacancy_validate_live_identity_trigger;
UPDATE vacancy_live_validation SET capture_contract_version=0 WHERE capture_contract_version=1;
ALTER TABLE vacancy_live_validation ENABLE TRIGGER vacancy_validate_live_identity_trigger;
ALTER TABLE vacancy_live_validation ENABLE TRIGGER vacancy_admit_capture_receipts_trigger;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION vacancy_admit_capture_receipts() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m jsonb; n integer; observed integer; root text := 'OBJECTID+%3E%3D+0+AND+OBJECTID+%3C%3D+9223372036854775807';
BEGIN
 IF TG_OP='UPDATE' THEN
  RAISE EXCEPTION 'vacancy validation is immutable; withdraw and re-admit after invalidating assessments';
 END IF;
 SELECT convert_from(c.manifest,'UTF8')::jsonb,g.observed_count INTO m,observed
 FROM ingest_captures c JOIN vacancy_live_generations g ON g.capture_id=c.id
 WHERE c.id=NEW.capture_id AND g.id=NEW.generation_id AND c.source_name=NEW.source_name
 AND c.published_at IS NOT NULL AND g.source_name=NEW.source_name;
 IF m IS NULL OR m->>'source' IS DISTINCT FROM NEW.source_name OR m->>'kind' IS DISTINCT FROM 'full_scan'
 OR (m->>'count')::integer IS DISTINCT FROM observed OR jsonb_typeof(m->'receipts') IS DISTINCT FROM 'array' THEN
  RAISE EXCEPTION 'vacancy admission requires current capture protocol and complete live HTTP receipts';
 END IF;
 n := jsonb_array_length(m->'receipts');
 IF NEW.source_name='union_assessed_value_0' THEN
  IF NOT (SELECT parser_admission_verified FROM vacancy_live_generations WHERE id=NEW.generation_id)
   OR n < 3 OR n > 2048 OR observed < 0 OR observed > 500000
   OR m->'receipts'->0->>'request' IS DISTINCT FROM NEW.source_url||'?f=json&returnCountOnly=true&where=1%3D1'
   OR m->'receipts'->1->>'request' IS DISTINCT FROM NEW.source_url||'?f=json&returnCountOnly=true&where='||root
   OR m->'receipts'->(n-1)->>'request' IS DISTINCT FROM m->'receipts'->0->>'request'
   OR (observed <= 2000 AND (n <> CASE WHEN observed=0 THEN 3 ELSE 5 END
      OR (observed>0 AND (m->'receipts'->2->>'request' IS DISTINCT FROM NEW.source_url||'?f=json&outFields=%2A&resultRecordCount=2000&returnGeometry=false&where='||root
       OR m->'receipts'->3->>'request' IS DISTINCT FROM m->'receipts'->1->>'request')))) THEN
   RAISE EXCEPTION 'vacancy admission requires current capture protocol and complete live HTTP receipts';
  END IF;
 ELSIF NEW.source_name='union_structures_171' THEN
  IF n < 5 OR (m->'receipts'->1->>'request') NOT LIKE '%returnIdsOnly=true%'
   OR (m->'receipts'->(n-2)->>'request') NOT LIKE '%returnIdsOnly=true%'
   OR (m->'receipts'->(n-1)->>'request') NOT LIKE '%returnCountOnly=true%' THEN
   RAISE EXCEPTION 'vacancy admission requires current capture protocol and complete live HTTP receipts';
  END IF;
 ELSE
  RAISE EXCEPTION 'vacancy admission requires current capture protocol and complete live HTTP receipts';
 END IF;
 IF EXISTS (SELECT 1 FROM jsonb_array_elements(m->'receipts') r WHERE
   coalesce((r->>'http_status')::integer,0)<>200 OR split_part(coalesce(r->>'request',''), '?', 1)<>NEW.source_url
   OR coalesce((r->>'response_complete')::boolean,false)=false
   OR (NEW.source_name='union_assessed_value_0' AND r->>'revision' IS DISTINCT FROM 'assessment-count-first-v1')
   OR (NEW.source_name='union_structures_171' AND coalesce(r->>'revision','')<>'')) THEN
  RAISE EXCEPTION 'vacancy admission requires current capture protocol and complete live HTTP receipts';
 END IF;
 IF NEW.effective_av_year IS NOT NULL AND NEW.effective_av_year > LEAST(EXTRACT(YEAR FROM NEW.source_as_of)::integer, EXTRACT(YEAR FROM NEW.reviewed_at)::integer) THEN
  RAISE EXCEPTION 'future assessment year cannot be admitted';
 END IF;
 NEW.capture_contract_version := 1;
 RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'parser-issued admission cannot be downgraded'; END $$;
