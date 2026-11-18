-- +goose Up
ALTER TABLE tax_foreclosure_cases ADD COLUMN active BOOLEAN NOT NULL DEFAULT false;
-- Preserve precisely the cases the pre-cutover API served, including the
-- file identity of the active import rather than every retained case for its source.
-- This is a serving-membership backfill, not a new source observation.
ALTER TABLE tax_foreclosure_cases DISABLE TRIGGER tax_foreclosure_cases_history;
UPDATE tax_foreclosure_cases c SET active=true
FROM tax_file_imports i
WHERE i.active AND i.county_id=c.county_id AND i.source_name=c.source_name
  AND i.source_url=c.source_url AND i.file_sha256=c.file_sha256 AND i.fetched_at=c.fetched_at;
ALTER TABLE tax_foreclosure_cases ENABLE TRIGGER tax_foreclosure_cases_history;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM tax_foreclosure_cases) THEN
        RAISE EXCEPTION 'cannot discard retained foreclosure membership';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE tax_foreclosure_cases DROP COLUMN active;
