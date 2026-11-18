-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds the NIST Phish Scale assessment for a campaign. The Phish Scale rates
-- how difficult a phishing message is for its target audience to detect
CREATE TABLE `phish_scale` (
    `id`                INTEGER PRIMARY KEY AUTO_INCREMENT,
    `campaign_id`       INTEGER NOT NULL UNIQUE,
    `team_id`           INTEGER,
    `premise_alignment` VARCHAR(16) NOT NULL DEFAULT '',
    `notes`             TEXT,
    `modified_date`     DATETIME
);

CREATE TABLE `phish_scale_cues` (
    `id`             INTEGER PRIMARY KEY AUTO_INCREMENT,
    `phish_scale_id` INTEGER NOT NULL,
    `cue_key`        VARCHAR(64) NOT NULL,
    `count`          INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX `idx_phish_scale_cues_phish_scale_id` ON `phish_scale_cues` (`phish_scale_id`);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP INDEX `idx_phish_scale_cues_phish_scale_id` ON `phish_scale_cues`;
DROP TABLE `phish_scale_cues`;
DROP TABLE `phish_scale`;
