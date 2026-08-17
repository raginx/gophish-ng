-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds visibility into IMAP monitor failures: a consecutive-error counter,
-- most recent login error message, and a count of reported emails that
-- didnt match a known campaign
ALTER TABLE imap ADD COLUMN consecutive_login_errors integer NOT NULL DEFAULT 0;
ALTER TABLE imap ADD COLUMN last_login_error varchar(255);
ALTER TABLE imap ADD COLUMN non_campaign_emails_count integer NOT NULL DEFAULT 0;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
