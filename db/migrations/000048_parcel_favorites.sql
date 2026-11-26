-- +goose Up
CREATE TABLE user_lead_states (
 user_id uuid NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
 id uuid NOT NULL DEFAULT gen_random_uuid(),
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 80 AND name = btrim(name)),
 name_key text GENERATED ALWAYS AS (lower(name)) STORED,
 position integer NOT NULL CHECK (position >= 0),
 revision bigint NOT NULL DEFAULT 1,
 PRIMARY KEY (user_id,id), UNIQUE (user_id,name_key)
);
CREATE TABLE user_crm_settings (
 user_id uuid PRIMARY KEY REFERENCES app_users(id) ON DELETE CASCADE,
 default_state_id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 1,
 FOREIGN KEY (user_id,default_state_id) REFERENCES user_lead_states(user_id,id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE user_parcels (
 user_id uuid NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
 parcel_id uuid NOT NULL REFERENCES parcels(id) ON DELETE RESTRICT,
 state_id uuid NOT NULL,
 favorite boolean NOT NULL DEFAULT true,
 excluded boolean NOT NULL DEFAULT false,
 exclusion_reason text NOT NULL DEFAULT '',
 favorited_at timestamptz NOT NULL DEFAULT now(),
 revision bigint NOT NULL DEFAULT 1,
 PRIMARY KEY (user_id,parcel_id),
 FOREIGN KEY (user_id,state_id) REFERENCES user_lead_states(user_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK ((excluded AND length(btrim(exclusion_reason)) BETWEEN 1 AND 500) OR (NOT excluded AND exclusion_reason = ''))
);
CREATE INDEX user_parcels_board_idx ON user_parcels(user_id,state_id,favorited_at,parcel_id) WHERE favorite;
CREATE TABLE user_parcel_exclusion_changes (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL,
 parcel_id uuid NOT NULL,
 excluded boolean NOT NULL,
 reason text NOT NULL DEFAULT '',
 changed_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (user_id,parcel_id) REFERENCES user_parcels(user_id,parcel_id) ON DELETE CASCADE
);
CREATE TABLE user_parcel_state_changes (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL,
 parcel_id uuid NOT NULL,
 old_state_name text NOT NULL,
 new_state_name text NOT NULL,
 changed_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (user_id,parcel_id) REFERENCES user_parcels(user_id,parcel_id) ON DELETE CASCADE
);
-- +goose Down
DROP TABLE user_parcel_state_changes;
DROP TABLE user_parcel_exclusion_changes;
DROP TABLE user_parcels;
DROP TABLE user_crm_settings;
DROP TABLE user_lead_states;
