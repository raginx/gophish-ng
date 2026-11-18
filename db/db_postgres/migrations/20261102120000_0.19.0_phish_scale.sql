-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds the NIST Phish Scale assessment for a campaign. The Phish Scale rates
-- how difficult a phishing message is for its target audience to detect
CREATE TABLE "phish_scale" (
    "id"                bigserial PRIMARY KEY,
    "campaign_id"       bigint NOT NULL UNIQUE,
    "team_id"           bigint,
    "premise_alignment" varchar(16) NOT NULL DEFAULT '',
    "notes"             text NOT NULL DEFAULT '',
    "modified_date"     timestamp
);

CREATE TABLE "phish_scale_cues" (
    "id"             bigserial PRIMARY KEY,
    "phish_scale_id" bigint NOT NULL,
    "cue_key"        varchar(64) NOT NULL,
    "count"          integer NOT NULL DEFAULT 0
);

CREATE INDEX idx_phish_scale_cues_phish_scale_id ON phish_scale_cues (phish_scale_id);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP INDEX idx_phish_scale_cues_phish_scale_id;
DROP TABLE "phish_scale_cues";
DROP TABLE "phish_scale";
