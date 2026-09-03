-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds the ability to render a templated HTML response after credentials
-- are submitted, as an alternative to redirecting to a URL.
ALTER TABLE `pages` ADD COLUMN `redirect_mode` TEXT;
ALTER TABLE `pages` ADD COLUMN `redirect_html` TEXT;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
ALTER TABLE `pages` DROP COLUMN `redirect_mode`;
ALTER TABLE `pages` DROP COLUMN `redirect_html`;
