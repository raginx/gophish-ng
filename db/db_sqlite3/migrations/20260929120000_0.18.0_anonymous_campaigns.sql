-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds support for anonymous campaigns via scrubbing. Privacy-sensitive
-- orgs can flag a campaign to have its recipient-identifying data
-- permanently removed once the campaign is done, keeping only the
-- aggregate outcome data (status, timing, report/click counts) needed for
-- reporting.
--
--   scrub_on_complete - when set, the campaign's results/events are
--                       scrubbed automatically as part of CompleteCampaign.
--   scrubbed_date     - set to the time the scrub ran; also acts as the
--                       "this campaign is anonymized" marker for the UI and
--                       makes re-scrubbing idempotent.
ALTER TABLE campaigns ADD COLUMN scrub_on_complete BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE campaigns ADD COLUMN scrubbed_date DATETIME;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
ALTER TABLE campaigns DROP COLUMN scrubbed_date;
ALTER TABLE campaigns DROP COLUMN scrub_on_complete;
