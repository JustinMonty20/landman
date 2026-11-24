-- +goose Up
-- Upgrades from pre-cutoff schemas must not let an older failed capture
-- recertify a row whose certification was already withdrawn by a newer scan.
INSERT INTO parcel_evidence_rejection_cutoff(county_id,source_name,source_parcel_id,observed_at,artifact_id)
SELECT c.county_id,c.source_name,c.source_parcel_id,latest.observed_at,latest.artifact_id
FROM parcel_evidence_current c
CROSS JOIN LATERAL (SELECT o.observed_at,o.artifact_id FROM parcel_evidence_observations o
WHERE o.county_id='union' AND o.source_name='union_county_gis'
ORDER BY o.observed_at DESC,o.id DESC LIMIT 1) latest
WHERE c.county_id='union' AND c.source_name='union_county_gis' AND c.parcel_row_version IS NULL
AND octet_length(c.source_parcel_id)<=2048
ON CONFLICT(county_id,source_name,source_parcel_id) DO UPDATE
SET observed_at=EXCLUDED.observed_at,artifact_id=EXCLUDED.artifact_id
WHERE parcel_evidence_rejection_cutoff.observed_at < EXCLUDED.observed_at;
-- A quarantined PID is not an indexable identity and must never be promoted
-- into a cutoff key during its next OBJECTID transition.
DELETE FROM parcel_source_object_identity WHERE octet_length(source_parcel_id)>2048;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bump_vacancy_source_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE changed boolean;
BEGIN
IF TG_OP='INSERT' THEN
 SELECT EXISTS(SELECT 1 FROM inserted WHERE county_id='union' AND source_name='union_county_gis') INTO changed;
ELSIF TG_OP='DELETE' THEN
 SELECT EXISTS(SELECT 1 FROM deleted WHERE county_id='union' AND source_name='union_county_gis') INTO changed;
ELSE
 -- xmin and therefore serving membership changes even for a no-op tuple rewrite.
 SELECT EXISTS(SELECT 1 FROM inserted WHERE county_id='union' AND source_name='union_county_gis')
 OR EXISTS(SELECT 1 FROM deleted WHERE county_id='union' AND source_name='union_county_gis') INTO changed;
END IF;
IF changed AND current_setting('landman.vacancy_revision_bumped',true) IS DISTINCT FROM '1' THEN
 UPDATE vacancy_source_revision SET revision=revision+1 WHERE county_id='union';
 PERFORM set_config('landman.vacancy_revision_bumped','1',true);
END IF;
RETURN NULL;
END $$;
-- +goose StatementEnd
-- Every shape writer takes the shared source revision lock before the topology
-- lock. Evidence writers already take source first; this removes lock inversion.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bump_vacancy_parcel_topology() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
IF (TG_OP = 'INSERT' AND NEW.county_id = 'union' AND NEW.source_name = 'union_county_gis')
   OR (TG_OP = 'DELETE' AND OLD.county_id = 'union' AND OLD.source_name = 'union_county_gis')
   OR (TG_OP = 'UPDATE' AND (OLD.county_id,OLD.source_name,OLD.source_parcel_id,OLD.shape)
       IS DISTINCT FROM (NEW.county_id,NEW.source_name,NEW.source_parcel_id,NEW.shape)
       AND ((OLD.county_id = 'union' AND OLD.source_name = 'union_county_gis')
         OR (NEW.county_id = 'union' AND NEW.source_name = 'union_county_gis'))) THEN
    IF current_setting('landman.vacancy_revision_bumped',true) IS DISTINCT FROM '1' THEN
      UPDATE vacancy_source_revision SET revision=revision+1 WHERE county_id='union';
      PERFORM set_config('landman.vacancy_revision_bumped','1',true);
    ELSE
      -- Take the row lock even if this transaction has already bumped it.
      PERFORM 1 FROM vacancy_source_revision WHERE county_id='union' FOR UPDATE;
    END IF;
    UPDATE vacancy_parcel_topology SET revision=revision+1 WHERE county_id='union';
END IF;
RETURN NULL;
END $$;
-- +goose StatementEnd
-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'vacancy revision and upgrade fences cannot be downgraded'; END $$;
