-- +goose Up
ALTER TABLE app_users ADD COLUMN profile_image_url text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE app_users DROP COLUMN profile_image_url;
