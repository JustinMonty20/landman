-- +goose Up
-- Receipt layout is a necessary protocol check, not a coverage attestation.
-- The operator must still review terms, snapshot, membership, and effective year.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION vacancy_admit_capture_receipts() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  RAISE EXCEPTION 'vacancy validation is immutable; withdraw and re-admit after invalidating assessments';
 END IF;
 IF NOT EXISTS (SELECT 1 FROM ingest_captures c
 CROSS JOIN LATERAL (SELECT convert_from(c.manifest,'UTF8')::jsonb AS body) m
 WHERE c.id=NEW.capture_id AND c.source_name=NEW.source_name AND c.published_at IS NOT NULL
 AND m.body->>'source'=NEW.source_name AND m.body->>'kind'='full_scan'
 AND (m.body->>'count')::integer=(SELECT observed_count FROM vacancy_live_generations WHERE id=NEW.generation_id)
 AND ((NEW.source_name='union_structures_171'
   AND jsonb_array_length(m.body->'receipts')>=5
   AND (m.body->'receipts'->1->>'request') LIKE '%returnIdsOnly=true%'
   AND (m.body->'receipts'->(jsonb_array_length(m.body->'receipts')-2)->>'request') LIKE '%returnIdsOnly=true%'
   AND (m.body->'receipts'->(jsonb_array_length(m.body->'receipts')-1)->>'request') LIKE '%returnCountOnly=true%'
   AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(m.body->'receipts') r WHERE coalesce(r->>'revision','')<>''))
 OR (NEW.source_name='union_assessed_value_0'
   AND jsonb_array_length(m.body->'receipts')>=3
   AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(m.body->'receipts') r
   WHERE r->>'revision' IS DISTINCT FROM 'assessment-count-first-v1')))
 AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(m.body->'receipts') r
 WHERE coalesce((r->>'http_status')::integer,0)<>200
 OR split_part(coalesce(r->>'request',''), '?', 1)<>NEW.source_url
 OR coalesce((r->>'response_complete')::boolean,false)=false)) THEN
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
-- Refuse a rollback that would silently re-enable bounded legacy admission.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'vacancy admission protocol cannot be downgraded'; END $$;
-- +goose StatementEnd
