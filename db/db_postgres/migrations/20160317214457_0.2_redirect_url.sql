-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
-- text rather than varchar(255): SQLite does not enforce the length and the
-- MySQL history later widens this column to TEXT, so PostgreSQL must not
-- introduce a 255-character limit on redirect URLs.
ALTER TABLE pages ADD COLUMN redirect_url text;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
ALTER TABLE pages DROP COLUMN redirect_url;
