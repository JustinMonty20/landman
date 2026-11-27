-- +goose Up
CREATE TABLE user_parcel_notes (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
    parcel_id uuid NOT NULL REFERENCES parcels(id) ON DELETE RESTRICT,
    body text NOT NULL CHECK (length(btrim(body)) > 0 AND octet_length(body) <= 4000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0)
);
CREATE INDEX user_parcel_notes_page_idx ON user_parcel_notes(user_id, parcel_id, created_at DESC, id DESC);
-- +goose Down
DROP TABLE user_parcel_notes;
