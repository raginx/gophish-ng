-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds per-campaign automatic completion. A campaign otherwise stays "In
-- progress" until an admin manually ends it, which also means options that
-- key off completion (e.g. scrub_on_complete) never fire on their own.
--
--   auto_complete_after_days - when > 0, the background worker completes the
--                              campaign this many days after its send-by date
--                              (or launch date, when no send-by date is set).
--                              0 keeps the previous manual-only behaviour.
ALTER TABLE `campaigns` ADD COLUMN `auto_complete_after_days` INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
ALTER TABLE `campaigns` DROP COLUMN `auto_complete_after_days`;
