-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
CREATE TABLE IF NOT EXISTS "mail_logs" (
    "id" bigserial primary key,
    "campaign_id" bigint,
    "user_id" bigint,
    "send_date" timestamp,
    "send_attempt" integer,
    "r_id" varchar(255),
    "processing" boolean);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP TABLE "mail_logs";
