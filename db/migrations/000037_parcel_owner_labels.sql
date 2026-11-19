-- +goose Up
CREATE TABLE parcel_owner_labels (
 parcel_id UUID PRIMARY KEY REFERENCES parcels(id) ON DELETE CASCADE,
 out_of_state_mailing TEXT NOT NULL CHECK (out_of_state_mailing IN ('yes','no','unknown')),
 entity_name_indicator TEXT NOT NULL CHECK (entity_name_indicator IN ('matched','not_detected','unknown')),
 corporate_name_indicator TEXT NOT NULL CHECK (corporate_name_indicator IN ('matched','not_detected','unknown')),
 mailing_state TEXT,
 matched_suffix TEXT,
 matched_owner_line SMALLINT,
 classified_at TIMESTAMPTZ NOT NULL,
 CONSTRAINT owner_corporate_subset CHECK (corporate_name_indicator <> 'matched' OR entity_name_indicator = 'matched'),
 CONSTRAINT owner_mailing_evidence CHECK ((out_of_state_mailing = 'unknown') = (mailing_state IS NULL) AND (mailing_state IS NULL OR (mailing_state IN ('AL','AK','AZ','AR','CA','CO','CT','DE','FL','GA','HI','ID','IL','IN','IA','KS','KY','LA','ME','MD','MA','MI','MN','MS','MO','MT','NE','NV','NH','NJ','NM','NY','NC','ND','OH','OK','OR','PA','RI','SC','SD','TN','TX','UT','VT','VA','WA','WV','WI','WY','DC','AS','GU','MP','PR','VI') AND (out_of_state_mailing = 'no') = (mailing_state = 'NC')))),
 CONSTRAINT owner_name_evidence CHECK (
  (entity_name_indicator = 'matched' AND matched_suffix IS NOT NULL AND matched_owner_line IS NOT NULL AND matched_suffix IN ('INC','INCORPORATED','CORP','CORPORATION','LLC','LP','LLP','LTD','LIMITED','PLC','PC') AND matched_owner_line IN (1,2) AND (corporate_name_indicator = 'matched') = (matched_suffix IN ('INC','INCORPORATED','CORP','CORPORATION')))
  OR (entity_name_indicator <> 'matched' AND corporate_name_indicator <> 'matched' AND matched_suffix IS NULL AND matched_owner_line IS NULL)
 )
);
CREATE TABLE parcel_owner_label_revision (
id BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
revision BIGINT NOT NULL DEFAULT 0,
last_writer_xid BIGINT
);
INSERT INTO parcel_owner_label_revision(id) VALUES (true);
-- A deferred row trigger acquires the shared revision lock only at commit, after
-- parcel locks. One transaction increments once, including multi-row backfills.
-- +goose StatementBegin
CREATE FUNCTION advance_parcel_owner_label_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE parcel_owner_label_revision SET revision = revision + 1,
    last_writer_xid = pg_current_xact_id()::text::bigint
  WHERE id AND last_writer_xid IS DISTINCT FROM pg_current_xact_id()::text::bigint;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER parcel_owner_label_revision_change
AFTER INSERT OR UPDATE OR DELETE ON parcel_owner_labels
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION advance_parcel_owner_label_revision();
CREATE INDEX parcel_owner_labels_classified_at ON parcel_owner_labels(classified_at DESC);
CREATE INDEX parcel_owner_labels_mailing_yes ON parcel_owner_labels(parcel_id) WHERE out_of_state_mailing='yes';
CREATE INDEX parcel_owner_labels_entity_matched ON parcel_owner_labels(parcel_id) WHERE entity_name_indicator='matched';
CREATE INDEX parcel_owner_labels_corporate_matched ON parcel_owner_labels(parcel_id) WHERE corporate_name_indicator='matched';
-- +goose Down
DROP TABLE parcel_owner_labels;
DROP FUNCTION advance_parcel_owner_label_revision();
DROP TABLE parcel_owner_label_revision;
