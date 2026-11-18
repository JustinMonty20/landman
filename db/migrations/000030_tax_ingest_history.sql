-- +goose Up
CREATE TABLE tax_ingest_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_name TEXT NOT NULL,
    capture_id UUID REFERENCES ingest_captures(id),
    table_name TEXT NOT NULL,
    record_id UUID NOT NULL,
    cause TEXT NOT NULL CHECK (cause IN ('source_observation','normalization_correction','offline_retry')),
    before_value JSONB,
    after_value JSONB,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX tax_ingest_history_record_idx ON tax_ingest_history (table_name, record_id, changed_at);
-- Preserve important typed values, not observation timestamps or transport metadata.
-- +goose StatementBegin
CREATE FUNCTION record_tax_ingest_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    previous JSONB;
    current_value JSONB;
    source TEXT;
    record UUID;
    reason TEXT;
    capture TEXT;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        previous := to_jsonb(OLD) - ARRAY['file_sha256','source_url','fetched_at','created_at','updated_at'];
    END IF;
    IF TG_OP <> 'DELETE' THEN
        current_value := to_jsonb(NEW) - ARRAY['file_sha256','source_url','fetched_at','created_at','updated_at'];
    END IF;
    IF previous IS NOT DISTINCT FROM current_value THEN
        RETURN COALESCE(NEW, OLD);
    END IF;
    IF TG_OP = 'DELETE' THEN
        record := OLD.id;
        IF TG_TABLE_NAME <> 'tax_foreclosure_parcels' THEN source := OLD.source_name; END IF;
    ELSE
        record := NEW.id;
        IF TG_TABLE_NAME <> 'tax_foreclosure_parcels' THEN source := NEW.source_name; END IF;
    END IF;
    IF TG_TABLE_NAME = 'tax_foreclosure_parcels' THEN
        SELECT source_name INTO source FROM tax_foreclosure_cases
        WHERE id = CASE WHEN TG_OP = 'DELETE' THEN OLD.case_id ELSE NEW.case_id END;
    END IF;
    IF source IS NULL THEN source := 'union_foreclosure'; END IF;
    reason := current_setting('landman.tax_change_cause', true);
    capture := current_setting('landman.tax_capture_id', true);
    IF reason IS NULL OR reason = '' THEN reason := 'source_observation'; END IF;
    INSERT INTO tax_ingest_history (source_name,capture_id,table_name,record_id,cause,before_value,after_value)
    VALUES (source,NULLIF(capture, '')::uuid,TG_TABLE_NAME,record,reason,previous,current_value);
    RETURN COALESCE(NEW, OLD);
END $$;
-- +goose StatementEnd
CREATE TRIGGER tax_delinquent_history AFTER INSERT OR UPDATE OR DELETE ON tax_delinquent_balances FOR EACH ROW EXECUTE FUNCTION record_tax_ingest_change();
CREATE TRIGGER tax_bills_history AFTER INSERT OR UPDATE OR DELETE ON tax_bills FOR EACH ROW EXECUTE FUNCTION record_tax_ingest_change();
CREATE TRIGGER tax_foreclosure_cases_history AFTER INSERT OR UPDATE OR DELETE ON tax_foreclosure_cases FOR EACH ROW EXECUTE FUNCTION record_tax_ingest_change();
CREATE TRIGGER tax_foreclosure_parcels_history AFTER INSERT OR UPDATE OR DELETE ON tax_foreclosure_parcels FOR EACH ROW EXECUTE FUNCTION record_tax_ingest_change();
CREATE TRIGGER tax_foreclosure_pipeline_history AFTER INSERT OR UPDATE OR DELETE ON tax_foreclosure_pipeline FOR EACH ROW EXECUTE FUNCTION record_tax_ingest_change();
-- +goose Down
DROP TRIGGER tax_foreclosure_pipeline_history ON tax_foreclosure_pipeline;
DROP TRIGGER tax_foreclosure_parcels_history ON tax_foreclosure_parcels;
DROP TRIGGER tax_foreclosure_cases_history ON tax_foreclosure_cases;
DROP TRIGGER tax_bills_history ON tax_bills;
DROP TRIGGER tax_delinquent_history ON tax_delinquent_balances;
DROP FUNCTION record_tax_ingest_change();
DROP TABLE tax_ingest_history;
