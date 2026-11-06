-- +goose Up
-- Persist the OAuth client id alongside the token so rclone remotes can be
-- provisioned with the same client the token was issued for (fixes repeated
-- re-authorization when rclone falls back to its own shared client).
ALTER TABLE oauth_tokens ADD COLUMN encrypted_client_id BLOB;

-- +goose Down
ALTER TABLE oauth_tokens DROP COLUMN encrypted_client_id;
