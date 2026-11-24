-- +goose Up
-- Admission records operator-reviewed evidence for exact live generations. Fixture
-- generations have a different table and cannot satisfy either foreign key.
CREATE TABLE vacancy_live_validation (
 generation_id UUID PRIMARY KEY REFERENCES vacancy_live_generations(id),
 capture_id UUID NOT NULL REFERENCES ingest_captures(id),
 source_name TEXT NOT NULL CHECK (source_name IN ('union_structures_171','union_assessed_value_0')),
 normalized_sha256 TEXT NOT NULL CHECK (normalized_sha256 ~ '^[0-9a-f]{64}$'),
 source_url TEXT NOT NULL CHECK (source_url LIKE 'https://%'),
 terms_evidence TEXT NOT NULL CHECK (length(trim(terms_evidence)) > 0),
 coverage_evidence TEXT NOT NULL CHECK (length(trim(coverage_evidence)) > 0),
 count_evidence TEXT NOT NULL CHECK (length(trim(count_evidence)) > 0),
 snapshot_evidence TEXT NOT NULL CHECK (length(trim(snapshot_evidence)) > 0),
 source_as_of DATE NOT NULL,
 reviewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 status_evidence TEXT,
 effective_av_year INTEGER CHECK (effective_av_year BETWEEN 1900 AND 9999),
 CHECK ((source_name='union_structures_171' AND status_evidence IS NOT NULL AND length(trim(status_evidence)) > 0 AND effective_av_year IS NULL)
     OR (source_name='union_assessed_value_0' AND status_evidence IS NULL AND effective_av_year IS NOT NULL))
);
-- A generation may not be attested as another source or as another capture.
-- +goose StatementBegin
CREATE FUNCTION vacancy_validate_live_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM vacancy_live_generations g WHERE g.id=NEW.generation_id
  AND g.capture_id=NEW.capture_id AND g.source_name=NEW.source_name
  AND g.normalized_sha256=NEW.normalized_sha256 AND g.observed_count>0
  AND NEW.source_as_of <= g.retrieved_at::date AND NEW.reviewed_at >= g.retrieved_at) THEN
  RAISE EXCEPTION 'vacancy validation must bind an unchanged live generation';
 END IF;
 IF NOT EXISTS (SELECT 1 FROM vacancy_latest_observations h WHERE h.source_name=NEW.source_name AND h.capture_id=NEW.capture_id) THEN
  RAISE EXCEPTION 'vacancy validation requires latest accepted capture';
 END IF;
 IF NOT EXISTS (SELECT 1 FROM ingest_captures c
  CROSS JOIN LATERAL (SELECT convert_from(c.manifest,'UTF8')::jsonb AS body) m
  WHERE c.id=NEW.capture_id AND c.source_name=NEW.source_name AND c.published_at IS NOT NULL
  AND m.body->>'source'=NEW.source_name
  AND (m.body->>'count')::integer=(SELECT observed_count FROM vacancy_live_generations WHERE id=NEW.generation_id)
  AND jsonb_array_length(m.body->'receipts')>0
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(m.body->'receipts') r
   WHERE coalesce((r->>'http_status')::integer,0)<>200
   OR coalesce((r->>'response_complete')::boolean,false)=false)) THEN
  RAISE EXCEPTION 'vacancy validation requires published complete live HTTP receipts';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_validate_live_identity_trigger BEFORE INSERT OR UPDATE ON vacancy_live_validation
 FOR EACH ROW EXECUTE FUNCTION vacancy_validate_live_identity();
-- A corrected replay may rewrite rows under the same generation ID. It must
-- withdraw the prior attestation and every derived result in the same transaction.
-- +goose StatementBegin
CREATE FUNCTION vacancy_invalidate_changed_generation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.normalized_sha256 IS DISTINCT FROM NEW.normalized_sha256
  OR OLD.normalizer_version IS DISTINCT FROM NEW.normalizer_version
  OR OLD.observed_count IS DISTINCT FROM NEW.observed_count THEN
  DELETE FROM vacancy_assessments WHERE structure_generation=OLD.id OR assessment_generation=OLD.id;
  DELETE FROM vacancy_live_validation WHERE generation_id=OLD.id;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER vacancy_invalidate_changed_generation_trigger BEFORE UPDATE ON vacancy_live_generations
 FOR EACH ROW EXECUTE FUNCTION vacancy_invalidate_changed_generation();
-- Future publication is keyed by stable parcel ID, but never inherits a fixture
-- generation's claim of completeness or a stale live source generation.
ALTER TABLE vacancy_assessments DROP CONSTRAINT vacancy_assessments_tier_check;
ALTER TABLE vacancy_assessments ADD CONSTRAINT vacancy_assessments_tier_check
 CHECK (tier IN ('strong_candidate','review','conflict','unknown'));
ALTER TABLE vacancy_assessments DROP CONSTRAINT vacancy_assessments_structure_generation_fkey;
ALTER TABLE vacancy_assessments DROP CONSTRAINT vacancy_assessments_assessment_generation_fkey;
ALTER TABLE vacancy_assessments ADD CONSTRAINT vacancy_assessments_structure_generation_fkey
 FOREIGN KEY (structure_generation) REFERENCES vacancy_live_validation(generation_id) NOT VALID;
ALTER TABLE vacancy_assessments ADD CONSTRAINT vacancy_assessments_assessment_generation_fkey
 FOREIGN KEY (assessment_generation) REFERENCES vacancy_live_validation(generation_id) NOT VALID;
-- +goose Down
ALTER TABLE vacancy_assessments DROP CONSTRAINT vacancy_assessments_structure_generation_fkey;
ALTER TABLE vacancy_assessments DROP CONSTRAINT vacancy_assessments_assessment_generation_fkey;
ALTER TABLE vacancy_assessments ADD CONSTRAINT vacancy_assessments_structure_generation_fkey
 FOREIGN KEY (structure_generation) REFERENCES vacancy_evidence_generations(id);
ALTER TABLE vacancy_assessments ADD CONSTRAINT vacancy_assessments_assessment_generation_fkey
 FOREIGN KEY (assessment_generation) REFERENCES vacancy_evidence_generations(id);
ALTER TABLE vacancy_assessments DROP CONSTRAINT vacancy_assessments_tier_check;
ALTER TABLE vacancy_assessments ADD CONSTRAINT vacancy_assessments_tier_check CHECK (tier='unknown');
DROP TRIGGER vacancy_invalidate_changed_generation_trigger ON vacancy_live_generations;
DROP FUNCTION vacancy_invalidate_changed_generation;
DROP TRIGGER vacancy_validate_live_identity_trigger ON vacancy_live_validation;
DROP FUNCTION vacancy_validate_live_identity;
DROP TABLE vacancy_live_validation;
