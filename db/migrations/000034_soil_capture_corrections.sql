-- +goose Up
CREATE TABLE soil_screening_survey_dates (
 run_id UUID NOT NULL REFERENCES soil_screening_runs(id),
 polygon_id UUID NOT NULL REFERENCES soil_polygons(id),
 survey_saved_on DATE NOT NULL,
 PRIMARY KEY (run_id, polygon_id)
);
ALTER TABLE soil_ingest_publications ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);
ALTER TABLE soil_ingest_publications DROP CONSTRAINT soil_ingest_publications_pkey;
ALTER TABLE soil_ingest_publications ADD PRIMARY KEY (capture_id, revision);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS (SELECT 1 FROM soil_ingest_publications WHERE revision > 1)
   OR EXISTS (SELECT 1 FROM soil_screening_survey_dates) THEN
RAISE EXCEPTION 'cannot discard retained soil correction or survey provenance';
END IF;
END $$;
-- +goose StatementEnd
DROP TABLE soil_screening_survey_dates;
ALTER TABLE soil_ingest_publications DROP CONSTRAINT soil_ingest_publications_pkey;
ALTER TABLE soil_ingest_publications DROP COLUMN revision;
ALTER TABLE soil_ingest_publications ADD PRIMARY KEY (capture_id);
