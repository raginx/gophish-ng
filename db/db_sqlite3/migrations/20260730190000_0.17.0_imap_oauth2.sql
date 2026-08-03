-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds OAuth2 support for IMAP email reporting
-- alongside the existing Basic Auth (username/password) fields, which
-- keep working unchanged. auth_type defaults to 'basic' so existing rows
-- keep behaving exactly as before.
ALTER TABLE imap ADD COLUMN auth_type varchar(255) NOT NULL DEFAULT 'basic';
ALTER TABLE imap ADD COLUMN oauth_provider varchar(255);
ALTER TABLE imap ADD COLUMN oauth_tenant_id varchar(255);
ALTER TABLE imap ADD COLUMN oauth_client_id varchar(255);
ALTER TABLE imap ADD COLUMN oauth_client_secret varchar(255);
ALTER TABLE imap ADD COLUMN oauth_auth_url varchar(255);
ALTER TABLE imap ADD COLUMN oauth_token_url varchar(255);
ALTER TABLE imap ADD COLUMN oauth_scopes varchar(255);
-- Encrypted (AES-256-GCM, see the crypto package) - base64 ciphertext is
-- longer than the plaintext token, hence text rather than varchar(255).
ALTER TABLE imap ADD COLUMN oauth_refresh_token_enc text;
ALTER TABLE imap ADD COLUMN oauth_access_token_enc text;
ALTER TABLE imap ADD COLUMN oauth_token_expiry datetime;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
